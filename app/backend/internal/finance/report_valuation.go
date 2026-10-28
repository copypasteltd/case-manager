package finance

import (
	"context"
	"encoding/json"
	"math/big"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// Valuation is a reporting view, never a payment allocation or ledger rewrite.
// A quote is base units per one source unit. Intervals are [from,to).
type SourceValuation struct {
	Status     string  `json:"status"`
	FXDate     string  `json:"fx_date"`
	BaseAmount *string `json:"base_amount"`
	Rate       *string `json:"rate"`
	RateIndex  *int    `json:"rate_index"`
	RateDate   *string `json:"rate_date"`
	RateSource *string `json:"rate_source"`
	ValidFrom  *string `json:"valid_from"`
	ValidTo    *string `json:"valid_to"`
}
type ValuationCurrency struct {
	Currency            *string `json:"currency"`
	CoveredCount        int64   `json:"covered_count"`
	MissingRateCount    int64   `json:"missing_rate_count"`
	UnpricedCount       int64   `json:"unpriced_count"`
	UnconfirmedCount    int64   `json:"unconfirmed_count"`
	CoveredAmount       string  `json:"covered_amount"`
	MissingNativeAmount string  `json:"missing_native_amount"`
}
type ReportValuation struct {
	SourceCoverageComplete bool                `json:"source_coverage_complete"`
	Book                   FXBook              `json:"book"`
	Exponent               int                 `json:"exponent"`
	Method                 string              `json:"method"`
	Rounding               string              `json:"rounding"`
	Coverage               string              `json:"coverage"`
	CoveredAmount          string              `json:"covered_amount"`
	TotalAmount            *string             `json:"total_amount"`
	CoveredCount           int64               `json:"covered_count"`
	MissingRateCount       int64               `json:"missing_rate_count"`
	UnpricedCount          int64               `json:"unpriced_count"`
	UnconfirmedCount       int64               `json:"unconfirmed_count"`
	Currencies             []ValuationCurrency `json:"currencies"`
}

// Exact decimal multiplication and symmetric half-up rounding, matching
// PostgreSQL numeric round (ties away from zero). No float conversion occurs.
func convertFXAmount(amount, rate string, exponent int) (string, error) {
	a, ok := new(big.Rat).SetString(amount)
	if !ok {
		return "", core.Invalid("amount", "折算金额无效")
	}
	r, ok := new(big.Rat).SetString(rate)
	if !ok || r.Sign() <= 0 || exponent < 0 || exponent > 6 {
		return "", core.Invalid("rate", "折算精度或汇率无效")
	}
	a.Mul(a, r)
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
	numerator := new(big.Int).Mul(a.Num(), scale)
	negative := numerator.Sign() < 0
	numerator.Abs(numerator)
	units, rest := new(big.Int), new(big.Int)
	units.QuoRem(numerator, a.Denom(), rest)
	if new(big.Int).Lsh(rest, 1).Cmp(a.Denom()) >= 0 {
		units.Add(units, big.NewInt(1))
	}
	if negative {
		units.Neg(units)
	}
	return new(big.Rat).SetFrac(units, scale).FloatString(6), nil
}
func valueSource(source ReportSource, book FXContent, exponent int, day string) (SourceValuation, error) {
	out := SourceValuation{FXDate: day}
	if source.State == "draft" {
		out.Status = "unconfirmed"
		return out, nil
	}
	if source.Currency == nil || source.ContributionAmount == nil {
		out.Status = "unpriced"
		return out, nil
	}
	rate := "1"
	if *source.Currency == book.BaseCurrency {
		out.Status = "identity"
	} else {
		out.Status = "missing_rate"
		for i := range book.Rates {
			r := &book.Rates[i]
			if r.Currency == *source.Currency && r.ValidFrom <= day && day < r.ValidTo {
				out.Status = "converted"
				rate = r.Rate
				out.RateIndex = &i
				out.RateDate = &r.RateDate
				out.RateSource = &r.Source
				out.ValidFrom = &r.ValidFrom
				out.ValidTo = &r.ValidTo
				break
			}
		}
		if out.Status == "missing_rate" {
			return out, nil
		}
	}
	out.Rate = &rate
	amount, err := convertFXAmount(*source.ContributionAmount, rate, exponent)
	if err != nil {
		return out, err
	}
	out.BaseAmount = &amount
	return out, nil
}

const valuationFacts = `facts(source_type,id,resource_id,revision_id,version,title,period_start,period_end,state,entry_kind,currency,original_amount,contribution_amount,category,supplier_id,owner_id,project_id,created_at,timezone)`

// Reversals reuse their original fact's quote date. Actual refunds use their
// own paid date; forecast periods use the start date fixed in their revision.
const valuationDate = `CASE WHEN f.entry_kind='reversal' AND f.source_type='cost' THEN
 (SELECT r.occurred_on FROM actual_cost_entries c JOIN actual_cost_entries o ON o.org_id=c.org_id AND o.id=c.reverses_id JOIN cost_revisions r ON r.org_id=o.org_id AND r.id=o.current_revision_id WHERE c.org_id=$1 AND c.id=f.id)
 WHEN f.entry_kind='reversal' AND f.source_type='payment' THEN
 (SELECT r.paid_on FROM payment_records p JOIN payment_records o ON o.org_id=p.org_id AND o.id=p.reverses_id JOIN payment_revisions r ON r.org_id=o.org_id AND r.id=o.current_revision_id WHERE p.org_id=$1 AND p.id=f.id)
 ELSE f.period_start::date END`

func applyReportValuation(ctx context.Context, tx pgx.Tx, a core.Actor, out *ReportPage) error {
	f := out.Filters
	if f.FXBookID == nil {
		return nil
	}
	book, err := readFXBook(ctx, tx, a.OrgID, *f.FXBookID, true)
	if err != nil {
		return err
	}
	var exponent int
	if err = tx.QueryRow(ctx, "SELECT exponent FROM currency_definitions WHERE code=$1", book.BaseCurrency).Scan(&exponent); err != nil {
		return err
	}
	raw, err := json.Marshal(book.Content)
	if err != nil {
		return err
	}
	base, _, err := reportSourceStatement(f)
	if err != nil {
		return err
	}
	// Aggregate the same authorized facts as the native-currency report inside
	// its repeatable-read transaction. No unbounded row transfer is required.
	query := `WITH ` + valuationFacts + ` AS (` + base + `), dated AS (SELECT f.*,` + valuationDate + ` fx_date FROM facts f), valued AS (
 SELECT f.*,q.rate,
 CASE WHEN f.state='draft' THEN 'unconfirmed'
 WHEN f.currency IS NULL OR f.contribution_amount IS NULL THEN 'unpriced'
 WHEN f.currency=$15 THEN 'identity' WHEN q.rate IS NULL THEN 'missing_rate' ELSE 'converted' END status,
 CASE WHEN f.state<>'draft' AND f.contribution_amount IS NOT NULL AND (f.currency=$15 OR q.rate IS NOT NULL)
 THEN round(f.contribution_amount::numeric*CASE WHEN f.currency=$15 THEN 1 ELSE q.rate END,$16::int) END base_amount
 FROM dated f LEFT JOIN LATERAL (SELECT (v->>'rate')::numeric rate FROM jsonb_array_elements($14::jsonb->'rates') v
 WHERE v->>'currency'=f.currency AND (v->>'valid_from')::date<=f.fx_date AND f.fx_date<(v->>'valid_to')::date) q ON true)
 SELECT currency,count(*) FILTER(WHERE status IN ('identity','converted')),count(*) FILTER(WHERE status='missing_rate'),
 count(*) FILTER(WHERE status='unpriced'),count(*) FILTER(WHERE status='unconfirmed'),coalesce(sum(base_amount),0)::text,
 coalesce(sum(contribution_amount::numeric) FILTER(WHERE status='missing_rate'),0)::text
 FROM valued GROUP BY currency ORDER BY currency NULLS LAST`
	args := append(reportArguments(a, f), raw, book.BaseCurrency, exponent)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	result := &ReportValuation{Book: book, Exponent: exponent, Method: book.Content.Method, Rounding: book.Content.Rounding, Currencies: []ValuationCurrency{}}
	result.Book.Content = nil
	result.SourceCoverageComplete = true
	if f.Kind == "forecast" {
		var forecast ForecastReport
		if err = json.Unmarshal(out.Summary, &forecast); err != nil {
			return err
		}
		result.SourceCoverageComplete = forecast.CoverageComplete
	}
	total := new(big.Rat)
	for rows.Next() {
		var v ValuationCurrency
		if err = rows.Scan(&v.Currency, &v.CoveredCount, &v.MissingRateCount, &v.UnpricedCount, &v.UnconfirmedCount, &v.CoveredAmount, &v.MissingNativeAmount); err != nil {
			rows.Close()
			return err
		}
		n, ok := new(big.Rat).SetString(v.CoveredAmount)
		if !ok {
			rows.Close()
			return core.Fail(500, "FX_TOTAL_INVALID", "折算汇总无效")
		}
		total.Add(total, n)
		v.CoveredAmount = n.FloatString(6)
		missing, ok := new(big.Rat).SetString(v.MissingNativeAmount)
		if !ok {
			rows.Close()
			return core.Fail(500, "FX_TOTAL_INVALID", "原币缺口无效")
		}
		v.MissingNativeAmount = missing.FloatString(6)
		result.CoveredCount += v.CoveredCount
		result.MissingRateCount += v.MissingRateCount
		result.UnpricedCount += v.UnpricedCount
		result.UnconfirmedCount += v.UnconfirmedCount
		result.Currencies = append(result.Currencies, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	result.CoveredAmount = total.FloatString(6)
	switch {
	case result.MissingRateCount+result.UnpricedCount == 0 && result.SourceCoverageComplete:
		result.Coverage = "complete"
		result.TotalAmount = &result.CoveredAmount
	case result.CoveredCount == 0:
		result.Coverage = "none"
	default:
		result.Coverage = "partial"
	}
	// A single bounded lookup supplies original dates for reversals on this page.
	reversed := []string{}
	for _, v := range out.Items {
		if v.EntryKind == "reversal" {
			reversed = append(reversed, v.ID)
		}
	}
	dates := map[string]string{}
	if len(reversed) > 0 {
		q := `SELECT c.id,r.occurred_on::text FROM actual_cost_entries c JOIN actual_cost_entries o ON o.org_id=c.org_id AND o.id=c.reverses_id JOIN cost_revisions r ON r.org_id=o.org_id AND r.id=o.current_revision_id WHERE c.org_id=$1 AND c.id=ANY($2::uuid[])
  UNION ALL SELECT p.id,r.paid_on::text FROM payment_records p JOIN payment_records o ON o.org_id=p.org_id AND o.id=p.reverses_id JOIN payment_revisions r ON r.org_id=o.org_id AND r.id=o.current_revision_id WHERE p.org_id=$1 AND p.id=ANY($2::uuid[])`
		r, e := tx.Query(ctx, q, a.OrgID, reversed)
		if e != nil {
			return e
		}
		for r.Next() {
			var id, day string
			if e = r.Scan(&id, &day); e != nil {
				r.Close()
				return e
			}
			dates[id] = day
		}
		e = r.Err()
		r.Close()
		if e != nil {
			return e
		}
	}
	for i := range out.Items {
		v := &out.Items[i]
		day := v.From
		if v.EntryKind == "reversal" {
			day = dates[v.ID]
			if day == "" {
				return core.Fail(500, "FX_REVERSAL_DATE_MISSING", "冲销原始日期不可用")
			}
		}
		value, e := valueSource(*v, *book.Content, exponent, day)
		if e != nil {
			return e
		}
		v.Valuation = &value
	}
	out.Valuation = result
	return nil
}
