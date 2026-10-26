package spend

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/platform"
)

const varianceColumns = `d.id,d.kind,d.document_no,d.project_id,d.creator_id,d.classification,d.lifecycle,d.approval_state,
    d.current_revision_id,d.approved_revision_id,d.request_id,d.version,d.deleted_at,d.created_at,d.updated_at,
    r.payload_schema_version,r.content_hash,r.payload,r.amount::text,NULL::text,r.line_values`
const varianceJoin = ` FROM purchase_variances d JOIN purchase_variance_revisions r ON r.org_id=d.org_id AND r.document_id=d.id AND r.id=d.current_revision_id `

func varianceMetadata(ctx context.Context, db platform.DBTX, org, id string) (Variance, error) {
	var out Variance
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	d, err := scan(db.QueryRow(ctx, "SELECT "+varianceColumns+varianceJoin+"WHERE d.org_id=$1 AND d.id=$2", org, id))
	if err != nil {
		return out, err
	}
	var raw []byte
	if err = db.QueryRow(ctx, "SELECT basis FROM purchase_variance_revisions WHERE org_id=$1 AND id=$2", org, d.CurrentRevisionID).Scan(&raw); err != nil {
		return out, err
	}
	out.Record = d
	err = json.Unmarshal(raw, &out.Basis)
	return out, err
}

func varianceEvidence(ctx context.Context, db platform.DBTX, a core.Actor, v Variance, strict bool) ([]SupplierEvidence, []fileref.Evidence, error) {
	suppliers, files, err := References(ctx, db, a, v.Record, v.Content, strict, strict)
	if err != nil {
		return nil, nil, err
	}
	beforeSuppliers, beforeFiles, err := References(ctx, db, a, v.Record, v.Basis.Content, strict, strict)
	if err != nil {
		return nil, nil, err
	}
	for _, other := range beforeSuppliers {
		found := false
		for _, current := range suppliers {
			if current.Supplier == other.Supplier {
				found = true
				break
			}
		}
		if !found {
			suppliers = append(suppliers, other)
		}
	}
	for _, other := range beforeFiles {
		found := false
		for _, current := range files {
			if current.VersionID == other.VersionID {
				found = true
				break
			}
		}
		if !found {
			files = append(files, other)
		}
	}
	return suppliers, files, nil
}

func presentVariance(ctx context.Context, db platform.DBTX, a core.Actor, v Variance) (Variance, error) {
	p, err := purchaseRead(ctx, db, a, v.Basis.PurchaseID)
	if err != nil {
		return Variance{}, err
	}
	v.Record, err = present(ctx, db, a, v.Record)
	if err != nil {
		return Variance{}, err
	}
	v.CanArchive = false
	v.CanExecute = false
	if p.Lifecycle == "completed" || p.Lifecycle == "cancelled" {
		v.CanEdit = false
		v.CanPrepareRequest = false
	}
	v.Suppliers, v.Attachments, err = varianceEvidence(ctx, db, a, v, false)
	if err != nil {
		return Variance{}, err
	}
	v.CanActivate = v.ApprovalState == "approved" && v.Lifecycle == "requested" && p.Lifecycle != "cancelled" && p.Lifecycle != "completed" && p.Version == v.Basis.PurchaseVersion && a.RunID == "" && projectWritable(ctx, db, a, p, false) == nil && authz.Require(ctx, db, a, "purchase_variance.activate", resource(v.Record)) == nil
	return v, nil
}

func VarianceApprovalRead(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Variance, error) {
	v, err := varianceMetadata(ctx, db, a.OrgID, id)
	if err != nil {
		return v, err
	}
	return presentVariance(ctx, db, a, v)
}
func (s *Service) Variance(ctx context.Context, a core.Actor, id string) (Variance, error) {
	return VarianceApprovalRead(ctx, s.DB, a, id)
}

func effectivePlan(ctx context.Context, db platform.DBTX, org string, d Record) (ExecutionPlan, error) {
	out := ExecutionPlan{PurchaseID: d.ID, PurchaseVersion: d.Version, LineValues: []LineValue{}}
	var content, values []byte
	err := db.QueryRow(ctx, `SELECT d.version,p.payload,p.amount::text,p.line_values,p.variance_id,p.revision_id,p.request_id,p.activated_at FROM spend_documents d CROSS JOIN LATERAL purchase_effective_plan(d.org_id,d.id) p WHERE d.org_id=$1 AND d.id=$2`, org, d.ID).Scan(&out.PurchaseVersion, &content, &out.Amount, &values, &out.VarianceID, &out.VarianceRevisionID, &out.VarianceRequestID, &out.ActivatedAt)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(content, &out.Content); err != nil {
		return out, err
	}
	err = json.Unmarshal(values, &out.LineValues)
	return out, err
}
func (s *Service) ExecutionPlan(ctx context.Context, a core.Actor, id string) (ExecutionPlan, error) {
	d, err := purchaseRead(ctx, s.DB, a, id)
	if err != nil {
		return ExecutionPlan{}, err
	}
	if d.ApprovalState != "approved" {
		return ExecutionPlan{}, core.Fail(409, "PURCHASE_NOT_APPROVED", "采购获批后才有执行基线")
	}
	plan, err := effectivePlan(ctx, s.DB, a.OrgID, d)
	if err != nil {
		return ExecutionPlan{}, err
	}
	plan.Suppliers, plan.Attachments, err = References(ctx, s.DB, a, d, plan.Content, false, false)
	return plan, err
}
func currentVarianceBasis(ctx context.Context, db platform.DBTX, org, id string) (VarianceBasis, error) {
	var out VarianceBasis
	var raw []byte
	if err := db.QueryRow(ctx, "SELECT purchase_variance_basis($1,$2)", org, id).Scan(&raw); err != nil {
		return out, platform.DatabaseError(err)
	}
	err := json.Unmarshal(raw, &out)
	return out, err
}

