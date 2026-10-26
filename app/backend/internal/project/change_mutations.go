package project

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
)

func normalizeChange(ctx context.Context, db platform.DBTX, in *ChangeCreate) error {
	c := &in.Content
	c.Title = strings.TrimSpace(c.Title)
	c.Category = strings.TrimSpace(c.Category)
	c.Scope = strings.TrimSpace(c.Scope)
	c.ScopeDelta = strings.TrimSpace(c.ScopeDelta)
	c.ScheduleDelta = strings.TrimSpace(c.ScheduleDelta)
	in.Reason = strings.TrimSpace(in.Reason)
	if core.Name(c.Title) != nil || core.Name(c.Category) != nil || core.Name(in.Reason) != nil || in.BaseProjectVersion < 1 || (in.BaseBudgetID != nil && !core.ValidID(*in.BaseBudgetID)) {
		return core.Invalid("change", "请填写完整标题、类别、准确基线和修订原因")
	}
	for _, v := range []string{c.Scope, c.ScopeDelta, c.ScheduleDelta} {
		if len([]rune(v)) < 1 || len([]rune(v)) > 20000 {
			return core.Invalid("content", "请填写完整拟实施范围、范围影响和时间影响说明")
		}
	}
	start, due := "", ""
	if c.StartDate != nil {
		start = *c.StartDate
		if start == "" {
			return core.Invalid("start_date", "未设置日期应使用null")
		}
	}
	if c.DueDate != nil {
		due = *c.DueDate
		if due == "" {
			return core.Invalid("due_date", "未设置日期应使用null")
		}
	}
	if e := core.DateRange(start, due); e != nil {
		return e
	}
	var exp int
	if e := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", c.Currency).Scan(&exp); e != nil {
		if e == pgx.ErrNoRows {
			return core.Invalid("currency", "请选择已配置币种")
		}
		return e
	}
	if len(c.BudgetLines) > 100 {
		return core.Invalid("budget_lines", "最多100项预算明细")
	}
	for i := range c.BudgetLines {
		l := &c.BudgetLines[i]
		l.Category = strings.TrimSpace(l.Category)
		if core.Name(l.Category) != nil || l.PeriodStart == "" || l.PeriodEnd == "" || core.DateRange(l.PeriodStart, l.PeriodEnd) != nil {
			return core.Invalid("budget_lines", fmt.Sprintf("第%d项预算类别或期间无效", i+1))
		}
		v, e := core.CheckCurrencyAmount(c.Currency, l.Amount, map[string]int{c.Currency: exp})
		if e != nil {
			return e
		}
		l.Amount = v.String()
		for j := 0; j < i; j++ {
			old := c.BudgetLines[j]
			if old.Category == l.Category && old.PeriodStart <= l.PeriodEnd && l.PeriodStart <= old.PeriodEnd {
				return core.Invalid("budget_lines", "同一类别的预算期间不能重叠")
			}
		}
	}
	if len(c.FileVersionIDs) > 20 {
		return core.Invalid("file_version_ids", "最多20个固定文件版本")
	}
	seen := map[string]bool{}
	for _, id := range c.FileVersionIDs {
		if !core.ValidID(id) || seen[id] {
			return core.Invalid("file_version_ids", "文件版本无效或重复")
		}
		seen[id] = true
	}
	if c.FileVersionIDs == nil {
		c.FileVersionIDs = []string{}
	}
	return nil
}
func exactChangeBase(in ChangeCreate, b ChangeBasis) error {
	if in.BaseProjectVersion != b.ProjectVersion || (in.BaseBudgetID == nil) != (b.BudgetID == nil) || (in.BaseBudgetID != nil && *in.BaseBudgetID != *b.BudgetID) {
		return core.Fail(409, "PROJECT_CHANGE_BASE_CHANGED", "项目或该币种预算已变化，请保留提案并重新核对基线")
	}
	return nil
}
func changeAmounts(c ChangeContent, b ChangeBasis) (string, error) {
	zero, _ := core.ParseDecimal("0")
	total, increase := zero, zero
	if c.BudgetLines == nil {
		return zero.String(), nil
	}
	for _, line := range c.BudgetLines {
		v, _ := core.ParseDecimal(line.Amount)
		total = total.Add(v)
		old := zero
		for _, x := range b.BudgetLines {
			if x.Category == line.Category && x.PeriodStart == line.PeriodStart && x.PeriodEnd == line.PeriodEnd {
				old, _ = core.ParseDecimal(x.Amount)
				break
			}
		}
		diff := v.Sub(old)
		if diff.Positive() {
			increase = increase.Add(diff)
		}
	}
	if _, e := core.ParseDecimal(total.String()); e != nil {
		return "", core.Invalid("budget_lines", "预算合计超出金额范围")
	}
	if _, e := core.ParseDecimal(increase.String()); e != nil {
		return "", core.Invalid("budget_lines", "审批增加合计超出金额范围")
	}
	old, e := core.ParseDecimal(b.BudgetTotal)
	if e != nil {
		return "", e
	}
	return total.Sub(old).String(), nil
}
func changeResult(d Change, event string, status int, reason string) core.Result {
	return core.Result{Status: status, Data: ChangeReceipt{ID: d.ID, Version: d.Version, RevisionID: d.CurrentRevisionID}, Event: core.Event{Type: event, ResourceType: "change", ResourceID: d.ID, Version: d.Version, Payload: map[string]any{"project_id": d.ProjectID, "revision_id": d.CurrentRevisionID, "state": d.State, "reason": reason}}}
}
func insertChangeRevision(ctx context.Context, tx pgx.Tx, a core.Actor, id, rid string, seq int64, base *string, in ChangeCreate, b ChangeBasis) error {
	payload, e := json.Marshal(in.Content)
	if e != nil {
		return e
	}
	basis, e := json.Marshal(b)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO project_change_revisions(id,org_id,change_id,seq,base_revision_id,payload,basis,content_hash,approval_amount,budget_delta,author_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,0,0,$9,$10)`, rid, a.OrgID, id, seq, base, payload, basis, strings.Repeat("0", 64), a.UserID, in.Reason)
	if e != nil {
		return e
	}
	for i, vid := range in.Content.FileVersionIDs {
		if _, e = tx.Exec(ctx, `INSERT INTO project_change_files(org_id,revision_id,version_id,ordinal) VALUES($1,$2,$3,$4)`, a.OrgID, rid, vid, i+1); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) CreateChange(ctx context.Context, c core.Command, pid string, in ChangeCreate) (core.Result, error) {
	authorize := func(tx pgx.Tx) error {
		p, e := getProject(ctx, tx, c.Actor.OrgID, pid, true)
		if e != nil {
			return e
		}
		if e = authz.Require(ctx, tx, c.Actor, "change.create", resource(p)); e != nil {
			return e
		}
		if e = authz.Require(ctx, tx, c.Actor, "budget.read", resource(p)); e != nil {
			return e
		}
		_, e = deliverableFiles(ctx, tx, c.Actor, p, in.Content.FileVersionIDs, false)
		return e
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		p, e := getProject(ctx, tx, c.Actor.OrgID, pid, true)
		if e != nil {
			return core.Result{}, e
		}
		if e = projectWritable(p); e != nil {
			return core.Result{}, e
		}
		if e = normalizeChange(ctx, tx, &in); e != nil {
			return core.Result{}, e
		}
		b, e := currentChangeBasis(ctx, tx, c.Actor.OrgID, pid, in.Content.Currency)
		if e != nil {
			return core.Result{}, e
		}
		if e = exactChangeBase(in, b); e != nil {
			return core.Result{}, e
		}
		if _, e = deliverableFiles(ctx, tx, c.Actor, p, in.Content.FileVersionIDs, true); e != nil {
			return core.Result{}, e
		}
		delta, e := changeAmounts(in.Content, b)
		if e != nil {
			return core.Result{}, e
		}
		id, rid := core.NewID(), core.NewID()
		_, e = tx.Exec(ctx, `INSERT INTO change_requests(id,org_id,project_id,title,state,scope_delta,budget_delta,currency,base_project_version,protocol_version,creator_id,classification,current_revision_id) VALUES($1,$2,$3,$4,'draft',$5,$6,$7,$8,1,$9,$10,$11)`, id, c.Actor.OrgID, pid, in.Content.Title, in.Content.ScopeDelta, delta, in.Content.Currency, p.Version, c.Actor.UserID, p.Classification, rid)
		if e != nil {
			return core.Result{}, e
		}
		if e = insertChangeRevision(ctx, tx, c.Actor, id, rid, 1, nil, in, b); e != nil {
			return core.Result{}, e
		}
		d, e := changeHead(ctx, tx, c.Actor.OrgID, id, false)
		return changeResult(d, "change.created", 201, in.Reason), e
	})
}
func lockChange(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Change, Project, error) {
	d, e := changeHead(ctx, db, a.OrgID, id, false)
	if e != nil {
		return d, Project{}, e
	}
	p, e := getProject(ctx, db, a.OrgID, d.ProjectID, true)
	if e != nil {
		return d, p, e
	}
	d, e = changeHead(ctx, db, a.OrgID, id, true)
	return d, p, e
}
func (s *Service) UpdateChange(ctx context.Context, c core.Command, id string, in ChangePatch) (core.Result, error) {
	check := func(tx pgx.Tx) error {
		_, p, e := changeRead(ctx, tx, c.Actor, id, "change.update")
		if e != nil {
			return e
		}
		_, e = getProject(ctx, tx, c.Actor.OrgID, p.ID, true)
		if e != nil {
			return e
		}
		_, e = deliverableFiles(ctx, tx, c.Actor, p, in.Content.FileVersionIDs, false)
		return e
	}
	return s.Commands.Do(ctx, c, check, func(tx pgx.Tx) (core.Result, error) {
		d, p, e := lockChange(ctx, tx, c.Actor, id)
		if e != nil {
			return core.Result{}, e
		}
		if e = check(tx); e != nil {
			return core.Result{}, e
		}
		if e = projectWritable(p); e != nil {
			return core.Result{}, e
		}
		if d.Version != in.ExpectedVersion || d.CurrentRevisionID != in.BaseRevisionID {
			return core.Result{}, core.Conflict()
		}
		if d.Lifecycle != "draft" || d.ApprovalState == "in_review" || d.ApprovalState == "approved" {
			return core.Result{}, core.Fail(409, "PROJECT_CHANGE_READ_ONLY", "仅未在审批或未获批准的草稿可以修订；旧批准不能用于新提案")
		}
		if e = normalizeChange(ctx, tx, &in.ChangeCreate); e != nil {
			return core.Result{}, e
		}
		var currency string
		if e = tx.QueryRow(ctx, "SELECT currency FROM change_requests WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id).Scan(&currency); e != nil {
			return core.Result{}, e
		}
		if currency != in.Content.Currency {
			return core.Result{}, core.Invalid("currency", "同一变更的币种不可替换；不同币种请另建变更")
		}
		b, e := currentChangeBasis(ctx, tx, c.Actor.OrgID, p.ID, in.Content.Currency)
		if e != nil {
			return core.Result{}, e
		}
		if e = exactChangeBase(in.ChangeCreate, b); e != nil {
			return core.Result{}, e
		}
		if _, e = deliverableFiles(ctx, tx, c.Actor, p, in.Content.FileVersionIDs, true); e != nil {
			return core.Result{}, e
		}
		delta, e := changeAmounts(in.Content, b)
		if e != nil {
			return core.Result{}, e
		}
		var seq int64
		if e = tx.QueryRow(ctx, "SELECT seq FROM project_change_revisions WHERE org_id=$1 AND id=$2", c.Actor.OrgID, d.CurrentRevisionID).Scan(&seq); e != nil {
			return core.Result{}, e
		}
		rid := core.NewID()
		base := d.CurrentRevisionID
		if e = insertChangeRevision(ctx, tx, c.Actor, id, rid, seq+1, &base, in.ChangeCreate, b); e != nil {
			return core.Result{}, e
		}
		_, e = tx.Exec(ctx, `UPDATE change_requests SET title=$3,scope_delta=$4,budget_delta=$5,currency=$6,base_project_version=$7,current_revision_id=$8,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, id, in.Content.Title, in.Content.ScopeDelta, delta, in.Content.Currency, p.Version, rid)
		if e != nil {
			return core.Result{}, e
		}
		d, e = changeHead(ctx, tx, c.Actor.OrgID, id, false)
		return changeResult(d, "change.revised", 200, in.Reason), e
	})
}

