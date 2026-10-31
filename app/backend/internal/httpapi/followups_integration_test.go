package httpapi

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/workbench"
	"copypaste.hk/platform/internal/workflow"
)

func TestBusinessFollowups(t *testing.T) {
	u, file, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	pid := *file.File.ProjectID
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	read := func(t *testing.T, token string) workbench.FollowupSummary {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/dashboard/followups", token, "", nil, nil)
		expect(t, r, 200, v)
		for _, hidden := range []string{"payload", "description", "content_hash", "file_version_ids", "amount", "submitted_by", "bucket", "object_key"} {
			if strings.Contains(r.Body.String(), `"`+hidden+`"`) {
				t.Fatal("non-summary field leaked", hidden)
			}
		}
		out := data[workbench.FollowupSummary](t, v)
		if out.Scope != "my_readable_followups" || len(out.Groups) != 3 || out.AsOf.IsZero() {
			t.Fatal("incomplete followup snapshot", out)
		}
		return out
	}
	group := func(t *testing.T, token, kind string) workbench.FollowupGroup {
		t.Helper()
		for _, g := range read(t, token).Groups {
			if g.Kind == kind {
				return g
			}
		}
		t.Fatal("missing group", kind)
		return workbench.FollowupGroup{}
	}
	t.Run("fixed personal read rejects caller filters and preserves the existing six groups", func(t *testing.T) {
		for _, g := range read(t, f.memberToken).Groups {
			if g.Count != 0 || len(g.Items) != 0 || g.HasMore {
				t.Fatal(g)
			}
		}
		for _, q := range []string{"?owner_id=" + f.alice, "?limit=1", "?kind=requests_to_revise", "?%zz"} {
			r, v := f.with(t).request("GET", "/v1/dashboard/followups"+q, f.memberToken, "", nil, nil)
			expect(t, r, 422, v)
		}
		r, v := f.with(t).request("GET", "/v1/dashboard/followups", "", "", nil, nil)
		expect(t, r, 401, v)
		r, v = f.with(t).request("GET", "/v1/dashboard/business", f.memberToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[workbench.Summary](t, v).Groups) != 6 {
			t.Fatal("existing client contract changed")
		}
	})
	bind := func(user, key string) string {
		id := core.NewID()
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
		 SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	for _, role := range []string{"request_manager", "contract_manager", "party_manager"} {
		bind(f.bob, role)
	}
	bind(f.alice, "approval_reviewer")
	bind(f.alice, "request_manager")
	bind(f.alice, "contract_reader")
	bind(f.alice, "party_manager")
	bind(f.alice, "deliverable_reviewer")
	bind(f.bob, "deliverable_reviewer")
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = memberMFAToken(f, "bob", "android")
	r, v := f.request("POST", "/v1/workflow-definitions", f.administratorToken(), core.NewID(), workflow.DefinitionInput{Key: "followups", Name: "Synthetic followup independent review", Kind: "contract", Category: "followups", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent review", AssignedUserID: f.alice}}, Reason: "isolated followup workflow"}, nil)
	expect(t, r, 201, v)
	r, v = f.request("POST", "/v1/parties", f.bobToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic followup supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated supplier"}, nil)
	expect(t, r, 201, v)
	supplier := data[party.Party](t, v)
	ptr := func(s string) *string { return &s }
	start, end := time.Now().AddDate(0, 0, -1).Format(time.DateOnly), time.Now().AddDate(0, 0, 30).Format(time.DateOnly)
	r, v = f.request("POST", "/v1/contracts", f.bobToken, core.NewID(), contracts.Create{Content: contracts.Content{Title: "Synthetic followup contract", Direction: ptr("payable"), Type: ptr("service"), Category: "followups", CounterpartyID: &supplier.ID, CounterpartyRevisionID: &supplier.CurrentRevisionID, OwnerID: &f.bob, Currency: ptr("HKD"), Amount: ptr("12.34"), StartDate: &start, EndDate: &end, DraftFileVersionID: &file.Version.ID}, ProjectID: file.File.ProjectID, Classification: "project_restricted", Reason: "isolated exact draft"}, nil)
	expect(t, r, 201, v)
	c := data[contracts.Contract](t, v)
	r, v = f.request("POST", "/v1/requests", f.bobToken, core.NewID(), workflow.RequestCreate{Kind: "contract", BusinessID: c.ID, ExpectedBusinessVersion: c.Version, BusinessRevisionID: c.CurrentRevisionID, BusinessContentHash: c.ContentHash, Reason: "exact application"}, nil)
	expect(t, r, 201, v)
	req := data[workflow.Request](t, v)
	getRequest := func(t *testing.T) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/requests/"+req.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[workflow.Request](t, v)
	}
	submitRequest := func(t *testing.T, q workflow.Request) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/requests/"+q.ID+"/submission-preview", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		preview := data[workflow.SubmissionPreview](t, v)
		r, v = f.with(t).request("POST", "/v1/requests/"+q.ID+"/submit", f.bobToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: q.Version, RevisionID: q.CurrentRevisionID, ContentHash: q.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "explicit current submission"}, nil)
		expect(t, r, 200, v)
		return data[workflow.Request](t, v)
	}
	t.Run("draft follows its applicant and exact current revision without disclosing payload", func(t *testing.T) {
		g := group(t, f.bobToken, "requests_to_revise")
		if g.Count != 1 || len(g.Items) != 1 || g.Items[0].ID != req.ID || g.Items[0].RevisionID == nil || *g.Items[0].RevisionID != req.CurrentRevisionID || g.Items[0].State != "draft" || group(t, f.aliceToken, "requests_to_revise").Count != 0 {
			t.Fatal(g)
		}
	})
	t.Run("ordinary submission return and resubmission recompute current attention", func(t *testing.T) {
		req = submitRequest(t, req)
		if group(t, f.bobToken, "requests_to_revise").Count != 0 {
			t.Fatal("submitted application remained in draft attention")
		}
		r, v := f.with(t).request("POST", "/v1/approval-steps/"+req.Steps[0].ID+"/decisions", f.aliceToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: req.Steps[0].Version, RequestVersion: req.Version, RevisionID: req.CurrentRevisionID, ContentHash: req.ContentHash, Decision: "return", Reason: "synthetic clarification required"}, nil)
		expect(t, r, 200, v)
		req = getRequest(t)
		g := group(t, f.bobToken, "requests_to_revise")
		if g.Count != 1 || g.Items[0].State != "returned" || g.Items[0].Version != req.Version {
			t.Fatal(g)
		}
		req = submitRequest(t, req)
		if group(t, f.bobToken, "requests_to_revise").Count != 0 {
			t.Fatal("resubmitted application remained in attention")
		}
	})
	create := func(t *testing.T, name string) project.Deliverable {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/projects/"+pid+"/deliverables", f.aliceToken, core.NewID(), project.DeliverableCreate{Name: name, AcceptanceCriteria: "Verify the fixed result and its complete formal attachment.", OwnerID: f.alice, Reason: "isolated followup deliverable"}, nil)
		expect(t, r, 201, v)
		id := data[project.DeliverableReceipt](t, v).ID
		r, v = f.with(t).request("GET", "/v1/deliverables/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.Deliverable](t, v)
	}
	get := func(t *testing.T, id string) project.Deliverable {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/deliverables/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.Deliverable](t, v)
	}
	submit := func(t *testing.T, d project.Deliverable) (project.Deliverable, project.DeliverableRevision) {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/submit", f.aliceToken, core.NewID(), project.DeliverableSubmit{ExpectedVersion: d.Version, ExpectedRevisionID: d.CurrentRevisionID, FileVersionIDs: []string{file.Version.ID}, Description: "Synthetic complete delivery result", Reason: "exact independent followup"}, nil)
		expect(t, r, 200, v)
		d = get(t, d.ID)
		r, v = f.with(t).request("GET", "/v1/deliverables/"+d.ID+"/revisions/"+*d.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return d, data[project.DeliverableRevision](t, v)
	}
	d := create(t, "Synthetic <em>literal</em> followup deliverable")
	t.Run("responsible draft keeps a real null revision and does not become another persons work", func(t *testing.T) {
		g := group(t, f.aliceToken, "deliverables_to_submit")
		if g.Count != 1 || len(g.Items) != 1 || g.Items[0].ID != d.ID || g.Items[0].RevisionID != nil || g.Items[0].ProjectID == nil || *g.Items[0].ProjectID != pid || group(t, f.bobToken, "deliverables_to_submit").Count != 0 {
			t.Fatal(g)
		}
	})
	// The browser run deliberately branches after ordinary source setup. Its
	// short seed results are not reported as the complete focused native run.
	if os.Getenv("CP_BROWSER_FOLLOWUPS_FIXTURE") == "1" {
		rec, body := f.request("POST", "/v1/approval-steps/"+req.Steps[0].ID+"/decisions", f.aliceToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: req.Steps[0].Version, RequestVersion: req.Version, RevisionID: req.CurrentRevisionID, ContentHash: req.ContentHash, Decision: "return", Reason: "synthetic browser clarification"}, nil)
		expect(t, rec, 200, body)
		submitted, _ := submit(t, d)
		rec, body = f.request("POST", "/v1/projects/"+pid+"/deliverables", f.bobToken, core.NewID(), project.DeliverableCreate{Name: "合成资料 · 本人待提交交付", AcceptanceCriteria: "核对约定标准与正式附件", OwnerID: f.bob, Reason: "isolated browser source"}, nil)
		expect(t, rec, 201, body)
		own := data[project.DeliverableReceipt](t, body)
		other := f.createProject("合成资料 · 入口边界", []string{f.bob})
		holdFollowupBrowser(t, f, map[string]string{"request": req.ID, "draft": own.ID, "review": submitted.ID, "project": pid, "other_project": other.ID})
		return
	}
	var rev project.DeliverableRevision
	t.Run("formal submission moves attention to an independent current reviewer", func(t *testing.T) {
		d, rev = submit(t, d)
		g := group(t, f.bobToken, "deliverables_to_review")
		if g.Count != 1 || g.Items[0].ID != d.ID || g.Items[0].RevisionID == nil || *g.Items[0].RevisionID != rev.ID || group(t, f.aliceToken, "deliverables_to_submit").Count != 0 || group(t, f.aliceToken, "deliverables_to_review").Count != 0 {
			t.Fatal(g)
		}
	})
	t.Run("reading attention has no audit write or delivery revision side effect", func(t *testing.T) {
		var before, after int64
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE org_id=$1", f.org).Scan(&before); err != nil {
			t.Fatal(err)
		}
		read(t, f.bobToken)
		read(t, f.bobToken)
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE org_id=$1", f.org).Scan(&after); err != nil || before != after {
			t.Fatal("attention read wrote audit", before, after, err)
		}
		if current := get(t, d.ID); current.Version != d.Version || current.State != "submitted" {
			t.Fatal("attention read changed delivery", current)
		}
	})
	t.Run("removing current review authority hides count while ordinary metadata remains readable", func(t *testing.T) {
		rows, err := f.owner.Query(ctx, `DELETE FROM role_actions ra WHERE ra.org_id=$1 AND ra.action='deliverable.decide'
        AND EXISTS(SELECT 1 FROM role_bindings b WHERE b.org_id=ra.org_id AND b.role_id=ra.role_id AND b.user_id=$2) RETURNING role_id`, f.org, f.bob)
		if err != nil {
			t.Fatal(err)
		}
		roles := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			roles = append(roles, id)
		}
		rows.Close()
		if len(roles) == 0 {
			t.Fatal("no actual review authority removed")
		}
		if g := group(t, f.bobToken, "deliverables_to_review"); g.Count != 0 || len(g.Items) != 0 {
			t.Fatal(g)
		}
		r, v := f.with(t).request("GET", "/v1/deliverables/"+d.ID, f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if data[project.Deliverable](t, v).CanDecide {
			t.Fatal("ordinary read still allowed review")
		}
		for _, id := range roles {
			exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'deliverable.decide')", f.org, id)
		}
		if group(t, f.bobToken, "deliverables_to_review").Count != 1 {
			t.Fatal("fresh review authority was not used")
		}
	})
	t.Run("request changes returns ownership and exact next revision then independent accept clears work", func(t *testing.T) {
		decide := func(decision string) {
			r, v := f.with(t).request("POST", "/v1/deliverables/"+d.ID+"/decisions", f.bobToken, core.NewID(), project.DeliverableDecisionInput{ExpectedVersion: d.Version, RevisionID: rev.ID, ContentHash: rev.ContentHash, Decision: decision, FilesVerified: true, CriteriaVerified: true, Reason: "synthetic independent verification"}, nil)
			expect(t, r, 200, v)
			d = get(t, d.ID)
		}
		decide("request_changes")
		if g := group(t, f.aliceToken, "deliverables_to_submit"); g.Count != 1 || g.Items[0].State != "changes_requested" || g.Items[0].RevisionID == nil || *g.Items[0].RevisionID != rev.ID || group(t, f.bobToken, "deliverables_to_review").Count != 0 {
			t.Fatal(g)
		}
		d, rev = submit(t, d)
		decide("accept")
		if d.State != "accepted" || group(t, f.aliceToken, "deliverables_to_submit").Count != 0 || group(t, f.bobToken, "deliverables_to_review").Count != 0 {
			t.Fatal("accepted work remained in attention", d)
		}
	})
	t.Run("complete count and deterministic five item window do not count other owners", func(t *testing.T) {
		for i := 0; i < 7; i++ {
			create(t, "Synthetic bounded draft "+string(rune('A'+i)))
		}
		g := group(t, f.aliceToken, "deliverables_to_submit")
		if g.Count != 7 || len(g.Items) != 5 || !g.HasMore || group(t, f.bobToken, "deliverables_to_submit").Count != 0 {
			t.Fatal(g)
		}
		for i := 1; i < len(g.Items); i++ {
			if g.Items[i-1].UpdatedAt.Before(g.Items[i].UpdatedAt) {
				t.Fatal("incorrect current update ordering")
			}
		}
	})
	t.Run("current classification removes inaccessible counts", func(t *testing.T) {
		exec("UPDATE role_bindings SET classification_ceiling='internal' WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		if g := group(t, f.aliceToken, "deliverables_to_submit"); g.Count != 0 || len(g.Items) != 0 {
			t.Fatal("classification change leaked count", g)
		}
	})
	t.Run("current human session rejects stale versions and executor identities", func(t *testing.T) {
		a, err := f.api.Identity.Authenticate(ctx, f.bobToken)
		if err != nil {
			t.Fatal(err)
		}
		service := workbench.Service{DB: f.api.DB}
		for _, change := range []func(*core.Actor){func(a *core.Actor) { a.RunID = core.NewID() }, func(a *core.Actor) { a.Channel = "telegram" }, func(a *core.Actor) { a.AuthVersion++ }, func(a *core.Actor) { a.OrgAuthVersion++ }, func(a *core.Actor) { a.SessionID = core.NewID() }} {
			copy := a
			change(&copy)
			if _, err := service.Followups(ctx, copy); err == nil {
				t.Fatal("stale session or executor identity accepted")
			}
		}
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE org_id=$1 AND id=$2", f.org, a.SessionID)
		if _, err := service.Followups(ctx, a); err == nil {
			t.Fatal("revoked session accepted")
		}
	})
}