func (s *Service) Variances(ctx context.Context, a core.Actor, purchase, cursor string, limit int) (core.Page[Variance], error) {
	out := core.Page[Variance]{Items: []Variance{}}
	if _, err := purchaseRead(ctx, s.DB, a, purchase); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := binding(a, "variances/"+purchase)
	var at *time.Time
	var before *string
	if cursor != "" {
		c, err := s.Pager.Decode(cursor, bound)
		if err != nil {
			return out, err
		}
		at = &c.UpdatedAt
		before = &c.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT d.id,d.updated_at FROM purchase_variances d WHERE d.org_id=$1 AND d.purchase_id=$2
        AND can_access(d.org_id,$3,'purchase_variance.read',d.kind,d.id,d.project_id,d.creator_id,d.classification,$4,$5)
        AND ($6::timestamptz IS NULL OR (d.updated_at,d.id)<($6,$7::uuid)) ORDER BY d.updated_at DESC,d.id DESC LIMIT $8`, a.OrgID, purchase, a.UserID, a.Channel, a.RunID != "", at, before, limit+1)
	if err != nil {
		return out, err
	}
	type key struct {
		id string
		at time.Time
	}
	keys := []key{}
	for rows.Next() {
		var k key
		if err = rows.Scan(&k.id, &k.at); err != nil {
			rows.Close()
			return out, err
		}
		keys = append(keys, k)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(keys) > limit {
		keys = keys[:limit]
		last := keys[limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: last.id, UpdatedAt: last.at, Binding: bound})
		out.NextCursor = &v
	}
	for _, k := range keys {
		v, err := s.Variance(ctx, a, k.id)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}

func (s *Service) VarianceRevision(ctx context.Context, a core.Actor, id, revision string) (VarianceRevision, error) {
	v, err := s.Variance(ctx, a, id)
	if err != nil {
		return VarianceRevision{}, err
	}
	if !v.CanReadHistory {
		return VarianceRevision{}, core.Missing()
	}
	out := VarianceRevision{}
	var payload, basis, values []byte
	err = s.DB.QueryRow(ctx, `SELECT id,document_id,seq,base_revision_id,payload_schema_version,payload,basis,content_hash,amount::text,line_values,author_id,reason,created_at
        FROM purchase_variance_revisions WHERE org_id=$1 AND document_id=$2 AND id=$3`, a.OrgID, id, revision).Scan(&out.ID, &out.DocumentID, &out.Seq, &out.BaseRevisionID, &out.PayloadSchemaVersion, &payload, &basis, &out.ContentHash, &out.Amount, &values, &out.AuthorID, &out.Reason, &out.CreatedAt)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(payload, &out.Content); err != nil {
		return out, err
	}
	if err = json.Unmarshal(basis, &out.Basis); err != nil {
		return out, err
	}
	if err = json.Unmarshal(values, &out.LineValues); err != nil {
		return out, err
	}
	v.Content = out.Content
	v.Basis = out.Basis
	out.Suppliers, out.Attachments, err = varianceEvidence(ctx, s.DB, a, v, false)
	return out, err
}
func (s *Service) VarianceHistory(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[VarianceRevision], error) {
	out := core.Page[VarianceRevision]{Items: []VarianceRevision{}}
	v, err := s.Variance(ctx, a, id)
	if err != nil {
		return out, err
	}
	if !v.CanReadHistory {
		return out, core.Missing()
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := binding(a, fmt.Sprint("variance-history/", id))
	var at *time.Time
	var before *string
	if cursor != "" {
		c, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &c.UpdatedAt
		before = &c.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT id,created_at FROM purchase_variance_revisions WHERE org_id=$1 AND document_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, at, before, limit+1)
	if err != nil {
		return out, err
	}
	type key struct {
		id string
		at time.Time
	}
	keys := []key{}
	for rows.Next() {
		var k key
		if err = rows.Scan(&k.id, &k.at); err != nil {
			rows.Close()
			return out, err
		}
		keys = append(keys, k)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(keys) > limit {
		keys = keys[:limit]
		last := keys[limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: last.id, UpdatedAt: last.at, Binding: bound})
		out.NextCursor = &v
	}
	for _, k := range keys {
		v, e := s.VarianceRevision(ctx, a, id, k.id)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}
