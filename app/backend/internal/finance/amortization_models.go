package finance

import "time"

type AmortizationInput struct {
	Base
	ExpectedPlanVersion int64  `json:"expected_plan_version"`
	Method              string `json:"method"`
	ServiceFrom         string `json:"service_from"`
	ServiceTo           string `json:"service_to"`
}
type AmortizationPublish struct {
	AmortizationInput
	PreviewSHA256 string `json:"preview_sha256"`
	Acknowledged  bool   `json:"acknowledged"`
}
type AmortizationStop struct {
	Base
	ExpectedPlanVersion int64 `json:"expected_plan_version"`
	Acknowledged        bool  `json:"acknowledged"`
}
type AmortizationPeriod struct {
	Sequence     int          `json:"sequence"`
	ServiceFrom  string       `json:"service_from"`
	ServiceTo    string       `json:"service_to"`
	RecognizedOn string       `json:"recognized_on"`
	Amount       string       `json:"amount"`
	Shares       []Allocation `json:"shares"`
}
type AmortizationContent struct {
	CostID         string               `json:"cost_id"`
	CostVersion    int64                `json:"cost_version"`
	CostRevisionID string               `json:"cost_revision_id"`
	CostSHA256     string               `json:"cost_sha256"`
	Currency       string               `json:"currency"`
	Exponent       int                  `json:"exponent"`
	Amount         string               `json:"amount"`
	Method         string               `json:"method"`
	Rounding       string               `json:"rounding"`
	ServiceFrom    string               `json:"service_from"`
	ServiceTo      string               `json:"service_to"`
	Periods        []AmortizationPeriod `json:"periods"`
}
type AmortizationPreview struct {
	PlanVersion   int64               `json:"plan_version"`
	ContentSHA256 string              `json:"content_sha256"`
	Content       AmortizationContent `json:"content"`
}
type AmortizationVersion struct {
	ID            string               `json:"id"`
	CostID        string               `json:"cost_id"`
	Version       int64                `json:"version"`
	State         string               `json:"state"`
	ContentSHA256 string               `json:"content_sha256"`
	PeriodCount   int                  `json:"period_count"`
	ActorID       string               `json:"actor_id"`
	Reason        string               `json:"reason"`
	CreatedAt     time.Time            `json:"created_at"`
	Content       *AmortizationContent `json:"content,omitempty"`
}
type AmortizationOverview struct {
	CostID    string               `json:"cost_id"`
	Version   int64                `json:"version"`
	CanManage bool                 `json:"can_manage"`
	Current   *AmortizationVersion `json:"current"`
}

// This evidence is present only on the independent amortization view. The
// source revision identifies the original expense, not a new incurred cost.
type AmortizationEvidence struct {
	PlanID       string `json:"plan_id"`
	PlanVersion  int64  `json:"plan_version"`
	PlanSHA256   string `json:"plan_sha256"`
	Sequence     int    `json:"sequence"`
	Method       string `json:"method"`
	ServiceFrom  string `json:"service_from"`
	ServiceTo    string `json:"service_to"`
	OriginalDate string `json:"original_date"`
}
