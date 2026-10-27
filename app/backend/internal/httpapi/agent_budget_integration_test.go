package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAgentTokenBudgets(t *testing.T) {
	f := setupWithBudget(t, false)
	ctx := context.Background()
	service := &agent.Service{DB: f.api.DB}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	p := f.createProject("Token budget demonstration", []string{f.bob})
	p2 := f.createProject("Separate project budget", []string{f.bob})
	for _, u := range []string{f.alice, f.bob} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'`, core.NewID(), f.org, u)
	}
	exec(`INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'`, core.NewID(), f.org)
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'project-briefing','budget-controlled',repeat('a',64),$3,repeat('b',64),'active',10000,1000,8,600,$4,'Isolated budget integration fixture')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	bridge := &agentworker.Bridge{Service: service}
	server := httptest.NewServer(bridge)
	defer server.Close()
	bridge.Address = strings.TrimPrefix(server.URL, "http://")
	credentials := map[string]string{}
	meter := func(lease agent.ExecutionLease, u agent.ExecutionUsage) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"type": "run.usage", "usage": u})
		req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+agentworker.ControlPath+"events", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+credentials[lease.RunID])
		req.Header.Set("Content-Type", "application/json")
		res, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			t.Fatal("meter transport failed")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatal("meter status", res.StatusCode)
		}
	}
	admin := f.administratorToken()
	var policy agent.BudgetPolicy
	publish := func(org, user, project, skill int64) {
		t.Helper()
		in := agent.BudgetPolicyInput{ExpectedVersion: policy.Version, OrganizationTokens: org, UserTokens: user, ProjectTokens: project, SkillTokens: skill, Reason: "Controlled budget policy revision"}
		rec, res := f.request("POST", "/v1/agent-budget/policies", admin, core.NewID(), in, nil)
		expect(t, rec, 201, res)
		policy = data[agent.BudgetPolicy](t, res)
	}
	conversation := func(token, project string) agent.Conversation {
		rec, res := f.request("POST", "/v1/conversations", token, core.NewID(), agent.ConversationCreate{Type: "project", ProjectID: &project, Title: "Budget-controlled conversation", MemberIDs: []string{}}, nil)
		expect(t, rec, 201, res)
		return data[agent.Conversation](t, res)
	}
	input := func(c agent.Conversation) agent.RunCreate {
		return agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "Read the authorized project within its reserved token allowance."}
	}
	create := func(token, project string, status int) agent.Run {
		c := conversation(token, project)
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", token, core.NewID(), input(c), nil)
		expect(t, rec, status, res)
		if status == 202 {
			return data[agent.Run](t, res)
		}
		return agent.Run{}
	}
	cancel := func(token string, r agent.Run) {
		rec, res := f.request("GET", "/v1/runs/"+r.ID, token, "", nil, nil)
		expect(t, rec, 200, res)
		r = data[agent.Run](t, res)
		rec, res = f.request("POST", "/v1/runs/"+r.ID+"/cancel", token, core.NewID(), agent.RunCancel{ExpectedVersion: r.Version, Action: "cancel", Reason: "Controlled budget cancellation"}, nil)
		expect(t, rec, 200, res)
	}
	summary := func(token, query string) agent.BudgetSummary {
		rec, res := f.request("GET", "/v1/agent-budget?"+query, token, "", nil, nil)
		expect(t, rec, 200, res)
		return data[agent.BudgetSummary](t, res)
	}
	entry := func(id string) agent.BudgetEntry {
		rec, res := f.request("GET", "/v1/agent-budget/entries?scope=organization&limit=100", admin, "", nil, nil)
		expect(t, rec, 200, res)
		for _, e := range data[core.Page[agent.BudgetEntry]](t, res).Items {
			if e.ID == id {
				return e
			}
		}
		t.Fatal("budget entry missing")
		return agent.BudgetEntry{}
	}
	start := func(r agent.Run) (jobqueue.Job, agent.ExecutionLease) {
		j, err := service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "budget.test", agent.ExecutionLeaseDuration)
		if err != nil || j == nil {
			t.Fatal("claim", err)
		}
		execution, err := service.IssueExecution(ctx, *j)
		if err != nil {
			t.Fatal(err)
		}
		credentials[execution.RunID] = execution.Credential
		if execution.RunID != r.ID {
			t.Fatal("wrong run claimed")
		}
		lease, err := service.LeaseForCredential(ctx, execution.Credential)
		if err != nil {
			t.Fatal(err)
		}
		if err = service.RecordVerifiedConfinement(ctx, lease, strings.Repeat("b", 64), strings.Repeat("c", 64), time.Now().Add(30*time.Second)); err != nil {
			t.Fatal(err)
		}
		if err = service.StartExecution(ctx, lease); err != nil {
			t.Fatal(err)
		}
		return *j, lease
	}
	stop := func(j jobqueue.Job) {
		if err := service.FinishStoppedExecution(ctx, j, "RUNTIME_EXECUTOR_EXITED"); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("no_policy_denies_atomically_and_admin_publishes_explicit_allowance", func(t *testing.T) {
		c := conversation(f.aliceToken, p.ID)
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), input(c), nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "AGENT_BUDGET_UNCONFIGURED" {
			t.Fatal("missing-policy reason")
		}
		var n int
		if err := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_runs)+(SELECT count(*) FROM conversation_messages)+(SELECT count(*) FROM agent_run_jobs)`).Scan(&n); err != nil || n != 0 {
			t.Fatal("denial left durable work", err)
		}
		publish(1000000000, 1000000000, 1000000000, 1000000000)
	})
	t.Run("policy_requires_human_mfa_authority_and_current_revision", func(t *testing.T) {
		in := agent.BudgetPolicyInput{ExpectedVersion: policy.Version, OrganizationTokens: 1, UserTokens: 1, ProjectTokens: 1, SkillTokens: 1, Reason: "Unauthorized change"}
		rec, res := f.request("POST", "/v1/agent-budget/policies", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 403, res)
		in.ExpectedVersion = 0
		rec, res = f.request("POST", "/v1/agent-budget/policies", admin, core.NewID(), in, nil)
		expect(t, rec, 409, res)
		rec, res = f.request("GET", "/v1/agent-budget?scope=organization", f.aliceToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/agent-budget?scope=user&scope_id="+f.bob, f.aliceToken, "", nil, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/agent-budget?month=2026-99", admin, "", nil, nil)
		expect(t, rec, 422, res)
	})
	t.Run("one_reservation_for_command_and_message_replay_then_queued_cancel_releases", func(t *testing.T) {
		c := conversation(f.aliceToken, p.ID)
		in := input(c)
		key := core.NewID()
		rec, res := f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, key, in, nil)
		expect(t, rec, 202, res)
		r := data[agent.Run](t, res)
		for _, k := range []string{key, core.NewID()} {
			rec, res = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, k, in, nil)
			expect(t, rec, 202, res)
			if data[agent.Run](t, res).ID != r.ID {
				t.Fatal("replay duplicated run")
			}
		}
		if e := entry(r.ID); e.ReservedTokens != 11000 || e.Status != "reserved" {
			t.Fatal("reservation not exact")
		}
		cancel(f.aliceToken, r)
		if e := entry(r.ID); e.ReservedTokens != 0 || e.ChargedTokens != 0 || e.Status != "released" {
			t.Fatal("queued cancel retained charge")
		}
	})
	t.Run("concurrent_organization_admission_cannot_overreserve", func(t *testing.T) {
		publish(11000, 1000000000, 1000000000, 1000000000)
		cs := []agent.Conversation{conversation(f.aliceToken, p.ID), conversation(f.bobToken, p.ID)}
		tokens := []string{f.aliceToken, f.bobToken}
		var wg sync.WaitGroup
		statuses := make([]int, 2)
		runs := make([]agent.Run, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				rec, res := f.request("POST", "/v1/conversations/"+cs[i].ID+"/runs", tokens[i], core.NewID(), input(cs[i]), nil)
				statuses[i] = rec.Code
				if rec.Code == 202 {
					runs[i] = data[agent.Run](t, res)
				}
			}(i)
		}
		wg.Wait()
		if statuses[0]+statuses[1] != 631 {
			t.Fatal("expected exactly one 202 and one 429", statuses)
		}
		for i, r := range runs {
			if r.ID != "" {
				cancel(tokens[i], r)
			}
		}
	})
	for _, scope := range []string{"user", "project", "skill"} {
		t.Run(scope+"_quota_is_shared_and_independently_enforced", func(t *testing.T) {
			limits := map[string]int64{"org": 1000000000, "user": 1000000000, "project": 1000000000, "skill": 1000000000}
			limits[scope] = 11000
			publish(limits["org"], limits["user"], limits["project"], limits["skill"])
			first := create(f.aliceToken, p.ID, 202)
			token, project := f.aliceToken, p2.ID
			if scope == "project" {
				token = f.bobToken
				project = p.ID
			}
			if scope == "skill" {
				token = f.bobToken
			}
			create(token, project, 429)
			if scope == "user" {
				other := create(f.bobToken, p2.ID, 202)
				cancel(f.bobToken, other)
			}
			if scope == "project" {
				other := create(f.bobToken, p2.ID, 202)
				cancel(f.bobToken, other)
			}
			cancel(f.aliceToken, first)
		})
	}
	t.Run("zero_allowance_stops_new_runs_without_revoking_existing_reservations", func(t *testing.T) {
		publish(1000000000, 1000000000, 1000000000, 1000000000)
		r := create(f.aliceToken, p.ID, 202)
		publish(0, 0, 0, 0)
		create(f.bobToken, p2.ID, 429)
		if entry(r.ID).ReservedTokens != 11000 {
			t.Fatal("policy edit rewrote reservation")
		}
		cancel(f.aliceToken, r)
		publish(1000000000, 1000000000, 1000000000, 1000000000)
	})
	t.Run("known_failed_usage_is_idempotent_and_cached_tokens_are_not_counted_twice", func(t *testing.T) {
		r := create(f.aliceToken, p.ID, 202)
		j, lease := start(r)
		u := agent.ExecutionUsage{InputTokens: 100, CachedInputTokens: 60, OutputTokens: 20, ReasoningOutputTokens: 5}
		meter(lease, u)
		meter(lease, u)
		u.OutputTokens++
		if core.PublicError(service.RecordExecutionUsage(ctx, lease, u)).Code != "RUNTIME_USAGE_CONFLICT" {
			t.Fatal("changed accounting accepted")
		}
		stop(j)
		if e := entry(r.ID); e.Status != "settled" || e.ChargedTokens != 120 || e.ReservedTokens != 0 || e.PendingTokens != 0 {
			t.Fatal("known failure accounting", e.Status, e.ChargedTokens)
		}
	})
	t.Run("over_limit_usage_remains_accounted_after_runtime_stop", func(t *testing.T) {
		r := create(f.aliceToken, p.ID, 202)
		j, lease := start(r)
		if err := service.RecordExecutionUsage(ctx, lease, agent.ExecutionUsage{InputTokens: 10001, OutputTokens: 1001}); err != nil {
			t.Fatal(err)
		}
		stop(j)
		if e := entry(r.ID); e.ChargedTokens != 11002 || e.Status != "settled" {
			t.Fatal("overshoot disappeared")
		}
	})

	t.Run("cancellation_can_record_usage_without_regaining_business_authority", func(t *testing.T) {
		r := create(f.aliceToken, p.ID, 202)
		j, lease := start(r)
		cancel(f.aliceToken, r)
		if err := service.RecordExecutionUsage(ctx, lease, agent.ExecutionUsage{InputTokens: 30, OutputTokens: 5}); err != nil {
			t.Fatal(err)
		}
		if core.PublicError(service.CheckExecution(ctx, lease)).Code != "RUNTIME_CANCEL_REQUESTED" {
			t.Fatal("accounting revived business authority")
		}
		stop(j)
		e := entry(r.ID)
		if e.Status != "settled" || e.RunState != "cancelled" || e.ChargedTokens != 35 {
			t.Fatal("cancelled usage lost")
		}
	})
	var pending agent.Run
	t.Run("started_unknown_usage_stays_pending_until_audited_reconciliation", func(t *testing.T) {
		r := create(f.aliceToken, p.ID, 202)
		j, _ := start(r)
		stop(j)
		e := entry(r.ID)
		if e.Status != "pending" || e.ChargedTokens != 11000 || e.PendingTokens != 11000 || e.ReservedTokens != 0 {
			t.Fatal("unknown cost fabricated as free")
		}
		in := agent.BudgetReconcileInput{ExpectedVersion: e.RunVersion, InputTokens: 70, OutputTokens: 10, EvidenceReference: "controlled-provider-reference-86", Reason: "Synthetic token usage reconciliation"}
		rec, res := f.request("POST", "/v1/agent-budget/entries/"+r.ID+"/reconcile", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 403, res)
		in.ExpectedVersion--
		rec, res = f.request("POST", "/v1/agent-budget/entries/"+r.ID+"/reconcile", admin, core.NewID(), in, nil)
		expect(t, rec, 409, res)
		in.ExpectedVersion++
		key := core.NewID()
		for i := 0; i < 2; i++ {
			rec, res = f.request("POST", "/v1/agent-budget/entries/"+r.ID+"/reconcile", admin, key, in, nil)
			expect(t, rec, 200, res)
			if e = data[agent.BudgetEntry](t, res); e.Status != "reconciled" || e.ChargedTokens != 80 || e.PendingTokens != 0 {
				t.Fatal("reconciliation did not release unused estimate")
			}
		}
		rec, res = f.request("POST", "/v1/agent-budget/entries/"+r.ID+"/reconcile", admin, core.NewID(), in, nil)
		expect(t, rec, 409, res)
		pending = create(f.aliceToken, p.ID, 202)
		job, _ := start(pending)
		stop(job)
	})
	t.Run("ledger_is_immutable_and_scope_cursor_cannot_be_reused", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, "UPDATE agent_budget_reservations SET estimated_tokens=1 WHERE run_id=$1", pending.ID); err == nil {
			t.Fatal("reservation changed")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE agent_budget_policies SET reason='rewritten' WHERE id=$1", policy.ID); err == nil {
			t.Fatal("policy history changed")
		}
		rec, res := f.request("GET", "/v1/agent-budget/entries?scope=organization&limit=1", admin, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[agent.BudgetEntry]](t, res)
		if page.NextCursor == nil {
			t.Fatal("no pagination")
		}
		rec, res = f.request("GET", "/v1/agent-budget/entries?scope=mine&cursor="+*page.NextCursor, admin, "", nil, nil)
		expect(t, rec, 400, res)
		s := summary(admin, "scope=organization")
		if s.PendingTokens != 11000 || s.ReservedTokens != 0 || s.UnbudgetedRuns != 0 {
			t.Fatal("aggregate disagrees with durable states")
		}
		own := summary(f.bobToken, "scope=mine")
		if own.ChargedTokens != 0 {
			t.Fatal("other actor usage leaked")
		}
	})
	t.Run("reservation_write_failure_rolls_back_run_message_and_queue", func(t *testing.T) {
		var before, after int64
		f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_runs").Scan(&before)
		exec(`CREATE FUNCTION budget_test_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled reservation failure'; END; $$;CREATE TRIGGER budget_test_failure BEFORE INSERT ON agent_budget_reservations FOR EACH ROW EXECUTE FUNCTION budget_test_failure()`)
		create(f.aliceToken, p2.ID, 500)
		exec(`DROP TRIGGER budget_test_failure ON agent_budget_reservations;DROP FUNCTION budget_test_failure()`)
		f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_runs").Scan(&after)
		if before != after {
			t.Fatal("failed reservation left run")
		}
	})
	if os.Getenv("CP_BROWSER_TOKEN_BUDGET_FIXTURE") == "1" && !t.Failed() {
		role := core.NewID()
		exec(`INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'budget_browser_manager','Synthetic budget manager',true)`, role, f.org)
		exec(`INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'policy.manage')`, f.org, role)
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) VALUES($1,$2,$3,$4,'org','project_restricted')`, core.NewID(), f.org, f.alice, role)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		holdBrowserBusinessFixture(t, f, map[string]any{"token_budget_fixture": true, "project_id": p.ID, "pending_run_id": pending.ID, "profile_id": profile})
	}
}
