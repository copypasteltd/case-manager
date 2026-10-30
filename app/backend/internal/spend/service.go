package spend

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const columns = `d.id,d.kind,d.document_no,d.project_id,d.creator_id,d.classification,d.lifecycle,d.approval_state,
    d.current_revision_id,d.approved_revision_id,d.request_id,d.version,d.deleted_at,d.created_at,d.updated_at,
    r.payload_schema_version,r.content_hash,r.payload,r.amount::text,r.employee_reimbursement_amount::text,
    COALESCE((SELECT jsonb_agg(jsonb_build_object('line_id',l.line_id,'amount',l.amount::text,'duplicate_receipt',false) ORDER BY l.position)
        FROM spend_lines l WHERE l.org_id=r.org_id AND l.revision_id=r.id),'[]'::jsonb)`
const currentJoin = ` FROM spend_documents d JOIN spend_revisions r ON r.org_id=d.org_id AND r.document_id=d.id AND r.id=d.current_revision_id `

func scan(row pgx.Row) (Record, error) {
	var d Record
	var payload, values []byte
	err := row.Scan(&d.ID, &d.Kind, &d.DocumentNo, &d.ProjectID, &d.CreatorID, &d.Classification, &d.Lifecycle, &d.ApprovalState, &d.CurrentRevisionID, &d.ApprovedRevisionID, &d.RequestID, &d.Version, &d.DeletedAt, &d.CreatedAt, &d.UpdatedAt, &d.PayloadSchemaVersion, &d.ContentHash, &payload, &d.Amount, &d.EmployeeReimbursementAmount, &values)
	if err != nil {
		return d, platform.DatabaseError(err)
	}
	if err = json.Unmarshal(payload, &d.Content); err != nil {
		return d, err
	}
	err = json.Unmarshal(values, &d.LineValues)
	return d, err
}
func metadata(ctx context.Context, db platform.DBTX, org, kind, id string) (Record, error) {
	if !ValidKind(kind) || !core.ValidID(id) {
		return Record{}, core.Missing()
	}
	return scan(db.QueryRow(ctx, "SELECT "+columns+currentJoin+"WHERE d.org_id=$1 AND d.kind=$2 AND d.id=$3", org, kind, id))
}
func resource(d Record) authz.Resource {
	return authz.Resource{Type: d.Kind, ID: d.ID, ProjectID: d.ProjectID, OwnerID: &d.CreatorID, Classification: d.Classification}
}
func projectWritable(ctx context.Context, db platform.DBTX, a core.Actor, d Record, lock bool) error {
	if d.ProjectID == nil {
		return nil
	}
	query := "SELECT state,classification_rank(classification)<=classification_rank($3) FROM projects WHERE org_id=$1 AND id=$2"
	if lock {
		query += " FOR UPDATE"
	}
	var state string
	var adequate bool
	if err := db.QueryRow(ctx, query, a.OrgID, *d.ProjectID, d.Classification).Scan(&state, &adequate); err != nil {
		return platform.DatabaseError(err)
	}
	if !adequate {
		return core.Invalid("classification", "申请分类不能低于所属项目")
	}
	if slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
		return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许修改申请")
	}
	return nil
}
func editable(d Record) bool {
	return d.DeletedAt == nil && d.Lifecycle == "draft" && slices.Contains([]string{"not_submitted", "returned", "withdrawn"}, d.ApprovalState)
}
func present(ctx context.Context, db platform.DBTX, a core.Actor, d Record) (Record, error) {
	if err := authz.Require(ctx, db, a, d.Kind+".read", resource(d)); err != nil {
		return Record{}, err
	}
	var err error
	d.Suppliers, d.Attachments, err = References(ctx, db, a, d, d.Content, false, false)
	if err != nil {
		return Record{}, err
	}
	writable := projectWritable(ctx, db, a, d, false) == nil
	canUpdate := authz.Require(ctx, db, a, d.Kind+".update", resource(d)) == nil
	d.CanEdit = writable && editable(d) && canUpdate
	d.CanArchive = writable && d.DeletedAt == nil && d.RequestID == nil && d.Lifecycle == "draft" && d.ApprovalState == "not_submitted" && authz.Require(ctx, db, a, d.Kind+".archive", resource(d)) == nil
	scope := authz.Resource{Type: "request", ProjectID: d.ProjectID, OwnerID: &a.UserID, Classification: d.Classification}
	d.CanPrepareRequest = writable && d.DeletedAt == nil && d.Lifecycle == "draft" && (d.RequestID == nil || d.ApprovalState == "rejected") && canUpdate && authz.Require(ctx, db, a, "request.create", scope) == nil && authz.Require(ctx, db, a, "request.read", scope) == nil
	d.CanReadHistory = canUpdate
	d.CanExecute = d.Kind == "purchase" && writable && d.DeletedAt == nil && d.ApprovalState == "approved" && a.RunID == "" && authz.Require(ctx, db, a, "purchase.execute", resource(d)) == nil
	if d.Kind == "purchase" && writable && d.DeletedAt == nil && d.ApprovalState == "approved" && !slices.Contains([]string{"cancelled", "completed"}, d.Lifecycle) {
		proposalScope := d
		proposalScope.ID = ""
		proposalScope.Kind = "purchase_variance"
		proposalScope.CreatorID = a.UserID
		d.CanCreateVariance = authz.Require(ctx, db, a, "purchase_variance.create", resource(proposalScope)) == nil && authz.Require(ctx, db, a, "purchase_variance.read", resource(proposalScope)) == nil
	}
	if d.Kind == "claim" {
		if err = duplicateWarnings(ctx, db, a, d, &d.LineValues); err != nil {
			return Record{}, err
		}
	}
	return d, nil
}
func (s *Service) Get(ctx context.Context, a core.Actor, kind, id string) (Record, error) {
	d, err := metadata(ctx, s.DB, a.OrgID, kind, id)
	if err != nil {
		return d, err
	}
	return present(ctx, s.DB, a, d)
}
func binding(a core.Actor, scope string) string {
	return core.Digest([]byte(fmt.Sprint(a.OrgID, "/", a.UserID, "/", a.AuthVersion, "/", a.OrgAuthVersion, "/", a.Channel, "/", a.RunID, "/spend/", scope)))
}
func (s *Service) List(ctx context.Context, a core.Actor, kind string, f Filter) (core.Page[Record], error) {
	out := core.Page[Record]{Items: []Record{}}
	stages := []string{"", "draft", "requested"}
	if kind == "purchase" {
		stages = append(stages, "ordering", "receiving", "cancelled", "completed")
	}
	if !ValidKind(kind) || len([]rune(f.Query)) > 200 || !slices.Contains(stages, f.Lifecycle) || !slices.Contains([]string{"", "not_submitted", "in_review", "approved", "returned", "rejected", "withdrawn"}, f.ApprovalState) || (f.ProjectID != nil && !core.ValidID(*f.ProjectID)) || (f.CompanyOnly && f.ProjectID != nil) {
		return out, core.Invalid("filter", "申请筛选无效")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	project := ""
	if f.ProjectID != nil {
		project = *f.ProjectID
	}
	bound := binding(a, fmt.Sprint(kind, "/", f.Query, "/", f.Lifecycle, "/", f.ApprovalState, "/", project))
	if f.CompanyOnly {
		bound += "/company-only"
	}
	var at *time.Time
	var id *string
	if f.Cursor != "" {
		c, err := s.Pager.Decode(f.Cursor, bound)
		if err != nil {
			return out, err
		}
		at = &c.UpdatedAt
		id = &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+columns+currentJoin+`WHERE d.org_id=$1 AND d.kind=$2 AND d.deleted_at IS NULL AND ($3='' OR d.lifecycle=$3)
        AND ($4='' OR d.approval_state=$4) AND ($5='' OR position(lower($5) IN lower(r.title))>0 OR position(lower($5) IN lower(d.document_no))>0)
        AND ($6::uuid IS NULL OR d.project_id=$6) AND (NOT $13 OR d.project_id IS NULL) AND can_access(d.org_id,$7,d.kind||'.read',d.kind,d.id,d.project_id,d.creator_id,d.classification,$8,$9)
        AND ($10::timestamptz IS NULL OR (d.updated_at,d.id)<($10,$11::uuid)) ORDER BY d.updated_at DESC,d.id DESC LIMIT $12`, a.OrgID, kind, f.Lifecycle, f.ApprovalState, f.Query, f.ProjectID, a.UserID, a.Channel, a.RunID != "", at, id, f.Limit+1, f.CompanyOnly)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		d, e := scan(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		d := out.Items[f.Limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: d.ID, UpdatedAt: d.UpdatedAt, Binding: bound})
		out.NextCursor = &cursor
	}
	for i, d := range out.Items {
		out.Items[i], err = present(ctx, s.DB, a, d)
		if err != nil {
			return core.Page[Record]{Items: []Record{}}, err
		}
	}
	return out, nil
}

