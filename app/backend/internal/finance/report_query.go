package finance

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// Dates are explicit business dates; To is exclusive. Owner means the original
// recorder for costs/cash and the fixed period owner for subscription forecasts.
type ReportQuery struct {
	Kind        string  `json:"kind"`
	From        string  `json:"from"`
	To          string  `json:"to"`
	Currency    string  `json:"currency"`
	Category    string  `json:"category"`
	Query       string  `json:"q"`
	ProjectID   *string `json:"project_id"`
	CompanyOnly bool    `json:"company_only"`
	SupplierID  *string `json:"supplier_id"`
	OwnerID     *string `json:"owner_id"`
	FXBookID    *string `json:"fx_book_id,omitempty"`
}

type ReportSource struct {
	SourceType         string                `json:"source_type"`
	ID                 string                `json:"id"`
	ResourceID         string                `json:"resource_id"`
	RevisionID         string                `json:"revision_id"`
	Version            int64                 `json:"version"`
	Title              string                `json:"title"`
	From               string                `json:"from"`
	To                 *string               `json:"to"`
	State              string                `json:"state"`
	EntryKind          string                `json:"entry_kind"`
	Currency           *string               `json:"currency"`
	OriginalAmount     *string               `json:"original_amount"`
	ContributionAmount *string               `json:"contribution_amount"`
	Category           string                `json:"category"`
	SupplierID         *string               `json:"supplier_id"`
	OwnerID            string                `json:"owner_id"`
	ProjectID          *string               `json:"project_id"`
	CreatedAt          time.Time             `json:"created_at"`
	Timezone           string                `json:"timezone"`
	Valuation          *SourceValuation      `json:"valuation,omitempty"`
	Amortization       *AmortizationEvidence `json:"amortization,omitempty"`
}

type ReportPage struct {
	Filters    ReportQuery      `json:"filters"`
	Scope      string           `json:"scope"`
	Summary    json.RawMessage  `json:"summary"`
	Items      []ReportSource   `json:"items"`
	NextCursor *string          `json:"next_cursor"`
	Valuation  *ReportValuation `json:"valuation,omitempty"`
}

func (f ReportQuery) Validate() error {
	start, err := time.Parse(time.DateOnly, f.From)
	end, last := time.Parse(time.DateOnly, f.To)
	if err != nil || last != nil || !end.After(start) || end.Sub(start) > 366*24*time.Hour || !slices.Contains([]string{"incurred", "cash", "forecast", "amortization"}, f.Kind) {
		return core.Invalid("report", "明确选择预测／发生／现金／摊销及1至366个自然日的期间，结束日不包含在内")
	}
	if (f.Currency != "" && !currencyPatternForFilter(f.Currency)) || len([]rune(f.Category)) > 50 || len([]rune(f.Query)) > 200 || strings.ContainsAny(f.Query+f.Category, "\x00") || f.CompanyOnly && f.ProjectID != nil {
		return core.Invalid("filter", "币种、类别、关键词或公司／项目范围无效")
	}
	for _, id := range []*string{f.ProjectID, f.SupplierID, f.OwnerID, f.FXBookID} {
		if id != nil && !core.ValidID(*id) {
			return core.Invalid("filter", "项目、供应方或经办／负责人标识无效")
		}
	}
	return nil
}

// Organization/user locks serialize every permission mutation with the read.
// A repeatable-read transaction keeps totals and sources in the same snapshot.
func reportAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, f ReportQuery, export bool) error {
	if err := reportIdentity(ctx, tx, a); err != nil {
		return err
	}
	r := authz.Resource{Type: "report", ProjectID: f.ProjectID, OwnerID: &a.UserID, Classification: "internal"}
	if err := authz.Require(ctx, tx, a, "report.read", r); err != nil {
		return err
	}
	if export {
		return authz.Require(ctx, tx, a, "report.export", r)
	}
	return nil
}

func reportSummary(ctx context.Context, tx pgx.Tx, a core.Actor, f ReportQuery) (json.RawMessage, error) {
	var v any
	var err error
	switch f.Kind {
	case "amortization":
		v, err = amortizationReport(ctx, tx, a, f)
	case "incurred":
		v, err = incurredFilteredReportInTx(ctx, tx, a, f.Kind, Filter{Query: f.Query, From: f.From, To: f.To, Currency: f.Currency, Category: f.Category, ProjectID: f.ProjectID, CompanyOnly: f.CompanyOnly}, f.SupplierID, f.OwnerID)
	case "cash":
		v, err = cashFilteredReportInTx(ctx, tx, a, PaymentFilter{Query: f.Query, From: f.From, To: f.To, Currency: f.Currency, Category: f.Category, ProjectID: f.ProjectID, CompanyOnly: f.CompanyOnly}, f.SupplierID, f.OwnerID)
	case "forecast":
		v, err = forecastReportInTx(ctx, tx, a, SubscriptionFilter{Query: f.Query, From: f.From, To: f.To, Currency: f.Currency, Category: f.Category, ProjectID: f.ProjectID, CompanyOnly: f.CompanyOnly, SupplierID: f.SupplierID, OwnerID: f.OwnerID})
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func reportIdentity(ctx context.Context, tx pgx.Tx, a core.Actor) error {
	var orgVersion, userVersion int64
	var state string
	if err := tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&orgVersion); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&userVersion, &state); err != nil {
		return err
	}
	if state != "active" || orgVersion != a.OrgAuthVersion || userVersion != a.AuthVersion {
		return core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新获取授权并生成新的报表")
	}
	return nil
}
