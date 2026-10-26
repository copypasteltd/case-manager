package contracts

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const contentColumns = `r.title,r.direction,r.type,r.category,r.counterparty_id,r.counterparty_revision_id,r.owner_id,
    r.currency,r.amount::text,r.tax_amount::text,r.start_date::text,r.end_date::text,r.renewal_terms,r.termination_terms,r.notes,r.draft_file_version_id,r.payment_schedule`
const columns = `d.id,d.version,d.contract_no,d.project_id,d.creator_id,d.parent_contract_id,d.classification,d.lifecycle,d.approval_state,
    d.current_revision_id,d.approved_revision_id,d.active_revision_id,d.request_id,d.deleted_at,d.created_at,d.updated_at,r.content_hash,r.payload_schema_version,` + contentColumns
const currentJoin = ` FROM contracts d JOIN contract_revisions r ON r.org_id=d.org_id AND r.contract_id=d.id AND r.id=d.current_revision_id `

func contentTargets(c *Content) []any {
	return []any{&c.Title, &c.Direction, &c.Type, &c.Category, &c.CounterpartyID, &c.CounterpartyRevisionID, &c.OwnerID, &c.Currency, &c.Amount, &c.TaxAmount, &c.StartDate, &c.EndDate, &c.RenewalTerms, &c.TerminationTerms, &c.Notes, &c.DraftFileVersionID, &c.PaymentSchedule}
}
func scan(row pgx.Row) (Contract, error) {
	var d Contract
	targets := []any{&d.ID, &d.Version, &d.ContractNo, &d.ProjectID, &d.CreatorID, &d.ParentContractID, &d.Classification, &d.Lifecycle, &d.ApprovalState, &d.CurrentRevisionID, &d.ApprovedRevisionID, &d.ActiveRevisionID, &d.RequestID, &d.DeletedAt, &d.CreatedAt, &d.UpdatedAt, &d.ContentHash, &d.PayloadSchemaVersion}
	err := row.Scan(append(targets, contentTargets(&d.Content)...)...)
	return d, platform.DatabaseError(err)
}
func metadata(ctx context.Context, db platform.DBTX, org, id string) (Contract, error) {
	if !core.ValidID(id) {
		return Contract{}, core.Missing()
	}
	return scan(db.QueryRow(ctx, "SELECT "+columns+currentJoin+"WHERE d.org_id=$1 AND d.id=$2", org, id))
}
func resource(d Contract) authz.Resource {
	return authz.Resource{Type: "contract", ID: d.ID, ProjectID: d.ProjectID, OwnerID: &d.CreatorID, Classification: d.Classification}
}
func projectWritable(ctx context.Context, db platform.DBTX, org string, project *string, class string, lock bool) error {
	if project == nil {
		return nil
	}
	query := "SELECT state,classification_rank(classification)<=classification_rank($3) FROM projects WHERE org_id=$1 AND id=$2"
	if lock {
		query += " FOR UPDATE"
	}
	var state string
	var adequate bool
	if err := db.QueryRow(ctx, query, org, *project, class).Scan(&state, &adequate); err != nil {
		return platform.DatabaseError(err)
	}
	if !adequate {
		return core.Invalid("classification", "合同分类不能低于所属项目")
	}
	if slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
		return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许修改合同")
	}
	return nil
}
func editable(d Contract) bool {
	return d.DeletedAt == nil && d.Lifecycle == "draft" && slices.Contains([]string{"not_submitted", "returned", "withdrawn"}, d.ApprovalState)
}
func present(ctx context.Context, db platform.DBTX, a core.Actor, d Contract) (Contract, error) {
	if err := authz.Require(ctx, db, a, "contract.read", resource(d)); err != nil {
		return Contract{}, err
	}
	if err := references(ctx, db, a, d, d.Content, &d.Counterparty, &d.DraftFile); err != nil {
		return Contract{}, err
	}
	signature, err := signatureMetadata(ctx, db, a.OrgID, d)
	if err != nil {
		return Contract{}, err
	}
	d.Signature = signature
	if err = signatureEvidence(ctx, db, a, d, d.Signature); err != nil {
		return Contract{}, err
	}
	writable := projectWritable(ctx, db, a.OrgID, d.ProjectID, d.Classification, false) == nil
	d.CanEdit = writable && editable(d) && authz.Require(ctx, db, a, "contract.update", resource(d)) == nil
	prepareScope := authz.Resource{Type: "request", ProjectID: d.ProjectID, OwnerID: &a.UserID, Classification: d.Classification}
	d.CanPrepareRequest = writable && d.DeletedAt == nil && d.Lifecycle == "draft" && (d.RequestID == nil || d.ApprovalState == "rejected") && authz.Require(ctx, db, a, "contract.update", resource(d)) == nil &&
		authz.Require(ctx, db, a, "request.create", prepareScope) == nil && authz.Require(ctx, db, a, "request.read", prepareScope) == nil
	canExecute := writable && a.RunID == "" && slices.Contains([]string{"web", "android", "wechat", "douyin"}, a.Channel) && authz.Require(ctx, db, a, "contract.execute", resource(d)) == nil
	approved := d.DeletedAt == nil && d.Lifecycle == "pending_signature" && d.ApprovalState == "approved" && d.ApprovedRevisionID != nil && *d.ApprovedRevisionID == d.CurrentRevisionID
	d.CanRegisterSignature = canExecute && approved && d.Signature == nil
	if canExecute && approved && d.Signature != nil {
		today, err := todayHK(ctx, db)
		if err != nil {
			return Contract{}, err
		}
		d.CanActivate = d.Signature.EffectiveOn <= today
		if d.CanActivate && d.ParentContractID != nil {
			parent, e := metadata(ctx, db, a.OrgID, *d.ParentContractID)
			if e != nil && !hiddenReference(e) {
				return Contract{}, e
			}
			d.CanActivate = e == nil && parent.Lifecycle == "effective" && parent.DeletedAt == nil && authz.Require(ctx, db, a, "contract.read", resource(parent)) == nil
		}
	}
	d.CanReadHistory = authz.Require(ctx, db, a, "contract.update", resource(d)) == nil
	d.CanTrash = writable && d.Lifecycle == "draft" && d.ApprovedRevisionID == nil && d.ActiveRevisionID == nil && d.RequestID == nil && d.ApprovalState == "not_submitted" && authz.Require(ctx, db, a, "contract.archive", resource(d)) == nil
	if d.CanTrash && d.DeletedAt == nil {
		var children bool
		if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM contracts WHERE org_id=$1 AND parent_contract_id=$2 AND deleted_at IS NULL)", a.OrgID, d.ID).Scan(&children); err != nil {
			return Contract{}, err
		}
		d.CanTrash = !children
	}
	return d, nil
}
func (s *Service) Get(ctx context.Context, a core.Actor, id string) (Contract, error) {
	d, err := metadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return Contract{}, err
	}
	return present(ctx, s.DB, a, d)
}
func binding(a core.Actor, scope string) string {
	return core.Digest([]byte(fmt.Sprint(a.OrgID, "/", a.UserID, "/", a.AuthVersion, "/", a.OrgAuthVersion, "/", a.Channel, "/", a.RunID, "/contracts/", scope)))
}
func (s *Service) List(ctx context.Context, a core.Actor, f Filter) (core.Page[Contract], error) {
	out := core.Page[Contract]{Items: []Contract{}}
	if len([]rune(f.Query)) > 200 || !slices.Contains([]string{"", "draft", "pending_signature", "effective", "completed", "terminated"}, f.Lifecycle) || (f.ProjectID != nil && !core.ValidID(*f.ProjectID)) {
		return out, core.Invalid("filter", "合同筛选无效")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	scope := ""
	if f.ProjectID != nil {
		scope = *f.ProjectID
	}
	bound := binding(a, fmt.Sprint(f.Query, "/", f.Lifecycle, "/", scope, "/", f.Trashed))
	var at *time.Time
	var before *string
	if f.Cursor != "" {
		c, err := s.Pager.Decode(f.Cursor, bound)
		if err != nil {
			return out, err
		}
		at = &c.UpdatedAt
		before = &c.ID
	}
	rows, err := s.DB.Query(ctx, "SELECT "+columns+currentJoin+`WHERE d.org_id=$1 AND (d.deleted_at IS NOT NULL)=$2 AND ($3='' OR d.lifecycle=$3)
        AND ($4='' OR position(lower($4) in lower(r.title))>0 OR position(lower($4) in lower(d.contract_no))>0)
        AND ($5::uuid IS NULL OR d.project_id=$5) AND can_access(d.org_id,$6,'contract.read','contract',d.id,d.project_id,d.creator_id,d.classification,$7,$8)
        AND ($9::timestamptz IS NULL OR (d.updated_at,d.id)<($9,$10::uuid)) ORDER BY d.updated_at DESC,d.id DESC LIMIT $11`, a.OrgID, f.Trashed, f.Lifecycle, f.Query, f.ProjectID, a.UserID, a.Channel, a.RunID != "", at, before, f.Limit+1)
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
	for i, d := range out.Items {
		out.Items[i], err = present(ctx, s.DB, a, d)
		if err != nil {
			return core.Page[Contract]{Items: []Contract{}}, err
		}
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		d := out.Items[f.Limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: d.ID, UpdatedAt: d.UpdatedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, nil
}
func normalize(ctx context.Context, db platform.DBTX, a core.Actor, d Contract, in Content, previous *Content) (Content, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Category = strings.TrimSpace(in.Category)
	if core.Name(in.Title) != nil || len([]rune(in.Category)) > 50 || len([]rune(in.RenewalTerms)) > 20000 || len([]rune(in.TerminationTerms)) > 20000 || len([]rune(in.Notes)) > 20000 {
		return in, core.Invalid("content", "请核对合同标题、路由类别和条款长度")
	}
	if in.Direction != nil && !slices.Contains([]string{"receivable", "payable", "none"}, *in.Direction) {
		return in, core.Invalid("direction", "合同收付方向无效")
	}
	if in.Type != nil {
		value := strings.TrimSpace(*in.Type)
		if core.Name(value) != nil {
			return in, core.Invalid("type", "合同类型无效")
		}
		in.Type = &value
	}
	for _, field := range []**string{&in.CounterpartyID, &in.CounterpartyRevisionID, &in.OwnerID, &in.DraftFileVersionID} {
		if *field != nil {
			value := strings.ToLower(**field)
			*field = &value
		}
	}
	for _, id := range []*string{in.CounterpartyID, in.CounterpartyRevisionID, in.OwnerID, in.DraftFileVersionID} {
		if id != nil && !core.ValidID(*id) {
			return in, core.Invalid("reference", "请使用准确的对象与资料修订")
		}
	}
	if (in.CounterpartyID == nil) != (in.CounterpartyRevisionID == nil) {
		return in, core.Invalid("counterparty_revision_id", "相对方与固定资料修订须同时指定或同时清空")
	}
	if in.CounterpartyID != nil {
		requireActive := true
		if previous != nil && previous.CounterpartyID != nil && previous.CounterpartyRevisionID != nil && *previous.CounterpartyID == *in.CounterpartyID && *previous.CounterpartyRevisionID == *in.CounterpartyRevisionID {
			requireActive = false
		}
		p, _, err := party.ResolveBusinessReference(ctx, db, a, *in.CounterpartyID, *in.CounterpartyRevisionID, requireActive)
		if err != nil {
			return in, err
		}
		var adequate bool
		if err = db.QueryRow(ctx, "SELECT classification_rank($1)<=classification_rank($2)", p.Classification, d.Classification).Scan(&adequate); err != nil {
			return in, err
		}
		if !adequate || (p.ProjectID != nil && (d.ProjectID == nil || *p.ProjectID != *d.ProjectID)) {
			return in, core.Invalid("counterparty_id", "相对方资料范围须匹配，分类不得高于合同")
		}
	}
	if in.OwnerID != nil {
		var exists bool
		if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE org_id=$1 AND id=$2)", a.OrgID, *in.OwnerID).Scan(&exists); err != nil {
			return in, err
		}
		if !exists {
			return in, core.Invalid("owner_id", "合同负责人不属于当前组织")
		}
	}
	if in.Currency == nil && (in.Amount != nil || in.TaxAmount != nil) {
		return in, core.Invalid("currency", "填写金额前须明确币种")
	}
	if in.Currency != nil {
		var exponent int
		if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", *in.Currency).Scan(&exponent); err != nil {
			if err == pgx.ErrNoRows {
				return in, core.Invalid("currency", "该币种不在支持的精度目录中")
			}
			return in, err
		}
		for _, field := range []**string{&in.Amount, &in.TaxAmount} {
			if *field == nil {
				continue
			}
			v, err := core.CheckCurrencyAmount(*in.Currency, **field, map[string]int{*in.Currency: exponent})
			if err != nil {
				return in, err
			}
			text := v.String()
			*field = &text
		}
		if in.Amount != nil && in.TaxAmount != nil {
			amount, _ := core.ParseDecimal(*in.Amount)
			tax, _ := core.ParseDecimal(*in.TaxAmount)
			if tax.Cmp(amount) > 0 {
				return in, core.Invalid("tax_amount", "税额不能高于合同总额")
			}
		}
	}
	for _, date := range []*string{in.StartDate, in.EndDate} {
		if date != nil && (*date == "" || core.Date(*date) != nil) {
			return in, core.Invalid("date", "请填写有效日期或留空")
		}
	}
	if in.StartDate != nil && in.EndDate != nil && *in.StartDate > *in.EndDate {
		return in, core.Invalid("end_date", "合同结束日期不能早于开始日期")
	}
	if in.DraftFileVersionID != nil {
		if _, err := fileref.Resolve(ctx, db, a, *in.DraftFileVersionID, d.ProjectID, d.Classification); err != nil {
			return in, err
		}
	}
	plan, err := normalizeSchedule(ctx, db, in, false)
	in.PaymentSchedule = plan
	return in, err
}
func contentHash(d Contract, value Content) (string, error) {
	raw, err := json.Marshal(struct {
		Schema         int     `json:"schema"`
		ContractID     string  `json:"contract_id"`
		ProjectID      *string `json:"project_id"`
		Parent         *string `json:"parent_contract_id"`
		Classification string  `json:"classification"`
		Content        Content `json:"content"`
	}{2, d.ID, d.ProjectID, d.ParentContractID, d.Classification, value})
	if err != nil {
		return "", err
	}
	canonical, err := core.CanonicalJSON(raw)
	if err != nil {
		return "", err
	}
	return core.Digest(canonical), nil
}
func insertRevision(ctx context.Context, tx pgx.Tx, a core.Actor, d Contract, revision string, seq int64, base *string, in Content, reason string) error {
	hash, err := contentHash(d, in)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO contract_revisions(id,org_id,contract_id,seq,base_revision_id,payload_schema_version,content_hash,title,direction,type,category,
        counterparty_id,counterparty_revision_id,owner_id,currency,amount,tax_amount,start_date,end_date,renewal_terms,termination_terms,notes,draft_file_version_id,author_id,reason,payment_schedule)
        VALUES($1,$2,$3,$4,$5,2,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`, revision, a.OrgID, d.ID, seq, base, hash, in.Title, in.Direction, in.Type, in.Category, in.CounterpartyID, in.CounterpartyRevisionID, in.OwnerID, in.Currency, in.Amount, in.TaxAmount, in.StartDate, in.EndDate, in.RenewalTerms, in.TerminationTerms, in.Notes, in.DraftFileVersionID, a.UserID, reason, in.PaymentSchedule)
	return err
}
func result(d Contract, event string, status int, reason string) core.Result {
	return core.Result{Status: status, Data: d, Event: core.Event{Type: event, ResourceType: "contract", ResourceID: d.ID, Version: d.Version, Payload: map[string]any{"revision_id": d.CurrentRevisionID, "reason": reason}}}
}
func (s *Service) Create(ctx context.Context, c core.Command, in Create) (out core.Result, commandErr error) {
	defer func() { out, commandErr = s.replay(ctx, c.Actor, out, commandErr) }()
	for _, field := range []**string{&in.ProjectID, &in.ParentContractID} {
		if *field != nil {
			value := strings.ToLower(**field)
			*field = &value
		}
	}
	if core.Name(in.Title) != nil || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("title", "请填写草稿标题与创建依据")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if !core.Classification(in.Classification) || (in.ProjectID != nil && !core.ValidID(*in.ProjectID)) || (in.Classification == "project_restricted" && in.ProjectID == nil) {
			return core.Invalid("classification", "请明确有效项目与合同分类")
		}
		if in.Classification == "confidential" {
			return core.Fail(422, "EXPLICIT_GRANT_REQUIRED", "严格受限合同须使用独立分类与对象授权流程")
		}
		r := authz.Resource{Type: "contract", ProjectID: in.ProjectID, OwnerID: &c.Actor.UserID, Classification: in.Classification}
		if err := authz.Require(ctx, tx, c.Actor, "contract.create", r); err != nil {
			return err
		}
		return authz.Require(ctx, tx, c.Actor, "contract.read", r)
	}, func(tx pgx.Tx) (core.Result, error) {
		if err := projectWritable(ctx, tx, c.Actor.OrgID, in.ProjectID, in.Classification, true); err != nil {
			return core.Result{}, err
		}
		d := Contract{ID: core.NewID(), ProjectID: in.ProjectID, ParentContractID: in.ParentContractID, Classification: in.Classification}
		if err := validateParent(ctx, tx, c.Actor, d); err != nil {
			return core.Result{}, err
		}
		value, err := normalize(ctx, tx, c.Actor, d, in.Content, nil)
		if err != nil {
			return core.Result{}, err
		}
		revision := core.NewID()
		if err = tx.QueryRow(ctx, "SELECT 'CP-C-'||nextval('contract_number_seq')::text").Scan(&d.ContractNo); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO contracts(id,org_id,contract_no,project_id,creator_id,parent_contract_id,classification,current_revision_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", d.ID, c.Actor.OrgID, d.ContractNo, d.ProjectID, c.Actor.UserID, d.ParentContractID, d.Classification, revision); err != nil {
			return core.Result{}, err
		}
		if err = insertRevision(ctx, tx, c.Actor, d, revision, 1, nil, value, in.Reason); err != nil {
			return core.Result{}, err
		}
		d, err = metadata(ctx, tx, c.Actor.OrgID, d.ID)
		if err != nil {
			return core.Result{}, err
		}
		d, err = present(ctx, tx, c.Actor, d)
		return result(d, "contract.created", 201, in.Reason), err
	})
}

func validateParent(ctx context.Context, tx pgx.Tx, a core.Actor, d Contract) error {
	if d.ParentContractID == nil {
		return nil
	}
	if !core.ValidID(*d.ParentContractID) {
		return core.Invalid("parent_contract_id", "主合同标识无效")
	}
	if _, err := tx.Exec(ctx, "SELECT id FROM contracts WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, *d.ParentContractID); err != nil {
		return err
	}
	p, err := metadata(ctx, tx, a.OrgID, *d.ParentContractID)
	if err != nil {
		return err
	}
	if err = authz.Require(ctx, tx, a, "contract.read", resource(p)); err != nil {
		return err
	}
	if p.DeletedAt != nil || p.Classification != d.Classification || (p.ProjectID == nil) != (d.ProjectID == nil) || (p.ProjectID != nil && *p.ProjectID != *d.ProjectID) {
		return core.Invalid("parent_contract_id", "补充协议须关联同范围、同分类且未回收的主合同记录")
	}
	return nil
}
