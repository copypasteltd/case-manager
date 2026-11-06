package deployment

import (
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateDeploymentInputs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "review.json")
	if err := os.WriteFile(p, []byte(`{"reviewed":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := privateFile(p, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := privateFile(p, 2); err == nil {
		t.Fatal("oversize input accepted")
	}
	var input StorageInput
	if decode([]byte(`{"reviewed":true,"unexpected_secret":"not accepted"}`), &input) == nil {
		t.Fatal("unknown operator fields accepted")
	}
	if decode([]byte(`{} {}`), &input) == nil {
		t.Fatal("second input document accepted")
	}
	if evidence(EvidenceFile{Path: p, SHA256: core.Digest([]byte("different prerequisite"))}) == nil {
		t.Fatal("external evidence digest mismatch accepted")
	}
}
func TestExternalSkillEvaluation(t *testing.T) {
	v := agent.SkillEvaluation{SetVersion: "synthetic-version", SetSHA256: core.Digest([]byte("whole dataset")), ResultsSHA256: core.Digest([]byte("whole external result file")), Reference: "immutable external assessment reference", Passed: 3}
	if v.Validate(true) != nil {
		t.Fatal("complete reviewed evidence rejected")
	}
	v.Failed = 1
	if v.Validate(true) == nil {
		t.Fatal("failed assessment published")
	}
	if v.Validate(false) != nil {
		t.Fatal("recording actual failure was forbidden")
	}
	v.Failed = 0
	v.SetVersion = ""
	if v.Validate(false) == nil {
		t.Fatal("versionless assessment accepted")
	}
}
