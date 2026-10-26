package contracts

import (
	"context"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

// Nil is unknown; an explicitly approved no_payment schedule is a separate fact.
func normalizeSchedule(ctx context.Context, db platform.DBTX, in Content, complete bool) (*PaymentSchedule, error) {
	if in.PaymentSchedule == nil {
		return nil, nil
	}
	p := *in.PaymentSchedule
	p.Reason = strings.TrimSpace(p.Reason)
	p.Items = append([]PaymentPlanItem{}, p.Items...)
	invalid := func(message string) (*PaymentSchedule, error) { return nil, core.Invalid("payment_schedule", message) }
	if len([]rune(p.Reason)) > 20000 || len(p.Items) > 100 {
		return invalid("收付款计划说明过长或节点超过 100 项")
	}
	if p.Mode == "no_payment" {
		if len(p.Items) != 0 || p.Reason == "" {
			return invalid("无收付义务须填写依据并清空收付款节点")
		}
		if in.Direction != nil && *in.Direction != "none" {
			return invalid("无收付计划须与合同无收付方向一致")
		}
		if in.Amount != nil {
			amount, err := core.ParseDecimal(*in.Amount)
			if err != nil || amount.Positive() {
				return invalid("无收付义务合同总额须为零")
			}
		}
		if complete && (in.Direction == nil || in.Amount == nil) {
			return invalid("送审前须确认无收付方向与零合同总额")
		}
		return &p, nil
	}
	if p.Mode != "scheduled" || len(p.Items) == 0 {
		return invalid("请选择有效计划类型并至少填写一个收付款节点")
	}
	if in.Direction != nil && *in.Direction == "none" {
		return invalid("有收付款节点时须选择收款或付款方向")
	}
	var exponent int
	if in.Currency != nil {
		if err := db.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", *in.Currency).Scan(&exponent); err != nil {
			return nil, platform.DatabaseError(err)
		}
	}
	seen := map[string]bool{}
	total, _ := core.ParseDecimal("0")
	allKnown := true
	for i := range p.Items {
		item := &p.Items[i]
		item.ID = strings.ToLower(item.ID)
		item.Title = strings.TrimSpace(item.Title)
		if !core.ValidID(item.ID) || seen[item.ID] || core.Name(item.Title) != nil || len([]rune(item.Conditions)) > 20000 {
			return invalid("节点须有唯一标识、有效标题和适当长度的条件")
		}
		seen[item.ID] = true
		if item.Amount != nil {
			if in.Currency == nil {
				return invalid("填写节点金额前须先确认合同币种")
			}
			amount, err := core.CheckCurrencyAmount(*in.Currency, *item.Amount, map[string]int{*in.Currency: exponent})
			if err != nil {
				return nil, err
			}
			if !amount.Positive() {
				return invalid("收付款节点金额须大于零")
			}
			text := amount.String()
			item.Amount = &text
			total = total.Add(amount)
		} else {
			allKnown = false
		}
		if item.DueDate != nil && core.Date(*item.DueDate) != nil {
			return invalid("收付款到期日无效")
		}
		if complete && (item.Amount == nil || item.DueDate == nil) {
			return invalid("送审前须补全每个收付款节点的金额和到期日")
		}
	}
	if allKnown && in.Amount != nil {
		amount, _ := core.ParseDecimal(*in.Amount)
		if total.Cmp(amount) != 0 {
			return invalid("各收付款节点金额合计须等于合同总额")
		}
	}
	if complete && (in.Direction == nil || in.Currency == nil || in.Amount == nil) {
		return invalid("送审前须确认计划的收付方向、币种和合同总额")
	}
	return &p, nil
}

// Schedule changes use the same immutable revision and request invalidation as full edits.
func (s *Service) Schedule(ctx context.Context, c core.Command, id string, in ScheduleInput) (core.Result, error) {
	return s.update(ctx, c, id, in.ExpectedVersion, in.BaseRevisionID, in.Reason, func(d Contract) Content { value := d.Content; value.PaymentSchedule = in.PaymentSchedule; return value })
}

func (s *Service) Milestones(ctx context.Context, a core.Actor, id string) (core.Page[Milestone], error) {
	out := core.Page[Milestone]{Items: []Milestone{}}
	d, err := metadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return out, err
	}
	if err = authz.Require(ctx, s.DB, a, "contract.read", resource(d)); err != nil {
		return out, err
	}
	rows, err := s.DB.Query(ctx, `SELECT id,contract_id,revision_id,plan_item_id,position,title,direction,currency,amount::text,due_date::text,conditions,created_at
	    FROM contract_milestones WHERE org_id=$1 AND contract_id=$2 ORDER BY position`, a.OrgID, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Milestone
		if err = rows.Scan(&m.ID, &m.ContractID, &m.RevisionID, &m.PlanItemID, &m.Position, &m.Title, &m.Direction, &m.Currency, &m.Amount, &m.DueDate, &m.Conditions, &m.CreatedAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, m)
	}
	return out, rows.Err()
}
