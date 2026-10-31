package finance

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

type SubscriptionPortfolioInput struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type SubscriptionPortfolioItem struct {
	ID                  string `json:"id"`
	Product             string `json:"product"`
	State               string `json:"state"`
	StoredPeriodCount   int    `json:"stored_period_count"`
	ExpectedPeriodCount int    `json:"expected_period_count"`
	MissingPeriodCount  int    `json:"missing_period_count"`
	CoverageKnown       bool   `json:"coverage_known"`
	CoverageComplete    bool   `json:"coverage_complete"`
}
type SubscriptionPortfolioSummary struct {
	Items                    []SubscriptionPortfolioItem `json:"items"`
	SubscriptionCount        int                         `json:"subscription_count"`
	StoredPeriodCount        int                         `json:"stored_period_count"`
	ExpectedPeriodCount      int                         `json:"expected_period_count"`
	MissingPeriodCount       int                         `json:"missing_period_count"`
	ScheduleCoverageKnown    bool                        `json:"schedule_coverage_known"`
	ScheduleCoverageComplete bool                        `json:"schedule_coverage_complete"`
	UniqueCostCount          int                         `json:"unique_cost_count"`
	UniquePaymentCount       int                         `json:"unique_payment_count"`
	SharedRecordCount        int                         `json:"shared_record_count"`
	SubscriptionSetComplete  bool                        `json:"subscription_set_complete"`
}

