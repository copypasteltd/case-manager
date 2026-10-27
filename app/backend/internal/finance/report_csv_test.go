package finance

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
)

func TestReportCSVTextSafetyAndExactDecimals(t *testing.T) {
	ptr := func(s string) *string { return &s }
	snapshot := reportSnapshot{Protocol: 1, Report: ReportPage{Filters: ReportQuery{Kind: "incurred"}, Scope: "authorized_records_only", Summary: json.RawMessage(`{"fx_coverage":"unconfigured","base_currency":null}`), Items: []ReportSource{{Title: " =HYPERLINK(\"https://invalid.example\",\"run\")", Category: "\t@SUM(1)", OriginalAmount: ptr("99999999999999.999999"), ContributionAmount: ptr("-1000.000001"), Currency: ptr("HKD")}}}}
	first, err := reportCSV(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(first, []byte{0xef, 0xbb, 0xbf}))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	last := rows[len(rows)-1]
	if len(last) != 23 || last[9] != "' =HYPERLINK(\"https://invalid.example\",\"run\")" || last[15] != "'\t@SUM(1)" || last[13] != "99999999999999.999999" || last[14] != "-1000.000001" {
		t.Fatalf("unsafe or coerced CSV: %#v", last)
	}
	snapshot.Report.Summary = json.RawMessage(`{ "base_currency" : null, "fx_coverage" : "unconfigured" }`)
	second, err := reportCSV(snapshot)
	if err != nil || !bytes.Equal(first, second) || core.Digest(first) != core.Digest(second) {
		t.Fatal("database JSON formatting changed the fixed download bytes")
	}
	if !strings.Contains(string(first), "\r\n") {
		t.Fatal("CSV row encoding changed")
	}
}
