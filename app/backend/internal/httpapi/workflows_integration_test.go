package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/workflow"
)

func TestWorkflowDefinitions(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	admin := f.administratorToken()
	makeInput := func(key, lower string, upper *string) workflow.DefinitionInput {
		return workflow.DefinitionInput{Key: key, Name: "Isolated rule " + key, Kind: "claim", Category: "travel", Currency: "HKD",
			MinAmount: lower, MaxAmount: upper, Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true,
			Steps: []workflow.StepInput{{Title: "Review", AssignedUserID: f.bob, RequiresMFA: true}}, Reason: "isolated database test only"}
	}
	value := func(s string) *string { return &s }
	var first, high workflow.Definition
	var lowInput workflow.DefinitionInput
	key := core.NewID()
	selectRule := func(amount string) (workflow.Definition, error) {
		tx, err := f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		return workflow.SelectDefinition(ctx, tx, f.org, "claim", "travel", "HKD", amount)
	}
	t.Run("no implicit company policy and unauthorized reads do not disclose rules", func(t *testing.T) {
		rec, res := f.request("GET", "/v1/workflow-definitions", admin, "", nil, nil)
		expect(t, rec, 200, res)
		if len(data[core.Page[workflow.Definition]](t, res).Items) != 0 {
			t.Fatal("invented policy")
		}
		rec, res = f.request("GET", "/v1/workflow-definitions", f.aliceToken, "", nil, nil)
		expect(t, rec, 404, res)
		_, err := selectRule("0")
		if core.PublicError(err).Code != "WORKFLOW_RULE_REQUIRED" {
			t.Fatal(err)
		}
	})
	t.Run("half open currency bounds publication replay and immutable history", func(t *testing.T) {
		lowInput = makeInput("travel.small", "0", value("100"))
		lowInput.Steps[0].AssignedUserID = strings.ToUpper(f.bob)
		rec, res := f.request("POST", "/v1/workflow-definitions", admin, key, lowInput, nil)
		expect(t, rec, 201, res)
		first = data[workflow.Definition](t, res)
		if first.Version != 1 || first.MinAmount != "0.000000" || len(first.Steps) != 1 || first.Steps[0].AssignedUserID != f.bob {
			t.Fatal(first)
		}
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, key, lowInput, nil)
		expect(t, rec, 201, res)
		if data[workflow.Definition](t, res).ID != first.ID {
			t.Fatal("duplicate revision")
		}
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), makeInput("travel.large", "100", nil), nil)
		expect(t, rec, 201, res)
		high = data[workflow.Definition](t, res)
		for _, amount := range []string{"0", "99.99", "99.990000"} {
			d, err := selectRule(amount)
			if err != nil || d.ID != first.ID {
				t.Fatal(amount, d, err)
			}
		}
		d, err := selectRule("100")
		if err != nil || d.ID != high.ID {
			t.Fatal(d, err)
		}
		if _, err = selectRule("99.991"); core.PublicError(err).Status != 422 {
			t.Fatal("unsupported currency precision", err)
		}
		for _, sql := range []string{"UPDATE workflow_definitions SET name='changed' WHERE id=$1", "DELETE FROM workflow_definitions WHERE id=$1",
			"UPDATE workflow_definition_steps SET requires_mfa=false WHERE definition_id=$1", "DELETE FROM workflow_definition_steps WHERE definition_id=$1"} {
			if _, err := f.api.DB.Exec(ctx, sql, first.ID); err == nil {
				t.Fatal("mutable definition", sql)
			}
		}
		if _, err = f.api.DB.Exec(ctx, "INSERT INTO workflow_definition_steps(org_id,definition_id,step_no,title,assigned_user_id,requires_mfa) VALUES($1,$2,2,'late step',$3,true)", f.org, first.ID, f.alice); err == nil {
			t.Fatal("late step appended")
		}
		// Even the migration owner must provide the complete sequence at commit.
		tx, err := f.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `INSERT INTO workflow_definitions(id,org_id,key,revision,name,kind,category,currency,min_amount,enabled,
			self_approval_policy,withdrawal_allowed,published_by,reason,steps)
			SELECT $1,org_id,'missing.steps',1,name,kind,category,currency,0,false,'deny',false,published_by,reason,steps FROM workflow_definitions WHERE id=$2`, core.NewID(), first.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err == nil {
			t.Fatal("incomplete sequence committed")
		}
	})
	t.Run("overlap precision and repeated assignee reject without fallback", func(t *testing.T) {
		for _, category := range []string{"travel", "*"} {
			in := makeInput("conflict."+strings.ReplaceAll(category, "*", "any"), "50", value("150"))
			in.Category = category
			rec, res := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
			expect(t, rec, 409, res)
			if res.Error.Code != "WORKFLOW_RULE_OVERLAP" {
				t.Fatal(res.Error)
			}
		}
		in := makeInput("other.currency", "0", nil)
		in.Currency = "JPY"
		in.MinAmount = "0.01"
		rec, res := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
		expect(t, rec, 422, res)
		in = makeInput("repeat.person", "0", nil)
		in.Steps = append(in.Steps, in.Steps[0])
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
		expect(t, rec, 422, res)
		in = makeInput("separate.category", "0", nil)
		in.Category = "software"
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
		expect(t, rec, 201, res)
	})
	t.Run("retirement is a new revision and old assignment remains fixed", func(t *testing.T) {
		in := lowInput
		in.ExpectedVersion = 1
		in.Enabled = false
		in.Steps = []workflow.StepInput{{Title: "New reviewer", AssignedUserID: f.alice, RequiresMFA: false}}
		rec, res := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
		expect(t, rec, 201, res)
		retired := data[workflow.Definition](t, res)
		if retired.Version != 2 || retired.Enabled {
			t.Fatal(retired)
		}
		rec, res = f.request("GET", "/v1/workflow-definitions/"+first.ID, admin, "", nil, nil)
		expect(t, rec, 200, res)
		if data[workflow.Definition](t, res).Steps[0].AssignedUserID != f.bob {
			t.Fatal("old rule rewritten")
		}
		rec, res = f.request("GET", "/v1/workflow-definitions/"+first.ID+"/history?limit=1", admin, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[workflow.Definition]](t, res)
		if len(page.Items) != 1 || page.Items[0].ID != retired.ID || page.NextCursor == nil {
			t.Fatal(page)
		}
		_, err := selectRule("50")
		if core.PublicError(err).Code != "WORKFLOW_RULE_REQUIRED" {
			t.Fatal("old rule revived", err)
		}
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
		expect(t, rec, 409, res)
	})
	t.Run("concurrent overlap publication has exactly one winner", func(t *testing.T) {
		var wg sync.WaitGroup
		statuses := make(chan int, 2)
		for _, name := range []string{"race.one", "race.two"} {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				in := makeInput(name, "0", nil)
				in.Kind = "purchase"
				rec, _ := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
				statuses <- rec.Code
			}(name)
		}
		wg.Wait()
		close(statuses)
		successes := 0
		for code := range statuses {
			if code == 201 {
				successes++
			} else if code != 409 && code != 412 {
				t.Fatal(code)
			}
		}
		if successes != 1 {
			t.Fatal("overlap published concurrently", successes)
		}
	})
	t.Run("audit failure rolls back revision steps version and idempotency", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, `CREATE FUNCTION fail_workflow_audit() RETURNS trigger LANGUAGE plpgsql AS $x$
			BEGIN IF NEW.action='workflow.definition.published' THEN RAISE EXCEPTION 'isolated audit failure'; END IF; RETURN NEW; END $x$;
			CREATE TRIGGER fail_workflow_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_workflow_audit()`); err != nil {
			t.Fatal(err)
		}
		var before, after int64
		if err := f.owner.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1", f.org).Scan(&before); err != nil {
			t.Fatal(err)
		}
		in := makeInput("audit.rollback", "0", nil)
		in.Kind = "contract"
		operation := core.NewID()
		rec, res := f.request("POST", "/v1/workflow-definitions", admin, operation, in, nil)
		expect(t, rec, 500, res)
		if err := f.owner.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1", f.org).Scan(&after); err != nil || before != after {
			t.Fatal(before, after, err)
		}
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM workflow_definitions WHERE key=$1", in.Key).Scan(&count); err != nil || count != 0 {
			t.Fatal(count, err)
		}
		if _, err := f.owner.Exec(ctx, "DROP TRIGGER fail_workflow_audit ON audit_events; DROP FUNCTION fail_workflow_audit()"); err != nil {
			t.Fatal(err)
		}
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, operation, in, nil)
		expect(t, rec, 201, res)
	})
	t.Run("inactive assignee blocks selection but permits explicit retirement", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, "UPDATE users SET status='disabled' WHERE id=$1", f.bob); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if _, err := f.owner.Exec(ctx, "UPDATE users SET status='active' WHERE id=$1", f.bob); err != nil {
				t.Error(err)
			}
		}()
		if _, err := selectRule("100"); core.PublicError(err).Code != "WORKFLOW_APPROVER_UNAVAILABLE" {
			t.Fatal(err)
		}
		in := makeInput("unavailable.person", "0", nil)
		in.Kind = "change"
		rec, res := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
		expect(t, rec, 422, res)
		in = makeInput("travel.large", "100", nil)
		in.ExpectedVersion = high.Version
		in.Enabled = false
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), in, nil)
		expect(t, rec, 201, res)
		rec, res = f.request("GET", "/v1/workflow-definitions/"+high.ID, admin, "", nil, nil)
		expect(t, rec, 200, res)
		if data[workflow.Definition](t, res).Steps[0].AssignedUserActive {
			t.Fatal("inactive reviewer hidden")
		}
		if _, err := selectRule("100"); core.PublicError(err).Code != "WORKFLOW_RULE_REQUIRED" {
			t.Fatal("old enabled revision revived", err)
		}
	})
	t.Run("database session proof and current authorization precede replay", func(t *testing.T) {
		actor, err := f.api.Identity.Authenticate(ctx, admin)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.owner.Exec(ctx, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID); err != nil {
			t.Fatal(err)
		}
		tx, err := f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		now := time.Now()
		actor.MFAVerifiedAt = &now
		if err = identity.RequireInteractiveSession(ctx, tx, actor, true); core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal("forged actor proof accepted", err)
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		rec, res := f.request("POST", "/v1/workflow-definitions", admin, key, lowInput, nil)
		expect(t, rec, 403, res)
		if _, err = f.owner.Exec(ctx, "UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE id=$1", actor.SessionID); err != nil {
			t.Fatal(err)
		}
		tx, err = f.api.DB.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		actor.RunID = core.NewID()
		if err = identity.RequireInteractiveSession(ctx, tx, actor, false); core.PublicError(err).Status != 403 {
			t.Fatal("agent decision channel allowed", err)
		}
		_ = tx.Rollback(ctx)
		if _, err = f.owner.Exec(ctx, "DELETE FROM role_actions WHERE org_id=$1 AND action='config.manage'", f.org); err != nil {
			t.Fatal(err)
		}
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, key, lowInput, nil)
		expect(t, rec, 404, res)
		rec, res = f.request("GET", "/v1/workflow-definitions/"+first.ID, admin, "", nil, nil)
		expect(t, rec, 404, res)
		var raw map[string]any
		if json.Unmarshal(res.Data, &raw) == nil && len(raw) != 0 {
			t.Fatal("revoked rule disclosed")
		}
	})
}