// Enumerate the actor's entire readable set in this exact conversation scope,
// then require every current participant to read every detailed dependency.
// A limit failure never returns a completed prefix or silently omits a row.
func AgentSubscriptionPortfolio(ctx context.Context, tx pgx.Tx, a core.Actor, scope ReviewScope, in SubscriptionPortfolioInput) (map[string]any, []AgentSummaryEvidence, SubscriptionPortfolioSummary, error) {
	out := SubscriptionPortfolioSummary{Items: []SubscriptionPortfolioItem{}}
	from, err := civilDate(in.From)
	if err != nil {
		return nil, nil, out, err
	}
	to, err := civilDate(in.To)
	if err != nil || !from.Before(to) || to.Sub(from) > 366*24*time.Hour {
		return nil, nil, out, core.Invalid("dates", "请明确开始日和不含结束日，最多366个自然日")
	}
	if err = ValidateAgentSummary(ctx, tx, a, scope.ProjectID, scope.Classification, scope.Audience, nil); err != nil {
		return nil, nil, out, err
	}
	rows, err := tx.Query(ctx, `SELECT s.id::text FROM subscriptions s WHERE s.org_id=$1
 AND s.project_id IS NOT DISTINCT FROM $4::uuid
 AND can_read_subscription(s.org_id,$2,s.id,$3,true)
 AND can_access(s.org_id,$2,'report.read','subscription',s.id,s.project_id,s.creator_id,s.classification,$3,true)
 AND array_position(ARRAY['internal','project_restricted','confidential'],s.classification)<=array_position(ARRAY['internal','project_restricted','confidential'],$5)
 ORDER BY s.id LIMIT 101`, a.OrgID, a.UserID, a.Channel, scope.ProjectID, scope.Classification)
	if err != nil {
		return nil, nil, out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, nil, out, err
	}
	if len(ids) > 100 {
		return nil, nil, out, core.Invalid("scope", "完整审阅最多100个可读订阅，未返回不完整前缀；请使用更明确的项目会话")
	}
	refs := []AgentSummaryEvidence{}
	seen := map[string]bool{}
	details := []any{}
	linked := map[string]map[string]bool{}
	costIDs, paymentIDs := map[string]bool{}, map[string]bool{}
	out.ScheduleCoverageKnown, out.ScheduleCoverageComplete = len(ids) > 0, len(ids) > 0
	for _, id := range ids {
		content, dependencies, e := AgentSubscriptionReview(ctx, tx, a, scope, SubscriptionReviewInput{SubscriptionID: id, From: in.From, To: in.To})
		if e != nil {
			return nil, nil, out, e
		}
		report := content["report"].(map[string]any)
		sub := report["subscription"].(Subscription)
		periods := report["periods"].([]SubscriptionPeriod)
		item := SubscriptionPortfolioItem{ID: sub.ID, Product: sub.Product, State: sub.State, StoredPeriodCount: len(periods), ExpectedPeriodCount: report["expected_scheduled_period_count"].(int), MissingPeriodCount: len(report["missing_scheduled_periods"].([]SubscriptionWindow)), CoverageKnown: report["coverage_known"].(bool), CoverageComplete: report["coverage_complete"].(bool)}
		out.Items = append(out.Items, item)
		out.StoredPeriodCount += item.StoredPeriodCount
		out.ExpectedPeriodCount += item.ExpectedPeriodCount
		out.MissingPeriodCount += item.MissingPeriodCount
		out.ScheduleCoverageKnown = out.ScheduleCoverageKnown && item.CoverageKnown
		out.ScheduleCoverageComplete = out.ScheduleCoverageComplete && item.CoverageComplete
		details = append(details, report)
		for _, dependency := range dependencies {
			encoded, _ := json.Marshal(dependency)
			if !seen[string(encoded)] {
				seen[string(encoded)] = true
				refs = append(refs, dependency)
			}
		}
		if err = reviewBounded(len(refs), 500); err != nil {
			return nil, nil, out, err
		}
		for _, kind := range []string{"costs", "payments"} {
			for _, record := range report[kind].([]map[string]any) {
				recordID := record["id"].(string)
				if kind == "costs" {
					costIDs[recordID] = true
				} else {
					paymentIDs[recordID] = true
				}
				key := kind + ":" + recordID
				if linked[key] == nil {
					linked[key] = map[string]bool{}
				}
				linked[key][id] = true
			}
		}
	}
	shared := []map[string]any{}
	for key, subscriptions := range linked {
		if len(subscriptions) > 1 {
			shared = append(shared, map[string]any{"record_key": key, "subscription_ids": stringMapKeys(subscriptions)})
		}
	}
	slices.SortFunc(shared, func(a, b map[string]any) int {
		if a["record_key"].(string) < b["record_key"].(string) {
			return -1
		}
		if a["record_key"] == b["record_key"] {
			return 0
		}
		return 1
	})
	out.SubscriptionCount, out.UniqueCostCount, out.UniquePaymentCount, out.SharedRecordCount, out.SubscriptionSetComplete = len(ids), len(costIDs), len(paymentIDs), len(shared), true
	if err = ValidateAgentSummary(ctx, tx, a, scope.ProjectID, scope.Classification, scope.Audience, refs); err != nil {
		return nil, nil, out, err
	}
	content, err := reviewEnvelope(ctx, tx, scope, "subscription_portfolio", map[string]any{"from": in.From, "to_exclusive": in.To, "date_basis": "subscription_local_start_date", "selection": "actor_readable_conversation_subscriptions_all_states", "summary": out, "reviews": details, "shared_records": shared, "money_notice": "不汇总订阅原费用或原付款金额。共享关联可能是正常分摊，不证明重复扣费；原币种、费用份额、实际资金分配和未归属退款须分别核对。"}, refs)
	if err != nil {
		return nil, nil, out, err
	}
	content["coverage_notice"] = "完整性仅针对读取时刻、发起人有权且允许AI处理、在当前会话项目及分类范围内的全部订阅（含所有状态）。全体参与者必须有权读取全部逐笔来源，否则整笔失败；不会披露或统计不可读订阅。公司会话仅含公司订阅，不代表所有项目或供应商实际收费。日程覆盖只说明应有账期是否已生成，不证明已付清、零义务、供应商停费或重复扣费。"
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil, nil, out, err
	}
	if len(encoded) > 89000 {
		return nil, nil, out, core.Invalid("scope", "完整逐笔快照超过受控大小，请缩小日期或使用准确项目范围；未返回不完整前缀")
	}
	return content, refs, out, nil
}
