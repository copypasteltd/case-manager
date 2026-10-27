package finance

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// AgentSummaryEvidence is a private authorization catalogue, never model text.
// Revision IDs keep the original allocation checks available after a draft changes.
type AgentSummaryEvidence struct {
	Type                string  `json:"type"`
	ID                  string  `json:"id"`
	RevisionID          string  `json:"revision_id"`
	SubscriptionID      string  `json:"subscription_id,omitempty"`
	PeriodID            string  `json:"period_id,omitempty"`
	DependencyProjectID *string `json:"dependency_project_id,omitempty"`
}

func financeClassRank(c string) int {
	switch c {
	case "internal":
		return 1
	case "project_restricted":
		return 2
	case "confidential":
		return 3
	}
	return 0
}

// ValidateAgentSummary rechecks current AI/channel policy and linked-resource
// permissions for every participant. A private input never becomes a shared total.
func ValidateAgentSummary(ctx context.Context, tx pgx.Tx, a core.Actor, project *string, class string, members []string, evidence []AgentSummaryEvidence) error {
	if len(members) == 0 || len(members) > 30 || len(evidence) > 500 || financeClassRank(class) == 0 {
		return core.Missing()
	}
	for _, user := range members {
		target := a
		target.UserID = user
		if target.RunID == "" {
			target.RunID = "finance-evidence"
		}
		if err := authz.Require(ctx, tx, target, "report.read", authz.Resource{Type: "report", ProjectID: project, OwnerID: &a.UserID, Classification: class}); err != nil {
			return err
		}
		for _, ref := range evidence {
			if handled, err := validateReviewDependency(ctx, tx, target, class, ref); handled {
				if err != nil {
					return err
				}
				continue
			}
			if ref.SubscriptionID != "" {
				if err := validateReviewRelation(ctx, tx, a.OrgID, project, ref); err != nil {
					return err
				}
			}
			var query string
			switch ref.Type {
			case "cost":
				query = `SELECT c.classification, can_read_cost(c.org_id,$2,c.id,$3,$4,true) AND can_read_cost(c.org_id,$2,c.id,c.current_revision_id,$4,true) AND can_access(c.org_id,$2,'report.read','cost',c.id,c.project_id,c.creator_id,c.classification,$4,true) FROM actual_cost_entries c WHERE c.org_id=$1 AND c.id=$5`
			case "payment":
				query = `SELECT p.classification, can_read_payment(p.org_id,$2,p.id,$4,true) AND can_access(p.org_id,$2,'report.read','payment',p.id,p.project_id,p.creator_id,p.classification,$4,true) AND EXISTS(SELECT 1 FROM payment_revisions r WHERE r.org_id=p.org_id AND r.payment_id=p.id AND r.id=$3) FROM payment_records p WHERE p.org_id=$1 AND p.id=$5`
			case "subscription":
				query = `SELECT s.classification, can_read_subscription(s.org_id,$2,s.id,$4,true) AND can_access(s.org_id,$2,'report.read','subscription',s.id,s.project_id,s.creator_id,s.classification,$4,true) AND EXISTS(SELECT 1 FROM subscription_revisions r WHERE r.org_id=s.org_id AND r.subscription_id=s.id AND r.id=$3) FROM subscriptions s WHERE s.org_id=$1 AND s.id=$5`
			case "subscription_period":
				query = `SELECT s.classification, can_read_subscription(s.org_id,$2,s.id,$4,true) AND can_access(s.org_id,$2,'report.read','subscription',s.id,s.project_id,s.creator_id,s.classification,$4,true) AND EXISTS(SELECT 1 FROM subscription_period_revisions r WHERE r.org_id=p.org_id AND r.period_id=p.id AND r.id=$3) FROM subscription_periods p JOIN subscriptions s ON s.org_id=p.org_id AND s.id=p.subscription_id WHERE p.org_id=$1 AND p.id=$5`
			default:
				return core.Missing()
			}
			var resourceClass string
			var allowed bool
			if err := tx.QueryRow(ctx, query, a.OrgID, user, ref.RevisionID, a.Channel, ref.ID).Scan(&resourceClass, &allowed); err != nil {
				if err == pgx.ErrNoRows {
					return core.Missing()
				}
				return err
			}
			if !allowed || financeClassRank(resourceClass) == 0 || financeClassRank(resourceClass) > financeClassRank(class) {
				return core.Missing()
			}
		}
	}
	return nil
}

