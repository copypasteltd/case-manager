package files

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"github.com/jackc/pgx/v5"
)

func (s *ObjectRetirementService) assess(ctx context.Context, tx pgx.Tx, org string, f File, kind, target, ownPlan string) (RetirementCandidate, []RetirementObject, error) {
	out := RetirementCandidate{Kind: kind, TargetID: target, FileID: f.ID, FileVersion: f.Version, Name: f.Name, BlockingReasons: []string{}}
	objects := []RetirementObject{}
	if !core.ValidID(target) || !slices.Contains([]string{"history", "committed_draft"}, kind) {
		return out, objects, core.Missing()
	}
	versionID := target
	var draft Upload
	if kind == "committed_draft" {
		var e error
		draft, e = upload(ctx, tx, org, target, false)
		if e != nil {
			return out, objects, e
		}
		if draft.ResultVersionID == nil {
			return out, objects, core.Missing()
		}
		versionID = *draft.ResultVersionID
	}
	v, e := scanVersion(tx.QueryRow(ctx, "SELECT "+versionColumns+" FROM file_versions WHERE org_id=$1 AND file_id=$2 AND id=$3", org, f.ID, versionID))
	if e != nil {
		return out, objects, e
	}
	out.SizeBytes = v.SizeBytes
	out.Seq = v.Seq
	if v.SizeBytes == "0" {
		out.BlockingReasons = append(out.BlockingReasons, "EMPTY_ORIGINAL_REVIEW_REQUIRED")
	}
	purgedHistory := false
	if kind == "committed_draft" && v.Availability == "purged" {
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM object_retirement_plans WHERE org_id=$1 AND kind='history' AND target_id=$2 AND state='completed')", org, versionID).Scan(&purgedHistory); e != nil {
			return out, objects, e
		}
	}
	if v.Availability != "available" && !purgedHistory && !(ownPlan != "" && kind == "history" && v.Availability == "retiring") {
		out.BlockingReasons = append(out.BlockingReasons, "ORIGINAL_UNAVAILABLE")
	}
	if kind == "history" {
		a, e := assessVersion(ctx, tx, org, f, target)
		if e != nil {
			return out, objects, e
		}
		out.Policy = a.Policy
		out.TrashPolicy = a.TrashPolicy
		out.RetainUntil = a.RetainUntil
		for _, code := range a.BlockingReasons {
			// A prior object receipt can mark one replica missing. Its immutable original
			// identity is still rechecked below, and never replaced by a newer replica.
			if ownPlan != "" && code == "REPLICA_EVIDENCE_INCOMPLETE" {
				continue
			}
			out.BlockingReasons = append(out.BlockingReasons, code)
		}
	} else {
		if draft.State != "committed" {
			out.BlockingReasons = append(out.BlockingReasons, "COMMITTED_DRAFT_REQUIRED")
		}
		var created time.Time
		var now time.Time
		if e = tx.QueryRow(ctx, "SELECT created_at,clock_timestamp() FROM upload_sessions WHERE org_id=$1 AND id=$2", org, draft.ID).Scan(&created, &now); e != nil {
			return out, objects, e
		}
		p, e := scanRetentionPolicy(tx.QueryRow(ctx, "SELECT "+retentionPolicyColumns+" FROM retention_policies WHERE org_id=$1 AND kind='draft' ORDER BY version DESC LIMIT 1", org))
		if e != nil {
			if core.PublicError(e).Status != 404 {
				return out, objects, e
			}
			out.BlockingReasons = append(out.BlockingReasons, "POLICY_REQUIRED")
		} else {
			out.Policy = &RetentionRule{ID: p.ID, Version: p.Version, Kind: p.Kind, Enabled: p.Enabled, RetainSeconds: p.RetainSeconds, ObservationSeconds: p.ObservationSeconds}
			// Retention starts at the immutable terminal commit timestamp.
			var committed time.Time
			if e = tx.QueryRow(ctx, "SELECT committed_at FROM upload_sessions WHERE org_id=$1 AND id=$2", org, draft.ID).Scan(&committed); e != nil {
				return out, objects, e
			}
			until := committed.Add(time.Duration(p.RetainSeconds) * time.Second)
			out.RetainUntil = &until
			if !p.Enabled {
				out.BlockingReasons = append(out.BlockingReasons, "POLICY_DISABLED")
			}
			if now.Before(until) {
				out.BlockingReasons = append(out.BlockingReasons, "RETENTION_PERIOD_ACTIVE")
			}
		}
		if now.Before(draft.ExpiresAt.Add(objectstore.MaxSignTTL)) {
			out.BlockingReasons = append(out.BlockingReasons, "UPLOAD_OR_PERMIT_ACTIVE")
		}
		held, e := retentionHeld(ctx, tx, org, f.ID)
		if e != nil {
			return out, objects, e
		}
		if held {
			out.BlockingReasons = append(out.BlockingReasons, "RETENTION_HOLD")
		}
		var active, agent, formal bool
		e = tx.QueryRow(ctx, `SELECT
   EXISTS(SELECT 1 FROM jobs WHERE org_id=$1 AND payload->>'session_id'=$2::text AND state IN ('queued','running','retry_wait')),
   EXISTS(SELECT 1 FROM agent_file_drafts WHERE org_id=$1 AND id=$2::uuid),
   EXISTS(SELECT 1 FROM file_versions WHERE org_id=$1 AND object_key=$3)
    OR EXISTS(SELECT 1 FROM file_replicas WHERE org_id=$1 AND object_key=$3)`, org, draft.ID, draft.DraftKey).Scan(&active, &agent, &formal)
		if e != nil {
			return out, objects, e
		}
		if active {
			out.BlockingReasons = append(out.BlockingReasons, "UPLOAD_JOB_ACTIVE")
		}
		if agent {
			out.BlockingReasons = append(out.BlockingReasons, "AGENT_RECOVERY_SOURCE")
		}
		if formal {
			out.BlockingReasons = append(out.BlockingReasons, "FORMAL_OBJECT_PROTECTED")
		}
		var attempt string
		e = tx.QueryRow(ctx, "SELECT id FROM upload_attempts WHERE org_id=$1 AND session_id=$2 AND draft_key=$3 AND state='active'", org, draft.ID, draft.DraftKey).Scan(&attempt)
		if e != nil && e != pgx.ErrNoRows {
			return out, objects, e
		}
		key, _ := objectstore.DraftKey(org, attempt)
		if e == pgx.ErrNoRows || key != draft.DraftKey || draft.FrozenSourceID == nil || draft.Provider == nil || draft.Bucket == nil {
			out.BlockingReasons = append(out.BlockingReasons, "SOURCE_CHANGED")
		} else {
			var source objectstore.Source
			if json.Unmarshal([]byte(*draft.FrozenSourceID), &source) != nil || source.Key != draft.DraftKey || source.ETag == "" || source.Size != draft.layout().Size {
				out.BlockingReasons = append(out.BlockingReasons, "SOURCE_CHANGED")
			} else {
				objects = append(objects, RetirementObject{Provider: *draft.Provider, Bucket: *draft.Bucket, Source: source})
			}
		}
	}
	rows, e := tx.Query(ctx, `SELECT id,provider,bucket,object_key,source_id,state,expected_sha256,actual_sha256,verified_at
   FROM file_replicas WHERE org_id=$1 AND version_id=$2 ORDER BY provider,id`, org, versionID)
	if e != nil {
		return out, objects, e
	}
	providers := map[string]bool{}
	count := 0
	valid := true
	size, _ := strconv.ParseInt(v.SizeBytes, 10, 64)
	for rows.Next() {
		var obj RetirementObject
		var key, state, expected string
		var raw, actual *string
		var verified *time.Time
		if e = rows.Scan(&obj.ReplicaID, &obj.Provider, &obj.Bucket, &key, &raw, &state, &expected, &actual, &verified); e != nil {
			rows.Close()
			return out, objects, e
		}
		count++
		if providers[obj.Provider] || raw == nil || actual == nil || verified == nil || expected != v.SHA256 || actual != nil && *actual != v.SHA256 || state != "verified" && !(state == "missing" && (purgedHistory || ownPlan != "" && kind == "history")) {
			valid = false
		}
		providers[obj.Provider] = true
		if raw == nil || json.Unmarshal([]byte(*raw), &obj.Source) != nil || obj.Source.Key != key || obj.Source.ETag == "" || obj.Source.Size != size {
			valid = false
		}
		original, _ := objectstore.VersionKey(org, f.ID, versionID)
		replicaPrefix := strings.TrimSuffix(original, "original") + "replicas/"
		if key != original {
			rest, ok := strings.CutPrefix(key, replicaPrefix)
			id, ok2 := strings.CutSuffix(rest, "/original")
			if !ok || !ok2 || !core.ValidID(id) {
				valid = false
			}
		}
		if kind == "history" {
			objects = append(objects, obj)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, objects, e
	}
	if !valid || count != 2 || !providers["oss"] || !providers["r2"] {
		if !slices.Contains(out.BlockingReasons, "REPLICA_EVIDENCE_INCOMPLETE") {
			out.BlockingReasons = append(out.BlockingReasons, "REPLICA_EVIDENCE_INCOMPLETE")
		}
	}
	if kind == "history" {
		var key, raw string
		var primary objectstore.Source
		if e = tx.QueryRow(ctx, "SELECT object_key,provider_source_id FROM file_versions WHERE org_id=$1 AND id=$2", org, versionID).Scan(&key, &raw); e != nil {
			return out, objects, e
		}
		if json.Unmarshal([]byte(raw), &primary) != nil || primary.Key != key || !slices.ContainsFunc(objects, func(o RetirementObject) bool { return o.Source == primary }) {
			out.BlockingReasons = append(out.BlockingReasons, "SOURCE_CHANGED")
		}
	}
	for i := range objects {
		obj := &objects[i]
		b := s.Backends[obj.Provider]
		if b == nil || b.Provider != obj.Provider || b.Bucket != obj.Bucket || len(b.BindingSHA256) != 64 {
			out.BlockingReasons = append(out.BlockingReasons, "CLEANUP_BACKEND_UNAVAILABLE")
			continue
		}
		obj.BindingSHA256 = b.BindingSHA256
		obj.ExclusiveNamespace = b.ExclusiveNamespace
		if obj.Provider == "oss" && (obj.Source.VersionID == "" || obj.Source.VersionID == "null") {
			out.BlockingReasons = append(out.BlockingReasons, "NATIVE_OBJECT_VERSION_REQUIRED")
		}
		if obj.Provider == "r2" && !b.ExclusiveNamespace {
			out.BlockingReasons = append(out.BlockingReasons, "EXCLUSIVE_NAMESPACE_REQUIRED")
		}
		var reserved, shared, busy bool
		e = tx.QueryRow(ctx, `SELECT
   EXISTS(SELECT 1 FROM object_retirement_barriers WHERE org_id=$1 AND object_key=$2 AND plan_id::text<>$3),
   EXISTS(SELECT 1 FROM file_versions WHERE org_id=$1 AND object_key=$2 AND id<>$4)
    OR EXISTS(SELECT 1 FROM file_replicas WHERE org_id=$1 AND object_key=$2 AND version_id<>$4),
   EXISTS(SELECT 1 FROM object_transfers WHERE org_id=$1 AND object_key=$2 AND state NOT IN ('complete','failed','uncertain'))`, org, obj.Source.Key, ownPlan, versionID).Scan(&reserved, &shared, &busy)
		if e != nil {
			return out, objects, e
		}
		if reserved {
			out.BlockingReasons = append(out.BlockingReasons, "ORIGINAL_ALREADY_RETIRED")
		}
		if shared {
			out.BlockingReasons = append(out.BlockingReasons, "SHARED_OBJECT_PROTECTED")
		}
		if busy {
			out.BlockingReasons = append(out.BlockingReasons, "OBJECT_WRITE_ACTIVE")
		}
	}
	if kind == "history" {
		var active bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_replicas r JOIN jobs j ON j.org_id=r.org_id AND j.id=r.active_job_id WHERE r.org_id=$1 AND r.version_id=$2 AND j.state IN ('queued','running','retry_wait'))`, org, versionID).Scan(&active)
		if e != nil {
			return out, objects, e
		}
		if active {
			out.BlockingReasons = append(out.BlockingReasons, "OBJECT_WRITE_ACTIVE")
		}
	}
	out.SourceSHA256 = digest(objects)
	slices.Sort(out.BlockingReasons)
	out.BlockingReasons = slices.Compact(out.BlockingReasons)
	out.Eligible = len(out.BlockingReasons) == 0
	return out, objects, nil
}
