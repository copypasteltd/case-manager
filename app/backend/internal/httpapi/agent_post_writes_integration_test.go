package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/knowledge"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Uses native PostgreSQL, authenticated HTTP and the actual MCP gateway.
// Native confinement proof is a controlled dependency, not a deployed runtime.
func TestAgentPostWrites(t *testing.T) {
	u, evidence, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	var diagnostics bytes.Buffer
	f.api.Logger = slog.New(slog.NewTextHandler(&diagnostics, &slog.HandlerOptions{Level: slog.LevelError}))
	defer func() {
		if t.Failed() {
			t.Log(diagnostics.String())
		}
	}()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='agent_user'`, core.NewID(), f.org, f.alice)
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'project-briefing','controlled-test',repeat('a',64),$3,repeat('b',64),'active',30000,4000,20,600,$4,'Isolated knowledge write test')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	h, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: evidence.File.ProjectID, Title: "Knowledge authoring conversation", MemberIDs: []string{}}, nil)
	expect(t, h, 201, v)
	c := data[agent.Conversation](t, v)
	service := &agent.Service{DB: f.api.DB}
	bridge := &agentworker.Bridge{Service: service, Verifier: &controlledNativeVerifier{}, Model: "controlled-test", WorkingDirectory: "controlled-empty-scope"}
	server := httptest.NewServer(bridge)
	defer server.Close()
	bridge.Address = strings.TrimPrefix(server.URL, "http://")
	credentials := map[string]string{}
	post := func(l agent.ExecutionLease, path string, body any, status int) map[string]json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, e := http.NewRequest("POST", server.URL+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+credentials[l.RunID])
		response, e := server.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var out map[string]json.RawMessage
		if e = json.NewDecoder(response.Body).Decode(&out); e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != status {
			t.Fatalf("owned update bridge returned %d, want %d", response.StatusCode, status)
		}
		return out
	}

	input := func(value *agent.PostWriteInput) agent.RunCreate {
		return agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Write only the selected knowledge draft from verified facts; separate internal publication requires reviewing the full saved revision.", PostWrite: value}
	}
	start := func(in agent.RunCreate) (agent.Run, agent.ExecutionLease) {
		h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(t, h, 202, v)
		run := data[agent.Run](t, v)
		j, e := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "post.fixture", agent.ExecutionLeaseDuration)
		if e != nil || j == nil {
			t.Fatal(e)
		}
		x, e := service.IssueExecution(ctx, *j)
		if e != nil {
			t.Fatal(e)
		}
		var prepared struct {
			Value *agent.PostWriteView `json:"authorized_post_write"`
		}
		if json.Unmarshal([]byte(x.PreparedContext), &prepared) != nil || (in.PostWrite != nil && prepared.Value == nil) || strings.Contains(x.PreparedContext, "session_id") {
			t.Fatal("invalid public knowledge preparation")
		}
		l, e := service.LeaseForCredential(ctx, x.Credential)
		if e != nil {
			t.Fatal(e)
		}
		l.Job = *j
		credentials[l.RunID] = x.Credential
		post(l, agentworker.ControlPath+"confinement", agentworker.NativeProfile{SDKVersion: "0.164.0", ProfileID: profile, Model: bridge.Model, WorkingDirectory: bridge.WorkingDirectory, ConfigurationSHA256: strings.Repeat("b", 64), ExpectedTools: []string{"company/read_resource", "company/search_resources", "company/inspect_operation", "company/draft_post", "company/publish_post"}}, 200)
		if e = service.StartExecution(ctx, l); e != nil {
			t.Fatal(e)
		}
		return run, l
	}
	tool := func(l agent.ExecutionLease, key, name string, in any) (json.RawMessage, error) {
		response := post(l, agentworker.MCPPath, map[string]any{"jsonrpc": "2.0", "id": key, "method": "tools/call", "params": map[string]any{"name": name, "arguments": in}}, 200)
		if raw := response["error"]; len(raw) > 0 {
			var rpcError struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(raw, &rpcError) != nil {
				t.Fatal("invalid MCP error")
			}
			return nil, core.Fail(409, rpcError.Message, "Controlled MCP authority denial")
		}
		var rpc struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			Output json.RawMessage `json:"structuredContent"`
		}
		if e := json.Unmarshal(response["result"], &rpc); e != nil {
			t.Fatalf("MCP %s returned no result: %s", name, string(response["error"]))
		}
		if rpc.IsError {
			return nil, core.Fail(409, rpc.Content[0].Text, "Controlled MCP denial")
		}
		return rpc.Output, nil
	}

	finish := func(l agent.ExecutionLease) {
		if e := service.FinishStoppedExecution(ctx, l.Job, "RUNTIME_EXECUTOR_EXITED"); e != nil {
			t.Fatal(e)
		}
	}
	getRun := func(id string) agent.Run {
		h, v := f.request("GET", "/v1/runs/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[agent.Run](t, v)
	}
	getPost := func(id string) knowledge.Post {
		h, v := f.request("GET", "/v1/posts/"+id, f.aliceToken, "", nil, nil)
		expect(t, h, 200, v)
		return data[knowledge.Post](t, v)
	}
	draftInput := func(p *knowledge.Post) agent.PostWriteInput {
		v := agent.PostWriteInput{Tool: "draft_post", Type: "project_log", Title: "Generated briefing " + core.NewID(), Classification: "project_restricted", Reason: "Save bounded draft only", SourceRefs: []knowledge.Source{{Type: "file", ID: evidence.File.ID, VersionID: &evidence.Version.ID}}}
		if p != nil {
			v.PostID = &p.ID
			v.ExpectedVersion = &p.Version
			v.BaseRevisionID = &p.CurrentRevisionID
			v.ContentHash = &p.ContentHash
			v.Type = p.Type
			v.Classification = p.Classification
		}
		return v
	}
	publishInput := func(p knowledge.Post) agent.PostWriteInput {
		return agent.PostWriteInput{Tool: "publish_post", PostID: &p.ID, ExpectedVersion: &p.Version, BaseRevisionID: &p.CurrentRevisionID, ContentHash: &p.ContentHash, Reason: "Human reviewed this exact full internal revision"}
	}
	args := func(run agent.Run, body string) map[string]any {
		v := map[string]any{"run_id": run.ID, "intent_sha256": run.PostWrite.IntentSHA256}
		if run.PostWrite.Input.Tool == "draft_post" {
			v["body"] = body
			v["summary"] = "Verified source summary"
			v["tags"] = []string{"verified", "project"}
		}
		return v
	}
	write := func(run agent.Run, l agent.ExecutionLease, body string) agent.PostWriteView {
		raw, e := tool(l, "write", run.PostWrite.Input.Tool, args(run, body))
		if e != nil {
			t.Fatal(e)
		}
		var out struct {
			Value       agent.PostWriteView     `json:"content"`
			OperationID *string                 `json:"operation_id"`
			Resources   []agent.ResultReference `json:"resource_refs"`
			Sources     []agent.ResultReference `json:"source_refs"`
		}
		if json.Unmarshal(raw, &out) != nil || out.OperationID == nil {
			t.Fatal("missing real knowledge result")
		}
		out.Value = *getRun(run.ID).PostWrite
		if e = service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "Knowledge operation saved; publication and external notification are separate.", ResourceRefs: out.Resources, SourceRefs: out.Sources, OperationIDs: []string{*out.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
			t.Fatal(e)
		}
		return out.Value
	}
	runTest := func(name string, fn func(*testing.T)) {
		t.Run(name, func(sub *testing.T) {
			previous := t
			t = sub
			f.t = sub
			defer func() { t = previous; f.t = previous }()
			fn(sub)
		})
	}
	var saved knowledge.Post
	var completedRun agent.Run

	runTest("generated_draft_has_real_version_and_idempotent_exact_receipts", func(sub *testing.T) {
		in := draftInput(nil)
		run, l := start(input(&in))
		request := args(run, "# Verified project briefing\n\nFacts from the fixed file only.")
		raw, e := tool(l, "draft", "draft_post", request)
		if e != nil {
			sub.Fatal(e)
		}
		var out struct {
			Value       agent.PostWriteView     `json:"content"`
			OperationID *string                 `json:"operation_id"`
			Resources   []agent.ResultReference `json:"resource_refs"`
			Sources     []agent.ResultReference `json:"source_refs"`
		}
		if json.Unmarshal(raw, &out) != nil || out.OperationID == nil {
			sub.Fatal("draft receipt missing")
		}
		out.Value = *getRun(run.ID).PostWrite
		p := out.Value.Result
		if p.State != "draft" || p.PublishedRevisionID != nil || p.Title != in.Title || p.Body != request["body"] || p.Version != 1 || len(p.SourceRefs) != 1 || p.ContentHash == "" {
			sub.Fatal("draft result differs from the real saved content")
		}
		replay, e := tool(l, "draft-repeat", "draft_post", request)
		if e != nil || !bytes.Equal(raw, replay) {
			sub.Fatal("repeat did not return the same operation", e)
		}
		request["body"] = "widened content"
		if _, e = tool(l, "draft-widened", "draft_post", request); e == nil {
			sub.Fatal("second content version accepted")
		}
		if e = service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "Draft saved; not published.", ResourceRefs: out.Resources, SourceRefs: out.Sources, OperationIDs: []string{*out.OperationID}, MissingFields: []string{}}, agent.ExecutionUsage{}); e != nil {
			sub.Fatal(e)
		}
		completedRun = getRun(run.ID)
		saved = getPost(p.ID)
		h, v := f.request("GET", "/v1/runs/"+run.ID+"/event-log?after_seq=0&limit=100", f.aliceToken, "", nil, nil)
		expect(sub, h, 200, v)
		if data[agent.EventPage](sub, v).Run.PostWrite == nil {
			sub.Fatal("event log lost knowledge receipt")
		}
	})
	runTest("draft_revision_then_separate_reviewed_internal_publication", func(sub *testing.T) {
		in := draftInput(&saved)
		run, l := start(input(&in))
		result := write(run, l, "# Revised verified briefing\n\nReview this full revision before publication.")
		saved = *result.Result
		if saved.Version != 2 || saved.State != "draft" || saved.PublishedRevisionID != nil {
			sub.Fatal("draft revision was published")
		}
		publish := publishInput(saved)
		run, l = start(input(&publish))
		a := args(run, "")
		a["body"] = "Cannot change content during publication"
		if _, e := tool(l, "publish-widened", "publish_post", a); e == nil {
			sub.Fatal("publication changed content")
		}
		result = write(run, l, "")
		saved = *result.Result
		if saved.State != "published" || saved.Version != 3 || saved.PublishedRevisionID == nil || *saved.PublishedRevisionID != saved.CurrentRevisionID || saved.ContentHash != *publish.ContentHash {
			sub.Fatal("published something other than the reviewed revision")
		}
	})
	runTest("new_draft_keeps_prior_published_revision", func(sub *testing.T) {
		old := *saved.PublishedRevisionID
		in := draftInput(&saved)
		run, l := start(input(&in))
		result := write(run, l, "Private new draft; not the published article.")
		saved = *result.Result
		if saved.State != "published" || saved.PublishedRevisionID == nil || *saved.PublishedRevisionID != old || saved.CurrentRevisionID == old || !saved.HasUnpublishedChanges {
			sub.Fatal("draft overwrote the published version")
		}
	})
	runTest("bounded_multibyte_content_saves_without_oversized_tool_receipt", func(sub *testing.T) {
		in := draftInput(nil)
		run, l := start(input(&in))
		body := strings.Repeat("已核实", 4500)
		a := args(run, body)
		raw, e := tool(l, "large-draft", "draft_post", a)
		if e != nil {
			sub.Fatal(e)
		}
		if len(raw) > 120000 {
			sub.Fatal("tool receipt exceeded gateway limit")
		}
		actual := getRun(run.ID).PostWrite
		if actual == nil || actual.Result == nil || actual.Result.Body != body {
			sub.Fatal("multibyte content was truncated")
		}
		finish(l)
	})

	runTest("receipt_failure_rolls_back_post_revision_and_commit", func(sub *testing.T) {
		in := draftInput(nil)
		run, l := start(input(&in))
		a := args(run, "Atomic draft")
		exec("ALTER TABLE agent_tool_receipts ADD CONSTRAINT controlled_post_receipt_failure CHECK(tool<>'draft_post') NOT VALID")
		_, e := tool(l, "receipt-failure", "draft_post", a)
		exec("ALTER TABLE agent_tool_receipts DROP CONSTRAINT controlled_post_receipt_failure")
		if e == nil {
			sub.Fatal("failed receipt accepted")
		}
		var count int
		if e = f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM posts WHERE org_id=$1 AND title=$2)+(SELECT count(*) FROM agent_post_commits WHERE run_id=$3)", f.org, in.Title, run.ID).Scan(&count); e != nil || count != 0 {
			sub.Fatal("failed receipt left a post or commit", e)
		}
		if _, e = tool(l, "exact-retry", "draft_post", a); e != nil {
			sub.Fatal(e)
		}
		finish(l)
	})
	runTest("ordinary_chat_and_read_receipts_cannot_authorize_writes", func(sub *testing.T) {
		run, l := start(input(nil))
		if _, e := tool(l, "fake-draft", "draft_post", map[string]any{"run_id": run.ID, "intent_sha256": strings.Repeat("a", 64), "body": "No grant", "summary": "", "tags": []string{}}); e == nil {
			sub.Fatal("chat authorized knowledge mutation")
		}
		finish(l)
		in := draftInput(nil)
		run, l = start(input(&in))
		if _, e := tool(l, "read-only", "read_resource", map[string]any{"type": "agent_post_write", "id": run.ID}); e != nil {
			sub.Fatal(e)
		}
		if e := service.CompleteExecution(ctx, l, agent.ProposedResult{Status: "completed", Summary: "Invented successful draft", SourceRefs: []agent.ResultReference{{Type: "agent_post_write", ID: run.ID}}, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}}, agent.ExecutionUsage{}); e == nil {
			sub.Fatal("read receipt completed a write task")
		}
		finish(l)
	})
	if os.Getenv("CP_BROWSER_POST_WRITE_FIXTURE") == "1" {
		_ = f.administratorToken()
		proofRole := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'post_browser_proof','Actionless local browser proof',true)", proofRole, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proofRole)
		exec("UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", f.org, f.alice)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		holdBrowserBusinessFixture(t, f, map[string]any{"agent_post_write_fixture": true, "conversation_id": c.ID, "project_id": *evidence.File.ProjectID, "post_id": saved.ID, "profile_id": profile, "completed_run_id": completedRun.ID})
		return
	}

	runTest("current_business_revision_changes_fence_all_subsequent_tools", func(sub *testing.T) {
		in := publishInput(saved)
		run, l := start(input(&in))
		title := "Human concurrent change"
		h, v := f.request("PATCH", "/v1/posts/"+saved.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": saved.Version, "base_revision_id": saved.CurrentRevisionID, "title": title, "body": saved.Body}, nil)
		expect(sub, h, 200, v)
		saved = data[knowledge.Post](sub, v)
		if _, e := tool(l, "stale-publish", "publish_post", args(run, "")); e == nil {
			sub.Fatal("silently published a new revision")
		}
		if _, e := tool(l, "stale-read", "read_resource", map[string]any{"type": "agent_post_write", "id": run.ID}); e == nil {
			sub.Fatal("stale authorization remained readable to execution")
		}
		finish(l)
	})
	runTest("invalid_bases_scope_and_metadata_are_rejected_before_admission", func(sub *testing.T) {
		in := publishInput(saved)
		in.ContentHash = new(string)
		*in.ContentHash = strings.Repeat("0", 64)
		h, v := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input(&in), nil)
		if h.Code < 400 {
			sub.Fatal("forged hash admitted", v)
		}
		in = draftInput(&saved)
		in.Type = "article"
		h, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input(&in), nil)
		if h.Code < 400 {
			sub.Fatal("metadata mutation admitted", v)
		}
		in = draftInput(nil)
		in.SourceRefs[0].VersionID = nil
		h, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input(&in), nil)
		if h.Code < 400 {
			sub.Fatal("floating source admitted", v)
		}
	})
	runTest("cancellation_before_write_has_no_business_side_effect", func(sub *testing.T) {
		in := draftInput(nil)
		run, l := start(input(&in))
		run = getRun(run.ID)
		h, v := f.request("POST", "/v1/runs/"+run.ID+"/cancel", f.aliceToken, core.NewID(), map[string]any{"expected_version": run.Version, "action": "cancel", "reason": "Stop before saving"}, nil)
		expect(sub, h, 200, v)
		if _, e := tool(l, "cancelled", "draft_post", args(run, "Must not be saved")); e == nil {
			sub.Fatal("cancelled run wrote")
		}
		finish(l)
		var count int
		if e := f.owner.QueryRow(ctx, "SELECT count(*) FROM posts WHERE org_id=$1 AND title=$2", f.org, in.Title).Scan(&count); e != nil || count != 0 {
			sub.Fatal("cancelled write left content", e)
		}
	})
	runTest("immutable_history_and_current_authority_remain_required", func(sub *testing.T) {
		if _, e := f.api.DB.Exec(ctx, "UPDATE agent_post_intents SET tool='publish_post' WHERE run_id=$1", completedRun.ID); e == nil {
			sub.Fatal("intent is mutable")
		}
		if _, e := f.api.DB.Exec(ctx, "DELETE FROM agent_post_commits WHERE run_id=$1", completedRun.ID); e == nil {
			sub.Fatal("commit is mutable")
		}
		h, v := f.request("GET", "/v1/conversations/"+c.ID+"/messages?limit=20", f.aliceToken, "", nil, nil)
		expect(sub, h, 200, v)
	})
	runTest("company_destination_cannot_absorb_project_sources", func(sub *testing.T) {
		original := c
		h, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", Title: "Company knowledge boundary", MemberIDs: []string{}}, nil)
		expect(sub, h, 201, v)
		c = data[agent.Conversation](sub, v)
		defer func() { c = original }()
		in := agent.PostWriteInput{Tool: "draft_post", Type: "article", Title: "Company internal article", Classification: "internal", Reason: "Only company facts"}
		request := input(&in)
		request.ContextRefs = []agent.SourceReference{{Type: "file", ID: evidence.File.ID, VersionID: &evidence.Version.ID}}
		h, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), request, nil)
		if h.Code < 400 {
			sub.Fatal("project source authorized for company publication", v)
		}
	})
	runTest("closing_project_fences_admitted_knowledge_work", func(sub *testing.T) {
		in := draftInput(nil)
		run, l := start(input(&in))
		var previous string
		if e := f.owner.QueryRow(ctx, "SELECT state FROM projects WHERE id=$1", *evidence.File.ProjectID).Scan(&previous); e != nil {
			sub.Fatal(e)
		}
		exec("UPDATE projects SET state='closing',version=version+1 WHERE id=$1", *evidence.File.ProjectID)
		if _, e := tool(l, "archived", "draft_post", args(run, "No closing writes")); e == nil {
			sub.Fatal("closing project still writable")
		}
		finish(l)
		exec("UPDATE projects SET state=$2,version=version+1 WHERE id=$1", *evidence.File.ProjectID, previous)
	})

	runTest("revoked_ai_policy_fences_read_and_write", func(sub *testing.T) {
		in := draftInput(nil)
		run, l := start(input(&in))
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',false,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
		if _, e := tool(l, "policy-revoked", "draft_post", args(run, "Cannot process restricted content")); e == nil {
			sub.Fatal("AI revocation ignored")
		}
		finish(l)
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
	})
	runTest("revoked_original_session_fences_write", func(sub *testing.T) {
		in := draftInput(nil)
		run, l := start(input(&in))
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=(SELECT session_id FROM agent_post_intents WHERE run_id=$1)", run.ID)
		if _, e := tool(l, "session-revoked", "draft_post", args(run, "Must not save")); e == nil {
			sub.Fatal("revoked original sender still wrote")
		}
		finish(l)
	})
}
