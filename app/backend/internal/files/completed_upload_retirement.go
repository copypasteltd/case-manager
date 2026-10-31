package files

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// A new upload has no file row yet. Folder scope must never masquerade as a
// formal file identity; replacements retain their additional file authority.
type CompletedDraftRetirementCreate struct {
	ExpectedFolderVersion int64  `json:"expected_folder_version"`
	ExpectedUploadVersion int64  `json:"expected_upload_version"`
	TargetID              string `json:"target_id"`
	Reason                string `json:"reason"`
}

func (s *ObjectRetirementService) folderActor(ctx context.Context, db platform.DBTX, a core.Actor, id string, mfa bool) (Folder, error) {
	return (&MultipartCleanupService{Files: s.Files}).actor(ctx, db, a, id, mfa)
}
func (s *ObjectRetirementService) planActor(ctx context.Context, db platform.DBTX, a core.Actor, p RetirementPlan, mfa bool) error {
	if p.Snapshot.Kind != "uncommitted_draft" {
		_, e := s.actor(ctx, db, a, p.Snapshot.FileID, mfa)
		return e
	}
	if p.ScopeFolderID == nil || *p.ScopeFolderID != p.Snapshot.FolderID {
		return core.Missing()
	}
	f, e := s.folderActor(ctx, db, a, p.Snapshot.FolderID, mfa)
	if e != nil {
		return e
	}
	u, e := upload(ctx, db, a.OrgID, p.Snapshot.TargetID, false)
	if e != nil {
		return e
	}
	return cleanupUploadAccess(ctx, db, a, f, u)
}
func (s *ObjectRetirementService) lockPlanTarget(ctx context.Context, tx pgx.Tx, org string, p RetirementPlan) error {
	if p.Snapshot.Kind == "uncommitted_draft" {
		if _, e := folder(ctx, tx, org, p.Snapshot.FolderID, true); e != nil {
			return e
		}
		if p.Snapshot.FileID != "" {
			if _, e := file(ctx, tx, org, p.Snapshot.FileID, true); e != nil {
				return e
			}
		}
		_, e := upload(ctx, tx, org, p.Snapshot.TargetID, true)
		return e
	}
	_, e := file(ctx, tx, org, p.Snapshot.FileID, true)
	return e
}
func (s *ObjectRetirementService) assessPlan(ctx context.Context, tx pgx.Tx, org string, p RetirementPlan, own string) (RetirementCandidate, []RetirementObject, error) {
	if e := s.lockPlanTarget(ctx, tx, org, p); e != nil {
		return RetirementCandidate{}, nil, e
	}
	if p.Snapshot.Kind == "uncommitted_draft" {
		f, e := folder(ctx, tx, org, p.Snapshot.FolderID, false)
		if e != nil {
			return RetirementCandidate{}, nil, e
		}
		return s.assessCompletedUpload(ctx, tx, org, f, p.Snapshot.TargetID, own)
	}
	f, e := file(ctx, tx, org, p.Snapshot.FileID, false)
	if e != nil {
		return RetirementCandidate{}, nil, e
	}
	return s.assess(ctx, tx, org, f, p.Snapshot.Kind, p.Snapshot.TargetID, own)
}