// AgentSummary uses the same projections as the ordinary finance API, in the
// caller's locked transaction. Company conversations mean company allocations;
// project conversations mean that project's allocations, never all-company sums.
func AgentSummary(ctx context.Context, tx pgx.Tx, a core.Actor, project *string, class string, members []string, month, view, currency string) (map[string]any, []AgentSummaryEvidence, error) {
	if len(month) != 7 {
		return nil, nil, core.Invalid("month", "请明确 YYYY-MM 月份")
	}
	from, e := time.Parse("2006-01", month)
	if e != nil || from.Year() < 2000 || from.Year() > 2100 {
		return nil, nil, core.Invalid("month", "月份须介于2000至2100年")
	}
	if view != "forecast" && view != "actual" && view != "cash" {
		return nil, nil, core.Invalid("view", "请选择 forecast、actual 或 cash，口径不能混算")
	}
	if currency != "" && (len(currency) != 3 || strings.Trim(currency, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "") {
		return nil, nil, core.Invalid("currency", "币种须为三位大写代码")
	}
	if err := ValidateAgentSummary(ctx, tx, a, project, class, members, nil); err != nil {
		return nil, nil, err
	}
	f := Filter{From: from.Format(time.DateOnly), To: from.AddDate(0, 1, 0).Format(time.DateOnly), ProjectID: project, CompanyOnly: project == nil, Currency: currency}
	var query string
	var args []any
	switch view {
	case "actual":
		query = "SELECT 'cost',c.id,c.current_revision_id" + currentJoin + incurredReportPredicate + " ORDER BY c.id LIMIT 501"
		args = []any{a.OrgID, a.UserID, a.Channel, true, f.From, f.To, f.Currency, "", f.ProjectID, f.CompanyOnly, (*string)(nil), (*string)(nil), ""}
	case "cash":
		query = "SELECT 'payment',p.id,p.current_revision_id" + paymentJoin + cashReportPredicate + " ORDER BY p.id LIMIT 501"
		args = []any{a.OrgID, a.UserID, a.Channel, true, f.From, f.To, f.Currency, "", f.ProjectID, f.CompanyOnly, (*string)(nil), (*string)(nil), ""}
	case "forecast":
		query = "SELECT 'subscription',s.id,s.current_revision_id" + subscriptionJoin + forecastReportScope + " ORDER BY s.id LIMIT 501"
		args = []any{a.OrgID, a.UserID, a.Channel, true, "", f.ProjectID, f.CompanyOnly, (*string)(nil), (*string)(nil), ""}
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	refs := []AgentSummaryEvidence{}
	for rows.Next() {
		var ref AgentSummaryEvidence
		if err = rows.Scan(&ref.Type, &ref.ID, &ref.RevisionID); err != nil {
			rows.Close()
			return nil, nil, err
		}
		refs = append(refs, ref)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(refs) > 500 {
		return nil, nil, core.Invalid("scope", "范围超过500项来源，请改用单个项目或更小的月份范围")
	}
	if err = ValidateAgentSummary(ctx, tx, a, project, class, members, refs); err != nil {
		return nil, nil, err
	}
	var report any
	switch view {
	case "actual":
		report, err = incurredReportInTx(ctx, tx, a, "incurred", f)
	case "cash":
		report, err = cashReportInTx(ctx, tx, a, PaymentFilter{From: f.From, To: f.To, Currency: currency, ProjectID: project, CompanyOnly: project == nil})
	case "forecast":
		report, err = forecastReportInTx(ctx, tx, a, SubscriptionFilter{From: f.From, To: f.To, Currency: currency, ProjectID: project, CompanyOnly: project == nil})
	}
	if err != nil {
		return nil, nil, err
	}
	// Preserve decimal strings and the exact ordinary report fields.
	raw, err := json.Marshal(report)
	if err != nil {
		return nil, nil, err
	}
	var projected map[string]any
	if err = json.Unmarshal(raw, &projected); err != nil {
		return nil, nil, err
	}
	var captured time.Time
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&captured); err != nil {
		return nil, nil, err
	}
	scope := "company_allocations"
	if project != nil {
		scope = "project_allocations"
	}
	return map[string]any{"month": month, "view": view, "scope": scope, "project_id": project, "currency_filter": currency, "captured_at": captured, "historical_snapshot": true, "untrusted_data": true, "input_count": len(refs), "report": projected,
		"coverage_notice": "仅包含发起人在本次会话范围内获准读取的财务记录；其他成员也须有权读取全部输入。公司范围仅指公司分摊，不等于全部项目总额。预测、发生支出与现金分别计量，原币种不合并；无记录不代表零支出，订阅停用不证明供应商已停止扣费。"}, refs, nil
}
