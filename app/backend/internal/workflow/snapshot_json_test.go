package workflow

import (
	"encoding/json"
	"testing"

	"copypaste.hk/platform/internal/core"
)

func TestLegacySnapshotDigestSurvivesPaymentScheduleUpgrade(t *testing.T) {
	// This is the exact old payload shape, before either extension existed.
	old := []byte(`{"kind":"contract","business_id":"old-business","business_revision_id":"old-revision","business_content_hash":"old-hash","contract_no":"CP-C-legacy","project_id":null,"classification":"internal","content":{"title":"Legacy draft","direction":null,"type":null,"category":"","counterparty_id":null,"counterparty_revision_id":null,"owner_id":null,"currency":null,"amount":null,"tax_amount":null,"start_date":null,"end_date":null,"renewal_terms":"","termination_terms":"","notes":"","draft_file_version_id":null},"counterparty":null,"attachments":[]}`)
	var p Snapshot
	if err := json.Unmarshal(old, &p); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"payload_schema_version": 1, "payload": json.RawMessage(old), "workflow": nil, "reviewer_evidence": []AuthorityEvidence{}})
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := core.CanonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := core.Digest(canonical)
	got, err := revisionHash(p, nil, nil)
	if err != nil || got != want {
		t.Fatal("legacy approval digest changed", got, want, err)
	}
	p.BusinessPayloadSchemaVersion = 2
	got, err = revisionHash(p, nil, nil)
	if err != nil || got == want {
		t.Fatal("new business schema did not change digest", err)
	}
}