func (s *ObjectRetirementService) assessCompletedUpload(ctx context.Context, tx pgx.Tx, org string, f Folder, target, own string) (RetirementCandidate, []RetirementObject, error) {
	u, e := upload(ctx, tx, org, target, false)
	if e != nil {
		return RetirementCandidate{}, nil, e
	}
	if u.FolderID != f.ID {
		return RetirementCandidate{}, nil, core.Missing()
	}
	out := RetirementCandidate{Kind: "uncommitted_draft", TargetID: u.ID, FolderID: f.ID, FolderVersion: f.Version, UploadVersion: u.Version, UploadState: u.State, Name: u.Name, SizeBytes: u.SizeBytes, BlockingReasons: []string{}}
	objects := []RetirementObject{}
	block := func(code string) { out.BlockingReasons = append(out.BlockingReasons, code) }
	if u.FileID != nil {
		v, e := file(ctx, tx, org, *u.FileID, false)
		if e != nil {
			return out, objects, e
		}
		out.FileID = v.ID
		out.FileVersion = v.Version
		if v.FolderID != f.ID || v.Classification != f.Classification {
			block("UPLOAD_TARGET_CHANGED")
		}
		held, e := retentionHeld(ctx, tx, org, v.ID)
		if e != nil {
			return out, objects, e
		}
		if held {
			block("RETENTION_HOLD")
		}
	}
	if u.RetirementPlanID != nil {
		if own == "" || *u.RetirementPlanID != own {
			block("ORIGINAL_ALREADY_RETIRED")
		} else {
			// Approval consumes precisely one session revision. All subsequent changes
			// are rejected by SQL; canonical input retains the pre-approval revision.
			out.UploadVersion--
		}
	}
	if u.ResultVersionID != nil || u.State == "committed" {
		block("FORMAL_OBJECT_PROTECTED")
	}
	if u.Classification == nil || *u.Classification != f.Classification || u.SourceChannel == nil || !slices.Contains([]string{"web", "android"}, *u.SourceChannel) {
		block("AGENT_RECOVERY_SOURCE")
	}
	if f.State != "active" {
		block("UPLOAD_TARGET_CHANGED")
	}
	if e = activeAncestors(ctx, tx, org, f.ID); e != nil {
		block("UPLOAD_TARGET_CHANGED")
	}
	var now time.Time
	var observed *time.Time
	var candidateID, candidateKey, candidateSource *string
	if e = tx.QueryRow(ctx, "SELECT clock_timestamp(),completed_observed_at,candidate_transfer_id,candidate_key,candidate_source_id FROM upload_sessions WHERE org_id=$1 AND id=$2", org, u.ID).Scan(&now, &observed, &candidateID, &candidateKey, &candidateSource); e != nil {
		return out, objects, e
	}
	if now.Before(u.ExpiresAt.Add(objectstore.MaxSignTTL)) {
		block("UPLOAD_OR_PERMIT_ACTIVE")
	}
	if observed == nil {
		block("COMPLETION_EVIDENCE_REQUIRED")
	}
	var original objectstore.Source
	var attempt string
	e = tx.QueryRow(ctx, "SELECT id FROM upload_attempts WHERE org_id=$1 AND session_id=$2 AND draft_key=$3 AND state='active'", org, u.ID, u.DraftKey).Scan(&attempt)
	if e != nil && e != pgx.ErrNoRows {
		return out, objects, e
	}
	expected, _ := objectstore.DraftKey(org, attempt)
	if e == pgx.ErrNoRows || expected != u.DraftKey || u.FrozenSourceID == nil || u.Provider == nil || u.Bucket == nil || json.Unmarshal([]byte(valueString(u.FrozenSourceID)), &original) != nil || original.Key != u.DraftKey || original.ETag == "" || original.Size != u.layout().Size || original.Size <= 0 {
		block("SOURCE_CHANGED")
	} else {
		objects = append(objects, RetirementObject{Provider: *u.Provider, Bucket: *u.Bucket, Source: original, Role: "upload_draft"})
	}
	var busy, agent, unknown bool
	e = tx.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM jobs WHERE org_id=$1 AND payload->>'session_id'=$2::text AND
   (state IN ('queued','running','retry_wait') OR updated_at>clock_timestamp()-$3::bigint*interval '1 second')),
 EXISTS(SELECT 1 FROM agent_file_drafts WHERE org_id=$1 AND id=$2::uuid),
 EXISTS(SELECT 1 FROM upload_attempts WHERE org_id=$1 AND session_id=$2::uuid AND draft_key<>$4 AND state IN ('starting','active','uncertain'))`, org, u.ID, int64(objectstore.MaxSignTTL/time.Second), u.DraftKey).Scan(&busy, &agent, &unknown)
	if e != nil {
		return out, objects, e
	}
	if busy {
		block("UPLOAD_JOB_ACTIVE")
	}
	if agent {
		block("AGENT_RECOVERY_SOURCE")
	}
	if unknown {
		block("UNCERTAIN_OBJECT_PROTECTED")
	}
	// Inspect every recorded promotion, not just the latest candidate pointer.
	rows, e := tx.Query(ctx, `SELECT id,file_id,version_id,object_key,state,source_digest,frozen_source,target_provider,target_bucket,completed_observed_at
 FROM object_transfers WHERE org_id=$1 AND session_id=$2 ORDER BY created_at,id LIMIT 65`, org, u.ID)
	if e != nil {
		return out, objects, e
	}
	identities := []any{cleanupSource(u), observed, original, candidateID, candidateKey, candidateSource}
	count := 0
	candidateMatched := false
	for rows.Next() {
		count++
		var id, fileID, versionID, key, state, sourceDigest, provider, bucket string
		var raw []byte
		var completed *time.Time
		if e = rows.Scan(&id, &fileID, &versionID, &key, &state, &sourceDigest, &raw, &provider, &bucket, &completed); e != nil {
			break
		}
		identities = append(identities, []any{id, fileID, versionID, key, state, sourceDigest, json.RawMessage(raw), provider, bucket, completed})
		if state != "complete" || completed == nil {
			block("UNCERTAIN_OBJECT_PROTECTED")
			continue
		}
		if observed == nil || completed.After(*observed) {
			v := *completed
			observed = &v
		}
		var source objectstore.Source
		expected, _ := objectstore.VersionKey(org, fileID, versionID)
		if key != expected || u.Provider == nil || provider != *u.Provider || u.Bucket == nil || bucket != *u.Bucket || sourceDigest != digest(original) || json.Unmarshal(raw, &source) != nil || source.Key != key || source.ETag == "" || source.Size != original.Size || u.FileID != nil && fileID != *u.FileID {
			block("SOURCE_CHANGED")
			continue
		}
		if candidateID != nil && id == *candidateID {
			var selected objectstore.Source
			candidateMatched = candidateKey != nil && *candidateKey == key && candidateSource != nil && json.Unmarshal([]byte(*candidateSource), &selected) == nil && selected == source
		}
		objects = append(objects, RetirementObject{Provider: provider, Bucket: bucket, Source: source, Role: "promotion_candidate"})
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, objects, e
	}
	if count > 63 || len(objects) > 64 {
		block("OBJECT_REVIEW_LIMIT")
	}
	if (u.State == "ready" || candidateID != nil || candidateKey != nil || candidateSource != nil) && !candidateMatched {
		block("SOURCE_CHANGED")
	}
	p, e := scanRetentionPolicy(tx.QueryRow(ctx, "SELECT "+retentionPolicyColumns+" FROM retention_policies WHERE org_id=$1 AND kind='draft' ORDER BY version DESC LIMIT 1", org))
	if e != nil {
		if core.PublicError(e).Status != 404 {
			return out, objects, e
		}
		block("POLICY_REQUIRED")
	} else {
		out.Policy = &RetentionRule{ID: p.ID, Version: p.Version, Kind: p.Kind, Enabled: p.Enabled, RetainSeconds: p.RetainSeconds, ObservationSeconds: p.ObservationSeconds}
		if !p.Enabled {
			block("POLICY_DISABLED")
		}
		if observed != nil {
			until := observed.Add(time.Duration(p.RetainSeconds) * time.Second)
			out.RetainUntil = &until
			if now.Before(until) {
				block("RETENTION_PERIOD_ACTIVE")
			}
		}
	}
	seen := map[string]bool{}
	for i := range objects {
		obj := &objects[i]
		b := s.Backends[obj.Provider]
		if b == nil || b.Provider != obj.Provider || b.Bucket != obj.Bucket || len(b.BindingSHA256) != 64 {
			block("CLEANUP_BACKEND_UNAVAILABLE")
			continue
		}
		obj.BindingSHA256 = b.BindingSHA256
		obj.ExclusiveNamespace = b.ExclusiveNamespace
		if obj.Provider == "oss" && (obj.Source.VersionID == "" || obj.Source.VersionID == "null") {
			block("NATIVE_OBJECT_VERSION_REQUIRED")
		}
		if obj.Provider == "r2" && !b.ExclusiveNamespace {
			block("EXCLUSIVE_NAMESPACE_REQUIRED")
		}
		if seen[obj.Source.Key] {
			block("SOURCE_CHANGED")
		}
		seen[obj.Source.Key] = true
		var reserved, formal, shared bool
		e = tx.QueryRow(ctx, `SELECT
   EXISTS(SELECT 1 FROM object_retirement_barriers WHERE org_id=$1 AND object_key=$2 AND plan_id::text<>$3),
   EXISTS(SELECT 1 FROM file_versions WHERE org_id=$1 AND object_key=$2) OR EXISTS(SELECT 1 FROM file_replicas WHERE org_id=$1 AND object_key=$2),
   EXISTS(SELECT 1 FROM upload_sessions WHERE org_id=$1 AND id<>$4 AND (draft_key=$2 OR candidate_key=$2))`, org, obj.Source.Key, own, u.ID).Scan(&reserved, &formal, &shared)
		if e != nil {
			return out, objects, e
		}
		if reserved {
			block("ORIGINAL_ALREADY_RETIRED")
		}
		if formal {
			block("FORMAL_OBJECT_PROTECTED")
		}
		if shared {
			block("SHARED_OBJECT_PROTECTED")
		}
	}
	out.ObjectCount = len(objects)
	out.SourceSHA256 = digest([]any{identities, objects})
	slices.Sort(out.BlockingReasons)
	out.BlockingReasons = slices.Compact(out.BlockingReasons)
	out.Eligible = len(out.BlockingReasons) == 0
	return out, objects, nil
}
func valueString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (s *ObjectRetirementService) CompletedDraftCandidates(ctx context.Context, a core.Actor, folderID string, filter Filter) (core.Page[RetirementCandidate], error) {
	out := core.Page[RetirementCandidate]{Items: []RetirementCandidate{}}
	tx, e := s.Files.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	f, e := s.folderActor(ctx, tx, a, folderID, false)
	if e != nil {
		return out, e
	}
	at, id, limit, binding, e := cleanupPage(s.Files, a, "completed-draft-candidates/"+folderID, filter)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, `SELECT id,created_at FROM upload_sessions WHERE org_id=$1 AND target_folder_id=$2 AND frozen_source_id IS NOT NULL
 AND committed_version_id IS NULL AND state<>'committed' AND source_channel IN ('web','android')
 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, folderID, at, id, limit+1)
	if e != nil {
		return out, e
	}
	type target struct {
		id string
		at time.Time
	}
	targets := []target{}
	for rows.Next() {
		var t target
		if e = rows.Scan(&t.id, &t.at); e != nil {
			break
		}
		targets = append(targets, t)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	for i, t := range targets {
		if i == limit {
			break
		}
		u, e := upload(ctx, tx, a.OrgID, t.id, false)
		if e != nil {
			return out, e
		}
		if e = cleanupUploadAccess(ctx, tx, a, f, u); e != nil {
			if slices.Contains([]int{403, 404}, core.PublicError(e).Status) {
				continue
			}
			return out, e
		}
		v, _, e := s.assessCompletedUpload(ctx, tx, a.OrgID, f, t.id, "")
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if len(targets) > limit {
		last := targets[limit-1]
		cursor := s.Files.Pager.Encode(platform.Cursor{Binding: binding, UpdatedAt: last.at, ID: last.id})
		out.NextCursor = &cursor
	}
	return out, nil
}

func (s *ObjectRetirementService) CompletedDraftPlans(ctx context.Context, a core.Actor, folderID string, filter Filter) (core.Page[RetirementPlan], error) {
	out := core.Page[RetirementPlan]{Items: []RetirementPlan{}}
	if _, e := s.folderActor(ctx, s.Files.DB, a, folderID, false); e != nil {
		return out, e
	}
	at, id, limit, binding, e := cleanupPage(s.Files, a, "completed-draft-plans/"+folderID, filter)
	if e != nil {
		return out, e
	}
	rows, e := s.Files.DB.Query(ctx, `SELECT id,created_at FROM object_retirement_plans WHERE org_id=$1 AND scope_folder_id=$2 AND kind='uncommitted_draft'
 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, folderID, at, id, limit+1)
	if e != nil {
		return out, e
	}
	type target struct {
		id string
		at time.Time
	}
	targets := []target{}
	for rows.Next() {
		var t target
		if e = rows.Scan(&t.id, &t.at); e != nil {
			break
		}
		targets = append(targets, t)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	for i, t := range targets {
		if i == limit {
			break
		}
		p, e := s.readPlan(ctx, s.Files.DB, a, t.id, false, false)
		if e != nil {
			if slices.Contains([]int{403, 404}, core.PublicError(e).Status) {
				continue
			}
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	if len(targets) > limit {
		last := targets[limit-1]
		cursor := s.Files.Pager.Encode(platform.Cursor{Binding: binding, UpdatedAt: last.at, ID: last.id})
		out.NextCursor = &cursor
	}
	return out, nil
}

func (s *ObjectRetirementService) CreateCompletedDraft(ctx context.Context, c core.Command, folderID string, in CompletedDraftRetirementCreate) (core.Result, error) {
	if !core.ValidID(folderID) || !core.ValidID(in.TargetID) || in.ExpectedFolderVersion < 1 || in.ExpectedUploadVersion < 1 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("target_id", "请核对目录、上传修订并说明清理原因")
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		f, e := s.folderActor(ctx, tx, c.Actor, folderID, false)
		if e != nil {
			return e
		}
		u, e := upload(ctx, tx, c.Actor.OrgID, in.TargetID, false)
		if e != nil {
			return e
		}
		return cleanupUploadAccess(ctx, tx, c.Actor, f, u)
	}, func(tx pgx.Tx) (core.Result, error) {
		f, e := folder(ctx, tx, c.Actor.OrgID, folderID, true)
		if e != nil {
			return core.Result{}, e
		}
		if _, e = upload(ctx, tx, c.Actor.OrgID, in.TargetID, true); e != nil {
			return core.Result{}, e
		}
		snapshot, objects, e := s.assessCompletedUpload(ctx, tx, c.Actor.OrgID, f, in.TargetID, "")
		if e != nil {
			return core.Result{}, e
		}
		if f.Version != in.ExpectedFolderVersion || snapshot.UploadVersion != in.ExpectedUploadVersion {
			return core.Result{}, core.Conflict()
		}
		if !snapshot.Eligible {
			return core.Result{}, core.Fail(409, "RETIREMENT_PROTECTED", "上传仍受保护或完成来源证据不完整，请刷新盘点")
		}
		var now time.Time
		if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
			return core.Result{}, e
		}
		ready := now.Add(time.Duration(snapshot.Policy.ObservationSeconds) * time.Second)
		raw, _ := json.Marshal(snapshot)
		raw, _ = core.CanonicalJSON(raw)
		objectRaw, _ := json.Marshal(objects)
		id := core.NewID()
		_, e = tx.Exec(ctx, `INSERT INTO object_retirement_plans(id,org_id,file_id,scope_folder_id,kind,target_id,snapshot,snapshot_sha256,objects,reason,created_at,ready_after,expires_at)
 VALUES($1,$2,NULLIF($3,'')::uuid,$4,'uncommitted_draft',$5,$6,$7,$8,$9,$10,$11,$12)`, id, c.Actor.OrgID, snapshot.FileID, folderID, in.TargetID, raw, core.Digest(raw), objectRaw, in.Reason, now, ready, ready.Add(7*24*time.Hour))
		if e != nil {
			return core.Result{}, e
		}
		for i, obj := range objects {
			if _, e = tx.Exec(ctx, "INSERT INTO object_retirement_items(id,org_id,plan_id,ordinal,provider) VALUES($1,$2,$3,$4,$5)", core.NewID(), c.Actor.OrgID, id, i, obj.Provider); e != nil {
				return core.Result{}, e
			}
		}
		p, e := retirementPlan(ctx, tx, c.Actor.OrgID, id, false)
		return core.Result{Status: 201, Data: p, Event: retirementEvent("file.original.retirement.created", p, in.Reason)}, e
	})
}
