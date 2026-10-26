package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	status "copypaste.hk/platform/internal/servicestatus"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Real HTTP permissions, signed observations, controller leases and durable
// receipts; no provider, bot, cloud store, container or virtual machine.
func TestAgentServiceSummary(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := &agent.Service{DB: f.api.DB}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	p := f.createProject("Service summary shared project", []string{f.bob})
	hidden := f.createProject("Service summary private project", nil)
	exec("UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='agent_user'", f.org)
	for _, b := range [][2]string{{f.alice, "status_operator"}, {f.bob, "status_reader"}, {f.alice, "agent_user"}, {f.bob, "agent_user"}} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, b[0], b[1])
	}
	for _, class := range []string{"internal", "project_restricted"} {
		exec(`INSERT INTO classification_policies(id,org_id,level,ai_allowed,allowed_channels,policy_version) SELECT $1,$2,$3,true,ARRAY['web','wechat','douyin','android','telegram'],max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level=$3`, core.NewID(), f.org, class)
	}
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = memberMFAToken(f, "bob", "android")
	create := func(project, name string) status.Record {
		r, v := f.request("POST", "/v1/services", f.aliceToken, core.NewID(), status.Create{Content: status.Content{Name: name, OwnerID: f.alice, Description: "Actual bounded service evidence"}, ProjectID: &project, Classification: "project_restricted", Reason: "controlled native summary fixture"}, nil)
		expect(t, r, 201, v)
		return data[status.Record](t, v)
	}
	d := create(p.ID, "Summary business API")
	private := create(hidden.ID, "Private service")
	public, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	r, v := f.request("POST", "/v1/services/"+d.ID+"/sources", f.aliceToken, core.NewID(), status.SourceInput{ExpectedVersion: d.Version, Reason: "native signed evidence", SourceContent: status.SourceContent{Name: "Real fixture observation source", Provider: "adapter", ExternalRef: "DO_NOT_DISCLOSE_EXTERNAL_REFERENCE", PublicKey: base64.StdEncoding.EncodeToString(public), Enabled: true, Thresholds: status.Thresholds{FailureCount: 1, RecoveryCount: 1, Interval: 60, StaleAfter: 180, LatencyMS: 1000}}}, nil)
	expect(t, r, 200, v)
	d = data[status.Record](t, v)
	source := d.Sources[0]
	push := func(seq string, healthy bool) {
		t.Helper()
		latency := int64(50)
		raw, _ := json.Marshal(status.SignedObservation{RevisionID: source.RevisionID, Sequence: seq, ObservedAt: time.Now().UTC().Add(-time.Second), Healthy: healthy, LatencyMS: &latency})
		req := httptest.NewRequest("POST", "/v1/status-sources/"+source.ID+"/observations", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CP-Status-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(key, status.SigningMessage(source.ID, raw))))
		out := httptest.NewRecorder()
		f.handler.ServeHTTP(out, req)
		if out.Code != 200 {
			t.Fatalf("signed ingestion: %d %s", out.Code, out.Body.String())
		}
	}
	push("1", false)
	for _, i := range []struct {
		title string
		ids   []string
	}{{"Visible actual incident", []string{d.ID}}, {"DO_NOT_DISCLOSE_PRIVATE_INCIDENT", []string{d.ID, private.ID}}} {
		r, v = f.request("POST", "/v1/incidents", f.aliceToken, core.NewID(), status.IncidentCreate{Title: i.title, Severity: "high", Description: "Verified fixture incident", TimelineNote: "Native evidence starts here", OwnerID: f.alice, ServiceIDs: i.ids, DedupKey: core.NewID(), Reason: "native incident evidence"}, nil)
		expect(t, r, 201, v)
	}
	r, v = f.request("POST", "/v1/services/"+d.ID+"/maintenance-windows", f.aliceToken, core.NewID(), status.MaintenanceInput{ExpectedVersion: d.Version, Start: time.Now().Add(-time.Minute), End: time.Now().Add(time.Hour), Reason: "Controlled active maintenance"}, nil)
	expect(t, r, 200, v)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'incident-summary','controlled-test',repeat('a',64),$3,repeat('b',64),'active',10000,1000,20,600,$4,'Native service summary fixture')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	r, v = f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "project", ProjectID: &p.ID, Title: "Service evidence", MemberIDs: []string{f.bob}}, nil)
	expect(t, r, 201, v)
	c := data[agent.Conversation](t, v)
	r, v = f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/service/"+d.ID+"?context_security_version=1&max_chars=4000", f.bobToken, "", nil, nil)
	expect(t, r, 200, v)
	r, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Summarize the actual service evidence", ContextRefs: []agent.SourceReference{{Type: "service", ID: d.ID}}}, nil)
	expect(t, r, 202, v)
	run := data[agent.Run](t, v)
	job, e := svc.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "service-summary.test", agent.ExecutionLeaseDuration)
	if e != nil || job == nil {
		t.Fatal("claim", e)
	}
	execution, e := svc.IssueExecution(ctx, *job)
	if e != nil {
		t.Fatal(e)
	}
	if execution.RunID != run.ID || !strings.Contains(execution.PreparedContext, "server_time") {
		t.Fatal("wrong prepared execution")
	}
	lease, e := svc.LeaseForCredential(ctx, execution.Credential)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.RecordVerifiedConfinement(ctx, lease, execution.Profile.ConfigurationSHA256, strings.Repeat("c", 64), time.Now().Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	if e = svc.StartExecution(ctx, lease); e != nil {
		t.Fatal(e)
	}
	call := func(name string, args any) (json.RawMessage, error) {
		raw, _ := json.Marshal(args)
		return svc.ExecutionTool(ctx, lease, core.NewID(), name, raw)
	}
	t.Run("service search enforces common scope", func(t *testing.T) {
		out, e := call("search_resources", map[string]any{"query": "service"})
		if e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(out), private.ID) {
			t.Fatal("private service leaked")
		}
	})
	t.Run("outside conversation scope is denied", func(t *testing.T) {
		if _, e := call("read_service_summary", map[string]any{"service_id": private.ID}); e == nil {
			t.Fatal("cross-project read succeeded")
		}
	})
	t.Run("time bounds and write tools are denied", func(t *testing.T) {
		for _, args := range []any{map[string]any{"service_id": d.ID, "from": time.Now().Add(-time.Hour)}, map[string]any{"service_id": d.ID, "from": time.Now().Add(-32 * 24 * time.Hour), "to": time.Now().Add(-time.Minute)}, map[string]any{"service_id": d.ID, "from": time.Now(), "to": time.Now().Add(time.Hour)}} {
			if _, e := call("read_service_summary", args); e == nil {
				t.Fatal("unbounded window")
			}
		}
		if _, e := call("update_project", map[string]any{}); e == nil {
			t.Fatal("incident skill wrote business content")
		}
	})
	args, _ := json.Marshal(map[string]any{"service_id": d.ID})
	invocation := core.NewID()
	out, e := svc.ExecutionTool(ctx, lease, invocation, "read_service_summary", args)
	if e != nil {
		t.Fatal(e)
	}
	var receipt struct {
		Content struct {
			ReportID  string            `json:"report_id"`
			State     status.Summary    `json:"state_as_of_capture"`
			Incidents []json.RawMessage `json:"incidents"`
			Captured  time.Time         `json:"captured_at"`
		} `json:"content"`
		Sources []agent.ResultReference `json:"source_refs"`
	}
	if e = json.Unmarshal(out, &receipt); e != nil {
		t.Fatal(e)
	}
	t.Run("snapshot preserves observed failures underneath maintenance and omits secrets", func(t *testing.T) {
		if receipt.Content.State.State != status.Maintenance || receipt.Content.State.ObservedState != status.Down || len(receipt.Content.Incidents) != 1 || receipt.Content.Captured.IsZero() {
			t.Fatal("incorrect captured facts", string(out))
		}
		for _, secret := range []string{"DO_NOT_DISCLOSE", "public_key", "signed_payload", "signature"} {
			if strings.Contains(string(out), secret) {
				t.Fatal("collector or private incident leaked", secret)
			}
		}
		if len(receipt.Sources) != 1 || receipt.Sources[0].VersionID == nil || *receipt.Sources[0].VersionID != receipt.Content.ReportID {
			t.Fatal("missing exact report source")
		}
	})
	t.Run("report and tool evidence cannot be rewritten or inserted without receipt", func(t *testing.T) {
		for _, q := range []string{"UPDATE agent_service_reports SET title='rewritten' WHERE run_id=$1", "DELETE FROM agent_service_reports WHERE run_id=$1"} {
			if _, e := f.owner.Exec(ctx, q, run.ID); e == nil {
				t.Fatal("immutable report changed")
			}
		}
		_, e := f.owner.Exec(ctx, `INSERT INTO agent_service_reports SELECT $2,org_id,run_id,conversation_id,actor_id,service_id,audience,related_service_ids,title,configuration_version,jsonb_set(content_text::jsonb,'{report_id}',to_jsonb($2::text))::text,document_sha256,created_at FROM agent_service_reports WHERE run_id=$1`, run.ID, core.NewID())
		if e == nil {
			t.Fatal("unreceipted snapshot committed")
		}
	})
	push("2", true)
	t.Run("new live observation does not rewrite old evidence or break replay", func(t *testing.T) {
		again, e := svc.ExecutionTool(ctx, lease, invocation, "read_service_summary", args)
		left, _ := core.CanonicalJSON(again)
		right, _ := core.CanonicalJSON(out)
		if e != nil || !bytes.Equal(left, right) {
			t.Fatal("historical receipt changed", e)
		}
		r, v := f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/service/"+d.ID+"?context_security_version=1&max_chars=10000&version_id="+receipt.Content.ReportID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		view := data[agent.ContextView](t, v)
		if view.VersionID == nil || *view.VersionID != receipt.Content.ReportID || !strings.Contains(view.Text, "Visible actual incident") {
			t.Fatal("fixed source unreadable")
		}
	})
	if e = svc.CompleteExecution(ctx, lease, agent.ProposedResult{Status: "completed", Summary: "Actual service evidence captured; current maintenance preserves the observed failure.", SourceRefs: receipt.Sources, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{InputTokens: 100, OutputTokens: 30}); e != nil {
		t.Fatal(e)
	}

	if os.Getenv("CP_BROWSER_SERVICE_SUMMARY_FIXTURE") == "1" && !t.Failed() {
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"service_summary_fixture": true, "service_id": d.ID, "service_report_id": receipt.Content.ReportID, "conversation_id": c.ID, "completed_run_id": run.ID, "profile_id": profile, "project_id": p.ID})
	}
	t.Run("current read policy still fences historical report", func(t *testing.T) {
		exec(`INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',false,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'`, core.NewID(), f.org)
		r, v := f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/service/"+d.ID+"?context_security_version=1&max_chars=4000&version_id="+receipt.Content.ReportID, f.bobToken, "", nil, nil)
		if r.Code == 200 {
			t.Fatal("revoked AI policy still exposed snapshot", v)
		}
	})
}