// ChangeApprovalRead is shared by the approval service without importing workflow here.
func ChangeApprovalRead(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Change, error) {
	d, p, e := changeRead(ctx, db, a, id, "change.read")
	if e != nil {
		return Change{}, e
	}
	d.Revision, e = readChangeRevision(ctx, db, a, d, p, d.CurrentRevisionID, false)
	return d, e
}
func ChangeApprovalDraft(ctx context.Context, db platform.DBTX, a core.Actor, id string, lock, complete bool) (Change, error) {
	d, p, e := changeRead(ctx, db, a, id, "change.update")
	if e != nil {
		return Change{}, e
	}
	if lock {
		d, p, e = lockChange(ctx, db, a, id)
		if e != nil {
			return Change{}, e
		}
	}
	if e = authz.Require(ctx, db, a, "change.submit", changeResource(d)); e != nil {
		return Change{}, e
	}
	if e = projectWritable(p); e != nil {
		return Change{}, e
	}
	if d.Lifecycle != "draft" || d.ApprovalState == "in_review" || d.ApprovalState == "approved" {
		return Change{}, core.Fail(409, "PROJECT_CHANGE_READ_ONLY", "当前变更不能替换送审内容")
	}
	d.Revision, e = readChangeRevision(ctx, db, a, d, p, d.CurrentRevisionID, complete)
	if e != nil {
		return Change{}, e
	}
	if complete {
		same, e := changeBasisCurrent(ctx, db, a.OrgID, d.Revision)
		if e != nil {
			return Change{}, e
		}
		if !same {
			return Change{}, core.Fail(409, "PROJECT_CHANGE_BASE_CHANGED", "项目或预算基线已变化，请修订后重新送审")
		}
		if len(d.Revision.Attachments) < 1 {
			return Change{}, core.Invalid("file_version_ids", "送审需至少一个正式依据文件")
		}
	}
	return d, nil
}
func (s *Service) ImplementChange(ctx context.Context, c core.Command, id string, in ChangeImplement) (core.Result, error) {
	if in.ExpectedVersion < 1 || in.BaseProjectVersion < 1 || !core.ValidID(in.RevisionID) || !core.ValidID(in.RequestRevisionID) || len(in.ContentHash) != 64 || strings.Trim(in.ContentHash, "0123456789abcdef") != "" || !in.ScopeVerified || !in.BudgetVerified || core.Name(strings.TrimSpace(in.Reason)) != nil {
		return core.Result{}, core.Invalid("implementation", "请固定准确批准修订，逐项人工核实范围与预算并填写实施原因")
	}
	check := func(tx pgx.Tx) error {
		d, p, e := changeRead(ctx, tx, c.Actor, id, "change.implement")
		if e != nil {
			return e
		}
		if e = authz.Require(ctx, tx, c.Actor, "project.update", resource(p)); e != nil {
			return e
		}
		if e = identity.RequireInteractiveSession(ctx, tx, c.Actor, true); e != nil {
			return e
		}
		_, e = getProject(ctx, tx, c.Actor.OrgID, p.ID, true)
		if e != nil {
			return e
		}
		_, e = readChangeRevision(ctx, tx, c.Actor, d, p, in.RevisionID, false)
		return e
	}
	return s.Commands.Do(ctx, c, check, func(tx pgx.Tx) (core.Result, error) {
		d, p, e := lockChange(ctx, tx, c.Actor, id)
		if e != nil {
			return core.Result{}, e
		}
		if e = check(tx); e != nil {
			return core.Result{}, e
		}
		if e = projectWritable(p); e != nil {
			return core.Result{}, e
		}
		if d.Version != in.ExpectedVersion || d.CurrentRevisionID != in.RevisionID || p.Version != in.BaseProjectVersion {
			return core.Result{}, core.Conflict()
		}
		if d.ApprovalState != "approved" || d.Lifecycle != "requested" || d.RequestID == nil {
			return core.Result{}, core.Fail(409, "PROJECT_CHANGE_NOT_APPROVED", "仅准确获批准且尚未实施的变更可以实施")
		}
		r, e := readChangeRevision(ctx, tx, c.Actor, d, p, in.RevisionID, true)
		if e != nil {
			return core.Result{}, e
		}
		if r.ContentHash != in.ContentHash {
			return core.Result{}, core.Conflict()
		}
		same, e := changeBasisCurrent(ctx, tx, c.Actor.OrgID, r)
		if e != nil {
			return core.Result{}, e
		}
		if !same {
			return core.Result{}, core.Fail(409, "PROJECT_CHANGE_BASE_CHANGED", "项目或预算已变化，请另提变更并重新审批")
		}
		var state, qrid string
		if e = tx.QueryRow(ctx, "SELECT state,current_revision_id FROM requests WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, *d.RequestID).Scan(&state, &qrid); e != nil {
			return core.Result{}, platform.DatabaseError(e)
		}
		if state != "approved" || qrid != in.RequestRevisionID {
			return core.Result{}, core.Conflict()
		}
		tag, e := tx.Exec(ctx, `INSERT INTO project_change_implementations(id,org_id,change_id,revision_id,request_revision_id,project_id,before_project_version,after_project_version,change_version,actor_id,session_id,mfa_verified_at,scope_verified,budget_verified,reason) SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,s.id,s.mfa_verified_at,$11,$12,$13 FROM sessions s WHERE s.org_id=$2 AND s.id=$14 AND s.user_id=$10`, core.NewID(), c.Actor.OrgID, id, r.ID, in.RequestRevisionID, p.ID, p.Version, p.Version+1, d.Version+1, c.Actor.UserID, in.ScopeVerified, in.BudgetVerified, strings.TrimSpace(in.Reason), c.Actor.SessionID)
		if e != nil {
			return core.Result{}, e
		}
		if tag.RowsAffected() != 1 {
			return core.Result{}, core.Forbidden()
		}
		d, e = changeHead(ctx, tx, c.Actor.OrgID, id, false)
		return changeResult(d, "change.implemented", 200, in.Reason), e
	})
}
