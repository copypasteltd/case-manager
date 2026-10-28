package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
)

// This is an execution correctness check with synthetic data, not a provider,
// confinement certification, billing, network performance or production check.
func TestAgentCostFreeExecution(t *testing.T) {
	current := t
	f := setup(t)
	ctx := context.Background()
	service := &agent.Service{DB: f.api.DB}
	exec := func(sql string, args ...any) {
		current.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			current.Fatal(err)
		}
	}
	count := func(sql string, args ...any) int64 {
		current.Helper()
		var n int64
		if err := f.owner.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			current.Fatal(err)
		}
		return n
	}
	p := f.createProject("Cost-free secretary workspace", []string{f.bob})
	for _, u := range []string{f.alice, f.bob} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'`, core.NewID(), f.org, u)
	}
	exec(`INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'`, core.NewID(), f.org)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'project-briefing','cost-scope-100',repeat('a',64),$3,repeat('b',64),'active',10000,1000,8,600,$4,'Synthetic execution scope fixture')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	admin := f.administratorToken()
	conversation := func(token string) agent.Conversation {
		current.Helper()
		rec, res := f.request("POST", "/v1/conversations", token, core.NewID(), agent.ConversationCreate{Type: "project", ProjectID: &p.ID, Title: "Secretary execution", MemberIDs: []string{}}, nil)
		expect(current, rec, 201, res)
		return data[agent.Conversation](current, res)
	}
	input := func(c agent.Conversation) agent.RunCreate {
		return agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Read the authorized project and provide a summary."}
	}
	create := func(token string) agent.Run {
		current.Helper()
		c := conversation(token)
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", token, core.NewID(), input(c), nil)
		expect(current, rec, 202, res)
		return data[agent.Run](current, res)
	}
	cancel := func(token string, r agent.Run) {
		current.Helper()
		rec, res := f.request("GET", "/v1/runs/"+r.ID, token, "", nil, nil)
		expect(current, rec, 200, res)
		r = data[agent.Run](current, res)
		rec, res = f.request("POST", "/v1/runs/"+r.ID+"/cancel", token, core.NewID(), agent.RunCancel{ExpectedVersion: r.Version, Action: "cancel", Reason: "Synthetic execution cancellation"}, nil)
		expect(current, rec, 200, res)
	}
	credentials := map[string]string{}
	start := func(r agent.Run) (jobqueue.Job, agent.ExecutionLease) {
		current.Helper()
		j, err := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "cost.scope.test", agent.ExecutionLeaseDuration)
		if err != nil || j == nil {
			current.Fatal("claim", err)
		}
		execution, err := service.IssueExecution(ctx, *j)
		if err != nil {
			current.Fatal(err)
		}
		if execution.RunID != r.ID {
			current.Fatal("wrong run claimed")
		}
		credentials[r.ID] = execution.Credential
		lease, err := service.LeaseForCredential(ctx, execution.Credential)
		if err != nil {
			current.Fatal(err)
		}
		if err = service.RecordVerifiedConfinement(ctx, lease, strings.Repeat("b", 64), strings.Repeat("c", 64), time.Now().Add(30*time.Second)); err != nil {
			current.Fatal(err)
		}
		if err = service.StartExecution(ctx, lease); err != nil {
			current.Fatal(err)
		}
		return *j, lease
	}
	stop := func(j jobqueue.Job) {
		current.Helper()
		if err := service.FinishStoppedExecution(ctx, j, "RUNTIME_EXECUTOR_EXITED"); err != nil {
			current.Fatal(err)
		}
	}
	complete := func(lease agent.ExecutionLease) {
		current.Helper()
		raw, _ := json.Marshal(map[string]any{"type": "project", "id": p.ID})
		if _, err := service.ExecutionTool(ctx, lease, core.NewID(), "read_resource", raw); err != nil {
			current.Fatal(err)
		}
		result := agent.ProposedResult{Status: "completed", Summary: "已依据授权资料整理项目摘要。", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{{Type: "project", ID: p.ID}}, OperationIDs: []string{}, MissingFields: []string{}, NextAction: ""}
		if err := service.CompleteExecution(ctx, lease, result, agent.ExecutionUsage{InputTokens: 100, OutputTokens: 20}); err != nil {
			current.Fatal(err)
		}
	}
	policy := core.NewID()
	legacy := func() agent.Run {
		current.Helper()
		r := create(f.aliceToken)
		exec(`INSERT INTO agent_budget_reservations(run_id,org_id,policy_id,actor_id,project_id,skill_id,month_start,estimated_tokens) VALUES($1,$2,$3,$4,$5,'project-briefing',date_trunc('month',clock_timestamp() AT TIME ZONE 'UTC')::date,11000)`, r.ID, f.org, policy, f.alice, p.ID)
		return r
	}

	runCase := func(name string, check func(*testing.T)) {
		t.Run(name, func(child *testing.T) { current = child; defer func() { current = t }(); check(child) })
	}
	runCase("fresh_installation_has_no_cost_policy_and_completes_verified_result", func(t *testing.T) {
		if count("SELECT count(*) FROM agent_budget_policies") != 0 {
			t.Fatal("fixture invented fee allowance")
		}
		r := create(f.aliceToken)
		_, lease := start(r)
		complete(lease)
		if count("SELECT count(*) FROM agent_run_results WHERE run_id=$1", r.ID) != 1 || count("SELECT count(*) FROM agent_budget_usage WHERE run_id=$1", r.ID) != 1 || count("SELECT count(*) FROM agent_budget_reservations") != 0 {
			t.Fatal("execution/result depends on fee reservation")
		}
	})
	runCase("zero_legacy_allowances_do_not_gate_new_or_started_runs", func(t *testing.T) {
		exec(`INSERT INTO agent_budget_policies(id,org_id,version,organization_tokens,user_tokens,project_tokens,skill_tokens,published_by,reason) VALUES($1,$2,1,0,0,0,0,$3,'Preserved pre-upgrade zero policy')`, policy, f.org, f.admin)
		r := create(f.aliceToken)
		_, lease := start(r)
		complete(lease)
	})
	var pending agent.Run
	runCase("exhausted_legacy_reservation_does_not_block_new_run_and_remains_readable", func(t *testing.T) {
		pending = legacy()
		j, lease := start(pending)
		stop(j)
		if count("SELECT pending_tokens FROM agent_budget_entries WHERE run_id=$1", pending.ID) != 11000 {
			t.Fatal("legacy unknown usage was rewritten")
		}
		if err := service.RecordExecutionUsage(ctx, lease, agent.ExecutionUsage{InputTokens: 1}); err == nil {
			t.Fatal("terminal execution accepted telemetry")
		}
		r := create(f.aliceToken)
		_, current := start(r)
		complete(current)
	})
	runCase("command_and_message_replays_create_one_job_without_reservation", func(t *testing.T) {
		c := conversation(f.aliceToken)
		in := input(c)
		key := core.NewID()
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, key, in, nil)
		expect(t, rec, 202, res)
		r := data[agent.Run](t, res)
		for _, replay := range []string{key, core.NewID()} {
			rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, replay, in, nil)
			expect(t, rec, 202, res)
			if data[agent.Run](t, res).ID != r.ID {
				t.Fatal("replay created another run")
			}
		}
		if count("SELECT count(*) FROM agent_run_jobs WHERE run_id=$1", r.ID) != 1 || count("SELECT count(*) FROM agent_budget_reservations WHERE run_id=$1", r.ID) != 0 {
			t.Fatal("admission created duplicate or fee reservation")
		}
		cancel(f.aliceToken, r)
	})
	runCase("per_user_capacity_and_same_conversation_guards_remain_atomic", func(t *testing.T) {
		c := conversation(f.aliceToken)
		in := input(c)
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 202, res)
		runs := []agent.Run{data[agent.Run](t, res)}
		rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input(c), nil)
		expect(t, rec, 409, res)
		runs = append(runs, create(f.aliceToken), create(f.aliceToken))
		c = conversation(f.aliceToken)
		before := count("SELECT count(*) FROM agent_runs")
		rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input(c), nil)
		expect(t, rec, 429, res)
		if res.Error.Code != "RUN_CAPACITY_EXCEEDED" || count("SELECT count(*) FROM agent_runs") != before {
			t.Fatal("technical capacity guard failed")
		}
		for _, r := range runs {
			cancel(f.aliceToken, r)
		}
	})
	runCase("legacy_reservation_storage_failure_does_not_participate_in_admission", func(t *testing.T) {
		exec(`CREATE FUNCTION budget_test_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic retired storage failure'; END; $$;CREATE TRIGGER budget_test_failure BEFORE INSERT ON agent_budget_reservations FOR EACH ROW EXECUTE FUNCTION budget_test_failure()`)
		r := create(f.aliceToken)
		cancel(f.aliceToken, r)
		exec(`DROP TRIGGER budget_test_failure ON agent_budget_reservations;DROP FUNCTION budget_test_failure()`)
	})
	runCase("controller_usage_is_authenticated_idempotent_and_immutable", func(t *testing.T) {
		r := create(f.aliceToken)
		j, lease := start(r)
		bridge := &agentworker.Bridge{Service: service}
		server := httptest.NewServer(bridge)
		defer server.Close()
		bridge.Address = strings.TrimPrefix(server.URL, "http://")
		// Exercise the service lease boundary, then the HTTP bridge with an invalid bearer.
		body, _ := json.Marshal(map[string]any{"type": "run.usage", "usage": agent.ExecutionUsage{InputTokens: 100, OutputTokens: 20}})
		req, _ := http.NewRequest("POST", server.URL+agentworker.ControlPath+"events", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer not-a-runtime-credential")
		req.Header.Set("Content-Type", "application/json")
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatal("unauthenticated usage accepted")
		}
		body, _ = json.Marshal(map[string]any{"type": "run.usage", "usage": agent.ExecutionUsage{InputTokens: 100, CachedInputTokens: 60, OutputTokens: 20}})
		req, _ = http.NewRequest("POST", server.URL+agentworker.ControlPath+"events", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+credentials[r.ID])
		req.Header.Set("Content-Type", "application/json")
		res, err = server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatal("authenticated controller telemetry rejected", res.StatusCode)
		}
		u := agent.ExecutionUsage{InputTokens: 100, CachedInputTokens: 60, OutputTokens: 20}
		for i := 0; i < 2; i++ {
			if err := service.RecordExecutionUsage(ctx, lease, u); err != nil {
				t.Fatal(err)
			}
		}
		if count("SELECT count(*) FROM agent_budget_usage WHERE run_id=$1", r.ID) != 1 {
			t.Fatal("usage duplicated")
		}
		u.OutputTokens++
		if core.PublicError(service.RecordExecutionUsage(ctx, lease, u)).Code != "RUNTIME_USAGE_CONFLICT" {
			t.Fatal("conflicting usage accepted")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE agent_budget_usage SET input_tokens=1 WHERE run_id=$1", r.ID); err == nil {
			t.Fatal("telemetry mutated")
		}
		stop(j)
	})
	runCase("usage_sanity_and_technical_context_limits_remain_enforced", func(t *testing.T) {
		r := create(f.aliceToken)
		j, lease := start(r)
		for _, u := range []agent.ExecutionUsage{{InputTokens: -1}, {InputTokens: 1, CachedInputTokens: 2}, {OutputTokens: 1000000000001}} {
			if core.PublicError(service.RecordExecutionUsage(ctx, lease, u)).Code != "RUNTIME_USAGE_INVALID" {
				t.Fatal("invalid telemetry accepted")
			}
		}
		result := agent.ProposedResult{Status: "awaiting_input", Summary: "需要补充资料。", ResourceRefs: []agent.ResultReference{}, SourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{"项目目标"}, NextAction: "补充项目目标"}
		if core.PublicError(service.CompleteExecution(ctx, lease, result, agent.ExecutionUsage{InputTokens: 10001})).Code != "RUNTIME_USAGE_BUDGET_EXCEEDED" {
			t.Fatal("technical context limit removed")
		}
		stop(j)
	})
	runCase("cancellation_records_known_usage_without_reviving_business_authority", func(t *testing.T) {
		r := create(f.aliceToken)
		j, lease := start(r)
		cancel(f.aliceToken, r)
		if err := service.RecordExecutionUsage(ctx, lease, agent.ExecutionUsage{InputTokens: 75, OutputTokens: 5}); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(map[string]any{"type": "project", "id": p.ID})
		if _, err := service.ExecutionTool(ctx, lease, core.NewID(), "read_resource", raw); err == nil {
			t.Fatal("cancelled run regained business authority")
		}
		stop(j)
	})
	runCase("telemetry_binds_exact_tenant_and_run_without_cost_policy", func(t *testing.T) {
		r := create(f.aliceToken)
		j, _ := start(r)
		other := core.NewID()
		if _, err := f.owner.Exec(ctx, `INSERT INTO agent_budget_usage(run_id,org_id,input_tokens,cached_input_tokens,cache_write_input_tokens,output_tokens,reasoning_output_tokens) VALUES($1,$2,1,0,0,0,0)`, r.ID, other); err == nil {
			t.Fatal("cross-tenant telemetry accepted")
		}
		stop(j)
	})
	runCase("legacy_cost_writes_are_retired_after_interactive_authorization", func(t *testing.T) {
		in := agent.BudgetPolicyInput{ExpectedVersion: 1, OrganizationTokens: 1, UserTokens: 1, ProjectTokens: 1, SkillTokens: 1, Reason: "Retired write probe"}
		rec, res := f.request("POST", "/v1/agent-budget/policies", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 403, res)
		for i := 0; i < 2; i++ {
			rec, res = f.request("POST", "/v1/agent-budget/policies", admin, core.NewID(), in, nil)
			expect(t, rec, 410, res)
			if res.Error.Code != "AI_COST_CONTROL_RETIRED" {
				t.Fatal("wrong retirement code")
			}
		}
		reconcile := agent.BudgetReconcileInput{ExpectedVersion: 1, InputTokens: 1, OutputTokens: 1, EvidenceReference: "synthetic-old-reference", Reason: "Retired reconciliation probe"}
		rec, res = f.request("POST", "/v1/agent-budget/entries/"+pending.ID+"/reconcile", admin, core.NewID(), reconcile, nil)
		expect(t, rec, 410, res)
		if count("SELECT count(*) FROM agent_budget_policies") != 1 || count("SELECT count(*) FROM agent_budget_reconciliations") != 0 {
			t.Fatal("retired API wrote cost records")
		}
		var allowed bool
		for _, table := range []string{"agent_budget_policies", "agent_budget_reservations", "agent_budget_reconciliations"} {
			if err := f.owner.QueryRow(ctx, "SELECT has_table_privilege('copypaste_app',$1,'INSERT')", table).Scan(&allowed); err != nil || allowed {
				t.Fatal("application retains retired write authority", table, err)
			}
		}
	})
	runCase("legacy_history_stays_immutable_scoped_and_cursor_bound", func(t *testing.T) {
		old := legacy()
		cancel(f.aliceToken, old)
		for _, sql := range []string{"UPDATE agent_budget_policies SET reason='rewrite'", "DELETE FROM agent_budget_reservations"} {
			if _, err := f.owner.Exec(ctx, sql); err == nil {
				t.Fatal("legacy history changed")
			}
		}
		rec, res := f.request("GET", "/v1/agent-budget?scope=organization", f.aliceToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/agent-budget?scope=user&scope_id="+f.bob, f.aliceToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/agent-budget/entries?scope=organization&limit=1", admin, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[agent.BudgetEntry]](t, res)
		if page.NextCursor == nil {
			t.Fatal("missing archive cursor")
		}
		rec, res = f.request("GET", "/v1/agent-budget/entries?scope=mine&cursor="+*page.NextCursor, admin, "", nil, nil)
		expect(t, rec, 400, res)
		rec, res = f.request("GET", "/v1/agent-budget?scope=organization", admin, "", nil, nil)
		expect(t, rec, 200, res)
		s := data[agent.BudgetSummary](t, res)
		if s.PendingTokens != 11000 || s.UnbudgetedRuns == 0 {
			t.Fatal("legacy archive fabricated new reservations")
		}
	})
	if os.Getenv("CP_BROWSER_COST_FREE_FIXTURE") == "1" && !t.Failed() {
		role := core.NewID()
		exec(`INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'archive_browser_manager','Synthetic archive reader',true)`, role, f.org)
		exec(`INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'policy.manage')`, f.org, role)
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')`, core.NewID(), f.org, f.alice, role)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		holdBrowserBusinessFixture(t, f, map[string]any{"cost_free_fixture": true, "project_id": p.ID, "pending_run_id": pending.ID, "profile_id": profile})
	}
}
