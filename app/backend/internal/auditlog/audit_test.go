package auditlog

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func TestAuditFilterValidation(t *testing.T) {
	valid := Filter{From: "2026-10-01T00:00:00+08:00", To: "2026-11-01T00:00:00+08:00"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Filter){
		func(f *Filter) { f.From = "2026-10-01" }, func(f *Filter) { f.To = f.From }, func(f *Filter) { f.To = "2028-01-01T00:00:00Z" },
		func(f *Filter) { f.ActorID = "other-organization" }, func(f *Filter) { f.Action = "anything' OR 1=1" }, func(f *Filter) { f.Channel = " web" },
	} {
		f := valid
		change(&f)
		if f.Validate() == nil {
			t.Fatalf("invalid filter accepted: %+v", f)
		}
	}
}

func TestAuditCSVIsDeterministicMetadataText(t *testing.T) {
	version := "9007199254740993"
	digest := strings.Repeat("a", 64)
	e := Event{ID: "00000000-0000-4000-8000-000000000001", OccurredAt: time.Date(2026, 10, 1, 8, 0, 0, 123000, time.FixedZone("HKT", 8*3600)), Channel: "web", Action: " \t=HYPERLINK(\"synthetic\")", Result: "succeeded", ResourceType: "project", ResourceVersion: &version, AfterDigest: &digest, RequestID: "synthetic,\"quoted\"\nrequest"}
	b, err := renderCSV([]Event{e})
	if err != nil {
		t.Fatal(err)
	}
	second, err := renderCSV([]Event{e})
	if err != nil || !bytes.Equal(b, second) {
		t.Fatal("CSV is not deterministic")
	}
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte{239, 187, 191}))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(rows[0]) != 15 || rows[1][1] != "2026-10-01T00:00:00.000123Z" || rows[1][9] != version || !strings.HasPrefix(rows[1][5], "'") || !strings.HasPrefix(rows[1][10], "'") {
		t.Fatalf("CSV data contract mismatch: %v", rows)
	}
	for _, column := range rows[0] {
		if column == "reason" || column == "session_id" || column == "payload" {
			t.Fatal("content/secret column exposed")
		}
	}
}

func TestAuditCSVRejectsOversizeWithoutTruncation(t *testing.T) {
	_, err := renderCSV([]Event{{ID: "synthetic", Channel: strings.Repeat("x", MaxExportBytes)}})
	if err == nil {
		t.Fatal("oversize export was silently accepted")
	}
}
