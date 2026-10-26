package finance

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"regexp"
	"slices"
	"strings"
)

var sourcePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,31}:[A-Za-z0-9._:/-]{1,166}$`)

func validBase(in Base) error {
	if core.RequireVersion(in.ExpectedVersion) != nil || !core.ValidID(in.RevisionID) || len(in.ContentHash) != 64 || strings.Trim(in.ContentHash, "0123456789abcdef") != "" || core.Name(in.Reason) != nil {
		return core.Invalid("revision_id", "请固定准确费用版本、修订及摘要，并说明原因")
	}
	return nil
}
func resource(c Cost) authz.Resource {
	return authz.Resource{Type: "cost", ID: c.ID, ProjectID: c.ProjectID, OwnerID: &c.CreatorID, Classification: c.Classification}
}
func projectIDs(c Cost, content Content) []string {
	ids := []string{}
	if c.ProjectID != nil {
		ids = append(ids, *c.ProjectID)
	}
	for _, v := range append(append([]Allocation{}, c.Allocations...), content.Allocations...) {
		if v.ProjectID != nil && !slices.Contains(ids, *v.ProjectID) {
			ids = append(ids, *v.ProjectID)
		}
	}
	slices.Sort(ids)
	return ids
}
func projects(ctx context.Context, db platform.DBTX, a core.Actor, c Cost, content Content, write bool) error {
	for _, id := range projectIDs(c, content) {
		if !core.ValidID(id) {
			return core.Invalid("project_id", "项目标识无效")
		}
		var state, class, creator string
		query := "SELECT state,classification,creator_id FROM projects WHERE org_id=$1 AND id=$2"
		if write {
			query += " FOR UPDATE"
		}
		if err := db.QueryRow(ctx, query, a.OrgID, id).Scan(&state, &class, &creator); err != nil {
			return platform.DatabaseError(err)
		}
		if err := authz.Require(ctx, db, a, "project.read", authz.Resource{Type: "project", ID: id, ProjectID: &id, OwnerID: &creator, Classification: class}); err != nil {
			return err
		}
		var adequate bool
		if err := db.QueryRow(ctx, "SELECT classification_rank($1)<=classification_rank($2)", class, c.Classification).Scan(&adequate); err != nil {
			return err
		}
		if !adequate {
			return core.Invalid("classification", "费用分类不能低于分摊项目")
		}
		if write && slices.Contains([]string{"closing", "archived", "cancelled"}, state) {
			return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许登记或更改费用")
		}
	}
	return nil
}
func normalize(ctx context.Context, db platform.DBTX, c Content, complete bool) (Content, error) {
	c.Title = strings.TrimSpace(c.Title)
	c.Category = strings.TrimSpace(c.Category)
	if core.Name(c.Title) != nil || len([]rune(c.Category)) > 50 || len([]rune(c.Notes)) > 20000 || len(c.Allocations) > 100 || len(c.EvidenceVersionIDs) > 20 {
		return c, core.Invalid("content", "请核对费用标题、类别、备注及明细数量")
	}
	if c.OccurredOn != nil && (*c.OccurredOn == "" || core.Date(*c.OccurredOn) != nil) {
		return c, core.Invalid("occurred_on", "请填写实际发生日期或留空")
	}
	exponent := 0
	if c.Currency != nil {
		if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", *c.Currency).Scan(&exponent); err != nil {
			return c, core.Invalid("currency", "币种尚未配置")
		}
	}
	if c.Amount != nil {
		if c.Currency == nil {
			return c, core.Invalid("currency", "填写金额前须选择币种")
		}
		v, err := core.CheckCurrencyAmount(*c.Currency, *c.Amount, map[string]int{*c.Currency: exponent})
		if err != nil {
			return c, err
		}
		text := v.String()
		c.Amount = &text
	}
	if c.Counterparty != nil {
		c.Counterparty = &PartyRef{PartyID: strings.ToLower(c.Counterparty.PartyID), RevisionID: strings.ToLower(c.Counterparty.RevisionID)}
		if !core.ValidID(c.Counterparty.PartyID) || !core.ValidID(c.Counterparty.RevisionID) {
			return c, core.Invalid("counterparty", "请固定准确供应方修订")
		}
	}
	if c.Allocations == nil {
		c.Allocations = []Allocation{}
	}
	sum := core.Decimal{}
	seen := map[string]bool{}
	for i := range c.Allocations {
		l := &c.Allocations[i]
		key := "company"
		if l.ProjectID != nil {
			v := strings.ToLower(*l.ProjectID)
			l.ProjectID = &v
			key = v
			if !core.ValidID(v) {
				return c, core.Invalid("project_id", "分摊项目无效")
			}
		}
		if seen[key] || c.Currency == nil {
			return c, core.Invalid("allocations", "分摊范围不能重复，须先选择币种")
		}
		seen[key] = true
		v, err := core.CheckCurrencyAmount(*c.Currency, l.Amount, map[string]int{*c.Currency: exponent})
		if err != nil || !v.Positive() {
			return c, core.Invalid("allocations", "各项分摊须为符合币种精度的正数")
		}
		l.Amount = v.String()
		sum = sum.Add(v)
	}
	c.EvidenceVersionIDs = append([]string{}, c.EvidenceVersionIDs...)
	for i, id := range c.EvidenceVersionIDs {
		c.EvidenceVersionIDs[i] = strings.ToLower(id)
		if !core.ValidID(id) {
			return c, core.Invalid("evidence_version_ids", "凭证版本无效")
		}
	}
	slices.Sort(c.EvidenceVersionIDs)
	for i := 1; i < len(c.EvidenceVersionIDs); i++ {
		if c.EvidenceVersionIDs[i] == c.EvidenceVersionIDs[i-1] {
			return c, core.Invalid("evidence_version_ids", "凭证版本不能重复")
		}
	}
	if complete {
		var today string
		if err := db.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today); err != nil {
			return c, err
		}
		if c.Amount == nil || c.Currency == nil || c.OccurredOn == nil || *c.OccurredOn > today || c.Category == "" || c.Counterparty == nil || len(c.Allocations) == 0 || len(c.EvidenceVersionIDs) == 0 {
			return c, core.Invalid("content", "确认实际费用须补齐日期、供应方、币种金额、类别、分摊及凭证")
		}
		amount, _ := core.ParseDecimal(*c.Amount)
		if !amount.Positive() || sum.Cmp(amount) != 0 {
			return c, core.Invalid("allocations", "项目与公司分摊合计必须等于费用总额")
		}
	}
	return c, nil
}
func references(ctx context.Context, db platform.DBTX, a core.Actor, c Cost, v Content, strict, approval bool) (*PartyEvidence, []fileref.Evidence, error) {
	var supplier *PartyEvidence
	files := []fileref.Evidence{}
	hidden := func(err error) bool { return slices.Contains([]int{403, 404, 409, 422}, core.PublicError(err).Status) }
	if v.Counterparty != nil {
		p, r, err := party.ResolveBusinessReference(ctx, db, a, v.Counterparty.PartyID, v.Counterparty.RevisionID, strict)
		if err != nil {
			if strict || !hidden(err) {
				return nil, nil, err
			}
		} else {
			var adequate bool
			if err = db.QueryRow(ctx, "SELECT classification_rank($1)<=classification_rank($2)", p.Classification, c.Classification).Scan(&adequate); err != nil {
				return nil, nil, err
			}
			if !adequate || (p.ProjectID != nil && (c.ProjectID == nil || *p.ProjectID != *c.ProjectID)) {
				if strict {
					return nil, nil, core.Invalid("counterparty", "供应方须与费用范围和分类匹配")
				}
			} else {
				supplier = &PartyEvidence{PartyRef: *v.Counterparty, Name: r.Name}
			}
		}
	}
	for _, id := range v.EvidenceVersionIDs {
		var e fileref.Evidence
		var err error
		if approval {
			e, err = fileref.ForApproval(ctx, db, a, id, c.ProjectID, c.Classification)
		} else {
			e, err = fileref.Resolve(ctx, db, a, id, c.ProjectID, c.Classification)
		}
		if err != nil {
			if strict || !hidden(err) {
				return nil, nil, err
			}
			continue
		}
		files = append(files, e)
	}
	return supplier, files, nil
}
func insertRevision(ctx context.Context, tx pgx.Tx, a core.Actor, c Cost, id string, seq int64, base *string, v Content, reason string) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO cost_revisions(id,org_id,cost_id,seq,base_revision_id,payload,content_hash,occurred_on,author_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, a.OrgID, c.ID, seq, base, payload, core.Digest(payload), v.OccurredOn, a.UserID, reason)
	if err != nil {
		return err
	}
	for i, l := range v.Allocations {
		if _, err = tx.Exec(ctx, "INSERT INTO cost_allocations(org_id,cost_id,revision_id,position,project_id,amount) VALUES($1,$2,$3,$4,$5,$6)", a.OrgID, c.ID, id, i+1, l.ProjectID, l.Amount); err != nil {
			return err
		}
	}
	for _, file := range v.EvidenceVersionIDs {
		if _, err = tx.Exec(ctx, "INSERT INTO cost_revision_files(org_id,revision_id,version_id) VALUES($1,$2,$3)", a.OrgID, id, file); err != nil {
			return err
		}
	}
	return nil
}
