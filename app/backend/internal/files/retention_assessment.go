package files

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type RetentionAssessmentInput struct {
	ExpectedFileVersion int64  `json:"expected_file_version"`
	Reason              string `json:"reason"`
}
type RetentionRule struct {
	ID                 string `json:"id"`
	Version            int64  `json:"version"`
	Kind               string `json:"kind"`
	Enabled            bool   `json:"enabled"`
	RetainSeconds      int64  `json:"retain_seconds"`
	ObservationSeconds int64  `json:"observation_seconds"`
}
type RetentionSnapshot struct {
	FileID                 string           `json:"file_id"`
	FileVersion            int64            `json:"file_version"`
	VersionID              string           `json:"version_id"`
	Seq                    int64            `json:"seq"`
	SizeBytes              string           `json:"size_bytes"`
	CheckedAt              time.Time        `json:"checked_at"`
	Policy                 *RetentionRule   `json:"policy"`
	TrashPolicy            *RetentionRule   `json:"trash_policy"`
	RetainUntil            *time.Time       `json:"retain_until"`
	LegalHold              bool             `json:"legal_hold"`
	ActiveHoldCount        int64            `json:"active_hold_count"`
	ReferenceCounts        map[string]int64 `json:"reference_counts"`
	ActiveRestoreCount     int64            `json:"active_restore_count"`
	ActiveUploadCount      int64            `json:"active_upload_count"`
	UnexpiredDownloadCount int64            `json:"unexpired_download_count"`
	IncompleteReplicaCount int64            `json:"incomplete_replica_count"`
	DerivedObjectCount     int64            `json:"derived_object_count"`
	EligibleForReview      bool             `json:"eligible_for_review"`
	ExecutionAvailable     bool             `json:"execution_available"`
	BlockingReasons        []string         `json:"blocking_reasons"`
}
type RetentionAssessment struct {
	ID             string            `json:"id"`
	Snapshot       RetentionSnapshot `json:"snapshot"`
	SnapshotSHA256 string            `json:"snapshot_sha256"`
	Reason         string            `json:"reason"`
	CreatedAt      time.Time         `json:"created_at"`
}

func readRetentionAssessment(row pgx.Row) (RetentionAssessment, error) {
	var out RetentionAssessment
	var raw []byte
	err := row.Scan(&out.ID, &raw, &out.SnapshotSHA256, &out.Reason, &out.CreatedAt)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	err = json.Unmarshal(raw, &out.Snapshot)
	return out, err
}

const assessmentColumns = `id,snapshot,snapshot_sha256,reason,created_at`

