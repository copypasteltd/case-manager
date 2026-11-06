package files

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"
)

// No body or provider key is exposed. Legacy external objects without native
// identity are explicitly blocked; this command only clears known DB text.
type DerivedTextFact struct {
	Extractor    string     `json:"extractor"`
	State        string     `json:"state"`
	SourceSHA256 string     `json:"source_sha256"`
	TextSHA256   *string    `json:"text_sha256"`
	Characters   *int       `json:"characters"`
	EngineSHA256 *string    `json:"engine_sha256"`
	PageCount    *int       `json:"page_count"`
	OCRPageCount *int       `json:"ocr_page_count"`
	IndexedAt    *time.Time `json:"indexed_at"`
}
type DerivedRetirementBasis struct {
	FileID               string            `json:"file_id"`
	FileVersion          int64             `json:"file_version"`
	VersionID            string            `json:"version_id"`
	SourceSHA256         string            `json:"source_sha256"`
	OriginalPlanID       *string           `json:"original_plan_id"`
	Policy               *RetentionRule    `json:"policy"`
	Texts                []DerivedTextFact `json:"texts"`
	UnknownExternalCount int               `json:"unknown_external_count"`
	Eligible             bool              `json:"eligible"`
	BlockingReasons      []string          `json:"blocking_reasons"`
}
type DerivedRetirementOverview struct {
	Basis       string                  `json:"basis"`
	BasisSHA256 string                  `json:"basis_sha256"`
	Plans       []DerivedRetirementPlan `json:"plans"`
	Complete    bool                    `json:"complete"`
}
type DerivedRetirementCreate struct {
	ExpectedFileVersion int64  `json:"expected_file_version"`
	BasisSHA256         string `json:"basis_sha256"`
	Reason              string `json:"reason"`
	Reviewed            bool   `json:"reviewed"`
}
type DerivedRetirementDecision struct {
	BasisSHA256 string `json:"basis_sha256"`
	Reason      string `json:"reason"`
	Reviewed    bool   `json:"reviewed"`
}
type DerivedRetirementReceipt struct {
	ID           string    `json:"id"`
	Action       string    `json:"action"`
	ActorID      string    `json:"actor_id"`
	Reason       string    `json:"reason"`
	RemovedCount int       `json:"removed_count"`
	CreatedAt    time.Time `json:"created_at"`
}
type DerivedRetirementPlan struct {
	ID          string                    `json:"id"`
	FileID      string                    `json:"file_id"`
	VersionID   string                    `json:"version_id"`
	Basis       string                    `json:"basis"`
	BasisSHA256 string                    `json:"basis_sha256"`
	ActorID     string                    `json:"actor_id"`
	Reason      string                    `json:"reason"`
	ReadyAfter  time.Time                 `json:"ready_after"`
	ExpiresAt   time.Time                 `json:"expires_at"`
	CreatedAt   time.Time                 `json:"created_at"`
	Receipt     *DerivedRetirementReceipt `json:"receipt"`
}