// A warning discloses neither hidden claim identifiers nor counts. The other
// claim and matching receipt must both be readable under today's permissions.
func duplicateWarnings(ctx context.Context, db platform.DBTX, a core.Actor, d Record, values *[]LineValue) error {
	for i, l := range d.Lines {
		if i >= len(*values) {
			break
		}
		(*values)[i].DuplicateReceipt = false
		visibleIDs := []string{}
		for _, id := range l.ReceiptVersionIDs {
			for _, file := range d.Attachments {
				if file.VersionID == id {
					visibleIDs = append(visibleIDs, id)
					break
				}
			}
		}
		var supplierID *string
		if l.Supplier != nil {
			for _, ref := range d.Suppliers {
				if ref.Supplier == *l.Supplier {
					supplierID = &l.Supplier.PartyID
					break
				}
			}
		}
		if len(visibleIDs) == 0 && (supplierID == nil || l.ExpenseDate == nil || (*values)[i].Amount == nil) {
			continue
		}
		err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM spend_documents other JOIN spend_lines ol ON ol.org_id=other.org_id AND ol.revision_id=other.current_revision_id
            JOIN spend_revisions revision ON revision.org_id=other.org_id AND revision.id=other.current_revision_id
            LEFT JOIN LATERAL jsonb_array_elements_text(ol.payload->'receipt_version_ids') ref ON true
            LEFT JOIN file_versions ov ON ov.org_id=other.org_id AND ov.id=ref.value::uuid
            LEFT JOIN files f ON f.org_id=ov.org_id AND f.id=ov.file_id LEFT JOIN folders folder ON folder.org_id=f.org_id AND folder.id=f.folder_id
            WHERE other.org_id=$1 AND other.kind='claim' AND other.id<>$2 AND other.deleted_at IS NULL
            AND can_access(other.org_id,$3,'claim.read','claim',other.id,other.project_id,other.creator_id,other.classification,$4,$5)
            AND ((f.state='active' AND folder.state='active' AND can_access(f.org_id,$3,'file.read','file',f.id,folder.project_id,f.creator_id,f.classification,$4,$5)
                AND ov.sha256 IN (SELECT sha256 FROM file_versions WHERE org_id=$1 AND id=ANY($6::uuid[])))
                OR ($7::uuid IS NOT NULL AND ol.payload->'supplier'->>'party_id'=$7::text AND ol.payload->>'expense_date'=$8::text
                    AND ol.amount=$9::numeric AND revision.currency=$10::text)))`, a.OrgID, d.ID, a.UserID, a.Channel, a.RunID != "", visibleIDs, supplierID, l.ExpenseDate, (*values)[i].Amount, d.Currency).Scan(&(*values)[i].DuplicateReceipt)
		if err != nil {
			return err
		}
	}
	return nil
}

// Approval adapters never grant business editing to an assigned reviewer.
func ApprovalRead(ctx context.Context, db platform.DBTX, a core.Actor, kind, id string) (Record, error) {
	d, err := metadata(ctx, db, a.OrgID, kind, id)
	if err != nil {
		return d, err
	}
	return present(ctx, db, a, d)
}
func ApprovalDraft(ctx context.Context, db platform.DBTX, a core.Actor, kind, id string, lock, complete bool) (Record, error) {
	d, err := metadata(ctx, db, a.OrgID, kind, id)
	if err != nil {
		return d, err
	}
	for _, action := range []string{kind + ".read", kind + ".update"} {
		if err = authz.Require(ctx, db, a, action, resource(d)); err != nil {
			return Record{}, err
		}
	}
	if err = projectWritable(ctx, db, a, d, lock); err != nil {
		return Record{}, err
	}
	if lock {
		if _, err = db.Exec(ctx, "SELECT id FROM spend_documents WHERE org_id=$1 AND kind=$2 AND id=$3 FOR UPDATE", a.OrgID, kind, id); err != nil {
			return Record{}, err
		}
		d, err = metadata(ctx, db, a.OrgID, kind, id)
		if err != nil {
			return d, err
		}
	}
	if d.DeletedAt != nil || d.Lifecycle != "draft" || !slices.Contains([]string{"not_submitted", "returned", "withdrawn", "rejected"}, d.ApprovalState) {
		return Record{}, core.Fail(409, "SPEND_READ_ONLY", "当前业务阶段不能准备送审内容")
	}
	if complete {
		if _, _, _, _, err = normalize(ctx, db, a, d, d.Content, &d.Content, true); err != nil {
			return Record{}, err
		}
	}
	d, err = present(ctx, db, a, d)
	if err != nil {
		return d, err
	}
	if complete && kind == "claim" {
		for i, v := range d.LineValues {
			if v.DuplicateReceipt && stringsBlank(d.Lines[i].DuplicateReceiptReason) {
				return Record{}, core.Invalid("duplicate_receipt_reason", "凭证或费用资料与可查阅的其他报销可能重复，请核实并填写说明")
			}
		}
	}
	return d, nil
}