func (s *Service) RetentionAssessment(ctx context.Context, a core.Actor, id string) (RetentionAssessment, error) {
	if !core.ValidID(id) {
		return RetentionAssessment{}, core.Missing()
	}
	out, err := readRetentionAssessment(s.DB.QueryRow(ctx, "SELECT "+assessmentColumns+" FROM retention_assessments WHERE org_id=$1 AND id=$2", a.OrgID, id))
	if err != nil {
		return out, err
	}
	_, err = retentionAccess(ctx, s.DB, a, out.Snapshot.FileID)
	return out, err
}
func (s *Service) AssessRetention(ctx context.Context, c core.Command, id string, in RetentionAssessmentInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if in.ExpectedFileVersion < 1 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请填写文件当前版本及评估原因")
	}
	var fileID string
	if err := s.DB.QueryRow(ctx, "SELECT file_id FROM file_versions WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id).Scan(&fileID); err != nil {
		return core.Result{}, platform.DatabaseError(err)
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return retentionActor(ctx, tx, c.Actor, fileID, false) }, func(tx pgx.Tx) (core.Result, error) {
		f, err := file(ctx, tx, c.Actor.OrgID, fileID, true)
		if err != nil {
			return core.Result{}, err
		}
		if err = retentionActor(ctx, tx, c.Actor, fileID, false); err != nil {
			return core.Result{}, err
		}
		if f.Version != in.ExpectedFileVersion {
			return core.Result{}, core.Conflict()
		}
		snapshot, err := assessVersion(ctx, tx, c.Actor.OrgID, f, id)
		if err != nil {
			return core.Result{}, err
		}
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return core.Result{}, err
		}
		raw, err = core.CanonicalJSON(raw)
		if err != nil {
			return core.Result{}, err
		}
		assessmentID := core.NewID()
		if _, err = tx.Exec(ctx, `INSERT INTO retention_assessments(id,org_id,file_id,version_id,requester_id,file_version,snapshot,snapshot_sha256,reason)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, assessmentID, c.Actor.OrgID, fileID, id, c.Actor.UserID, f.Version, raw, core.Digest(raw), in.Reason); err != nil {
			return core.Result{}, err
		}
		out, err := readRetentionAssessment(tx.QueryRow(ctx, "SELECT "+assessmentColumns+" FROM retention_assessments WHERE id=$1", assessmentID))
		return core.Result{Status: 201, Data: out, Event: core.Event{Type: "file.retention.assessed", ResourceType: "retention_assessment", ResourceID: assessmentID, Version: 1,
			Payload: map[string]any{"file_id": fileID, "version_id": id, "snapshot_sha256": core.Digest(raw), "reason": in.Reason}}}, err
	})
}
func assessVersion(ctx context.Context, tx pgx.Tx, org string, f File, id string) (RetentionSnapshot, error) {
	out := RetentionSnapshot{FileID: f.ID, FileVersion: f.Version, VersionID: id, LegalHold: f.LegalHold, ReferenceCounts: map[string]int64{}, BlockingReasons: []string{}}
	var created time.Time
	var trashed *time.Time
	var refs []byte
	var verified int64
	// One statement snapshot captures all dependency counts. This report is
	// evidence for review, never a lease or authorization to delete later.
	err := tx.QueryRow(ctx, `SELECT statement_timestamp(),v.seq,v.size_bytes::text,v.created_at,f.trashed_at,
        (SELECT count(*) FROM retention_holds h WHERE h.org_id=v.org_id AND h.file_id=v.file_id
            AND (h.version_id IS NULL OR h.version_id=v.id) AND (h.valid_until IS NULL OR h.valid_until>statement_timestamp())
            AND NOT EXISTS(SELECT 1 FROM retention_hold_releases z WHERE z.org_id=h.org_id AND z.hold_id=h.id)),
        (SELECT coalesce(jsonb_object_agg(owner_type,n),'{}'::jsonb) FROM (
            SELECT r.owner_type,count(*) n FROM file_version_references r WHERE r.org_id=v.org_id AND r.version_id=v.id
            AND NOT EXISTS(SELECT 1 FROM file_reference_releases z WHERE z.org_id=r.org_id AND z.reference_id=r.id) GROUP BY r.owner_type) q),
        (SELECT count(*) FROM file_history_restores r WHERE r.org_id=v.org_id AND r.source_version_id=v.id
            AND r.state IN ('queued','copying','verifying','retry_wait','ready') AND r.expires_at>statement_timestamp()),
        (SELECT count(*) FROM upload_sessions u WHERE u.org_id=v.org_id AND u.base_version_id=v.id
            AND u.state NOT IN ('committed','aborted','expired','failed') AND u.expires_at>statement_timestamp()),
        (SELECT count(*) FROM download_permits d WHERE d.org_id=v.org_id AND d.version_id=v.id AND d.expires_at>statement_timestamp()),
        (SELECT count(*) FROM file_replicas r WHERE r.org_id=v.org_id AND r.version_id=v.id AND r.state<>'verified'),
        (SELECT count(DISTINCT provider) FROM file_replicas r WHERE r.org_id=v.org_id AND r.version_id=v.id AND r.state='verified'
            AND r.expected_sha256=v.sha256 AND r.actual_sha256=v.sha256 AND r.verified_at IS NOT NULL AND r.source_id IS NOT NULL),
        (SELECT count(*) FROM derived_artifacts d WHERE d.org_id=v.org_id AND d.version_id=v.id AND d.output_key IS NOT NULL)
        FROM file_versions v JOIN files f ON f.org_id=v.org_id AND f.id=v.file_id WHERE v.org_id=$1 AND v.id=$2`, org, id).
		Scan(&out.CheckedAt, &out.Seq, &out.SizeBytes, &created, &trashed, &out.ActiveHoldCount, &refs, &out.ActiveRestoreCount,
			&out.ActiveUploadCount, &out.UnexpiredDownloadCount, &out.IncompleteReplicaCount, &verified, &out.DerivedObjectCount)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(refs, &out.ReferenceCounts); err != nil {
		return out, err
	}
	kinds := []string{"formal_history"}
	if f.State == "trashed" {
		kinds = append(kinds, "trash")
	}
	for _, kind := range kinds {
		p, e := scanRetentionPolicy(tx.QueryRow(ctx, "SELECT "+retentionPolicyColumns+" FROM retention_policies WHERE org_id=$1 AND kind=$2 ORDER BY version DESC LIMIT 1", org, kind))
		if e != nil {
			if core.PublicError(e).Status != 404 {
				return out, e
			}
			if !slices.Contains(out.BlockingReasons, "POLICY_REQUIRED") {
				out.BlockingReasons = append(out.BlockingReasons, "POLICY_REQUIRED")
			}
			continue
		}
		rule := &RetentionRule{ID: p.ID, Version: p.Version, Kind: p.Kind, Enabled: p.Enabled, RetainSeconds: p.RetainSeconds, ObservationSeconds: p.ObservationSeconds}
		base := created
		if kind == "trash" {
			out.TrashPolicy = rule
			if trashed != nil {
				base = *trashed
			}
		} else {
			out.Policy = rule
		}
		until := base.Add(time.Duration(p.RetainSeconds) * time.Second)
		if out.RetainUntil == nil || until.After(*out.RetainUntil) {
			out.RetainUntil = &until
		}
		if !p.Enabled && !slices.Contains(out.BlockingReasons, "POLICY_DISABLED") {
			out.BlockingReasons = append(out.BlockingReasons, "POLICY_DISABLED")
		}
	}
	if out.RetainUntil != nil && out.CheckedAt.Before(*out.RetainUntil) {
		out.BlockingReasons = append(out.BlockingReasons, "RETENTION_PERIOD_ACTIVE")
	}
	if f.CurrentVersionID != nil && *f.CurrentVersionID == id {
		out.BlockingReasons = append(out.BlockingReasons, "CURRENT_VERSION")
	}
	for _, condition := range []struct {
		yes  bool
		code string
	}{
		{out.LegalHold, "LEGAL_HOLD"}, {out.ActiveHoldCount > 0, "RETENTION_HOLD"}, {len(out.ReferenceCounts) > 0, "BUSINESS_REFERENCE"},
		{out.ActiveRestoreCount > 0, "RESTORE_ACTIVE"}, {out.ActiveUploadCount > 0, "UPLOAD_ACTIVE"}, {out.UnexpiredDownloadCount > 0, "DOWNLOAD_PERMIT_ACTIVE"},
		{out.IncompleteReplicaCount > 0 || verified < 2, "REPLICA_EVIDENCE_INCOMPLETE"}, {out.DerivedObjectCount > 0, "DERIVED_OBJECT_REVIEW_REQUIRED"},
	} {
		if condition.yes {
			out.BlockingReasons = append(out.BlockingReasons, condition.code)
		}
	}
	out.EligibleForReview = len(out.BlockingReasons) == 0
	// This endpoint remains a read-only assessment. Known originals use the
	// separate fixed-plan retirement workflow and permanent dependency barrier.
	return out, nil
}