func derivedAccess(ctx context.Context, tx pgx.Tx, a core.Actor, id string, mfa bool) (File, error) {
	if e := identity.RequireInteractiveSession(ctx, tx, a, mfa); e != nil {
		return File{}, e
	}
	if e := retentionActor(ctx, tx, a, id, mfa); e != nil {
		return File{}, e
	}
	return file(ctx, tx, a.OrgID, id, true)
}
func derivedBasis(ctx context.Context, tx pgx.Tx, a core.Actor, f File, version string) (DerivedRetirementBasis, error) {
	b := DerivedRetirementBasis{FileID: f.ID, FileVersion: f.Version, VersionID: version, Texts: []DerivedTextFact{}, BlockingReasons: []string{}}
	var head bool
	var availability string
	e := tx.QueryRow(ctx, `SELECT sha256,availability,coalesce($4::uuid=id,false) FROM file_versions WHERE org_id=$1 AND file_id=$2 AND id=$3`, a.OrgID, f.ID, version, f.CurrentVersionID).Scan(&b.SourceSHA256, &availability, &head)
	if e != nil {
		return b, platform.DatabaseError(e)
	}
	if e = tx.QueryRow(ctx, `SELECT (SELECT id FROM object_retirement_plans WHERE org_id=$1 AND file_id=$2 AND kind='history' AND target_id=$3 AND state='completed' ORDER BY created_at DESC,id DESC LIMIT 1)`, a.OrgID, f.ID, version).Scan(&b.OriginalPlanID); e != nil {
		return b, e
	}
	var hold bool
	if e = tx.QueryRow(ctx, `SELECT $3::boolean OR EXISTS(SELECT 1 FROM retention_holds h WHERE h.org_id=$1 AND h.file_id=$2 AND (h.valid_until IS NULL OR h.valid_until>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM retention_hold_releases r WHERE r.org_id=h.org_id AND r.hold_id=h.id))`, a.OrgID, f.ID, f.LegalHold).Scan(&hold); e != nil {
		return b, e
	}
	var p RetentionRule
	e = tx.QueryRow(ctx, `SELECT id,version,kind,enabled,retain_seconds,observation_seconds FROM retention_policies WHERE org_id=$1 AND kind='formal_history' ORDER BY version DESC LIMIT 1`, a.OrgID).Scan(&p.ID, &p.Version, &p.Kind, &p.Enabled, &p.RetainSeconds, &p.ObservationSeconds)
	if e != nil && e != pgx.ErrNoRows {
		return b, e
	}
	if e == nil {
		b.Policy = &p
	}
	rows, e := tx.Query(ctx, `SELECT extractor,state,source_sha256,text_sha256,character_count,engine_sha256,page_count,ocr_page_count,indexed_at FROM file_text_derivations WHERE org_id=$1 AND file_id=$2 AND version_id=$3 ORDER BY extractor LIMIT 17`, a.OrgID, f.ID, version)
	if e != nil {
		return b, e
	}
	for rows.Next() {
		var v DerivedTextFact
		if e = rows.Scan(&v.Extractor, &v.State, &v.SourceSHA256, &v.TextSHA256, &v.Characters, &v.EngineSHA256, &v.PageCount, &v.OCRPageCount, &v.IndexedAt); e != nil {
			rows.Close()
			return b, e
		}
		b.Texts = append(b.Texts, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return b, e
	}
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM derived_artifacts WHERE org_id=$1 AND version_id=$2 AND output_key IS NOT NULL`, a.OrgID, version).Scan(&b.UnknownExternalCount); e != nil {
		return b, e
	}
	if head {
		b.BlockingReasons = append(b.BlockingReasons, "CURRENT_HEAD")
	}
	if availability != "purged" || b.OriginalPlanID == nil {
		b.BlockingReasons = append(b.BlockingReasons, "ORIGINAL_RETIREMENT_NOT_COMPLETE")
	}
	if hold {
		b.BlockingReasons = append(b.BlockingReasons, "ACTIVE_HOLD")
	}
	if b.Policy == nil || !p.Enabled {
		b.BlockingReasons = append(b.BlockingReasons, "POLICY_NOT_ENABLED")
	}
	ready := 0
	for _, v := range b.Texts {
		if v.State == "queued" {
			b.BlockingReasons = append(b.BlockingReasons, "EXTRACTION_IN_FLIGHT")
		}
		if v.State == "ready" {
			ready++
		}
	}
	if len(b.Texts) > 16 {
		b.BlockingReasons = append(b.BlockingReasons, "DERIVATION_BOUND_EXCEEDED")
	}
	if ready == 0 {
		b.BlockingReasons = append(b.BlockingReasons, "NO_KNOWN_TEXT_BODY")
	}
	b.Eligible = len(b.BlockingReasons) == 0
	return b, nil
}

const derivedPlanColumns = `p.id,p.file_id,p.version_id,p.basis,p.basis_sha256,p.actor_id,p.reason,p.ready_after,p.expires_at,p.created_at,d.id,d.action,d.actor_id,d.reason,d.removed_count,d.created_at`

func scanDerivedPlan(row pgx.Row) (DerivedRetirementPlan, error) {
	var p DerivedRetirementPlan
	var id, action, actor, reason *string
	var count *int
	var at *time.Time
	e := row.Scan(&p.ID, &p.FileID, &p.VersionID, &p.Basis, &p.BasisSHA256, &p.ActorID, &p.Reason, &p.ReadyAfter, &p.ExpiresAt, &p.CreatedAt, &id, &action, &actor, &reason, &count, &at)
	if e != nil {
		return p, platform.DatabaseError(e)
	}
	if id != nil {
		p.Receipt = &DerivedRetirementReceipt{*id, *action, *actor, *reason, *count, *at}
	}
	return p, nil
}
func readDerivedPlan(ctx context.Context, tx pgx.Tx, org, id string) (DerivedRetirementPlan, error) {
	return scanDerivedPlan(tx.QueryRow(ctx, `SELECT `+derivedPlanColumns+` FROM file_derived_retirement_plans p LEFT JOIN file_derived_retirement_decisions d ON d.org_id=p.org_id AND d.plan_id=p.id WHERE p.org_id=$1 AND p.id=$2`, org, id))
}
func (s *Service) DerivedRetirement(ctx context.Context, a core.Actor, id, version string) (DerivedRetirementOverview, error) {
	out := DerivedRetirementOverview{Plans: []DerivedRetirementPlan{}, Complete: true}
	if !core.ValidID(id) || !core.ValidID(version) {
		return out, core.Missing()
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(ctx)
	f, e := derivedAccess(ctx, tx, a, id, false)
	if e != nil {
		return out, e
	}
	b, e := derivedBasis(ctx, tx, a, f, version)
	if e != nil {
		return out, e
	}
	raw, _ := json.Marshal(b)
	out.Basis = string(raw)
	out.BasisSHA256 = core.Digest(raw)
	rows, e := tx.Query(ctx, `SELECT `+derivedPlanColumns+` FROM file_derived_retirement_plans p LEFT JOIN file_derived_retirement_decisions d ON d.org_id=p.org_id AND d.plan_id=p.id WHERE p.org_id=$1 AND p.file_id=$2 AND p.version_id=$3 ORDER BY p.created_at DESC,p.id DESC LIMIT 101`, a.OrgID, id, version)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		p, e := scanDerivedPlan(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Plans = append(out.Plans, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Plans) > 100 {
		return out, core.Fail(409, "DERIVED_HISTORY_BOUND", "派生清理历史超过完整审阅上限")
	}
	return out, tx.Commit(ctx)
}
func (s *Service) CreateDerivedRetirement(ctx context.Context, c core.Command, id, version string, in DerivedRetirementCreate) (core.Result, error) {
	if !core.ValidID(id) || !core.ValidID(version) {
		return core.Result{}, core.Missing()
	}
	if in.ExpectedFileVersion < 1 || !shaPattern.MatchString(in.BasisSHA256) || core.Name(in.Reason) != nil || !in.Reviewed {
		return core.Result{}, core.Invalid("derived_retirement", "请核对准确派生资料、文件版本和清理依据")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := derivedAccess(ctx, tx, c.Actor, id, true); return e }, func(tx pgx.Tx) (core.Result, error) {
		f, e := derivedAccess(ctx, tx, c.Actor, id, true)
		if e != nil {
			return core.Result{}, e
		}
		if f.Version != in.ExpectedFileVersion {
			return core.Result{}, core.Conflict()
		}
		b, e := derivedBasis(ctx, tx, c.Actor, f, version)
		if e != nil {
			return core.Result{}, e
		}
		raw, _ := json.Marshal(b)
		if core.Digest(raw) != in.BasisSHA256 {
			return core.Result{}, core.Conflict()
		}
		if !b.Eligible {
			return core.Result{}, core.Fail(409, "DERIVED_RETIREMENT_BLOCKED", "原件须已完成历史清理、解除保留且存在有效文字资料")
		}
		wait := max(int64(86400), b.Policy.ObservationSeconds)
		plan := core.NewID()
		_, e = tx.Exec(ctx, `INSERT INTO file_derived_retirement_plans(id,org_id,file_id,version_id,actor_id,session_id,basis,basis_sha256,reason,created_at,ready_after,expires_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,t,t+make_interval(secs=>$10),t+make_interval(secs=>$10)+interval '7 days' FROM (SELECT clock_timestamp() t) stamp`, plan, c.Actor.OrgID, id, version, c.Actor.UserID, c.Actor.SessionID, string(raw), in.BasisSHA256, in.Reason, wait)
		if e != nil {
			return core.Result{}, e
		}
		p, e := readDerivedPlan(ctx, tx, c.Actor.OrgID, plan)
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "file.derived.retirement.created", ResourceType: "file", ResourceID: id, Version: f.Version, Payload: map[string]any{"plan_id": plan, "basis_sha256": in.BasisSHA256, "reason": in.Reason}}}, e
	})
}
func (s *Service) DecideDerivedRetirement(ctx context.Context, c core.Command, id, action string, in DerivedRetirementDecision) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if (action != "execute" && action != "cancel") || !shaPattern.MatchString(in.BasisSHA256) || core.Name(in.Reason) != nil || !in.Reviewed {
		return core.Result{}, core.Invalid("derived_retirement", "请重新核对准确清理计划和操作依据")
	}
	authorize := func(tx pgx.Tx) error {
		p, e := readDerivedPlan(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return e
		}
		_, e = derivedAccess(ctx, tx, c.Actor, p.FileID, true)
		return e
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		p, e := readDerivedPlan(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		f, e := derivedAccess(ctx, tx, c.Actor, p.FileID, true)
		if e != nil {
			return core.Result{}, e
		}
		p, e = readDerivedPlan(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return core.Result{}, e
		}
		if p.Receipt != nil || p.BasisSHA256 != in.BasisSHA256 {
			return core.Result{}, core.Conflict()
		}
		count := 0
		if action == "execute" {
			b, e := derivedBasis(ctx, tx, c.Actor, f, p.VersionID)
			if e != nil {
				return core.Result{}, e
			}
			raw, _ := json.Marshal(b)
			if !b.Eligible || core.Digest(raw) != p.BasisSHA256 {
				return core.Result{}, core.Fail(409, "DERIVED_BASIS_CHANGED", "文件、原件清理、保留政策或派生资料已变化，请取消后重建计划")
			}
			var due bool
			if e = tx.QueryRow(ctx, `SELECT clock_timestamp()>=$1 AND clock_timestamp()<$2`, p.ReadyAfter, p.ExpiresAt).Scan(&due); e != nil {
				return core.Result{}, e
			}
			if !due {
				return core.Result{}, core.Fail(409, "DERIVED_OBSERVATION_REQUIRED", "须完成观察期且在计划有效期内执行")
			}
			for _, v := range b.Texts {
				if v.State == "ready" {
					count++
				}
			}
		}
		_, e = tx.Exec(ctx, `INSERT INTO file_derived_retirement_decisions(id,org_id,plan_id,actor_id,session_id,action,reason,reviewed,removed_count) VALUES($1,$2,$3,$4,$5,$6,$7,true,$8)`, core.NewID(), c.Actor.OrgID, id, c.Actor.UserID, c.Actor.SessionID, action, in.Reason, count)
		if e != nil {
			return core.Result{}, e
		}
		if action == "execute" {
			tag, e := tx.Exec(ctx, `UPDATE file_text_derivations SET state='blocked',body=NULL,text_sha256=NULL,character_count=NULL,page_count=NULL,ocr_page_count=NULL,error_code='FILE_TEXT_RETIRED' WHERE org_id=$1 AND file_id=$2 AND version_id=$3 AND state='ready'`, c.Actor.OrgID, p.FileID, p.VersionID)
			if e != nil {
				return core.Result{}, e
			}
			if int(tag.RowsAffected()) != count {
				return core.Result{}, core.Conflict()
			}
		}
		p, e = readDerivedPlan(ctx, tx, c.Actor.OrgID, id)
		return core.Result{Data: p, Event: core.Event{Type: "file.derived.retirement." + action, ResourceType: "file", ResourceID: p.FileID, Version: f.Version, Payload: map[string]any{"plan_id": id, "basis_sha256": p.BasisSHA256, "removed_count": count, "reason": in.Reason}}}, e
	})
}
