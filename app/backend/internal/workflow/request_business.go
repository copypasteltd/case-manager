package workflow

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/spend"
	"encoding/json"
	"time"
)

// BusinessSubject carries shared routing metadata and discriminated immutable
// content. Purchase and claim content never masquerades as contract fields.
type BusinessSubject struct {
	ID, Kind, CreatorID, Title, Category, Classification, Lifecycle, ApprovalState, CurrentRevisionID, ContentHash string
	Version                                                                                                        int64
	ProjectID, Currency, Amount, RequestID                                                                         *string
	DeletedAt                                                                                                      *time.Time
	Payload                                                                                                        Snapshot
}

func contractSubject(d contracts.Contract) (BusinessSubject, error) {
	raw, err := json.Marshal(d.Content)
	if err != nil {
		return BusinessSubject{}, err
	}
	p := Snapshot{BusinessPayloadSchemaVersion: d.PayloadSchemaVersion, Kind: "contract", BusinessID: d.ID, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, ContractNo: d.ContractNo, ProjectID: d.ProjectID, Classification: d.Classification, Content: raw, Counterparty: d.Counterparty, Attachments: []fileref.Evidence{}}
	if d.DraftFile != nil {
		p.Attachments = append(p.Attachments, *d.DraftFile)
	}
	return BusinessSubject{ID: d.ID, Kind: "contract", CreatorID: d.CreatorID, Title: d.Title, Category: d.Category, Classification: d.Classification, Lifecycle: d.Lifecycle, ApprovalState: d.ApprovalState, CurrentRevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash, Version: d.Version, ProjectID: d.ProjectID, Currency: d.Currency, Amount: d.Amount, RequestID: d.RequestID, DeletedAt: d.DeletedAt, Payload: p}, nil
}
func spendSubject(d spend.Record) (BusinessSubject, error) {
	values := append([]spend.LineValue{}, d.LineValues...)
	for i := range values {
		values[i].DuplicateReceipt = false
	}
	raw, err := json.Marshal(d.Content)
	if err != nil {
		return BusinessSubject{}, err
	}
	p := Snapshot{BusinessPayloadSchemaVersion: d.PayloadSchemaVersion, Kind: d.Kind, BusinessID: d.ID, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, DocumentNo: d.DocumentNo, ProjectID: d.ProjectID, Classification: d.Classification, Content: raw, Amount: d.Amount, EmployeeReimbursementAmount: d.EmployeeReimbursementAmount, Suppliers: d.Suppliers, Attachments: d.Attachments, LineValues: values}
	return BusinessSubject{ID: d.ID, Kind: d.Kind, CreatorID: d.CreatorID, Title: d.Title, Category: d.Category, Classification: d.Classification, Lifecycle: d.Lifecycle, ApprovalState: d.ApprovalState, CurrentRevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash, Version: d.Version, ProjectID: d.ProjectID, Currency: d.Currency, Amount: d.Amount, RequestID: d.RequestID, DeletedAt: d.DeletedAt, Payload: p}, nil
}
func varianceSubject(v spend.Variance) (BusinessSubject, error) {
	d, err := spendSubject(v.Record)
	if err == nil {
		d.Payload.PurchaseVariance = &v.Basis
	}
	return d, err
}
func changeSubject(d project.Change) (BusinessSubject, error) {
	raw, e := json.Marshal(d.Revision.Content)
	if e != nil {
		return BusinessSubject{}, e
	}
	currency, amount, pid := d.Revision.Content.Currency, d.Revision.ApprovalAmount, d.ProjectID
	p := Snapshot{ProjectChangeBudgetDelta: &d.Revision.BudgetDelta, ProjectChangeBasis: &d.Revision.Basis, BusinessPayloadSchemaVersion: 1, Kind: "change", BusinessID: d.ID, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.Revision.ContentHash, ProjectID: &pid, Classification: d.Classification, Content: raw, Amount: &amount, Attachments: d.Revision.Attachments}
	return BusinessSubject{ID: d.ID, Kind: "change", CreatorID: d.CreatorID, Title: d.Revision.Content.Title, Category: d.Revision.Content.Category, Classification: d.Classification, Lifecycle: d.Lifecycle, ApprovalState: d.ApprovalState, CurrentRevisionID: d.CurrentRevisionID, ContentHash: d.Revision.ContentHash, Version: d.Version, ProjectID: &pid, Currency: &currency, Amount: &amount, RequestID: d.RequestID, Payload: p}, nil
}
func subject(ctx context.Context, db platform.DBTX, a core.Actor, kind, id string, lock, complete bool) (BusinessSubject, error) {
	switch kind {
	case "change":
		d, e := project.ChangeApprovalDraft(ctx, db, a, id, lock, complete)
		if e != nil {
			return BusinessSubject{}, e
		}
		return changeSubject(d)
	case "contract":
		d, err := contracts.ApprovalDraft(ctx, db, a, id, lock, complete)
		if err != nil {
			return BusinessSubject{}, err
		}
		return contractSubject(d)
	case "purchase_variance":
		v, err := spend.VarianceApprovalDraft(ctx, db, a, id, lock, complete)
		if err != nil {
			return BusinessSubject{}, err
		}
		return varianceSubject(v)
	case "purchase", "claim":
		d, err := spend.ApprovalDraft(ctx, db, a, kind, id, lock, complete)
		if err != nil {
			return BusinessSubject{}, err
		}
		return spendSubject(d)
	default:
		return BusinessSubject{}, core.Invalid("kind", "此业务的送审适配器尚未接入")
	}
}
func snapshot(d BusinessSubject) Snapshot { return d.Payload }
func businessRead(ctx context.Context, db platform.DBTX, a core.Actor, r Request) (BusinessSubject, error) {
	switch r.Kind {
	case "change":
		d, e := project.ChangeApprovalRead(ctx, db, a, r.BusinessID)
		if e != nil {
			return BusinessSubject{}, e
		}
		return changeSubject(d)
	case "contract":
		d, err := contracts.ApprovalRead(ctx, db, a, r.BusinessID)
		if err != nil {
			return BusinessSubject{}, err
		}
		return contractSubject(d)
	case "purchase_variance":
		v, err := spend.VarianceApprovalRead(ctx, db, a, r.BusinessID)
		if err != nil {
			return BusinessSubject{}, err
		}
		return varianceSubject(v)
	case "purchase", "claim":
		d, err := spend.ApprovalRead(ctx, db, a, r.Kind, r.BusinessID)
		if err != nil {
			return BusinessSubject{}, err
		}
		return spendSubject(d)
	default:
		return BusinessSubject{}, core.Missing()
	}
}
func businessResource(d BusinessSubject) authz.Resource {
	return authz.Resource{Type: d.Kind, ID: d.ID, ProjectID: d.ProjectID, OwnerID: &d.CreatorID, Classification: d.Classification}
}
func businessTable(kind string) (string, error) {
	switch kind {
	case "change":
		return "change_requests", nil
	case "contract":
		return "contracts", nil
	case "purchase", "claim":
		return "spend_documents", nil
	case "purchase_variance":
		return "purchase_variances", nil
	default:
		return "", core.Missing()
	}
}
func lockBusiness(ctx context.Context, db platform.DBTX, a core.Actor, kind, id string) error {
	if kind == "purchase_variance" {
		if _, err := db.Exec(ctx, `SELECT p.id FROM spend_documents p JOIN purchase_variances v ON v.org_id=p.org_id AND v.purchase_id=p.id WHERE v.org_id=$1 AND v.id=$2 FOR UPDATE OF p`, a.OrgID, id); err != nil {
			return err
		}
	}
	table, err := businessTable(kind)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, "SELECT id FROM "+table+" WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id)
	return err
}
func businessReviewing(d BusinessSubject) bool {
	lifecycle := "draft"
	if d.Kind != "contract" {
		lifecycle = "requested"
	}
	return d.DeletedAt == nil && d.Lifecycle == lifecycle && d.ApprovalState == "in_review"
}
func setBusinessState(ctx context.Context, db platform.DBTX, a core.Actor, d BusinessSubject, requestID, state string) error {
	table, err := businessTable(d.Kind)
	if err != nil {
		return err
	}
	lifecycle := "draft"
	var approved *string
	if state == "approved" {
		approved = &d.CurrentRevisionID
		if d.Kind == "contract" {
			lifecycle = "pending_signature"
		} else {
			lifecycle = "requested"
		}
	} else if d.Kind != "contract" && state == "in_review" {
		lifecycle = "requested"
	}
	_, err = db.Exec(ctx, "UPDATE "+table+" SET request_id=$3,approval_state=$4,lifecycle=$5,approved_revision_id=$6,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", a.OrgID, d.ID, requestID, state, lifecycle, approved)
	return err
}
func snapshotSuppliers(p Snapshot) ([]spend.Supplier, error) {
	if p.Kind == "change" {
		return []spend.Supplier{}, nil
	}
	if p.Kind == "contract" {
		var c contracts.Content
		if err := json.Unmarshal(p.Content, &c); err != nil {
			return nil, err
		}
		out := []spend.Supplier{}
		if c.CounterpartyID != nil && c.CounterpartyRevisionID != nil {
			out = append(out, spend.Supplier{PartyID: *c.CounterpartyID, RevisionID: *c.CounterpartyRevisionID})
		}
		return out, nil
	}
	var c spend.Content
	if err := json.Unmarshal(p.Content, &c); err != nil {
		return nil, err
	}
	refs := spend.Suppliers(c)
	if p.PurchaseVariance != nil {
		for _, ref := range spend.Suppliers(p.PurchaseVariance.Content) {
			found := false
			for _, current := range refs {
				if current == ref {
					found = true
					break
				}
			}
			if !found {
				refs = append(refs, ref)
			}
		}
	}
	return refs, nil
}
func redactSnapshot(ctx context.Context, db platform.DBTX, a core.Actor, p Snapshot) (Snapshot, error) {
	p.Counterparty = nil
	p.Suppliers = nil
	refs, err := snapshotSuppliers(p)
	if err != nil {
		return p, err
	}
	for _, ref := range refs {
		_, v, err := party.ResolveBusinessReference(ctx, db, a, ref.PartyID, ref.RevisionID, false)
		if err == nil {
			if p.Kind == "contract" {
				p.Counterparty = &contracts.PartyReference{ID: ref.PartyID, RevisionID: v.ID, Name: v.Name, Kind: v.Kind}
			} else {
				p.Suppliers = append(p.Suppliers, spend.SupplierEvidence{Supplier: ref, Name: v.Name, Kind: v.Kind})
			}
		} else if !hiddenBusinessLink(err) {
			return p, err
		}
	}
	visible := []fileref.Evidence{}
	for _, v := range p.Attachments {
		_, err := fileref.Resolve(ctx, db, a, v.VersionID, p.ProjectID, p.Classification)
		if err == nil {
			visible = append(visible, v)
		} else if !hiddenBusinessLink(err) {
			return p, err
		}
	}
	p.Attachments = visible
	return p, nil
}
func hiddenBusinessLink(err error) bool {
	status := core.PublicError(err).Status
	return status == 403 || status == 404 || status == 409 || status == 422
}
func revisionHash(p Snapshot, d *Definition, proofs []AuthorityEvidence) (string, error) {
	if proofs == nil {
		proofs = []AuthorityEvidence{}
	}
	raw, err := json.Marshal(map[string]any{"payload_schema_version": 1, "payload": p, "workflow": d, "reviewer_evidence": proofs})
	if err != nil {
		return "", err
	}
	canonical, err := core.CanonicalJSON(raw)
	if err != nil {
		return "", err
	}
	return core.Digest(canonical), nil
}
func requireRequest(ctx context.Context, db platform.DBTX, a core.Actor, r Request, action string) error {
	if err := authz.Require(ctx, db, a, "request.read", requestResource(r)); err != nil {
		return err
	}
	if _, err := businessRead(ctx, db, a, r); err != nil {
		return err
	}
	if action != "request.read" {
		return authz.Require(ctx, db, a, action, requestResource(r))
	}
	return nil
}
