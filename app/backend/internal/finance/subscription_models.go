package finance

import (
	"time"

	"copypaste.hk/platform/internal/fileref"
)

type SubscriptionContent struct {
	Product                  string                   `json:"product"`
	Supplier                 *PartyRef                `json:"supplier"`
	OwnerID                  *string                  `json:"owner_id"`
	Category                 string                   `json:"category"`
	StartDate                *string                  `json:"start_date"`
	TrialUntil               *string                  `json:"trial_until"`
	Cycle                    *string                  `json:"cycle"`
	Timezone                 string                   `json:"timezone"`
	FirstChargeDate          *string                  `json:"first_charge_date"`
	AnchorDay                *int                     `json:"anchor_day"`
	CustomIntervalDays       *int                     `json:"custom_interval_days"`
	RenewalMode              string                   `json:"renewal_mode"`
	PaymentMethodLabel       string                   `json:"payment_method_label"`
	ReminderDays             []int                    `json:"reminder_days"`
	VendorState              string                   `json:"vendor_state"`
	VendorCheckedAt          *string                  `json:"vendor_checked_at"`
	VendorEvidenceVersionIDs []string                 `json:"vendor_evidence_version_ids"`
	Prices                   []SubscriptionPriceInput `json:"prices"`
	Notes                    string                   `json:"notes"`
}

type SubscriptionCreate struct {
	SubscriptionContent
	SourceKey      string  `json:"source_key"`
	ProjectID      *string `json:"project_id"`
	Classification string  `json:"classification"`
	Reason         string  `json:"reason"`
}

type SubscriptionPatch struct {
	SubscriptionContent
	Base
}

type SubscriptionPriceChange struct {
	Base
	SubscriptionPriceInput
}

type SubscriptionTransition struct {
	Base
	Action                   string   `json:"action"`
	EffectiveOn              *string  `json:"effective_on"`
	VendorState              string   `json:"vendor_state"`
	VendorCheckedAt          *string  `json:"vendor_checked_at"`
	VendorEvidenceVersionIDs []string `json:"vendor_evidence_version_ids"`
}

type SubscriptionGenerate struct {
	Base
	From string `json:"from"`
	To   string `json:"to"`
}

type SubscriptionUnexpectedPeriod struct {
	Base
	StartDate string `json:"start_date"`
}

type Subscription struct {
	SubscriptionContent
	ID                string             `json:"id"`
	Version           int64              `json:"version"`
	SourceKey         string             `json:"source_key"`
	ProjectID         *string            `json:"project_id"`
	CreatorID         string             `json:"creator_id"`
	Classification    string             `json:"classification"`
	State             string             `json:"state"`
	CurrentRevisionID string             `json:"current_revision_id"`
	ContentHash       string             `json:"content_hash"`
	CancelEffectiveOn *string            `json:"cancel_effective_on"`
	PausedOn          *string            `json:"paused_on"`
	NextChargeDate    *string            `json:"next_charge_date"`
	Signals           []string           `json:"signals"`
	SupplierEvidence  *PartyEvidence     `json:"supplier_evidence"`
	Attachments       []fileref.Evidence `json:"attachments"`
	OwnerDisplayName  *string            `json:"owner_display_name"`
	OwnerAvailable    bool               `json:"owner_available"`
	CanEdit           bool               `json:"can_edit"`
	CanManage         bool               `json:"can_manage"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

type SubscriptionRevision struct {
	SubscriptionContent
	ID               string             `json:"id"`
	Seq              int64              `json:"seq"`
	BaseRevisionID   *string            `json:"base_revision_id"`
	ContentHash      string             `json:"content_hash"`
	AuthorID         string             `json:"author_id"`
	Reason           string             `json:"reason"`
	CreatedAt        time.Time          `json:"created_at"`
	SupplierEvidence *PartyEvidence     `json:"supplier_evidence"`
	Attachments      []fileref.Evidence `json:"attachments"`
	OwnerDisplayName *string            `json:"owner_display_name"`
	OwnerAvailable   bool               `json:"owner_available"`
}

type SubscriptionPrice struct {
	SubscriptionPriceInput
	ID         string `json:"id"`
	RevisionID string `json:"revision_id"`
	Position   int    `json:"position"`
}

type SubscriptionEvent struct {
	ID          string    `json:"id"`
	Version     int64     `json:"version"`
	RevisionID  string    `json:"revision_id"`
	Kind        string    `json:"kind"`
	State       string    `json:"state"`
	PeriodID    *string   `json:"period_id"`
	EffectiveOn *string   `json:"effective_on"`
	ActorID     *string   `json:"actor_id"`
	JobID       *string   `json:"job_id"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
}

type SubscriptionOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type SubscriptionPeriod struct {
	ID                      string    `json:"id"`
	SubscriptionID          string    `json:"subscription_id"`
	Product                 string    `json:"product"`
	Version                 int64     `json:"version"`
	CurrentRevisionID       string    `json:"current_revision_id"`
	ContentHash             string    `json:"content_hash"`
	StartDate               string    `json:"start_date"`
	EndDate                 string    `json:"end_date"`
	ChargeType              string    `json:"charge_type"`
	ConfigurationRevisionID string    `json:"configuration_revision_id"`
	PriceID                 string    `json:"price_id"`
	Currency                string    `json:"currency"`
	Amount                  string    `json:"amount"`
	Timezone                string    `json:"timezone"`
	Category                string    `json:"category"`
	OwnerID                 string    `json:"owner_id"`
	Supplier                PartyRef  `json:"supplier"`
	State                   string    `json:"state"`
	RecordedState           string    `json:"recorded_state"`
	ActualAmount            *string   `json:"actual_amount"`
	ActualFingerprint       string    `json:"actual_fingerprint"`
	CostIDs                 []string  `json:"cost_ids"`
	Anomalies               []string  `json:"anomalies"`
	ReviewReason            *string   `json:"review_reason"`
	ReviewCurrent           bool      `json:"review_current"`
	CanManage               bool      `json:"can_manage"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type SubscriptionPeriodBase struct {
	ExpectedVersion int64  `json:"expected_version"`
	RevisionID      string `json:"revision_id"`
	ContentHash     string `json:"content_hash"`
	Reason          string `json:"reason"`
}

// A durable queue entry is not a notification delivery receipt.
type SubscriptionReminder struct {
	ID                   string    `json:"id"`
	PeriodID             string    `json:"period_id"`
	DaysBefore           int       `json:"days_before"`
	RemindOn             string    `json:"remind_on"`
	RecipientID          string    `json:"recipient_id"`
	RecipientDisplayName string    `json:"recipient_display_name"`
	PeriodStartDate      string    `json:"period_start_date"`
	Timezone             string    `json:"timezone"`
	DeliveryState        string    `json:"delivery_state"`
	CreatedAt            time.Time `json:"created_at"`
}

type SubscriptionPeriodRevision struct {
	ID                      string    `json:"id"`
	Seq                     int64     `json:"seq"`
	BaseRevisionID          *string   `json:"base_revision_id"`
	ContentHash             string    `json:"content_hash"`
	ConfigurationRevisionID string    `json:"configuration_revision_id"`
	PriceID                 string    `json:"price_id"`
	Currency                string    `json:"currency"`
	Amount                  string    `json:"amount"`
	Timezone                string    `json:"timezone"`
	Category                string    `json:"category"`
	OwnerID                 string    `json:"owner_id"`
	Supplier                PartyRef  `json:"supplier"`
	Reason                  string    `json:"reason"`
	CreatedAt               time.Time `json:"created_at"`
}

type SubscriptionPeriodEvent struct {
	ID                    string                     `json:"id"`
	Version               int64                      `json:"version"`
	RevisionID            string                     `json:"revision_id"`
	Kind                  string                     `json:"kind"`
	State                 string                     `json:"state"`
	CostLinkID            *string                    `json:"cost_link_id"`
	ReviewFingerprint     *string                    `json:"review_fingerprint"`
	AcknowledgeDifference bool                       `json:"acknowledge_difference"`
	InvoiceMissing        bool                       `json:"invoice_missing"`
	ActorID               *string                    `json:"actor_id"`
	JobID                 *string                    `json:"job_id"`
	EvidenceVersionIDs    []string                   `json:"evidence_version_ids"`
	Attachments           []fileref.Evidence         `json:"attachments"`
	CostSnapshot          []SubscriptionCostSnapshot `json:"cost_snapshot"`
	Reason                string                     `json:"reason"`
	CreatedAt             time.Time                  `json:"created_at"`
}

type SubscriptionCostSnapshot struct {
	CostID      string `json:"cost_id"`
	RevisionID  string `json:"revision_id"`
	State       string `json:"state"`
	ContentHash string `json:"content_hash"`
	Currency    string `json:"currency"`
	Amount      string `json:"amount"`
}

type SubscriptionPeriodReforecast struct {
	SubscriptionPeriodBase
	ConfigurationRevisionID string `json:"configuration_revision_id"`
	ConfigurationHash       string `json:"configuration_hash"`
}

type SubscriptionPeriodLink struct {
	SubscriptionPeriodBase
	CostID          string `json:"cost_id"`
	CostVersion     int64  `json:"cost_version"`
	CostRevisionID  string `json:"cost_revision_id"`
	CostContentHash string `json:"cost_content_hash"`
}

type SubscriptionPeriodDecision struct {
	SubscriptionPeriodBase
	ExpectedActualFingerprint *string  `json:"expected_actual_fingerprint"`
	Action                    string   `json:"action"`
	AcknowledgeDifference     bool     `json:"acknowledge_difference"`
	InvoiceMissing            bool     `json:"invoice_missing"`
	EvidenceVersionIDs        []string `json:"evidence_version_ids"`
}

type SubscriptionGeneration struct {
	SubscriptionID    string   `json:"subscription_id"`
	Version           int64    `json:"version"`
	From              string   `json:"from"`
	To                string   `json:"to"`
	GeneratedIDs      []string `json:"generated_ids"`
	ExistingCount     int      `json:"existing_count"`
	MissingPriceDates []string `json:"missing_price_dates"`
	CoverageComplete  bool     `json:"coverage_complete"`
}

type SubscriptionFilter struct {
	Query, State, Currency, Category, From, To, Cursor string
	ProjectID, SupplierID, OwnerID                     *string
	CompanyOnly, ForecastOnly                          bool
	Limit                                              int
}

type ForecastTotal struct {
	Currency       string `json:"currency"`
	ExpectedAmount string `json:"expected_amount"`
	PeriodCount    int64  `json:"period_count"`
	AwaitingCount  int64  `json:"awaiting_count"`
	ReviewedCount  int64  `json:"reviewed_count"`
	AnomalyCount   int64  `json:"anomaly_count"`
}

type ForecastReport struct {
	Kind                        string          `json:"kind"`
	From                        string          `json:"from"`
	To                          string          `json:"to"`
	CalendarBasis               string          `json:"calendar_basis"`
	Timezones                   []string        `json:"timezones"`
	AsOf                        time.Time       `json:"as_of"`
	BaseCurrency                *string         `json:"base_currency"`
	FXCoverage                  string          `json:"fx_coverage"`
	Totals                      []ForecastTotal `json:"totals"`
	CoverageComplete            bool            `json:"coverage_complete"`
	MissingPeriodCount          int64           `json:"missing_period_count"`
	MissingPriceCount           int64           `json:"missing_price_count"`
	DraftCount                  int64           `json:"draft_count"`
	CoverageLimitReached        bool            `json:"coverage_limit_reached"`
	IncompleteSubscriptionCount int64           `json:"incomplete_subscription_count"`
}
