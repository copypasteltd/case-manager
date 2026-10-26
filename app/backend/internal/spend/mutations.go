package spend

import (
	"context"
	"encoding/json"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"github.com/jackc/pgx/v5"
)

func stringsBlank(v string) bool { return strings.TrimSpace(v) == "" }
func result(d Record, event string, status int, reason string) core.Result {
	return core.Result{Status: status, Data: d, Event: core.Event{Type: event, ResourceType: d.Kind, ResourceID: d.ID, Version: d.Version, Payload: map[string]any{"revision_id": d.CurrentRevisionID, "reason": reason}}}
}
func insertRevision(ctx context.Context, tx pgx.Tx, a core.Actor, d Record, revision string, seq int64, base *string, c Content, values []LineValue, amount, employee *string, reason string) error {
	hash, err := contentHash(d, c, values, amount, employee)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO spend_revisions(id,org_id,document_id,seq,base_revision_id,payload_schema_version,payload,content_hash,amount,employee_reimbursement_amount,author_id,reason)
        VALUES($1,$2,$3,$4,$5,1,$6,$7,$8,$9,$10,$11)`, revision, a.OrgID, d.ID, seq, base, payload, hash, amount, employee, a.UserID, reason); err != nil {
		return err
	}
	for i, l := range c.Lines {
		raw, e := json.Marshal(l)
		if e != nil {
			return e
		}
		if _, err = tx.Exec(ctx, "INSERT INTO spend_lines(org_id,revision_id,position,line_id,payload,amount) VALUES($1,$2,$3,$4,$5,$6)", a.OrgID, revision, i+1, l.ID, raw, values[i].Amount); err != nil {
			return err
		}
	}
	for _, s := range Suppliers(c) {
		if _, err = tx.Exec(ctx, "INSERT INTO spend_revision_suppliers(org_id,revision_id,party_id,party_revision_id) VALUES($1,$2,$3,$4)", a.OrgID, revision, s.PartyID, s.RevisionID); err != nil {
			return err
		}
	}
	for _, id := range FileVersionIDs(c) {
		if _, err = tx.Exec(ctx, "INSERT INTO spend_revision_files(org_id,revision_id,version_id) VALUES($1,$2,$3)", a.OrgID, revision, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Create(ctx context.Context, c core.Command, kind string, in Create) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.replay(ctx, c.Actor, out, commandErr) }()
	if !ValidKind(kind) || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请填写有效业务类型和创建依据")
	}
	if in.ProjectID != nil {
		v := strings.ToLower(*in.ProjectID)
		in.ProjectID = &v
	}
	scope := Record{Kind: kind, CreatorID: c.Actor.UserID, ProjectID: in.ProjectID, Classification: in.Classification}
	authorize := func(tx pgx.Tx) error {
		if !core.Classification(in.Classification) || (in.ProjectID != nil && !core.ValidID(*in.ProjectID)) || (in.Classification == "project_restricted" && in.ProjectID == nil) {
			return core.Invalid("classification", "请明确有效项目与资料分类")
		}
		if err := authz.Require(ctx, tx, c.Actor, kind+".create", resource(scope)); err != nil {
			return err
		}
		return authz.Require(ctx, tx, c.Actor, kind+".read", resource(scope))
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		d := scope
		d.ID = core.NewID()
		d.CurrentRevisionID = core.NewID()
		if err := projectWritable(ctx, tx, c.Actor, d, true); err != nil {
			return core.Result{}, err
		}
		content, values, amount, employee, err := normalize(ctx, tx, c.Actor, d, in.Content, nil, false)
		if err != nil {
			return core.Result{}, err
		}
		prefix := "CP-P-"
		if kind == "claim" {
			prefix = "CP-E-"
		}
		if err = tx.QueryRow(ctx, "SELECT $1::text||nextval('spend_number_seq')::text", prefix).Scan(&d.DocumentNo); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO spend_documents(id,org_id,kind,document_no,project_id,creator_id,classification,current_revision_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, d.ID, c.Actor.OrgID, kind, d.DocumentNo, d.ProjectID, d.CreatorID, d.Classification, d.CurrentRevisionID); err != nil {
			return core.Result{}, err
		}
		if err = insertRevision(ctx, tx, c.Actor, d, d.CurrentRevisionID, 1, nil, content, values, amount, employee, in.Reason); err != nil {
			return core.Result{}, err
		}
		d, err = metadata(ctx, tx, c.Actor.OrgID, kind, d.ID)
		if err != nil {
			return core.Result{}, err
		}
		d, err = present(ctx, tx, c.Actor, d)
		return result(d, kind+".created", 201, in.Reason), err
	})
}
func (s *Service) mutate(ctx context.Context, c core.Command, kind, id, action string, version int64, fn func(pgx.Tx, Record) (core.Result, error)) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.replay(ctx, c.Actor, out, commandErr) }()
	if core.RequireVersion(version) != nil {
		return core.Result{}, core.Conflict()
	}
	check := func(tx pgx.Tx) (Record, error) {
		d, err := metadata(ctx, tx, c.Actor.OrgID, kind, id)
		if err != nil {
			return d, err
		}
		for _, action := range []string{kind + ".read", action} {
			if err = authz.Require(ctx, tx, c.Actor, action, resource(d)); err != nil {
				return Record{}, err
			}
		}
		if action == "purchase.execute" {
			if err = identity.RequireInteractiveSession(ctx, tx, c.Actor, true); err != nil {
				return Record{}, err
			}
		}
		return d, nil
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, err := check(tx); return err }, func(tx pgx.Tx) (core.Result, error) {
		d, err := check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(ctx, tx, c.Actor, d, true); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "SELECT id FROM spend_documents WHERE org_id=$1 AND kind=$2 AND id=$3 FOR UPDATE", c.Actor.OrgID, kind, id); err != nil {
			return core.Result{}, err
		}
		d, err = check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if d.Version != version {
			return core.Result{}, core.Conflict()
		}
		raw, err := json.Marshal(d)
		if err != nil {
			return core.Result{}, err
		}
		out, err := fn(tx, d)
		out.Event.BeforeDigest = core.Digest(raw)
		return out, err
	})
}
func (s *Service) Update(ctx context.Context, c core.Command, kind, id string, in Patch) (core.Result, error) {
	if !core.ValidID(in.BaseRevisionID) || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("base_revision_id", "请固定草稿版本并填写修改依据")
	}
	return s.mutate(ctx, c, kind, id, kind+".update", in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if !editable(d) {
			return core.Result{}, core.Fail(409, "SPEND_READ_ONLY", "当前阶段不允许修改业务内容")
		}
		if d.CurrentRevisionID != strings.ToLower(in.BaseRevisionID) {
			return core.Result{}, core.Conflict()
		}
		content, values, amount, employee, err := normalize(ctx, tx, c.Actor, d, in.Content, &d.Content, false)
		if err != nil {
			return core.Result{}, err
		}
		var seq int64
		if err = tx.QueryRow(ctx, "SELECT seq FROM spend_revisions WHERE org_id=$1 AND id=$2", c.Actor.OrgID, d.CurrentRevisionID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		revision := core.NewID()
		if err = insertRevision(ctx, tx, c.Actor, d, revision, seq+1, &d.CurrentRevisionID, content, values, amount, employee, in.Reason); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE spend_documents SET current_revision_id=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, revision); err != nil {
			return core.Result{}, err
		}
		d, err = metadata(ctx, tx, c.Actor.OrgID, kind, id)
		if err != nil {
			return core.Result{}, err
		}
		d, err = present(ctx, tx, c.Actor, d)
		return result(d, kind+".revised", 200, in.Reason), err
	})
}
func (s *Service) Archive(ctx context.Context, c core.Command, kind, id string, in ArchiveInput) (core.Result, error) {
	if core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请填写归档依据")
	}
	return s.mutate(ctx, c, kind, id, kind+".archive", in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if d.DeletedAt != nil || d.RequestID != nil || d.Lifecycle != "draft" || d.ApprovalState != "not_submitted" {
			return core.Result{}, core.Fail(409, "SPEND_ARCHIVE_REFUSED", "仅可归档未建立审批申请的草稿")
		}
		if _, err := tx.Exec(ctx, "UPDATE spend_documents SET deleted_at=clock_timestamp(),version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		d, err := metadata(ctx, tx, c.Actor.OrgID, kind, id)
		if err != nil {
			return core.Result{}, err
		}
		d, err = present(ctx, tx, c.Actor, d)
		return result(d, kind+".archived", 200, in.Reason), err
	})
}
func (s *Service) replay(ctx context.Context, a core.Actor, out core.Result, err error) (core.Result, error) {
	if err != nil || !out.Replayed {
		return out, err
	}
	raw, e := json.Marshal(out.Data)
	if e != nil {
		return core.Result{}, e
	}
	var d Record
	if e = json.Unmarshal(raw, &d); e != nil {
		return core.Result{}, e
	}
	current, e := metadata(ctx, s.DB, a.OrgID, d.Kind, d.ID)
	if e != nil {
		return core.Result{}, e
	}
	if _, e = present(ctx, s.DB, a, current); e != nil {
		return core.Result{}, e
	}
	d, e = present(ctx, s.DB, a, d)
	if e != nil {
		return core.Result{}, e
	}
	if d.Version != current.Version {
		d.CanEdit = false
		d.CanArchive = false
		d.CanPrepareRequest = false
		d.CanExecute = false
	}
	out.Data = d
	return out, nil
}
