package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/workflow"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSequentialApprovals(t *testing.T) {
	u, old, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	bind := func(t *testing.T, user, key string) string {
		t.Helper()
		id := core.NewID()
		exec(t, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
        SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	for _, key := range []string{"contract_manager", "party_manager", "request_manager"} {
		bind(t, f.alice, key)
	}
	bobReview, bobParty := "", ""
	for _, user := range []string{f.bob, f.member} {
		bind(t, user, "contract_reader")
		p := bind(t, user, "party_manager")
		b := bind(t, user, "approval_reviewer")
		if user == f.bob {
			bobReview = b
			bobParty = p
		}
	}
	exec(t, "INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'member')", f.org, *old.File.ProjectID, f.member)
	// Obtain genuine MFA sessions, then remove the temporary MFA-enrollment role.
	for _, entry := range []struct{ user, login, channel string }{{f.bob, "bob", "android"}, {f.member, "member", "wechat"}} {
		id := bind(t, entry.user, "file_scan_reviewer")
		token := memberMFAToken(f, entry.login, entry.channel)
		exec(t, "UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE id=$1", id)
		if entry.user == f.bob {
			f.bobToken = token
		} else {
			f.memberToken = token
		}
	}
	admin := f.administratorToken()
	text := func(s string) *string { return &s }
	rec, res := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic approval counterparty", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: old.File.ProjectID, Classification: "project_restricted", Reason: "isolated approval fixture"}, nil)
	expect(t, rec, 201, res)
	p := data[party.Party](t, res)
	complete := contracts.Create{Content: contracts.Content{Title: "Contract awaiting real approval", Direction: text("payable"), Type: text("service"), Category: "initial", CounterpartyID: &p.ID, CounterpartyRevisionID: &p.CurrentRevisionID,
		OwnerID: &f.alice, Currency: text("HKD"), Amount: text("12.34"), StartDate: text("2026-11-01"), EndDate: text("2027-10-31"), DraftFileVersionID: &old.Version.ID}, ProjectID: old.File.ProjectID, Classification: "project_restricted", Reason: "isolated complete contract"}
	getContract := func(t *testing.T, id string) contracts.Contract {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/contracts/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[contracts.Contract](t, v)
	}
	getRequest := func(t *testing.T, id, token string) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/requests/"+id, token, "", nil, nil)
		expect(t, r, 200, v)
		return data[workflow.Request](t, v)
	}
	newDraft := func(t *testing.T, category string, incomplete bool) (contracts.Contract, workflow.Request) {
		t.Helper()
		in := complete
		in.Category = category
		if incomplete {
			in.Currency = nil
			in.Amount = nil
			in.DraftFileVersionID = nil
		}
		r, v := f.with(t).request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		d := data[contracts.Contract](t, v)
		input := workflow.RequestCreate{Kind: "contract", BusinessID: d.ID, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, Reason: "prepare fixed request draft"}
		r, v = f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), input, nil)
		expect(t, r, 201, v)
		return getContract(t, d.ID), data[workflow.Request](t, v)
	}
	publish := func(t *testing.T, category string, steps []workflow.StepInput, self string, withdraw bool, version int64) workflow.Definition {
		t.Helper()
		in := workflow.DefinitionInput{ExpectedVersion: version, Key: "approval." + category, Name: "Isolated approval " + category, Kind: "contract", Category: category, Currency: "HKD", MinAmount: "0", Enabled: true,
			SelfApprovalPolicy: self, WithdrawalAllowed: withdraw, Steps: steps, Reason: "explicit isolated policy only"}
		r, v := f.with(t).request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
		expect(t, r, 201, v)
		return data[workflow.Definition](t, v)
	}
	standard := []workflow.StepInput{{Title: "First review", AssignedUserID: f.bob, RequiresMFA: true}, {Title: "Second review", AssignedUserID: f.member, RequiresMFA: false}}
	preview := func(t *testing.T, r workflow.Request) workflow.SubmissionPreview {
		t.Helper()
		rec, res := f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		return data[workflow.SubmissionPreview](t, res)
	}
	submitInput := func(r workflow.Request, p workflow.SubmissionPreview) workflow.SubmitInput {
		return workflow.SubmitInput{ExpectedVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, WorkflowDefinitionID: p.Workflow.ID, PreviewHash: p.PreviewHash, Reason: "human confirms the fixed preview"}
	}
	submit := func(t *testing.T, r workflow.Request) workflow.Request {
		t.Helper()
		p := preview(t, r)
		rec, res := f.with(t).request("POST", "/v1/requests/"+r.ID+"/submit", f.aliceToken, core.NewID(), submitInput(r, p), nil)
		expect(t, rec, 200, res)
		return data[workflow.Request](t, res)
	}
	decision := func(r workflow.Request, step workflow.ApprovalStep, kind string) workflow.DecisionInput {
		return workflow.DecisionInput{ExpectedVersion: step.Version, RequestVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, Decision: kind, Reason: "human decision on exact snapshot"}
	}
	decide := func(t *testing.T, r workflow.Request, seq int, token, kind, key string) workflow.Request {
		t.Helper()
		rec, res := f.with(t).request("POST", "/v1/approval-steps/"+r.Steps[seq].ID+"/decisions", token, key, decision(r, r.Steps[seq], kind), nil)
		expect(t, rec, 200, res)
		return data[workflow.Request](t, res)
	}
	var mainContract contracts.Contract
	var main workflow.Request
	var firstInput workflow.DecisionInput
	var firstKey string
	t.Run("drafts are explicit and incomplete or unmatched submissions cannot invent a policy", func(t *testing.T) {
		_, r := newDraft(t, "incomplete", true)
		rec, res := f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 422, res)
		_, r = newDraft(t, "missing", false)
		rec, res = f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "WORKFLOW_RULE_REQUIRED" {
			t.Fatal(res.Error)
		}
		rec, res = f.with(t).request("GET", "/v1/requests/"+r.ID, admin, "", nil, nil)
		expect(t, rec, 404, res)
	})
	t.Run("submission fixes the exact policy and file and rejects stale preview without partial instances", func(t *testing.T) {
		policy := publish(t, "sequential", standard, "deny", true, 0)
		mainContract, main = newDraft(t, "sequential", false)
		p := preview(t, main)
		bad := submitInput(main, p)
		bad.PreviewHash = strings.Repeat("0", 64)
		rec, res := f.with(t).request("POST", "/v1/requests/"+main.ID+"/submit", f.aliceToken, core.NewID(), bad, nil)
		expect(t, rec, 409, res)
		var n int
		err := f.owner.QueryRow(ctx, "SELECT count(*) FROM approval_instances WHERE request_id=$1", main.ID).Scan(&n)
		if err != nil || n != 0 {
			t.Fatal(n, err)
		}
		main = submit(t, main)
		if main.State != "in_review" || main.Workflow == nil || main.Workflow.ID != policy.ID || len(main.Steps) != 2 || main.Steps[0].State != "active" || main.Steps[1].State != "pending" || main.Payload.Attachments[0].VersionID != old.Version.ID {
			t.Fatal(main)
		}
		if _, err = f.api.DB.Exec(ctx, "UPDATE request_revisions SET reason='tamper' WHERE id=$1", main.CurrentRevisionID); err == nil {
			t.Fatal("mutable request snapshot")
		}
		err = f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type='approval_snapshot' AND owner_id=$1 AND version_id=$2", main.CurrentRevisionID, old.Version.ID).Scan(&n)
		if err != nil || n != 1 {
			t.Fatal(n, err)
		}
		current := getContract(t, mainContract.ID)
		rec, res = f.with(t).request("PATCH", "/v1/contracts/"+current.ID, f.aliceToken, core.NewID(), contracts.Patch{Content: current.Content, ExpectedVersion: current.Version, BaseRevisionID: current.CurrentRevisionID, Reason: "submitted evidence must be read only"}, nil)
		expect(t, rec, 409, res)
	})
	t.Run("two manual reviewers advance one step and concurrent final decisions have one winner", func(t *testing.T) {
		firstKey = core.NewID()
		firstInput = decision(main, main.Steps[0], "approve")
		before := main
		main = decide(t, main, 0, f.bobToken, "approve", firstKey)
		if main.State != "in_review" || main.Steps[0].State != "approved" || main.Steps[1].State != "active" {
			t.Fatal(main)
		}
		rec, res := f.with(t).request("POST", "/v1/approval-steps/"+before.Steps[0].ID+"/decisions", f.bobToken, firstKey, firstInput, nil)
		expect(t, rec, 200, res)
		if data[workflow.Request](t, res).Version != main.Version {
			t.Fatal("replayed duplicate decision")
		}
		input := decision(main, main.Steps[1], "approve")
		codes := make(chan int, 2)
		var wg sync.WaitGroup
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, _ := f.request("POST", "/v1/approval-steps/"+main.Steps[1].ID+"/decisions", f.memberToken, core.NewID(), input, nil)
				codes <- r.Code
			}()
		}
		wg.Wait()
		close(codes)
		success, conflict := 0, 0
		for code := range codes {
			if code == 200 {
				success++
			} else if code == 409 {
				conflict++
			} else {
				t.Fatal(code)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal(success, conflict)
		}
		main = getRequest(t, main.ID, f.aliceToken)
		d := getContract(t, mainContract.ID)
		if main.State != "approved" || d.Lifecycle != "pending_signature" || d.ApprovedRevisionID == nil || *d.ApprovedRevisionID != main.Payload.BusinessRevisionID || d.ActiveRevisionID != nil {
			t.Fatal(main, d)
		}
	})
	t.Run("return edit and resubmit preserve prior instances and bind the newly selected rule", func(t *testing.T) {
		rule := publish(t, "returning", standard, "deny", true, 0)
		d, r := newDraft(t, "returning", false)
		r = submit(t, r)
		oldRevision, oldInstance := r.CurrentRevisionID, *r.ActiveInstanceID
		drainNotifications(t, f)
		approvalNotice := requireBusinessNotification(t, f, f.bobToken, r.ID, "approval", true)
		if notificationFor(t, f, f.aliceToken, r.ID) != nil {
			t.Fatal("applicant received another employee's active approval step")
		}
		r = decide(t, r, 0, f.bobToken, "return", core.NewID())
		if r.State != "returned" || r.Steps[1].State != "cancelled" {
			t.Fatal(r)
		}
		drainNotifications(t, f)
		returned := requireBusinessNotification(t, f, f.aliceToken, r.ID, "request_result", true)
		oldApproval := requireBusinessNotification(t, f, f.bobToken, r.ID, "approval", false)
		if returned.CurrentState != "returned" || oldApproval.ID != approvalNotice.ID {
			t.Fatal("returned workflow notification lost its current-state boundary")
		}
		d = getContract(t, d.ID)
		in := contracts.Patch{Content: d.Content, ExpectedVersion: d.Version, BaseRevisionID: d.CurrentRevisionID, Reason: "revise after returned decision"}
		in.Notes = "Changed after return"
		rec, res := f.with(t).request("PATCH", "/v1/contracts/"+d.ID, f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 200, res)
		d = data[contracts.Contract](t, res)
		r = getRequest(t, r.ID, f.aliceToken)
		if r.State != "draft" || r.CanSubmit {
			t.Fatal("stale request may submit", r)
		}
		rec, res = f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "REQUEST_DRAFT_STALE" {
			t.Fatal(res.Error)
		}
		rec, res = f.with(t).request("POST", "/v1/requests/"+r.ID+"/revisions", f.aliceToken, core.NewID(), workflow.RequestRevise{ExpectedVersion: r.Version, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, Reason: "confirm revised business evidence"}, nil)
		expect(t, rec, 200, res)
		r = data[workflow.Request](t, res)
		newRule := publish(t, "returning", standard, "deny", false, rule.Version)
		r = submit(t, r)
		if r.CurrentRevisionID == oldRevision || *r.ActiveInstanceID == oldInstance || r.Workflow.ID != newRule.ID || r.CanWithdraw {
			t.Fatal(r)
		}
		rec, res = f.with(t).request("GET", "/v1/requests/"+r.ID+"/revisions/"+oldRevision, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		historic := data[workflow.Revision](t, res)
		if historic.Payload.ContractContent().Notes != "" || historic.Workflow.ID != rule.ID || historic.Steps[0].State != "returned" || historic.Steps[1].State != "cancelled" {
			t.Fatal(historic)
		}
		rec, res = f.with(t).request("POST", "/v1/requests/"+r.ID+"/withdraw", f.aliceToken, core.NewID(), workflow.WithdrawInput{ExpectedVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, Reason: "must obey fixed withdrawal policy"}, nil)
		expect(t, rec, 409, res)
	})
	t.Run("rejection is terminal and a fresh application keeps the rejected history", func(t *testing.T) {
		publish(t, "rejecting", standard, "deny", true, 0)
		d, r := newDraft(t, "rejecting", false)
		r = submit(t, r)
		r = decide(t, r, 0, f.bobToken, "reject", core.NewID())
		rejected := r.ID
		rec, res := f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 409, res)
		d = getContract(t, d.ID)
		rec, res = f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "contract", BusinessID: d.ID, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, Reason: "new application after terminal rejection"}, nil)
		expect(t, rec, 201, res)
		next := data[workflow.Request](t, res)
		if next.ID == rejected || next.State != "draft" || getRequest(t, rejected, f.aliceToken).State != "rejected" {
			t.Fatal(next)
		}
		if current := getContract(t, d.ID); current.RequestID == nil || *current.RequestID != next.ID || current.ApprovalState != "not_submitted" {
			t.Fatal(current)
		}
	})
	t.Run("allowed withdrawal preserves an executed decision and resubmission creates a new instance", func(t *testing.T) {
		publish(t, "withdrawing", standard, "deny", true, 0)
		_, r := newDraft(t, "withdrawing", false)
		r = submit(t, r)
		r = decide(t, r, 0, f.bobToken, "approve", core.NewID())
		old := r.CurrentRevisionID
		in := workflow.WithdrawInput{ExpectedVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, Reason: "applicant withdraws the remaining step"}
		key := core.NewID()
		rec, res := f.with(t).request("POST", "/v1/requests/"+r.ID+"/withdraw", f.aliceToken, key, in, nil)
		expect(t, rec, 200, res)
		r = data[workflow.Request](t, res)
		if r.State != "withdrawn" || r.Steps[0].State != "approved" || r.Steps[1].State != "cancelled" {
			t.Fatal(r)
		}
		r = submit(t, r)
		if r.CurrentRevisionID == old || r.Steps[0].State != "active" {
			t.Fatal(r)
		}
		rec, res = f.with(t).request("POST", "/v1/requests/"+r.ID+"/withdraw", f.aliceToken, key, in, nil)
		expect(t, rec, 200, res)
		if data[workflow.Request](t, res).State != "withdrawn" || getRequest(t, r.ID, f.aliceToken).State != "in_review" {
			t.Fatal("withdraw replay mutated the new instance")
		}
	})
	t.Run("self approval is denied until a new explicit policy allows it", func(t *testing.T) {
		binding := bind(t, f.alice, "approval_reviewer")
		defer exec(t, "UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE id=$1", binding)
		own := []workflow.StepInput{{Title: "Explicit own review", AssignedUserID: f.alice, RequiresMFA: false}}
		rule := publish(t, "ownreview", own, "deny", true, 0)
		_, r := newDraft(t, "ownreview", false)
		rec, res := f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "SELF_APPROVAL_DENIED" {
			t.Fatal(res.Error)
		}
		publish(t, "ownreview", own, "allow", true, rule.Version)
		r = submit(t, r)
		r = decide(t, r, 0, f.aliceToken, "approve", core.NewID())
		if r.State != "approved" {
			t.Fatal(r)
		}
	})
	t.Run("unavailable reviewers and missing grants block submission without skipping", func(t *testing.T) {
		publish(t, "unavailable", standard, "deny", true, 0)
		_, r := newDraft(t, "unavailable", false)
		exec(t, "UPDATE users SET status='disabled' WHERE id=$1", f.bob)
		rec, res := f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "WORKFLOW_APPROVER_UNAVAILABLE" {
			t.Fatal(res.Error)
		}
		exec(t, "UPDATE users SET status='active' WHERE id=$1", f.bob)
		exec(t, "UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1", bobReview)
		rec, res = f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "APPROVER_PERMISSION_REQUIRED" {
			t.Fatal(res.Error)
		}
		exec(t, "UPDATE role_bindings SET valid_until=NULL WHERE id=$1", bobReview)
		r = submit(t, r)
		if r.Steps[0].AssignedUserID != f.bob {
			t.Fatal("silently reassigned reviewer")
		}
	})

	t.Run("missing attachment or current attachment grants block decisions and cached disclosure", func(t *testing.T) {
		publish(t, "attachments", standard, "deny", true, 0)
		_, r := newDraft(t, "attachments", false)
		r = submit(t, r)
		input := decision(r, r.Steps[0], "approve")
		exec(t, "UPDATE file_versions SET availability='missing' WHERE id=$1", old.Version.ID)
		rec, res := f.with(t).request("POST", "/v1/approval-steps/"+r.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), input, nil)
		expect(t, rec, 422, res)
		if visible := getRequest(t, r.ID, f.bobToken); len(visible.Payload.Attachments) != 0 || visible.State != "in_review" {
			t.Fatal(visible)
		}
		exec(t, "UPDATE file_versions SET availability='available' WHERE id=$1", old.Version.ID)
		exec(t, "UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND user_id=$2 AND role_id IN (SELECT id FROM roles WHERE key='project_manager' AND org_id=$1)", f.org, f.bob)
		rec, res = f.with(t).request("POST", "/v1/approval-steps/"+r.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), input, nil)
		expect(t, rec, 404, res)
		exec(t, "UPDATE role_bindings SET valid_until=NULL WHERE org_id=$1 AND user_id=$2 AND role_id IN (SELECT id FROM roles WHERE key='project_manager' AND org_id=$1)", f.org, f.bob)
		exec(t, "UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE id=$1", bobParty)
		rec, res = f.with(t).request("POST", "/v1/approval-steps/"+main.Steps[0].ID+"/decisions", f.bobToken, firstKey, firstInput, nil)
		expect(t, rec, 404, res)
		if visible := getRequest(t, main.ID, f.bobToken); visible.Payload.Counterparty != nil {
			t.Fatal("fixed snapshot leaked hidden party name")
		}
		exec(t, "UPDATE role_bindings SET valid_until=NULL WHERE id=$1", bobParty)
	})
	t.Run("MFA uses the actual session and agents or another device cannot impersonate a reviewer", func(t *testing.T) {
		publish(t, "identity", standard, "deny", true, 0)
		_, r := newDraft(t, "identity", false)
		r = submit(t, r)
		input := decision(r, r.Steps[0], "approve")
		actor, err := f.api.Identity.Authenticate(ctx, f.bobToken)
		if err != nil {
			t.Fatal(err)
		}
		var actual *time.Time
		if err = f.owner.QueryRow(ctx, "SELECT mfa_verified_at FROM sessions WHERE id=$1", actor.SessionID).Scan(&actual); err != nil {
			t.Fatal(err)
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		raw, _ := json.Marshal(input)
		service := workflow.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}, Pager: platform.Pager{Key: f.api.Config.CursorKey}}
		now := time.Now()
		actor.MFAVerifiedAt = &now
		command := core.Command{Actor: actor, RequestID: core.NewID(), Route: "POST /v1/approval-steps/{id}/decisions", Key: core.NewID(), Body: raw}
		if _, err = service.Decide(ctx, command, r.Steps[0].ID, input); core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal("trusted forged MFA timestamp", err)
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		command.Actor.RunID = core.NewID()
		command.Key = core.NewID()
		if _, err = service.Decide(ctx, command, r.Steps[0].ID, input); err == nil {
			t.Fatal("agent decided for human")
		}
		command.Actor.RunID = ""
		command.Actor.Channel = "wechat"
		command.Key = core.NewID()
		if _, err = service.Decide(ctx, command, r.Steps[0].ID, input); core.PublicError(err).Code != "AUTHORIZATION_CHANGED" {
			t.Fatal("accepted another device context", err)
		}
		rec, res := f.with(t).request("POST", "/v1/approval-steps/"+r.Steps[1].ID+"/decisions", f.memberToken, core.NewID(), decision(r, r.Steps[1], "approve"), nil)
		expect(t, rec, 409, res)
		bad := input
		bad.ContentHash = strings.Repeat("0", 64)
		rec, res = f.with(t).request("POST", "/v1/approval-steps/"+r.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), bad, nil)
		expect(t, rec, 409, res)
		r = decide(t, r, 0, f.bobToken, "approve", core.NewID())
		if r.Steps[1].State != "active" {
			t.Fatal(r)
		}
	})
	t.Run("task and request cursors bind current identity and durable authority is fixed", func(t *testing.T) {
		rec, res := f.with(t).request("GET", "/v1/approval-tasks?limit=1", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		tasks := data[core.Page[workflow.ApprovalStep]](t, res)
		if len(tasks.Items) != 1 || tasks.Items[0].AssignedUserID != f.bob || tasks.Items[0].State != "active" || tasks.NextCursor == nil {
			t.Fatal(tasks)
		}
		rec, res = f.with(t).request("GET", "/v1/approval-tasks?limit=1&cursor="+*tasks.NextCursor, f.memberToken, "", nil, nil)
		expect(t, rec, 400, res)
		rec, res = f.with(t).request("GET", "/v1/requests?limit=1", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[workflow.Request]](t, res)
		if page.NextCursor == nil {
			t.Fatal(page)
		}
		rec, res = f.with(t).request("GET", "/v1/requests?limit=1&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		expect(t, rec, 400, res)
		rec, res = f.with(t).request("GET", "/v1/requests", admin, "", nil, nil)
		expect(t, rec, 200, res)
		if len(data[core.Page[workflow.Request]](t, res).Items) != 0 {
			t.Fatal("administrator read business requests")
		}
		var raw []byte
		if err := f.owner.QueryRow(ctx, "SELECT reviewer_evidence FROM request_revisions WHERE id=$1", main.CurrentRevisionID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var proofs []workflow.AuthorityEvidence
		if err := json.Unmarshal(raw, &proofs); err != nil || len(proofs) != 2 || len(proofs[0].Checks[0].RoleBindingIDs) == 0 {
			t.Fatal(string(raw), err)
		}
		if _, err := f.owner.Exec(ctx, "UPDATE approval_steps SET assigned_user_name='tampered',version=version+1 WHERE id=$1", main.Steps[0].ID); err == nil {
			t.Fatal("rewrote fixed step")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE approval_decisions SET reason='rewritten' WHERE step_id=$1", main.Steps[0].ID); err == nil {
			t.Fatal("rewrote decision")
		}
		raw, _ = json.Marshal(main)
		for _, secret := range []string{"object_key", "authority_evidence", "session_id"} {
			if strings.Contains(string(raw), secret) {
				t.Fatal("exposed internal approval proof", secret)
			}
		}
	})

	failAudit := func(t *testing.T, action string) {
		t.Helper()
		exec(t, "CREATE FUNCTION fail_approval_audit() RETURNS trigger LANGUAGE plpgsql AS $x$ BEGIN IF NEW.action='"+action+"' THEN RAISE EXCEPTION 'isolated audit failure'; END IF; RETURN NEW; END $x$; CREATE TRIGGER fail_approval_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_approval_audit()")
	}
	dropAudit := func(t *testing.T) {
		t.Helper()
		exec(t, "DROP TRIGGER IF EXISTS fail_approval_audit ON audit_events; DROP FUNCTION IF EXISTS fail_approval_audit()")
	}
	t.Run("request creation audit failure rolls back the business link and fixed references", func(t *testing.T) {
		rec, res := f.with(t).request("POST", "/v1/contracts", f.aliceToken, core.NewID(), complete, nil)
		expect(t, rec, 201, res)
		d := data[contracts.Contract](t, res)
		in := workflow.RequestCreate{Kind: "contract", BusinessID: d.ID, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, Reason: "audit atomicity"}
		key := core.NewID()
		failAudit(t, "request.created")
		defer dropAudit(t)
		rec, res = f.with(t).request("POST", "/v1/requests", f.aliceToken, key, in, nil)
		expect(t, rec, 500, res)
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM requests WHERE business_id=$1", d.ID).Scan(&n); err != nil || n != 0 {
			t.Fatal(n, err)
		}
		if getContract(t, d.ID).RequestID != nil {
			t.Fatal("orphan business request link")
		}
		dropAudit(t)
		rec, res = f.with(t).request("POST", "/v1/requests", f.aliceToken, key, in, nil)
		expect(t, rec, 201, res)
	})
	t.Run("submission audit failure rolls back instances steps snapshot references and retry result", func(t *testing.T) {
		publish(t, "auditsubmit", standard, "deny", true, 0)
		_, r := newDraft(t, "auditsubmit", false)
		p := preview(t, r)
		in := submitInput(r, p)
		key := core.NewID()
		failAudit(t, "request.submitted")
		defer dropAudit(t)
		rec, res := f.with(t).request("POST", "/v1/requests/"+r.ID+"/submit", f.aliceToken, key, in, nil)
		expect(t, rec, 500, res)
		var instances, revisions int
		if err := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM approval_instances WHERE request_id=$1),(SELECT count(*) FROM request_revisions WHERE request_id=$1)", r.ID).Scan(&instances, &revisions); err != nil || instances != 0 || revisions != 1 {
			t.Fatal(instances, revisions, err)
		}
		if getRequest(t, r.ID, f.aliceToken).Version != r.Version {
			t.Fatal("partial submission")
		}
		dropAudit(t)
		rec, res = f.with(t).request("POST", "/v1/requests/"+r.ID+"/submit", f.aliceToken, key, in, nil)
		expect(t, rec, 200, res)
	})
	t.Run("decision audit failure leaves the exact step active and permits a safe retry", func(t *testing.T) {
		publish(t, "auditdecision", standard, "deny", true, 0)
		_, r := newDraft(t, "auditdecision", false)
		r = submit(t, r)
		in := decision(r, r.Steps[0], "approve")
		key := core.NewID()
		failAudit(t, "approval.step.decided")
		defer dropAudit(t)
		rec, res := f.with(t).request("POST", "/v1/approval-steps/"+r.Steps[0].ID+"/decisions", f.bobToken, key, in, nil)
		expect(t, rec, 500, res)
		current := getRequest(t, r.ID, f.aliceToken)
		if current.Version != r.Version || current.Steps[0].Version != r.Steps[0].Version || current.Steps[0].Decision != nil || current.Steps[1].State != "pending" {
			t.Fatal(current)
		}
		dropAudit(t)
		rec, res = f.with(t).request("POST", "/v1/approval-steps/"+r.Steps[0].ID+"/decisions", f.bobToken, key, in, nil)
		expect(t, rec, 200, res)
	})
	t.Run("withdrawal audit failure does not cancel active or pending steps", func(t *testing.T) {
		publish(t, "auditwithdraw", standard, "deny", true, 0)
		_, r := newDraft(t, "auditwithdraw", false)
		r = submit(t, r)
		in := workflow.WithdrawInput{ExpectedVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, Reason: "audit atomicity"}
		key := core.NewID()
		failAudit(t, "request.withdrawn")
		defer dropAudit(t)
		rec, res := f.with(t).request("POST", "/v1/requests/"+r.ID+"/withdraw", f.aliceToken, key, in, nil)
		expect(t, rec, 500, res)
		current := getRequest(t, r.ID, f.aliceToken)
		if current.Version != r.Version || current.State != "in_review" || current.Steps[0].State != "active" || current.Steps[1].State != "pending" {
			t.Fatal(current)
		}
		dropAudit(t)
		rec, res = f.with(t).request("POST", "/v1/requests/"+r.ID+"/withdraw", f.aliceToken, key, in, nil)
		expect(t, rec, 200, res)
	})
	t.Run("changing rules invalidates previews while a retired rule preserves submitted responsibilities", func(t *testing.T) {
		rule := publish(t, "fixedrule", standard, "deny", true, 0)
		_, r := newDraft(t, "fixedrule", false)
		p := preview(t, r)
		oldInput := submitInput(r, p)
		currentRule := publish(t, "fixedrule", standard, "deny", false, rule.Version)
		rec, res := f.with(t).request("POST", "/v1/requests/"+r.ID+"/submit", f.aliceToken, core.NewID(), oldInput, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "SUBMISSION_PREVIEW_CHANGED" {
			t.Fatal(res.Error)
		}
		r = submit(t, r)
		retire := workflow.DefinitionInput{ExpectedVersion: currentRule.Version, Key: currentRule.Key, Name: currentRule.Name, Kind: currentRule.Kind, Category: currentRule.Category, Currency: currentRule.Currency, MinAmount: currentRule.MinAmount, MaxAmount: currentRule.MaxAmount, Enabled: false, SelfApprovalPolicy: currentRule.SelfApprovalPolicy, WithdrawalAllowed: currentRule.WithdrawalAllowed, Steps: standard, Reason: "retire future routing only"}
		rec, res = f.with(t).request("POST", "/v1/workflow-definitions", admin, core.NewID(), retire, nil)
		expect(t, rec, 201, res)
		r = decide(t, r, 0, f.bobToken, "approve", core.NewID())
		r = decide(t, r, 1, f.memberToken, "approve", core.NewID())
		if r.State != "approved" || r.Workflow.ID != currentRule.ID {
			t.Fatal(r)
		}
	})

	t.Run("database boundaries reject out of order steps incomplete instances and terminal rewrites", func(t *testing.T) {
		publish(t, "database", standard, "deny", true, 0)
		_, r := newDraft(t, "database", false)
		r = submit(t, r)
		if _, err := f.owner.Exec(ctx, "UPDATE approval_steps SET state='active',version=version+1 WHERE id=$1", r.Steps[1].ID); err == nil {
			t.Fatal("activated later step early")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE approval_instances SET state='approved' WHERE id=$1", *r.ActiveInstanceID); err == nil {
			t.Fatal("approved incomplete normalized steps")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE requests SET state='approved',version=version+1 WHERE id=$1", r.ID); err == nil {
			t.Fatal("request diverged from business")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE requests SET state='draft',version=version+1 WHERE id=$1", main.ID); err == nil {
			t.Fatal("rewrote approved terminal request")
		}
		if _, err := f.owner.Exec(ctx, `INSERT INTO approval_steps(id,org_id,request_id,revision_id,instance_id,step_no,title,assigned_user_id,assigned_user_name,requires_mfa,authority_evidence,state)
            VALUES($1,$2,$3,$4,$5,3,'late inserted reviewer',$6,'unplanned',false,'{}','pending')`, core.NewID(), f.org, r.ID, r.CurrentRevisionID, *r.ActiveInstanceID, f.bob); err == nil {
			t.Fatal("appended reviewer to fixed definition")
		}
		tx, err := f.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		revision := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO request_revisions(id,org_id,request_id,seq,base_revision_id,business_revision_id,business_content_hash,payload_schema_version,payload,workflow_definition_id,workflow_snapshot,reviewer_evidence,content_hash,author_id,reason)
            SELECT $1,org_id,request_id,seq+1,id,business_revision_id,business_content_hash,payload_schema_version,payload,workflow_definition_id,workflow_snapshot,reviewer_evidence,content_hash,author_id,'incomplete instance boundary fixture' FROM request_revisions WHERE id=$2`, revision, main.CurrentRevisionID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, "INSERT INTO approval_instances(id,org_id,request_id,revision_id,definition_id,state) VALUES($1,$2,$3,$4,$5,'in_review')", core.NewID(), f.org, main.ID, revision, main.Workflow.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err == nil {
			t.Fatal("committed an instance without all fixed steps")
		}
		var n int
		if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM request_revisions WHERE id=$1", revision).Scan(&n); err != nil || n != 0 {
			t.Fatal("orphan incomplete snapshot", n, err)
		}
	})

}
