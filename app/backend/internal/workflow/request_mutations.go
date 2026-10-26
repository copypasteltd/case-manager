package workflow

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/spend"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"regexp"
	"strings"
)

var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validBusinessBase(version int64, revision, hash, reason string) error {
	if core.RequireVersion(version) != nil || !core.ValidID(revision) || !digestPattern.MatchString(hash) || core.Name(reason) != nil {
		return core.Invalid("business_revision_id", "请固定当前业务版本、内容摘要及操作依据")
	}
	return nil
}
func requestResult(r Request, event string, status int, reason string) core.Result {
	return core.Result{Status: status, Data: r, Event: core.Event{Type: event, ResourceType: "request", ResourceID: r.ID, Version: r.Version, Payload: map[string]any{"reason": reason, "kind": r.Kind, "business_id": r.BusinessID, "revision_id": r.CurrentRevisionID}}}
}
func insertRequestRevision(ctx context.Context, tx pgx.Tx, a core.Actor, r Request, id string, seq int64, base *string, payload Snapshot, flow *Definition, proofs []AuthorityEvidence, reason string) (string, error) {
	hash, err := revisionHash(payload, flow, proofs)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if proofs == nil {
		proofs = []AuthorityEvidence{}
	}
	evidence, err := json.Marshal(proofs)
	if err != nil {
		return "", err
	}
	var definition *string
	var snapshot any
	if flow != nil {
		definition = &flow.ID
		raw, e := json.Marshal(flow)
		if e != nil {
			return "", e
		}
		snapshot = raw
	}
	_, err = tx.Exec(ctx, `INSERT INTO request_revisions(id,org_id,request_id,seq,base_revision_id,business_revision_id,business_content_hash,
        payload_schema_version,payload,workflow_definition_id,workflow_snapshot,reviewer_evidence,content_hash,author_id,reason)
        VALUES($1,$2,$3,$4,$5,$6,$7,1,$8,$9,$10,$11,$12,$13,$14)`, id, a.OrgID, r.ID, seq, base, payload.BusinessRevisionID, payload.BusinessContentHash, data, definition, snapshot, evidence, hash, a.UserID, reason)
	return hash, err
}
func lockRequest(ctx context.Context, tx pgx.Tx, a core.Actor, id, action string) (Request, BusinessSubject, error) {
	r, err := requestMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return r, BusinessSubject{}, err
	}
	if err = requireRequest(ctx, tx, a, r, action); err != nil {
		return r, BusinessSubject{}, err
	}
	// For reviewer mutations the same business lock order is used, without
	// requiring the reviewer to hold the business editing action.
	d, err := businessRead(ctx, tx, a, r)
	if err != nil {
		return r, d, err
	}
	if d.ProjectID != nil {
		if _, err = tx.Exec(ctx, "SELECT id FROM projects WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, *d.ProjectID); err != nil {
			return r, d, err
		}
	}
	if err = lockBusiness(ctx, tx, a, r.Kind, r.BusinessID); err != nil {
		return r, d, err
	}
	if _, err = tx.Exec(ctx, "SELECT id FROM requests WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id); err != nil {
		return r, d, err
	}
	r, err = requestMetadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return r, d, err
	}
	if err = requireRequest(ctx, tx, a, r, action); err != nil {
		return r, d, err
	}
	d, err = businessRead(ctx, tx, a, r)
	return r, d, err
}
func (s *Service) CreateRequest(ctx context.Context, c core.Command, in RequestCreate) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.requestReplay(ctx, c.Actor, out, commandErr) }()
	in.BusinessID = strings.ToLower(in.BusinessID)
	in.BusinessRevisionID = strings.ToLower(in.BusinessRevisionID)
	if !core.ValidID(in.BusinessID) || (in.Kind != "contract" && in.Kind != "purchase_variance" && in.Kind != "change" && !spend.ValidKind(in.Kind)) {
		return core.Result{}, core.Invalid("kind", "请指定已接入的业务类型与准确对象")
	}
	if err := validBusinessBase(in.ExpectedBusinessVersion, in.BusinessRevisionID, in.BusinessContentHash, in.Reason); err != nil {
		return core.Result{}, err
	}
	authorize := func(tx pgx.Tx) error {
		d, e := businessRead(ctx, tx, c.Actor, Request{Kind: in.Kind, BusinessID: in.BusinessID})
		if e != nil {
			return e
		}
		if d.Kind == "change" {
			if e = authz.Require(ctx, tx, c.Actor, "change.submit", businessResource(d)); e != nil {
				return e
			}
		}
		if e = authz.Require(ctx, tx, c.Actor, d.Kind+".update", businessResource(d)); e != nil {
			return e
		}
		r := Request{BusinessID: d.ID, Kind: in.Kind, ProjectID: d.ProjectID, ApplicantID: c.Actor.UserID, Classification: d.Classification}
		if e = authz.Require(ctx, tx, c.Actor, "request.create", requestResource(r)); e != nil {
			return e
		}
		return authz.Require(ctx, tx, c.Actor, "request.read", requestResource(r))
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		d, err := subject(ctx, tx, c.Actor, in.Kind, in.BusinessID, true, false)
		if err != nil {
			return core.Result{}, err
		}
		if d.Version != in.ExpectedBusinessVersion || d.CurrentRevisionID != in.BusinessRevisionID || d.ContentHash != in.BusinessContentHash {
			return core.Result{}, core.Conflict()
		}
		if d.RequestID != nil {
			existing, e := requestMetadata(ctx, tx, c.Actor.OrgID, *d.RequestID)
			if e != nil {
				return core.Result{}, e
			}
			if existing.State != "rejected" {
				return core.Result{}, core.Fail(409, "REQUEST_ALREADY_EXISTS", "此业务已有申请，请继续处理该申请")
			}
		}
		id, revision := core.NewID(), core.NewID()
		r := Request{ID: id, Kind: in.Kind, BusinessID: d.ID, ApplicantID: c.Actor.UserID, ProjectID: d.ProjectID, Classification: d.Classification}
		if _, err = tx.Exec(ctx, `INSERT INTO requests(id,org_id,kind,business_id,title,applicant_id,project_id,classification,state,current_revision_id)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,'draft',$9)`, id, c.Actor.OrgID, in.Kind, d.ID, d.Title, c.Actor.UserID, d.ProjectID, d.Classification, revision); err != nil {
			return core.Result{}, err
		}
		if _, err = insertRequestRevision(ctx, tx, c.Actor, r, revision, 1, nil, snapshot(d), nil, nil, in.Reason); err != nil {
			return core.Result{}, err
		}
		if err = setBusinessState(ctx, tx, c.Actor, d, id, "not_submitted"); err != nil {
			return core.Result{}, err
		}
		r, err = requestMetadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		r, err = presentRequest(ctx, tx, c.Actor, r)
		return requestResult(r, "request.created", 201, in.Reason), err
	})
}
func (s *Service) ReviseRequest(ctx context.Context, c core.Command, id string, in RequestRevise) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.requestReplay(ctx, c.Actor, out, commandErr) }()
	in.BusinessRevisionID = strings.ToLower(in.BusinessRevisionID)
	if core.RequireVersion(in.ExpectedVersion) != nil {
		return core.Result{}, core.Conflict()
	}
	if err := validBusinessBase(in.ExpectedBusinessVersion, in.BusinessRevisionID, in.BusinessContentHash, in.Reason); err != nil {
		return core.Result{}, err
	}
	authorize := func(tx pgx.Tx) error {
		r, e := requestMetadata(ctx, tx, c.Actor.OrgID, id)
		if e != nil {
			return e
		}
		if r.ApplicantID != c.Actor.UserID {
			return core.Missing()
		}
		if e = requireRequest(ctx, tx, c.Actor, r, "request.update"); e != nil {
			return e
		}
		d, e := businessRead(ctx, tx, c.Actor, r)
		if e != nil {
			return e
		}
		return authz.Require(ctx, tx, c.Actor, d.Kind+".update", businessResource(d))
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		r, d, err := lockRequest(ctx, tx, c.Actor, id, "request.update")
		if err != nil {
			return core.Result{}, err
		}
		if r.Version != in.ExpectedVersion || d.Version != in.ExpectedBusinessVersion || d.CurrentRevisionID != in.BusinessRevisionID || d.ContentHash != in.BusinessContentHash {
			return core.Result{}, core.Conflict()
		}
		if !draftState(r.State) || d.RequestID == nil || *d.RequestID != r.ID {
			return core.Result{}, core.Fail(409, "REQUEST_READ_ONLY", "当前申请不能修改送审草稿")
		}
		d, err = subject(ctx, tx, c.Actor, r.Kind, r.BusinessID, false, false)
		if err != nil {
			return core.Result{}, err
		}
		var seq int64
		if err = tx.QueryRow(ctx, "SELECT seq FROM request_revisions WHERE org_id=$1 AND request_id=$2 AND id=$3", c.Actor.OrgID, id, r.CurrentRevisionID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		revision := core.NewID()
		if _, err = insertRequestRevision(ctx, tx, c.Actor, r, revision, seq+1, &r.CurrentRevisionID, snapshot(d), nil, nil, in.Reason); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE requests SET current_revision_id=$3,state='draft',active_instance_id=NULL,title=$4,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, revision, d.Title); err != nil {
			return core.Result{}, err
		}
		if err = setBusinessState(ctx, tx, c.Actor, d, r.ID, "not_submitted"); err != nil {
			return core.Result{}, err
		}
		r, err = requestMetadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		r, err = presentRequest(ctx, tx, c.Actor, r)
		return requestResult(r, "request.revised", 200, in.Reason), err
	})
}

func (s *Service) requestReplay(ctx context.Context, a core.Actor, result core.Result, err error) (core.Result, error) {
	if err != nil || !result.Replayed {
		return result, err
	}
	raw, e := json.Marshal(result.Data)
	if e != nil {
		return core.Result{}, e
	}
	var r Request
	if e = json.Unmarshal(raw, &r); e != nil {
		return core.Result{}, e
	}
	current, e := requestMetadata(ctx, s.DB, a.OrgID, r.ID)
	if e != nil {
		return core.Result{}, e
	}
	if e = requireRequest(ctx, s.DB, a, current, "request.read"); e != nil {
		return core.Result{}, e
	}
	if r.Payload, e = redactSnapshot(ctx, s.DB, a, r.Payload); e != nil {
		return core.Result{}, e
	}
	r.CanSubmit = false
	r.CanRevise = false
	r.CanWithdraw = false
	for i := range r.Steps {
		r.Steps[i].CanDecide = false
	}
	result.Data = r
	return result, nil
}
