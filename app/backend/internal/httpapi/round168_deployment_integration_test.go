package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/deployment"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRound168InitialStorageProvision(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in := deployment.StorageInput{OrganizationID: f.org, ActorID: f.admin, Reason: "isolated initial metadata, no cloud claim", Primary: "oss", OSSBucket: "fixture-source", R2Bucket: "fixture-replica", LogicalLimitBytes: "2000000000000", Reviewed: true}
	if _, err := deployment.ProvisionStorage(ctx, f.api.DB, in); err == nil {
		t.Fatal("application identity provisioned deployment configuration")
	}
	id, err := deployment.ProvisionStorage(ctx, f.owner, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := deployment.ProvisionStorage(ctx, f.owner, in)
	if err != nil || again != id {
		t.Fatal("exact initial configuration was not idempotent", err)
	}
	changed := in
	changed.Primary = "r2"
	if _, err = deployment.ProvisionStorage(ctx, f.owner, changed); err == nil {
		t.Fatal("initial provision silently switched an existing primary")
	}
	changed = in
	changed.LogicalLimitBytes = "2000000000001"
	if _, err = deployment.ProvisionStorage(ctx, f.owner, changed); err == nil {
		t.Fatal("initial provision silently changed existing quota")
	}
	var n int
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM storage_targets WHERE org_id=$1 AND enabled", f.org).Scan(&n); err != nil || n != 2 {
		t.Fatal("dual targets missing or duplicated", err)
	}
	if _, err = f.owner.Exec(ctx, "UPDATE deployment_configuration_receipts SET reason='rewrite' WHERE id=$1", id); err == nil {
		t.Fatal("deployment receipt changed")
	}
}

// This static producer and prerequisite data are isolated doubles. They prove
// installation transactions and exact binding, never live model/Linux acceptance.
func TestRound168DeploymentProfile(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("declared native Node tool is required")
	}
	node, err = filepath.Abs(node)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := func(name, body string) deployment.EvidenceFile {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return deployment.EvidenceFile{Path: p, SHA256: core.Digest([]byte(body))}
	}
	var inspection deployment.Inspection
	inspection.Profile.ID = core.NewID()
	inspection.Profile.SkillID = "project-briefing"
	inspection.Profile.SkillVersion = "synthetic-168"
	inspection.Profile.SkillSHA256 = strings.Repeat("a", 64)
	inspection.Profile.ConfigurationSHA256 = strings.Repeat("b", 64)
	inspection.Profile.RuntimeVersion = "codex-sdk-0.164.0"
	inspection.SkillState = "published"
	inspection.Ready = true
	inspection.MissingTools = []string{}
	inspection.Limits.MaxToolCalls = 5
	inspection.Limits.TimeoutSeconds = 300
	raw, _ := json.Marshal(inspection)
	script := file("static-inspection.mjs", "process.stdout.write("+string(mustJSON168(string(raw)))+");")
	dataset := file("dataset.json", "synthetic external dataset")
	results := file("results.json", "synthetic external assessment results")
	in := deployment.AgentInput{OrganizationID: f.org, ActorID: f.admin, Reason: "isolated deployment transaction fixture", Reviewed: true, NodePath: node, DescribeScript: script, Inspection: deployment.InspectionInput{SkillID: "project-briefing", ProfileID: inspection.Profile.ID, Model: "synthetic", WorkingDirectory: dir, MCPEndpoint: "http://127.0.0.1:18080/mcp"}, ExpectedSkillSHA256: inspection.Profile.SkillSHA256, ExpectedConfigurationSHA256: inspection.Profile.ConfigurationSHA256, MaxInputTokens: 10000, MaxOutputTokens: 1000, Evaluation: agent.SkillEvaluation{SetVersion: "synthetic-set-168", SetSHA256: dataset.SHA256, ResultsSHA256: results.SHA256, Reference: "isolated external assessment double", Passed: 2}, Dataset: dataset, Results: results, NativeIsolation: file("native.txt", "synthetic external confinement attestation"), ModelContract: file("model.txt", "synthetic external model contract attestation")}
	id, err := deployment.InstallProfile(ctx, f.owner, in, inspection)
	if err != nil {
		t.Fatal(err)
	}
	again, err := deployment.InstallProfile(ctx, f.owner, in, inspection)
	if err != nil || again != id {
		t.Fatal("exact installation did not reconcile", err)
	}
	var n int
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_profile_events WHERE org_id=$1 AND profile_id=$2 AND source='installed'", f.org, inspection.Profile.ID).Scan(&n); err != nil || n != 1 {
		t.Fatal("installation observer missing or duplicated", err)
	}
	wrong := inspection
	wrong.Profile.ConfigurationSHA256 = strings.Repeat("c", 64)
	if _, err = deployment.InstallProfile(ctx, f.owner, in, wrong); err == nil {
		t.Fatal("caller report replaced real inspection")
	}
	in.NativeIsolation.SHA256 = strings.Repeat("d", 64)
	if _, err = deployment.InspectProfile(ctx, in); err == nil {
		t.Fatal("changed prerequisite accepted")
	}
}
func mustJSON168(v any) []byte { raw, _ := json.Marshal(v); return raw }
