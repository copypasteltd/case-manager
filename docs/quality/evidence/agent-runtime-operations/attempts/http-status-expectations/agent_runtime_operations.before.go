package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"os"
	"strings"
	"testing"
	"time"
)

// Actual HTTP, ordinary run creation and durable native PG; profiles and
// identities are isolated synthetic data. No model or external service runs.
func TestAgentRuntimeOperations(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	current := t
	exec := func(q string, args ...any) {
		current.Helper()
		if _, err := f.owner.Exec(ctx, q, args...); err != nil {
			current.Fatal(err)
		}
	}
	run := func(n string, fn func()) {
		if !t.Run(n, func(st *testing.T) { current = st; f.t = st; defer func() { current = t; f.t = t }(); fn() }) {
			t.FailNow()
		}
	}
	bind := func(user, role string) string {
		id := core.NewID()
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key=$4", id, f.org, user, role)
		return id
	}
	manager := bind(f.bob, "agent_runtime_manager")
	bind(f.member, "agent_runtime_reader")
	bind(f.alice, "agent_user")
	exec("UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='agent_user'", f.org)
	exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'internal',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='internal'", core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	f.bobToken = memberMFAToken(f, "bob", "android")
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.memberToken = f.login("member", "android").AccessToken
	profile := func() string {
		exec("UPDATE agent_runtime_profiles SET state='retired' WHERE org_id=$1 AND state='active'", f.org)
		id := core.NewID()
		exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,telegram_configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'project-briefing','synthetic-141',repeat('a',64),$3,repeat('b',64),repeat('c',64),'active',10000,1000,8,600,$4,'PRIVATE_INSTALLATION_NOTE isolated fixture')`, id, f.org, agentworker.RuntimeVersion, f.admin)
		return id
	}
	get := func(id string) agent.ProfileDetails {
		current.Helper()
		r, v := f.request("GET", "/v1/agent/runtime-profiles/"+id, f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		if strings.Contains(r.Body.String(), "PRIVATE_") || strings.Contains(r.Body.String(), "installation_reason") || strings.Contains(r.Body.String(), "conversation_id") {
			current.Fatal("profile metadata leaked private prose")
		}
		return data[agent.ProfileDetails](current, v)
	}
	input := func(v agent.ProfileDetails) agent.ProfileRetire {
		return agent.ProfileRetire{ExpectedVersion: v.Item.Version, ActiveBasisSHA256: v.Impact.ActiveBasisSHA256, ReviewedImpact: true, Reason: "PRIVATE_RETIREMENT_NOTE 已核对准确配置和当前任务影响"}
	}
	retire := func(id string, in agent.ProfileRetire, token, key string, headers map[string]string, status int) agent.ProfileDetails {
		current.Helper()
		r, v := f.request("POST", "/v1/agent/runtime-profiles/"+id+"/retire", token, key, in, headers)
		expect(current, r, status, v)
		if status == 200 {
			return data[agent.ProfileDetails](current, v)
		}
		return agent.ProfileDetails{}
	}
	newRun := func(id string) agent.Run {
		current.Helper()
		r, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", Title: "PRIVATE_CONVERSATION_TITLE", MemberIDs: []string{}}, nil)
		expect(current, r, 201, v)
		c := data[agent.Conversation](current, v)
		r, v = f.request("POST", "/v1/conversations/"+c.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, ProfileID: id, ClientMessageID: core.NewID(), Message: "PRIVATE_REQUEST_BODY synthetic request"}, nil)
		expect(current, r, 202, v)
		return data[agent.Run](current, v)
	}
	login := func(device string) (string, map[string]string) {
		var cipher []byte
		if err := f.owner.QueryRow(ctx, "SELECT secret_cipher FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.bob).Scan(&cipher); err != nil {
			current.Fatal(err)
		}
		secret, err := identity.Open(f.api.Config.MFAKey, cipher, f.org+":"+f.bob)
		if err != nil {
			current.Fatal(err)
		}
		exec("UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", f.org, f.bob, time.Now().Unix()/30-1)
		c := f.login("bob", device)
		h := map[string]string{}
		if device == "web" {
			h["Origin"] = "http://localhost:5178"
		}
		r, v := f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": c.Challenge, "code": authenticatorCode(secret, time.Now())}, h)
		expect(current, r, 200, v)
		l := data[identity.LoginResult](current, v)
		if device == "web" {
			h["X-CSRF-Token"] = l.CSRFToken
			for _, cookie := range r.Result().Cookies() {
				if cookie.Name == "cp_session" {
					h["Cookie"] = "cp_session=" + cookie.Value
				}
			}
			return "", h
		}
		return l.AccessToken, h
	}
	for _, device := range []string{"web", "wechat", "douyin", "android"} {
		run(device+" profile retirement and replay", func() {
			id := profile()
			token, h := login(device)
			r, v := f.request("GET", "/v1/agent/runtime-profiles?state=active&skill_id=project-briefing", token, "", nil, h)
			expect(current, r, 200, v)
			list := data[core.Page[agent.ProfileOperation]](current, v)
			if len(list.Items) != 1 || list.Items[0].ID != id {
				current.Fatal("incorrect configuration inventory")
			}
			before := get(id)
			key := core.NewID()
			in := input(before)
			after := retire(id, in, token, key, h, 200)
			retire(id, in, token, key, h, 200)
			if after.Item.State != "retired" || after.Item.Version != 2 || len(after.History) != 2 || after.History[0].ActorID == nil || *after.History[0].ActorID != f.bob {
				current.Fatal("missing exact human retirement")
			}
			var receipts, audits int
			if err := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM agent_profile_retirements WHERE profile_id=$1),(SELECT count(*) FROM audit_events WHERE resource_id=$1 AND action='agent.profile_retired')`, id).Scan(&receipts, &audits); err != nil || receipts != 1 || audits != 1 {
				current.Fatal("duplicate or missing immutable facts", err)
			}
		})
	}
	run("separate roles private metadata and invalid filters", func() {
		id := profile()
		for _, path := range []string{"/v1/agent/runtime-profiles", "/v1/agent/runtime-profiles/" + id} {
			r, v := f.request("GET", path, f.memberToken, "", nil, nil)
			expect(current, r, 200, v)
			r, v = f.request("GET", path, f.aliceToken, "", nil, nil)
			expect(current, r, 404, v)
		}
		retire(id, input(get(id)), f.memberToken, core.NewID(), nil, 404)
		for _, q := range []string{"state=bad", "skill_id=bad", "state=active&state=retired", "unknown=yes", "limit=0"} {
			r, v := f.request("GET", "/v1/agent/runtime-profiles?"+q, f.bobToken, "", nil, nil)
			expect(current, r, 400, v)
		}
		r, v := f.request("GET", "/v1/agent/runtime-profiles/"+core.NewID(), f.bobToken, "", nil, nil)
		expect(current, r, 404, v)
		r, v = f.request("GET", "/v1/agent/runtime-profiles/"+id+"?unknown=yes", f.bobToken, "", nil, nil)
		expect(current, r, 400, v)
	})
	run("explicit review current version and recent MFA", func() {
		id := profile()
		in := input(get(id))
		in.ReviewedImpact = false
		retire(id, in, f.bobToken, core.NewID(), nil, 400)
		in.ReviewedImpact = true
		in.ExpectedVersion++
		retire(id, in, f.bobToken, core.NewID(), nil, 409)
		in.ExpectedVersion--
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		retire(id, in, f.bobToken, core.NewID(), nil, 403)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
	})
	run("active task changes invalidate the reviewed impact", func() {
		id := profile()
		before := get(id)
		r := newRun(id)
		after := get(id)
		if after.Impact.Queued != 1 || before.Impact.ActiveBasisSHA256 == after.Impact.ActiveBasisSHA256 {
			current.Fatal("actual new run missing from impact")
		}
		retire(id, input(before), f.bobToken, core.NewID(), nil, 409)
		response, v := f.request("POST", "/v1/runs/"+r.ID+"/cancel", f.aliceToken, core.NewID(), agent.RunCancel{ExpectedVersion: r.Version, Action: "cancel", Reason: "结束合成任务"}, nil)
		expect(current, response, 200, v)
		retire(id, input(after), f.bobToken, core.NewID(), nil, 409)
		retire(id, input(get(id)), f.bobToken, core.NewID(), nil, 200)
	})
	run("retired profile rejects new runs and active execution authority", func() {
		id := profile()
		r := newRun(id)
		svc := &agent.Service{DB: f.api.DB}
		job, err := svc.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "runtime-141", agent.ExecutionLeaseDuration)
		if err != nil || job == nil {
			current.Fatal("missing actual job", err)
		}
		execution, err := svc.IssueExecution(ctx, *job)
		if err != nil || execution.RunID != r.ID {
			current.Fatal("actual execution was not issued", err)
		}
		lease, err := svc.LeaseForCredential(ctx, execution.Credential)
		if err != nil {
			current.Fatal(err)
		}
		if _, _, err = svc.ExecutionScope(ctx, lease); err != nil {
			current.Fatal("pre-retirement scope unavailable", err)
		}
		before := get(id)
		retire(id, input(before), f.bobToken, core.NewID(), nil, 200)
		if _, _, err = svc.ExecutionScope(ctx, lease); err == nil {
			current.Fatal("retired profile kept execution authority")
		}
		if err = svc.FinishStoppedExecution(ctx, *job, "RUNTIME_PROFILE_RETIRED"); err != nil {
			current.Fatal(err)
		}
		rr, v := f.request("POST", "/v1/conversations/"+r.ConversationID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: 1, ContextSecurityVersion: 1, ProfileID: id, ClientMessageID: core.NewID(), Message: "must reject retired profile"}, nil)
		expect(current, rr, 409, v)
	})
	run("audit failure rolls back retirement receipt and journal", func() {
		id := profile()
		in := input(get(id))
		exec(`CREATE FUNCTION fail_profile_141() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='agent.profile_retired' THEN RAISE EXCEPTION 'synthetic audit fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_profile_141 BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_profile_141()`)
		retire(id, in, f.bobToken, core.NewID(), nil, 500)
		exec("DROP TRIGGER fail_profile_141 ON audit_events; DROP FUNCTION fail_profile_141()")
		after := get(id)
		var n int
		err := f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_profile_retirements WHERE profile_id=$1", id).Scan(&n)
		if err != nil || n != 0 || after.Item.State != "active" || len(after.History) != 1 {
			current.Fatal("partial retirement escaped rollback", err)
		}
	})
	run("database installation boundary and immutable receipts", func() {
		id := profile()
		var write bool
		if err := f.owner.QueryRow(ctx, "SELECT has_table_privilege('copypaste_app','agent_runtime_profiles','INSERT') OR has_table_privilege('copypaste_app','agent_runtime_profiles','UPDATE') OR has_table_privilege('copypaste_app','agent_runtime_profiles','DELETE')").Scan(&write); err != nil || write {
			current.Fatal("application gained installation privilege", err)
		}
		rejected := func(q string, args ...any) {
			current.Helper()
			if _, err := f.api.DB.Exec(ctx, q, args...); err == nil {
				current.Fatal("forbidden database mutation accepted")
			}
		}
		rejected("INSERT INTO agent_profile_events(org_id,profile_id,version,state,source) VALUES($1,$2,20,'retired','retired')", f.org, id)
		rejected("UPDATE agent_runtime_profiles SET state='retired' WHERE id=$1", id)
		rejected(`INSERT INTO agent_profile_retirements(id,org_id,profile_id,expected_version,event_version,active_basis_sha256,reviewed_impact,actor_id,session_id,reason) SELECT $1,$2,$3,1,2,agent_profile_active_basis($2,$3),true,$4,id,'unauthorized reader' FROM sessions WHERE user_id=$4 AND revoked_at IS NULL LIMIT 1`, core.NewID(), f.org, id, f.member)
		retire(id, input(get(id)), f.bobToken, core.NewID(), nil, 200)
		rejected("UPDATE agent_profile_retirements SET reason='rewrite' WHERE profile_id=$1", id)
		rejected("DELETE FROM agent_profile_events WHERE profile_id=$1", id)
		if _, err := f.owner.Exec(ctx, "UPDATE agent_runtime_profiles SET state='active' WHERE id=$1", id); err == nil {
			current.Fatal("retired configuration reactivated")
		}
	})
	run("complete pagination binds filters and current reader", func() {
		r, v := f.request("GET", "/v1/agent/runtime-profiles?limit=1", f.bobToken, "", nil, nil)
		expect(current, r, 200, v)
		page := data[core.Page[agent.ProfileOperation]](current, v)
		if len(page.Items) != 1 || page.NextCursor == nil {
			current.Fatal("missing actual pagination")
		}
		for _, token := range []string{f.memberToken} {
			r, v = f.request("GET", "/v1/agent/runtime-profiles?limit=1&cursor="+*page.NextCursor, token, "", nil, nil)
			expect(current, r, 400, v)
		}
		r, v = f.request("GET", "/v1/agent/runtime-profiles?limit=1&state=active&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		expect(current, r, 400, v)
		seen := map[string]bool{page.Items[0].ID: true}
		for page.NextCursor != nil {
			r, v = f.request("GET", "/v1/agent/runtime-profiles?limit=1&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
			expect(current, r, 200, v)
			page = data[core.Page[agent.ProfileOperation]](current, v)
			for _, p := range page.Items {
				if seen[p.ID] {
					current.Fatal("duplicate history page")
				}
				seen[p.ID] = true
			}
		}
		if len(seen) < 9 {
			current.Fatal("incomplete installed configuration history")
		}
	})
	run("current role is required before successful replay", func() {
		id := profile()
		in := input(get(id))
		key := core.NewID()
		retire(id, in, f.bobToken, key, nil, 200)
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp(),version=version+1 WHERE id=$1", manager)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		retire(id, in, f.bobToken, key, nil, 404)
	})
	run("browser_workbench", func() {
		if os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") != "1" {
			return
		}
		bind(f.alice, "agent_runtime_manager")
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		id := profile()
		newRun(id)
		holdBrowserBusinessFixture(current, f, map[string]any{"fixture_kind": "agent-runtime-operations", "profile_id": id})
		var state string
		var events, receipts, audits int
		err := f.owner.QueryRow(ctx, `SELECT state,(SELECT count(*) FROM agent_profile_events WHERE profile_id=p.id),(SELECT count(*) FROM agent_profile_retirements WHERE profile_id=p.id),(SELECT count(*) FROM audit_events WHERE resource_id=p.id AND action='agent.profile_retired') FROM agent_runtime_profiles p WHERE id=$1`, id).Scan(&state, &events, &receipts, &audits)
		if err != nil || state != "retired" || events != 2 || receipts != 1 || audits != 1 {
			current.Fatal("browser did not persist exactly one retirement", err)
		}
	})
}
