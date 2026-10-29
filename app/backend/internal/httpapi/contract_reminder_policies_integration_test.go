package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/notification"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/workflow"
)

func TestContractReminderPolicies(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	bind := func(user, role string) {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, user, role)
	}
	admin := f.administratorToken()
	get := func(t *testing.T, token string, status int) notification.ContractPolicyState {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/notification-contract-policy", token, "", nil, nil)
		expect(t, r, status, v)
		if status != 200 {
			return notification.ContractPolicyState{}
		}
		return data[notification.ContractPolicyState](t, v)
	}
	post := func(t *testing.T, token, key string, in notification.ContractPolicyInput, status int) notification.ContractPolicy {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/notification-contract-policy", token, key, in, nil)
		expect(t, r, status, v)
		if status != 201 {
			return notification.ContractPolicy{}
		}
		return data[notification.ContractPolicy](t, v)
	}
	input := func() notification.ContractPolicyInput {
		s := get(t, admin, 200)
		v := int64(0)
		if s.Current != nil {
			v = s.Current.Version
		}
		return notification.ContractPolicyInput{ExpectedVersion: v, Enabled: true, Timezone: "Asia/Hong_Kong", DaysBefore: []int{7, 0}, Reason: "Synthetic explicit business reminder policy"}
	}
	service := &notification.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}, Pager: platform.Pager{Key: f.api.Config.CursorKey}}
	t.Run("new roles are independent and never auto bound", func(t *testing.T) {
		get(t, admin, 404)
		get(t, f.memberToken, 404)
		bind(f.admin, "notification_policy_manager")
		bind(f.bob, "notification_policy_reader")
		s := get(t, admin, 200)
		if s.Current != nil || !s.CanPublish {
			t.Fatal(s)
		}
		if get(t, f.bobToken, 200).CanPublish {
			t.Fatal("reader gained write")
		}
		post(t, f.bobToken, core.NewID(), input(), 404)
	})
	t.Run("strict input recent MFA and valid business rules", func(t *testing.T) {
		for _, change := range []func(*notification.ContractPolicyInput){func(v *notification.ContractPolicyInput) { v.Timezone = "Local" }, func(v *notification.ContractPolicyInput) { v.Timezone = "Asia/Unknown" }, func(v *notification.ContractPolicyInput) { v.DaysBefore = []int{7, 7} }, func(v *notification.ContractPolicyInput) { v.DaysBefore = []int{-1} }, func(v *notification.ContractPolicyInput) { v.DaysBefore = []int{366} }, func(v *notification.ContractPolicyInput) { v.DaysBefore = []int{} }, func(v *notification.ContractPolicyInput) { v.Reason = " " }} {
			v := input()
			change(&v)
			post(t, admin, core.NewID(), v, 422)
		}
		for _, path := range []string{"/v1/notification-contract-policy?limit=1", "/v1/notification-contract-policy-history?limit=1&limit=1", "/v1/notification-contract-policy-history?unknown=1", "/v1/notification-contract-policy-history?cursor=%zz"} {
			r, v := f.with(t).request("GET", path, admin, "", nil, nil)
			expect(t, r, 422, v)
		}
		r, v := f.with(t).request("POST", "/v1/notification-contract-policy", admin, core.NewID(), map[string]any{"expected_version": 0, "enabled": true, "timezone": "Asia/Hong_Kong", "days_before": []int{7}, "reason": "explicit", "recipients": []string{f.admin}}, nil)
		expect(t, r, 422, v)
		ids, e := identity.NewService(f.api.DB, f.api.Config)
		if e != nil {
			t.Fatal(e)
		}
		actor, e := ids.Authenticate(ctx, admin)
		if e != nil {
			t.Fatal(e)
		}
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		post(t, admin, core.NewID(), input(), 403)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE id=$1", actor.SessionID)
	})
	var first notification.ContractPolicy
	var initial notification.ContractPolicyInput
	key := core.NewID()
	t.Run("publish canonical revision with stable idempotency and atomic evidence", func(t *testing.T) {
		initial = input()
		initial.DaysBefore = []int{7, 0}
		first = post(t, admin, key, initial, 201)
		again := post(t, admin, key, initial, 201)
		a, _ := json.Marshal(first)
		b, _ := json.Marshal(again)
		if string(a) != string(b) || first.Version != 1 || len(first.DaysBefore) != 2 || first.DaysBefore[0] != 0 || first.CreatedBy != f.admin {
			t.Fatal(first, again)
		}
		var n int
		if e := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_events WHERE org_id=$1 AND resource_id=$2 AND action='notification.contract_policy.publish')+(SELECT count(*) FROM outbox_events WHERE org_id=$1 AND aggregate_id=$2 AND event_type='notification.contract_policy.publish')`, f.org, first.ID).Scan(&n); e != nil || n != 2 {
			t.Fatal(n, e)
		}
		post(t, admin, core.NewID(), initial, 409)
	})
	t.Run("organization policy does not grant contract content", func(t *testing.T) {
		r, v := f.with(t).request("GET", "/v1/files/"+file.File.ID, admin, "", nil, nil)
		expect(t, r, 404, v)
	})

	// Real synthetic contracts go through draft, independent approval, fixed signed
	// file and explicit activation. Scheduling never relies on SQL-forged lifecycle.
	for _, role := range []string{"contract_manager", "contract_operator", "party_manager", "request_manager"} {
		bind(f.alice, role)
	}
	bind(f.bob, "approval_reviewer")
	bind(f.bob, "contract_reader")
	bind(f.bob, "party_manager")
	f.aliceToken = memberMFAToken(f, "alice", "android")
	r, v := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "reminder-policy", Name: "Synthetic reminder contract review", Kind: "contract", Category: "reminder", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent review", AssignedUserID: f.bob}}, Reason: "isolated explicit workflow"}, nil)
	expect(t, r, 201, v)
	r, v = f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic reminder counterparty", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated reminder fixture"}, nil)
	expect(t, r, 201, v)
	supplier := data[party.Party](t, v)
	var today string
	if e := f.owner.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today); e != nil {
		t.Fatal(e)
	}
	clock, e := time.Parse(time.DateOnly, today)
	if e != nil {
		t.Fatal(e)
	}
	ptr := func(s string) *string { return &s }
	contract := func(title string, lead int) contracts.Contract {
		t.Helper()
		start := clock.AddDate(0, 0, -40).Format(time.DateOnly)
		end := clock.AddDate(0, 0, lead).Format(time.DateOnly)
		r, v := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), contracts.Create{Content: contracts.Content{Title: title, Direction: ptr("payable"), Type: ptr("service"), Category: "reminder", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.alice, Currency: ptr("HKD"), Amount: ptr("12.34"), StartDate: &start, EndDate: &end, DraftFileVersionID: &file.Version.ID}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated effective reminder contract"}, nil)
		expect(t, r, 201, v)
		c := data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "contract", BusinessID: c.ID, ExpectedBusinessVersion: c.Version, BusinessRevisionID: c.CurrentRevisionID, BusinessContentHash: c.ContentHash, Reason: "exact contract approval"}, nil)
		expect(t, r, 201, v)
		req := data[workflow.Request](t, v)
		r, v = f.request("GET", "/v1/requests/"+req.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.request("POST", "/v1/requests/"+req.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: req.Version, RevisionID: req.CurrentRevisionID, ContentHash: req.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "explicit exact submission"}, nil)
		expect(t, r, 200, v)
		req = data[workflow.Request](t, v)
		r, v = f.request("POST", "/v1/approval-steps/"+req.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: req.Steps[0].Version, RequestVersion: req.Version, RevisionID: req.CurrentRevisionID, ContentHash: req.ContentHash, Decision: "approve", Reason: "independent approval"}, nil)
		expect(t, r, 200, v)
		r, v = f.request("GET", "/v1/contracts/"+c.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		c = data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/contracts/"+c.ID+"/signature", f.aliceToken, core.NewID(), contracts.SignatureInput{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, SignedOn: start, SignedFileVersionID: file.Version.ID, Attestation: "Synthetic exact signed file", Reason: "explicit signature"}, nil)
		expect(t, r, 201, v)
		c = data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/contracts/"+c.ID+"/transition", f.aliceToken, core.NewID(), contracts.ActivationInput{ExpectedVersion: c.Version, Action: "activate", RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, SignatureID: c.Signature.ID, Reason: "explicit activation"}, nil)
		expect(t, r, 200, v)
		return data[contracts.Contract](t, v)
	}
	due := contract("Synthetic contract due in seven days", 7)
	historic := contract("Synthetic old contract excluded from policy backfill", -1)
	fallback := notification.ContractRule{OrgID: f.org, Timezone: "UTC", DaysBefore: []int{365}}
	t.Run("real scheduler uses published policy and excludes prepublication thresholds", func(t *testing.T) {
		out, e := service.ScheduleContracts(ctx, fallback)
		if e != nil || !out.Enabled || out.Queued != 1 {
			t.Fatal(out, e)
		}
		var policyID, tz string
		var advance int
		if e = f.owner.QueryRow(ctx, "SELECT policy_id,timezone,days_before FROM notification_due_events WHERE org_id=$1 AND contract_id=$2", f.org, due.ID).Scan(&policyID, &tz, &advance); e != nil || policyID != first.ID || tz != "Asia/Hong_Kong" || advance != 7 {
			t.Fatal(policyID, tz, advance, e)
		}
		var count int
		if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_due_events WHERE org_id=$1 AND contract_id=$2", f.org, historic.ID).Scan(&count); e != nil || count != 0 {
			t.Fatal(count, e)
		}
		_, queued, e := service.ScheduleContractPolicies(ctx, "")
		if e != nil || queued != 0 {
			t.Fatal(queued, e)
		}
	})
	t.Run("leased delivery rechecks actual owner and remains idempotent", func(t *testing.T) {
		drainNotifications(t, f)
		requireBusinessNotification(t, f, f.aliceToken, due.ID, "contract_due", true)
		if notificationFor(t, f, f.bobToken, due.ID) != nil {
			t.Fatal("reader received owner reminder")
		}
		out, e := service.ScheduleContracts(ctx, fallback)
		if e != nil || out.Queued != 0 {
			t.Fatal(out, e)
		}
	})
	t.Run("disable overrides legacy and keeps queued or delivered facts", func(t *testing.T) {
		in := input()
		in.Enabled = false
		in.DaysBefore = []int{}
		p := post(t, admin, core.NewID(), in, 201)
		if p.Enabled || p.Version != 2 || p.DaysBefore == nil {
			t.Fatal(p)
		}
		_ = contract("Synthetic due contract remains unscheduled while disabled", 0)
		out, e := service.ScheduleContracts(ctx, fallback)
		if e != nil || out.Enabled || out.Queued != 0 {
			t.Fatal(out, e)
		}
		requireBusinessNotification(t, f, f.aliceToken, due.ID, "contract_due", true)
	})
	t.Run("two writers cannot publish from the same expected version", func(t *testing.T) {
		in := input()
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.with(t).request("POST", "/v1/notification-contract-policy", admin, core.NewID(), in, nil)
				codes <- r.Code
			}()
		}
		wg.Wait()
		close(codes)
		counts := map[int]int{}
		for code := range codes {
			counts[code]++
		}
		if counts[201] != 1 || counts[409] != 1 {
			t.Fatal(counts)
		}
	})
	t.Run("history is immutable paged and cursor bound", func(t *testing.T) {
		r, v := f.with(t).request("GET", "/v1/notification-contract-policy-history?limit=1", admin, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[notification.ContractPolicy]](t, v)
		if len(page.Items) != 1 || page.Items[0].Version != 3 || page.NextCursor == nil {
			t.Fatal(page)
		}
		r, v = f.with(t).request("GET", "/v1/notification-contract-policy-history?limit=1&cursor="+*page.NextCursor, admin, "", nil, nil)
		expect(t, r, 200, v)
		if data[core.Page[notification.ContractPolicy]](t, v).Items[0].Version != 2 {
			t.Fatal(v)
		}
		r, v = f.with(t).request("GET", "/v1/notification-contract-policy-history?limit=1&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		expect(t, r, 400, v)
		for _, q := range []string{"UPDATE notification_contract_policies SET enabled=false WHERE id=$1", "DELETE FROM notification_contract_policies WHERE id=$1"} {
			if _, e := f.owner.Exec(ctx, q, first.ID); e == nil {
				t.Fatal("history changed")
			}
		}
	})
	t.Run("four human channels allowed but runtime TG sessions rejected", func(t *testing.T) {
		bind(f.member, "notification_policy_reader")
		for _, channel := range []string{"web", "wechat", "douyin", "android"} {
			if channel != "web" {
				get(t, f.login("member", channel).AccessToken, 200)
				continue
			}
			r, v := f.with(t).request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "member", Password: "member-test-password-long", Device: "web"}, map[string]string{"Origin": "http://localhost:5178"})
			expect(t, r, 200, v)
			cookies := []string{}
			for _, cookie := range r.Result().Cookies() {
				cookies = append(cookies, cookie.Name+"="+cookie.Value)
			}
			r, v = f.with(t).request("GET", "/v1/notification-contract-policy", "", "", nil, map[string]string{"Cookie": strings.Join(cookies, "; ")})
			expect(t, r, 200, v)
		}
		ids, e := identity.NewService(f.api.DB, f.api.Config)
		if e != nil {
			t.Fatal(e)
		}
		a, e := ids.Authenticate(ctx, admin)
		if e != nil {
			t.Fatal(e)
		}
		for _, channel := range []string{"agent", "telegram"} {
			copy := a
			copy.Channel = channel
			copy.RunID = core.NewID()
			if _, e = service.ContractPolicy(ctx, copy); e == nil {
				t.Fatal("executor read policy")
			}
		}
	})
	t.Run("policy requires atomic evidence and valid database rules", func(t *testing.T) {
		current := get(t, admin, 200).Current
		for _, days := range []string{"ARRAY[7,7]", "ARRAY[366]", "ARRAY[7,0]", "ARRAY[NULL]::int[]", "ARRAY[0]"} {
			tx, e := f.owner.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			_, e = tx.Exec(ctx, `INSERT INTO notification_contract_policies(id,org_id,version,enabled,timezone,days_before,effective_on,created_by,reason,created_at) SELECT $1,$2,$3,true,'Asia/Hong_Kong',`+days+`,(stamp AT TIME ZONE 'Asia/Hong_Kong')::date,$4,'synthetic missing evidence',stamp FROM (SELECT clock_timestamp() stamp) at`, core.NewID(), f.org, current.Version+1, f.admin)
			if e == nil {
				e = tx.Commit(ctx)
			}
			_ = tx.Rollback(ctx)
			if e == nil {
				t.Fatal("database accepted invalid policy or missing evidence", days)
			}
		}
	})
	t.Run("audit failure rolls back policy and replay rechecks current authority", func(t *testing.T) {
		version := get(t, admin, 200).Current.Version
		exec(`CREATE FUNCTION reject_reminder_audit_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='notification.contract_policy.publish' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END; $$; CREATE TRIGGER reject_reminder_audit_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_reminder_audit_fixture()`)
		post(t, admin, core.NewID(), input(), 500)
		exec("DROP TRIGGER reject_reminder_audit_fixture ON audit_events; DROP FUNCTION reject_reminder_audit_fixture()")
		if get(t, admin, 200).Current.Version != version {
			t.Fatal("partial policy survived failed audit")
		}
		exec("UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND user_id=$2 AND role_id IN (SELECT id FROM roles WHERE org_id=$1 AND key='notification_policy_manager')", f.org, f.admin)
		post(t, admin, key, initial, 404)
		exec("UPDATE role_bindings SET valid_until=NULL WHERE org_id=$1 AND user_id=$2 AND role_id IN (SELECT id FROM roles WHERE org_id=$1 AND key='notification_policy_manager')", f.org, f.admin)
	})
	if !t.Failed() && (os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") == "1" || os.Getenv("CP_BROWSER_MOBILE_FIXTURE") == "1") {
		bind(f.alice, "notification_policy_manager")
		// The fixed publication evidence precedes the browser's own independent edits.
		holdBrowserBusinessFixture(t, f, map[string]any{"contract_reminders_fixture": true, "current_policy_version": get(t, admin, 200).Current.Version, "contract_id": due.ID, "historic_contract_id": historic.ID, "project_id": file.File.ProjectID})
	}
}
