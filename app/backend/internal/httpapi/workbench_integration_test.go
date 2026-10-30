package httpapi

import (
	"context"
	"os"
	"testing"
	"time"

	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/workbench"
	"copypaste.hk/platform/internal/workflow"
	"strings"
)

func TestBusinessWorkbench(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	bind := func(user, key string) string {
		id := core.NewID()
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	read := func(t *testing.T, token string) workbench.Summary {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/dashboard/business", token, "", nil, nil)
		expect(t, r, 200, v)
		return data[workbench.Summary](t, v)
	}
	group := func(t *testing.T, value workbench.Summary, kind string) workbench.Group {
		t.Helper()
		for _, g := range value.Groups {
			if g.Kind == kind {
				return g
			}
		}
		t.Fatal("missing group", kind)
		return workbench.Group{}
	}
	t.Run("strict read has no caller-controlled actor filters and empty authorized work", func(t *testing.T) {
		v := read(t, f.memberToken)
		if len(v.Groups) != 6 || v.Scope != "my_readable_work" || v.ContractTimezone != "Asia/Hong_Kong" {
			t.Fatal(v)
		}
		for _, g := range v.Groups {
			if g.Count != 0 || len(g.Items) != 0 || g.HasMore {
				t.Fatal(g)
			}
		}
		for _, q := range []string{"?owner_id=" + f.alice, "?limit=1", "?kind=contracts", "?actor=x&actor=y", "?%zz"} {
			r, v := f.request("GET", "/v1/dashboard/business"+q, f.memberToken, "", nil, nil)
			expect(t, r, 422, v)
		}
		r, vv := f.request("GET", "/v1/dashboard/business", "", "", nil, nil)
		expect(t, r, 401, vv)
	})
	for _, key := range []string{"contract_manager", "contract_operator", "party_manager", "request_manager", "subscription_manager"} {
		bind(f.bob, key)
	}
	for _, key := range []string{"party_manager", "request_manager", "subscription_reader"} {
		bind(f.alice, key)
	}
	contractReadBinding := bind(f.alice, "contract_reader")
	reviewBinding := bind(f.alice, "approval_reviewer")
	f.bobToken = memberMFAToken(f, "bob", "android")
	admin := f.administratorToken()
	r, v := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "workbench", Name: "Synthetic workbench independent review", Kind: "contract", Category: "workbench", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent review", AssignedUserID: f.alice}}, Reason: "isolated explicit workflow"}, nil)
	expect(t, r, 201, v)
	r, v = f.request("POST", "/v1/parties", f.bobToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic workbench supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated source basis"}, nil)
	expect(t, r, 201, v)
	supplier := data[party.Party](t, v)
	var today string
	if err := f.owner.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today); err != nil {
		t.Fatal(err)
	}
	clock, err := time.Parse(time.DateOnly, today)
	if err != nil {
		t.Fatal(err)
	}
	ptr := func(s string) *string { return &s }
	contract := func(title string, lead int, effective bool) (contracts.Contract, workflow.Request) {
		t.Helper()
		start := clock.AddDate(0, 0, -40).Format(time.DateOnly)
		end := clock.AddDate(0, 0, lead).Format(time.DateOnly)
		r, v := f.request("POST", "/v1/contracts", f.bobToken, core.NewID(), contracts.Create{Content: contracts.Content{Title: title, Direction: ptr("payable"), Type: ptr("service"), Category: "workbench", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.alice, Currency: ptr("HKD"), Amount: ptr("12.34"), StartDate: &start, EndDate: &end, DraftFileVersionID: &file.Version.ID}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "synthetic exact workbench contract"}, nil)
		expect(t, r, 201, v)
		c := data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/requests", f.bobToken, core.NewID(), workflow.RequestCreate{Kind: "contract", BusinessID: c.ID, ExpectedBusinessVersion: c.Version, BusinessRevisionID: c.CurrentRevisionID, BusinessContentHash: c.ContentHash, Reason: "exact independent review"}, nil)
		expect(t, r, 201, v)
		req := data[workflow.Request](t, v)
		r, v = f.request("GET", "/v1/requests/"+req.ID+"/submission-preview", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.request("POST", "/v1/requests/"+req.ID+"/submit", f.bobToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: req.Version, RevisionID: req.CurrentRevisionID, ContentHash: req.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "explicit submit"}, nil)
		expect(t, r, 200, v)
		req = data[workflow.Request](t, v)
		if !effective {
			return c, req
		}
		r, v = f.request("POST", "/v1/approval-steps/"+req.Steps[0].ID+"/decisions", f.aliceToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: req.Steps[0].Version, RequestVersion: req.Version, RevisionID: req.CurrentRevisionID, ContentHash: req.ContentHash, Decision: "approve", Reason: "independent exact approval"}, nil)
		expect(t, r, 200, v)
		r, v = f.request("GET", "/v1/contracts/"+c.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		c = data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/contracts/"+c.ID+"/signature", f.bobToken, core.NewID(), contracts.SignatureInput{ExpectedVersion: c.Version, RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, SignedOn: start, SignedFileVersionID: file.Version.ID, Attestation: "Synthetic fixed signature", Reason: "explicit signature"}, nil)
		expect(t, r, 201, v)
		c = data[contracts.Contract](t, v)
		r, v = f.request("POST", "/v1/contracts/"+c.ID+"/transition", f.bobToken, core.NewID(), contracts.ActivationInput{ExpectedVersion: c.Version, Action: "activate", RevisionID: c.CurrentRevisionID, ContentHash: c.ContentHash, SignatureID: c.Signature.ID, Reason: "explicit activation"}, nil)
		expect(t, r, 200, v)
		return data[contracts.Contract](t, v), req
	}
	var pending []workflow.Request
	t.Run("only currently assigned active approvals are counted before bounded samples", func(t *testing.T) {
		for range 7 {
			_, req := contract("待我审批：<b>合成</b>服务合同", 7, false)
			pending = append(pending, req)
		}
		g := group(t, read(t, f.aliceToken), "approvals")
		if g.Count != 7 || len(g.Items) != 5 || !g.HasMore {
			t.Fatal(g)
		}
		r, v := f.request("GET", "/v1/approval-tasks?limit=100", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		ordinary := data[core.Page[workflow.ApprovalStep]](t, v)
		if int64(len(ordinary.Items)) != g.Count {
			t.Fatal("queue count differs", ordinary, g)
		}
		for i, item := range g.Items {
			if item.ID != ordinary.Items[i].RequestID || item.RelatedID == nil || *item.RelatedID != ordinary.Items[i].ID {
				t.Fatal("wrong exact queue order", g, ordinary)
			}
		}
		if group(t, read(t, f.bobToken), "approvals").Count != 0 {
			t.Fatal("applicant inherited another person's steps")
		}
	})
	var due, overdue contracts.Contract
	t.Run("active contract revision owner and inclusive Hong Kong horizon define expiry", func(t *testing.T) {
		due, _ = contract("我负责：30天边界合同", 30, true)
		overdue, _ = contract("我负责：已到期合同", -1, true)
		_, _ = contract("不在30天内：31天合同", 31, true)
		s := read(t, f.aliceToken)
		g := group(t, s, "contracts")
		if g.Count != 2 || g.HasMore || g.Items[0].ID != overdue.ID || g.Items[0].State != "overdue" || g.Items[1].ID != due.ID || g.Items[1].State != "due_soon" || g.Items[1].RevisionID != *due.ActiveRevisionID || s.ContractThrough != *due.EndDate {
			t.Fatal(s)
		}
		if group(t, read(t, f.bobToken), "contracts").Count != 0 {
			t.Fatal("creator replaced an explicit owner")
		}
	})
	var sub finance.Subscription
	var periodID string
	t.Run("only generated due owned billing periods need review without fabricating cash", func(t *testing.T) {
		anchor := clock.Day()
		first := clock.Format(time.DateOnly)
		input := finance.SubscriptionCreate{SubscriptionContent: finance.SubscriptionContent{Product: "我负责：订阅账期待核对", Supplier: &finance.PartyRef{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, OwnerID: &f.alice, Category: "workbench", StartDate: &first, Cycle: ptr("month"), Timezone: "Asia/Hong_Kong", FirstChargeDate: &first, AnchorDay: &anchor, RenewalMode: "automatic", VendorState: "unknown", ReminderDays: []int{0}, VendorEvidenceVersionIDs: []string{}, Prices: []finance.SubscriptionPriceInput{{EffectiveFrom: first, Currency: "HKD", Amount: "100", Plan: "Synthetic monthly"}}}, SourceKey: "manual:" + core.NewID(), ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "synthetic billing source"}
		r, v := f.request("POST", "/v1/subscriptions", f.bobToken, core.NewID(), input, nil)
		expect(t, r, 201, v)
		sub = data[finance.Subscription](t, v)
		base := func() finance.Base {
			return finance.Base{ExpectedVersion: sub.Version, RevisionID: sub.CurrentRevisionID, ContentHash: sub.ContentHash, Reason: "explicit current basis"}
		}
		r, v = f.request("POST", "/v1/subscriptions/"+sub.ID+"/transition", f.bobToken, core.NewID(), finance.SubscriptionTransition{Base: base(), Action: "activate", VendorEvidenceVersionIDs: []string{}}, nil)
		expect(t, r, 200, v)
		sub = data[finance.Subscription](t, v)
		if group(t, read(t, f.aliceToken), "subscription_periods").Count != 0 {
			t.Fatal("invented ungenerated period")
		}
		r, v = f.request("POST", "/v1/subscriptions/"+sub.ID+"/periods/generate", f.bobToken, core.NewID(), finance.SubscriptionGenerate{Base: base(), From: first, To: clock.AddDate(0, 2, 0).Format(time.DateOnly)}, nil)
		expect(t, r, 200, v)
		g := group(t, read(t, f.aliceToken), "subscription_periods")
		if g.Count != 1 || g.Items[0].State != "awaiting_confirmation" || g.Items[0].DueOn == nil || *g.Items[0].DueOn != first || g.Items[0].RelatedID == nil || *g.Items[0].RelatedID != sub.ID {
			t.Fatal(g)
		}
		periodID = g.Items[0].ID
		r, v = f.request("GET", "/v1/subscription-periods?state=awaiting_confirmation&owner_id="+f.alice, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[finance.SubscriptionPeriod]](t, v).Items) != 1 {
			t.Fatal("ordinary review list differs", v)
		}
	})
	t.Run("summary reads mutate no domain audit notification or financial evidence", func(t *testing.T) {
		fingerprint := func() string {
			t.Helper()
			var s string
			err := f.owner.QueryRow(ctx, `SELECT md5(jsonb_build_array((SELECT count(*) FROM audit_events),(SELECT count(*) FROM outbox_events),(SELECT count(*) FROM subscription_periods),(SELECT count(*) FROM actual_cost_entries),(SELECT count(*) FROM payment_records),(SELECT count(*) FROM notification_due_events),(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM subscription_periods p),(SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM contracts c))::text)`).Scan(&s)
			if err != nil {
				t.Fatal(err)
			}
			return s
		}
		before := fingerprint()
		for range 3 {
			read(t, f.aliceToken)
		}
		if fingerprint() != before {
			t.Fatal("read caused a business side effect")
		}
	})
	t.Run("completed steps disappear without changing their historical facts", func(t *testing.T) {
		req := pending[0]
		r, v := f.request("POST", "/v1/approval-steps/"+req.Steps[0].ID+"/decisions", f.aliceToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: req.Steps[0].Version, RequestVersion: req.Version, RevisionID: req.CurrentRevisionID, ContentHash: req.ContentHash, Decision: "reject", Reason: "synthetic current step decision"}, nil)
		expect(t, r, 200, v)
		g := group(t, read(t, f.aliceToken), "approvals")
		if g.Count != 6 || len(g.Items) != 5 || !g.HasMore {
			t.Fatal(g)
		}
		for _, item := range g.Items {
			if item.ID == req.ID {
				t.Fatal("completed step persisted")
			}
		}
	})
	t.Run("revoked domain authority removes counts and unrelated readers see no work", func(t *testing.T) {
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE id=$1", reviewBinding)
		if group(t, read(t, f.aliceToken), "approvals").Count != 0 {
			t.Fatal("revoked approval authority leaked count")
		}
		reviewBinding = bind(f.alice, "approval_reviewer")
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE id=$1", contractReadBinding)
		if group(t, read(t, f.aliceToken), "contracts").Count != 0 || group(t, read(t, f.aliceToken), "approvals").Count != 0 {
			t.Fatal("missing business-read authority leaked work")
		}
		contractReadBinding = bind(f.alice, "contract_reader")
		for _, g := range read(t, f.memberToken).Groups {
			if g.Count != 0 {
				t.Fatal("unrelated responsible work leaked", g)
			}
		}
	})
	t.Run("runtime TG stale authority and revoked sessions cannot read human work", func(t *testing.T) {
		a, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		service := workbench.Service{DB: f.api.DB}
		for _, change := range []func(*core.Actor){func(v *core.Actor) { v.RunID = core.NewID() }, func(v *core.Actor) { v.Channel = "telegram" }, func(v *core.Actor) { v.AuthVersion++ }, func(v *core.Actor) { v.OrgAuthVersion++ }, func(v *core.Actor) { v.SessionID = core.NewID() }} {
			copy := a
			change(&copy)
			if _, err := service.Read(ctx, copy); err == nil {
				t.Fatal("stale or executor actor accepted")
			}
		}
	})
	t.Run("an actual revoked session and canceled read return no successful snapshot", func(t *testing.T) {
		token := f.login("member", "douyin").AccessToken
		actor, err := f.api.Identity.Authenticate(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE org_id=$1 AND id=$2", f.org, actor.SessionID)
		service := workbench.Service{DB: f.api.DB}
		if _, err := service.Read(ctx, actor); err == nil {
			t.Fatal("revoked session accepted")
		}
		actor, err = f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := service.Read(canceled, actor); err == nil {
			t.Fatal("canceled read fabricated a snapshot")
		}
	})
	t.Run("four interactive client channels share current readable work", func(t *testing.T) {
		for _, channel := range []string{"wechat", "douyin", "android"} {
			token := f.login("member", channel).AccessToken
			s := read(t, token)
			for _, g := range s.Groups {
				if g.Count != 0 {
					t.Fatal(channel, s)
				}
			}
		}
		r, v := f.request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "member", Password: "member-test-password-long", Device: "web"}, map[string]string{"Origin": "http://localhost:5178"})
		expect(t, r, 200, v)
		cookies := []string{}
		for _, cookie := range r.Result().Cookies() {
			cookies = append(cookies, cookie.Name+"="+cookie.Value)
		}
		r, v = f.request("GET", "/v1/dashboard/business", "", "", nil, map[string]string{"Cookie": strings.Join(cookies, "; ")})
		expect(t, r, 200, v)
		for _, g := range data[workbench.Summary](t, v).Groups {
			if g.Count != 0 {
				t.Fatal("browser inherited another person's work")
			}
		}

	})
	if os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") == "1" {
		enrollmentRole := bind(f.alice, "file_scan_reviewer")
		f.aliceToken = memberMFAToken(f, "alice", "android")
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE id=$1", enrollmentRole)

		// Restore only the fixture membership needed to exercise links through
		// ordinary APIs; this is not a production data change.
		exec("INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'member') ON CONFLICT DO NOTHING", f.org, *file.File.ProjectID, f.bob)
		values := map[string]any{"workbench_fixture": true, "workbench_review_binding": reviewBinding, "project_id": *file.File.ProjectID, "contract_id": due.ID, "overdue_contract_id": overdue.ID, "request_id": pending[1].ID, "subscription_id": sub.ID, "period_id": periodID, "fixture_kind": "human-business-workbench"}
		// The shared holder privately stores the synthetic credentials and owns
		// its API lifecycle. Neither secrets nor real records enter evidence.
		holdBrowserBusinessFixture(t, f, values)
	}
}
