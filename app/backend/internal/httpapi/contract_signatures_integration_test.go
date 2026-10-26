package httpapi

import (
	"context"
	"encoding/json"
	"os"
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

func TestContractSignatureAndActivation(t *testing.T) {
	u, original, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for user, keys := range map[string][]string{f.alice: {"contract_manager", "party_manager", "request_manager", "contract_operator"}, f.bob: {"contract_reader", "party_manager", "approval_reviewer"}} {
		for _, key := range keys {
			exec(t, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
            SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, user, key)
		}
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	admin := f.administratorToken()
	rec, res := f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "execution", Name: "Isolated execution policy", Kind: "contract", Category: "execution", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Independent reviewer", AssignedUserID: f.bob}}, Reason: "isolated explicit test policy"}, nil)
	expect(t, rec, 201, res)
	rec, res = f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic signature counterparty", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: original.File.ProjectID, Classification: "project_restricted", Reason: "isolated signature fixture"}, nil)
	expect(t, rec, 201, res)
	counterparty := data[party.Party](t, res)
	var today string
	if err := f.owner.QueryRow(ctx, "SELECT (clock_timestamp() AT TIME ZONE 'Asia/Hong_Kong')::date::text").Scan(&today); err != nil {
		t.Fatal(err)
	}
	clock, err := time.Parse(time.DateOnly, today)
	if err != nil {
		t.Fatal(err)
	}
	future := clock.AddDate(0, 0, 7).Format(time.DateOnly)
	end := clock.AddDate(1, 0, 0).Format(time.DateOnly)
	text := func(v string) *string { return &v }
	get := func(t *testing.T, id string) contracts.Contract {
		t.Helper()
		rec, res := f.with(t).request("GET", "/v1/contracts/"+id, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		return data[contracts.Contract](t, res)
	}
	var fixturePlan *contracts.PaymentSchedule
	checkedApprovalNotifications := false
	newContract := func(t *testing.T, start string, parent *string, approve bool) contracts.Contract {
		t.Helper()
		input := contracts.Create{Content: contracts.Content{PaymentSchedule: fixturePlan, Title: "Synthetic signed contract", Direction: text("payable"), Type: text("service"), Category: "execution", CounterpartyID: &counterparty.ID, CounterpartyRevisionID: &counterparty.CurrentRevisionID, OwnerID: &f.alice, Currency: text("HKD"), Amount: text("12.34"), StartDate: &start, EndDate: &end, DraftFileVersionID: &original.Version.ID}, ProjectID: original.File.ProjectID, Classification: "project_restricted", ParentContractID: parent, Reason: "isolated execution contract"}
		if fixturePlan != nil && fixturePlan.Mode == "no_payment" {
			input.Direction = text("none")
			input.Amount = text("0")
		}
		rec, res := f.with(t).request("POST", "/v1/contracts", f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 201, res)
		d := data[contracts.Contract](t, res)
		if !approve {
			return d
		}
		rec, res = f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "contract", BusinessID: d.ID, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, Reason: "prepare exact draft"}, nil)
		expect(t, rec, 201, res)
		r := data[workflow.Request](t, res)
		rec, res = f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		p := data[workflow.SubmissionPreview](t, res)
		rec, res = f.with(t).request("POST", "/v1/requests/"+r.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, WorkflowDefinitionID: p.Workflow.ID, PreviewHash: p.PreviewHash, Reason: "human confirms exact approval preview"}, nil)
		expect(t, rec, 200, res)
		r = data[workflow.Request](t, res)
		checkNotifications := !checkedApprovalNotifications
		if checkNotifications {
			drainNotifications(t, f)
			requireBusinessNotification(t, f, f.bobToken, r.ID, "approval", true)
		}
		rec, res = f.with(t).request("POST", "/v1/approval-steps/"+r.Steps[0].ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: r.Steps[0].Version, RequestVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, Decision: "approve", Reason: "independent human review"}, nil)
		expect(t, rec, 200, res)
		if checkNotifications {
			drainNotifications(t, f)
			requireBusinessNotification(t, f, f.aliceToken, r.ID, "request_result", false)
			requireBusinessNotification(t, f, f.bobToken, r.ID, "approval", false)
			checkedApprovalNotifications = true
		}
		return get(t, d.ID)
	}
	signatureInput := func(d contracts.Contract) contracts.SignatureInput {
		return contracts.SignatureInput{ExpectedVersion: d.Version, RevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash, SignedOn: today, SignedFileVersionID: original.Version.ID, Attestation: "Isolated synthetic evidence; verified against exact approved revision", Reason: "record an actual signature fact in controlled fixture"}
	}
	sign := func(t *testing.T, d contracts.Contract) contracts.Contract {
		t.Helper()
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), signatureInput(d), nil)
		expect(t, rec, 201, res)
		return data[contracts.Contract](t, res)
	}
	activation := func(d contracts.Contract) contracts.ActivationInput {
		if d.Signature == nil {
			t.Fatal("signature fixture unavailable")
		}
		return contracts.ActivationInput{ExpectedVersion: d.Version, Action: "activate", RevisionID: d.CurrentRevisionID, ContentHash: d.ContentHash, SignatureID: d.Signature.ID, Reason: "human confirms recorded signature and reached approved date"}
	}
	activate := func(t *testing.T, d contracts.Contract) contracts.Contract {
		t.Helper()
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/transition", f.aliceToken, core.NewID(), activation(d), nil)
		expect(t, rec, 200, res)
		return data[contracts.Contract](t, res)
	}
	var signedFuture, active contracts.Contract
	var signatureKey string
	var cachedSignature contracts.SignatureInput

	t.Run("only the exact approved content and an actual nonfuture signature can be recorded", func(t *testing.T) {
		draft := newContract(t, today, nil, false)
		if _, err := f.owner.Exec(ctx, "UPDATE contracts SET active_revision_id=current_revision_id WHERE id=$1", draft.ID); err == nil {
			t.Fatal("draft claimed active content")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE contracts SET approved_revision_id=current_revision_id WHERE id=$1", draft.ID); err == nil {
			t.Fatal("draft claimed approved content")
		}
		rec, res := f.with(t).request("POST", "/v1/contracts/"+draft.ID+"/signature", f.aliceToken, core.NewID(), signatureInput(draft), nil)
		expect(t, rec, 409, res)
		d := newContract(t, future, nil, true)
		input := signatureInput(d)
		input.RevisionID = core.NewID()
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 409, res)
		input = signatureInput(d)
		input.SignedOn = future
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 422, res)
		input = signatureInput(d)
		input.Attestation = " "
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 422, res)
		if current := get(t, d.ID); current.Signature != nil || current.Version != d.Version {
			t.Fatal(current)
		}
		signedFuture = d
	})
	t.Run("signature fixes the approved request and signed file without activating future content", func(t *testing.T) {
		cachedSignature = signatureInput(signedFuture)
		signatureKey = core.NewID()
		rec, res := f.with(t).request("POST", "/v1/contracts/"+signedFuture.ID+"/signature", f.aliceToken, signatureKey, cachedSignature, nil)
		expect(t, rec, 201, res)
		signedFuture = data[contracts.Contract](t, res)
		signature := signedFuture.Signature
		if signature == nil || signature.EffectiveOn != future || signature.RevisionID != signedFuture.CurrentRevisionID || signature.SignedFile == nil || signature.SignedFile.VersionID != original.Version.ID || signedFuture.Lifecycle != "pending_signature" || signedFuture.ActiveRevisionID != nil || signedFuture.CanActivate || signedFuture.CanRegisterSignature {
			t.Fatal(signedFuture)
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE org_id=$1 AND owner_type='contract_signature' AND owner_id=$2 AND version_id=$3", f.org, signature.ID, original.Version.ID).Scan(&n); err != nil || n != 1 {
			t.Fatal(n, err)
		}
		var actor, session string
		var verified time.Time
		if err := f.owner.QueryRow(ctx, "SELECT actor_id,session_id,mfa_verified_at FROM contract_signatures WHERE id=$1", signature.ID).Scan(&actor, &session, &verified); err != nil || actor != f.alice || session == "" || time.Since(verified) > time.Minute {
			t.Fatal("missing actual session proof", err)
		}
		rec, res = f.with(t).request("POST", "/v1/contracts/"+signedFuture.ID+"/signature", f.aliceToken, signatureKey, cachedSignature, nil)
		expect(t, rec, 201, res)
		if data[contracts.Contract](t, res).Signature.ID != signature.ID {
			t.Fatal("replayed signature duplicated")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE contract_signatures SET signed_on=signed_on-1 WHERE id=$1", signature.ID); err == nil {
			t.Fatal("rewrote signature")
		}
	})
	t.Run("future activation is blocked by service and deferred database evidence", func(t *testing.T) {
		rec, res := f.with(t).request("POST", "/v1/contracts/"+signedFuture.ID+"/transition", f.aliceToken, core.NewID(), activation(signedFuture), nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "CONTRACT_NOT_YET_EFFECTIVE" {
			t.Fatal(res.Error)
		}
		if _, err := f.owner.Exec(ctx, "UPDATE contracts SET lifecycle='effective',active_revision_id=current_revision_id,version=version+1 WHERE id=$1", signedFuture.ID); err == nil {
			t.Fatal("activated without event and date")
		}
		if current := get(t, signedFuture.ID); current.Version != signedFuture.Version || current.ActiveRevisionID != nil {
			t.Fatal(current)
		}
	})
	t.Run("reached approved date requires explicit activation and preserves all approved content", func(t *testing.T) {
		d := newContract(t, today, nil, true)
		d = sign(t, d)
		if d.Lifecycle != "pending_signature" || !d.CanActivate {
			t.Fatal("signature auto activated", d)
		}
		active = activate(t, d)
		if active.Lifecycle != "effective" || active.ActiveRevisionID == nil || *active.ActiveRevisionID != active.CurrentRevisionID || active.CanEdit || active.CanRegisterSignature || active.CanActivate {
			t.Fatal(active)
		}
		if _, err := f.owner.Exec(ctx, "UPDATE contracts SET active_revision_id=NULL WHERE id=$1", active.ID); err == nil {
			t.Fatal("cleared active content")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE contracts SET lifecycle='draft',version=version+1 WHERE id=$1", active.ID); err == nil {
			t.Fatal("returned signed contract to draft")
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM contract_lifecycle_events WHERE contract_id=$1)+(SELECT count(*) FROM contract_milestones WHERE contract_id=$1)", active.ID).Scan(&n); err != nil || n != 1 {
			t.Fatal(n, err)
		}
	})
	t.Run("two devices cannot record duplicate signature facts", func(t *testing.T) {
		d := newContract(t, today, nil, true)
		input := signatureInput(d)
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec, _ := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), input, nil)
				codes <- rec.Code
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
		current := get(t, d.ID)
		input = signatureInput(current)
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "CONTRACT_SIGNATURE_EXISTS" {
			t.Fatal(res.Error)
		}
	})
	t.Run("contract due scheduling requires an explicit organization timezone and advance policy", func(t *testing.T) {
		s := &notification.Service{DB: f.api.DB}
		disabled, e := s.ScheduleContracts(ctx, notification.ContractRule{})
		if e != nil || disabled.Enabled || disabled.Queued != 0 {
			t.Fatal(disabled, e)
		}
		rule := notification.ContractRule{OrgID: f.org, Timezone: "Asia/Hong_Kong", DaysBefore: []int{365}}
		first, e := s.ScheduleContracts(ctx, rule)
		if e != nil || !first.Enabled || first.Queued != 1 {
			t.Fatal(first, e)
		}
		again, e := s.ScheduleContracts(ctx, rule)
		if e != nil || again.Queued != 0 {
			t.Fatal("due policy duplicated its current lead", again, e)
		}
		_, e = f.api.DB.Exec(ctx, `INSERT INTO notification_due_events(id,org_id,contract_id,revision_id,due_on,days_before,timezone,event_id)
 SELECT $1,org_id,contract_id,revision_id,due_on,days_before-1,timezone,event_id FROM notification_due_events LIMIT 1`, core.NewID())
		if e == nil {
			t.Fatal("database accepted a due fact without its matching actual outbox event")
		}
		drainNotifications(t, f)
		n := requireBusinessNotification(t, f, f.aliceToken, active.ID, "contract_due", true)
		if n.CurrentState != "effective" || n.Target.WebPath != "/contracts/"+active.ID || notificationFor(t, f, f.bobToken, active.ID) != nil {
			t.Fatal(n)
		}
	})
	t.Run("concurrent activation has one event and old signature replay does not activate again", func(t *testing.T) {
		d := newContract(t, today, nil, true)
		input := signatureInput(d)
		key := core.NewID()
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, key, input, nil)
		expect(t, rec, 201, res)
		d = data[contracts.Contract](t, res)
		in := activation(d)
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec, _ := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/transition", f.aliceToken, core.NewID(), in, nil)
				codes <- rec.Code
			}()
		}
		wg.Wait()
		close(codes)
		counts := map[int]int{}
		for code := range codes {
			counts[code]++
		}
		if counts[200] != 1 || counts[409] != 1 {
			t.Fatal(counts)
		}
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, key, input, nil)
		expect(t, rec, 201, res)
		old := data[contracts.Contract](t, res)
		if old.Lifecycle != "pending_signature" || old.CanActivate || old.Signature.ID != d.Signature.ID {
			t.Fatal("replay rewrote historic result or offered stale action", old)
		}
	})
	t.Run("a supplement waits for its parent and never replaces the parents effective content", func(t *testing.T) {
		parent := newContract(t, today, nil, true)
		child := sign(t, newContract(t, today, &parent.ID, true))
		rec, res := f.with(t).request("POST", "/v1/contracts/"+child.ID+"/transition", f.aliceToken, core.NewID(), activation(child), nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "CONTRACT_PARENT_NOT_EFFECTIVE" {
			t.Fatal(res.Error)
		}
		parent = activate(t, sign(t, parent))
		child = activate(t, child)
		current := get(t, parent.ID)
		if child.Lifecycle != "effective" || current.CurrentRevisionID != parent.CurrentRevisionID || current.ActiveRevisionID == nil || *current.ActiveRevisionID != *parent.ActiveRevisionID {
			t.Fatal("supplement replaced original effective record")
		}
	})

	makePlan := func() *contracts.PaymentSchedule {
		return &contracts.PaymentSchedule{Mode: "scheduled", Reason: "Fixed synthetic schedule", Items: []contracts.PaymentPlanItem{
			{ID: core.NewID(), Title: "Initial payment", Amount: text("6"), DueDate: &today, Conditions: "After signature"},
			{ID: core.NewID(), Title: "Final payment", Amount: text("6.34"), DueDate: &future, Conditions: "After acceptance"}}}
	}
	milestones := func(t *testing.T, id, token string, status int) []contracts.Milestone {
		t.Helper()
		rec, res := f.with(t).request("GET", "/v1/contracts/"+id+"/milestones", token, "", nil, nil)
		expect(t, rec, status, res)
		if status != 200 {
			return nil
		}
		return data[core.Page[contracts.Milestone]](t, res).Items
	}
	t.Run("schedule revisions are exact immutable facts and reject stale invalid or incomplete plans", func(t *testing.T) {
		d := newContract(t, today, nil, false)
		plan := makePlan()
		plan.Items[0].DueDate = nil
		input := contracts.ScheduleInput{ExpectedVersion: d.Version, BaseRevisionID: d.CurrentRevisionID, PaymentSchedule: plan, Reason: "record partial draft terms"}
		key := core.NewID()
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/milestones", f.aliceToken, key, input, nil)
		expect(t, rec, 200, res)
		current := data[contracts.Contract](t, res)
		if current.CurrentRevisionID == d.CurrentRevisionID || current.ContentHash == d.ContentHash || current.PayloadSchemaVersion != 2 || current.PaymentSchedule.Items[0].Amount == nil || *current.PaymentSchedule.Items[0].Amount != "6.000000" {
			t.Fatal(current)
		}
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/milestones", f.aliceToken, key, input, nil)
		expect(t, rec, 200, res)
		if data[contracts.Contract](t, res).CurrentRevisionID != current.CurrentRevisionID {
			t.Fatal("replay duplicated schedule revision")
		}
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/milestones", f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 409, res)
		rec, res = f.with(t).request("GET", "/v1/contracts/"+d.ID+"/revisions/"+d.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		if data[contracts.Revision](t, res).PaymentSchedule != nil {
			t.Fatal("previous revision was changed")
		}
		for _, change := range []func(*contracts.PaymentSchedule){
			func(p *contracts.PaymentSchedule) { p.Items[1].ID = p.Items[0].ID }, func(p *contracts.PaymentSchedule) { p.Items[0].Amount = text("0") },
			func(p *contracts.PaymentSchedule) { p.Items[0].Amount = text("0.001") }, func(p *contracts.PaymentSchedule) { p.Items[0].Amount = text("7") },
			func(p *contracts.PaymentSchedule) { p.Items[0].DueDate = text("2026-02-30") }, func(p *contracts.PaymentSchedule) { p.Mode = "no_payment"; p.Items = []contracts.PaymentPlanItem{} },
		} {
			invalid := makePlan()
			change(invalid)
			in := contracts.ScheduleInput{ExpectedVersion: current.Version, BaseRevisionID: current.CurrentRevisionID, PaymentSchedule: invalid, Reason: "reject contradictory schedule"}
			rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/milestones", f.aliceToken, core.NewID(), in, nil)
			expect(t, rec, 422, res)
		}
		rec, res = f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: "contract", BusinessID: d.ID, ExpectedBusinessVersion: current.Version, BusinessRevisionID: current.CurrentRevisionID, BusinessContentHash: current.ContentHash, Reason: "draft incomplete terms"}, nil)
		expect(t, rec, 201, res)
		r := data[workflow.Request](t, res)
		rec, res = f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 422, res)
		if len(milestones(t, d.ID, f.aliceToken, 200)) != 0 {
			t.Fatal("draft plan became execution facts")
		}
		milestones(t, d.ID, admin, 404)
	})
	t.Run("activation materializes only the exact approved plan without recording actual money movement", func(t *testing.T) {
		fixturePlan = makePlan()
		defer func() { fixturePlan = nil }()
		d := newContract(t, today, nil, true)
		if len(milestones(t, d.ID, f.aliceToken, 200)) != 0 {
			t.Fatal("approval materialized milestones")
		}
		d = sign(t, d)
		if len(milestones(t, d.ID, f.aliceToken, 200)) != 0 {
			t.Fatal("signature materialized milestones")
		}
		input := activation(d)
		key := core.NewID()
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/transition", f.aliceToken, key, input, nil)
		expect(t, rec, 200, res)
		d = data[contracts.Contract](t, res)
		rows := milestones(t, d.ID, f.aliceToken, 200)
		if len(rows) != 2 || rows[0].PlanItemID != fixturePlan.Items[0].ID || rows[1].Amount != "6.340000" || rows[0].RevisionID != d.CurrentRevisionID || rows[0].Direction != "payable" || rows[0].Currency != "HKD" {
			t.Fatal(rows)
		}
		rec, res = f.with(t).request("GET", "/v1/requests/"+*d.RequestID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		r := data[workflow.Request](t, res)
		if r.Payload.BusinessPayloadSchemaVersion != 2 || r.Payload.ContractContent().PaymentSchedule.Items[1].ID != rows[1].PlanItemID {
			t.Fatal("approval lost exact plan")
		}
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/transition", f.aliceToken, key, input, nil)
		expect(t, rec, 200, res)
		if again := milestones(t, d.ID, f.aliceToken, 200); len(again) != 2 || again[0].ID != rows[0].ID {
			t.Fatal("activation replay duplicated milestones")
		}
		for _, sql := range []string{"UPDATE contract_milestones SET amount=99 WHERE contract_id=$1", "DELETE FROM contract_milestones WHERE contract_id=$1"} {
			if _, err := f.owner.Exec(ctx, sql, d.ID); err == nil {
				t.Fatal("immutable execution terms changed")
			}
		}
		if _, err := f.owner.Exec(ctx, `INSERT INTO contract_milestones SELECT gen_random_uuid(),org_id,contract_id,revision_id,gen_random_uuid(),position,title,direction,currency,amount,due_date,conditions,created_at FROM contract_milestones WHERE id=$1`, rows[0].ID); err == nil {
			t.Fatal("unapproved milestone inserted")
		}
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/milestones", f.aliceToken, core.NewID(), contracts.ScheduleInput{ExpectedVersion: d.Version, BaseRevisionID: d.CurrentRevisionID, PaymentSchedule: makePlan(), Reason: "attempt approved term rewrite"}, nil)
		expect(t, rec, 409, res)
	})

	t.Run("explicit no payment terms are distinct from unknown and create no execution nodes", func(t *testing.T) {
		fixturePlan = &contracts.PaymentSchedule{Mode: "no_payment", Reason: "Synthetic no consideration agreement", Items: []contracts.PaymentPlanItem{}}
		defer func() { fixturePlan = nil }()
		d := sign(t, newContract(t, today, nil, true))
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/transition", f.aliceToken, core.NewID(), activation(d), nil)
		expect(t, rec, 200, res)
		current := data[contracts.Contract](t, res)
		if current.PaymentSchedule == nil || current.PaymentSchedule.Mode != "no_payment" || len(milestones(t, d.ID, f.aliceToken, 200)) != 0 {
			t.Fatal(current)
		}
	})
	t.Run("legacy schema one drafts and digests survive new schedule revisions", func(t *testing.T) {
		source := newContract(t, today, nil, false)
		id, revision := core.NewID(), core.NewID()
		payload, _ := json.Marshal(source.Content)
		var content map[string]json.RawMessage
		_ = json.Unmarshal(payload, &content)
		delete(content, "payment_schedule")
		raw, _ := json.Marshal(map[string]any{"schema": 1, "contract_id": id, "project_id": source.ProjectID, "parent_contract_id": nil, "classification": source.Classification, "content": content})
		canonical, _ := core.CanonicalJSON(raw)
		hash := core.Digest(canonical)
		tx, err := f.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, `INSERT INTO contracts(id,org_id,contract_no,project_id,creator_id,classification,current_revision_id) VALUES($1,$2,$6,$3,$4,'project_restricted',$5)`, id, f.org, source.ProjectID, f.alice, revision, "CP-C-legacy-"+id)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO contract_revisions(id,org_id,contract_id,seq,base_revision_id,payload_schema_version,content_hash,title,direction,type,category,counterparty_id,counterparty_revision_id,owner_id,currency,amount,tax_amount,start_date,end_date,renewal_terms,termination_terms,notes,draft_file_version_id,author_id,reason)
		 SELECT $1,org_id,$2,1,NULL,1,$3,title,direction,type,category,counterparty_id,counterparty_revision_id,owner_id,currency,amount,tax_amount,start_date,end_date,renewal_terms,termination_terms,notes,draft_file_version_id,author_id,'original schema one fixture' FROM contract_revisions WHERE id=$4`, revision, id, hash, source.CurrentRevisionID)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		legacy := get(t, id)
		if legacy.PayloadSchemaVersion != 1 || legacy.ContentHash != hash || legacy.PaymentSchedule != nil {
			t.Fatal(legacy)
		}
		rec, res := f.with(t).request("POST", "/v1/contracts/"+id+"/milestones", f.aliceToken, core.NewID(), contracts.ScheduleInput{ExpectedVersion: legacy.Version, BaseRevisionID: revision, PaymentSchedule: makePlan(), Reason: "explicit revision with known terms"}, nil)
		expect(t, rec, 200, res)
		current := data[contracts.Contract](t, res)
		if current.PayloadSchemaVersion != 2 || current.CurrentRevisionID == revision {
			t.Fatal(current)
		}
		rec, res = f.with(t).request("GET", "/v1/contracts/"+id+"/revisions/"+revision, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		old := data[contracts.Revision](t, res)
		if old.PayloadSchemaVersion != 1 || old.ContentHash != hash || old.PaymentSchedule != nil {
			t.Fatal("legacy facts changed")
		}
	})
	failAudit := func(t *testing.T, event string) {
		t.Helper()
		exec(t, `CREATE FUNCTION fail_contract_execution_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='`+event+`' THEN RAISE EXCEPTION 'isolated execution audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_contract_execution_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_contract_execution_audit()`)
	}
	dropAudit := func(t *testing.T) {
		t.Helper()
		exec(t, "DROP TRIGGER IF EXISTS fail_contract_execution_audit ON audit_events; DROP FUNCTION IF EXISTS fail_contract_execution_audit()")
	}
	t.Run("signature audit failure rolls back signature file pin head version and replay result", func(t *testing.T) {
		d := newContract(t, today, nil, true)
		in := signatureInput(d)
		key := core.NewID()
		var pinsBefore int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE org_id=$1 AND owner_type='contract_signature'", f.org).Scan(&pinsBefore); err != nil {
			t.Fatal(err)
		}
		failAudit(t, "contract.signature.registered")
		defer dropAudit(t)
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, key, in, nil)
		expect(t, rec, 500, res)
		if current := get(t, d.ID); current.Signature != nil || current.Version != d.Version {
			t.Fatal(current)
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE org_id=$1 AND owner_type='contract_signature'", f.org).Scan(&n); err != nil || n != pinsBefore {
			t.Fatal(n, err)
		}
		dropAudit(t)
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, key, in, nil)
		expect(t, rec, 201, res)
	})
	t.Run("activation audit failure preserves signed pending content and retries safely", func(t *testing.T) {
		fixturePlan = makePlan()
		defer func() { fixturePlan = nil }()
		d := sign(t, newContract(t, today, nil, true))
		in := activation(d)
		key := core.NewID()
		failAudit(t, "contract.activated")
		defer dropAudit(t)
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/transition", f.aliceToken, key, in, nil)
		expect(t, rec, 500, res)
		if current := get(t, d.ID); current.Lifecycle != "pending_signature" || current.ActiveRevisionID != nil || current.Version != d.Version {
			t.Fatal(current)
		}
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM contract_lifecycle_events WHERE contract_id=$1", d.ID).Scan(&n); err != nil || n != 0 {
			t.Fatal(n, err)
		}
		dropAudit(t)
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/transition", f.aliceToken, key, in, nil)
		expect(t, rec, 200, res)
	})
	t.Run("manual session MFA is checked before replay and actor timestamps cannot spoof it", func(t *testing.T) {
		actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		actual := actor.MFAVerifiedAt
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		defer exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		service := contracts.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
		raw, _ := json.Marshal(cachedSignature)
		now := time.Now()
		actor.MFAVerifiedAt = &now
		_, err = service.RegisterSignature(ctx, core.Command{Actor: actor, RequestID: core.NewID(), Key: signatureKey, Route: "POST /v1/contracts/" + signedFuture.ID + "/signature", Body: raw}, signedFuture.ID, cachedSignature)
		if core.PublicError(err).Code != "REAUTHENTICATION_REQUIRED" {
			t.Fatal(err)
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=$2 WHERE id=$1", actor.SessionID, actual)
		actor.RunID = core.NewID()
		_, err = service.RegisterSignature(ctx, core.Command{Actor: actor, RequestID: core.NewID(), Key: core.NewID(), Route: "POST /v1/contracts/" + signedFuture.ID + "/signature", Body: raw}, signedFuture.ID, cachedSignature)
		if status := core.PublicError(err).Status; status != 403 && status != 404 {
			t.Fatal(err)
		}
	})
	t.Run("current file access and availability fence recording and cached linked disclosure", func(t *testing.T) {
		d := newContract(t, today, nil, true)
		in := signatureInput(d)
		exec(t, "UPDATE file_versions SET availability='missing' WHERE id=$1", original.Version.ID)
		rec, res := f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 422, res)
		exec(t, "UPDATE file_versions SET availability='available' WHERE id=$1", original.Version.ID)
		exec(t, "UPDATE role_bindings SET valid_until=clock_timestamp()-interval '1 second' WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='project_manager')", f.org, f.alice)
		defer exec(t, "UPDATE role_bindings SET valid_until=NULL WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='project_manager')", f.org, f.alice)
		rec, res = f.with(t).request("POST", "/v1/contracts/"+d.ID+"/signature", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 404, res)
		rec, res = f.with(t).request("POST", "/v1/contracts/"+signedFuture.ID+"/signature", f.aliceToken, signatureKey, cachedSignature, nil)
		expect(t, rec, 201, res)
		value := data[contracts.Contract](t, res)
		if value.Signature.SignedFile != nil || value.DraftFile != nil {
			t.Fatal("cached signature leaked file after access expiry")
		}
	})
	if !t.Failed() && (os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") == "1" || os.Getenv("CP_BROWSER_MOBILE_FIXTURE") == "1") {
		fixturePlan = makePlan()
		editable := newContract(t, today, nil, false)
		conflict := newContract(t, today, nil, true)
		if os.Getenv("CP_BROWSER_MOBILE_FIXTURE") == "1" {
			rec, res = f.request("GET", "/v1/classification-policies", admin, "", nil, nil)
			expect(t, rec, 200, res)
			for _, policy := range data[core.Page[identity.ClassificationPolicy]](t, res).Items {
				if policy.Level == "confidential" {
					continue
				}
				existing := make(map[string]bool)
				for _, channel := range policy.AllowedChannels {
					existing[channel] = true
				}
				if len(existing) == 4 && existing["web"] && existing["android"] && existing["wechat"] && existing["douyin"] {
					continue
				}
				rec, res = f.request("POST", "/v1/classification-policies", admin, core.NewID(), identity.ClassificationPolicyInput{ExpectedVersion: policy.Version, BasePolicyID: policy.ID, Level: policy.Level, AIAllowed: policy.AIAllowed, AllowedChannels: []string{"web", "android", "wechat", "douyin"}, Reason: "Synthetic isolated mobile channel proof"}, nil)
				expect(t, rec, 201, res)
			}
		}
		holdBrowserBusinessFixture(t, f, map[string]any{"contract_id": editable.ID, "conflict_contract_id": conflict.ID, "future_contract_id": signedFuture.ID,
			"file_version_id": original.Version.ID, "file_id": original.File.ID, "file_seq": original.Version.Seq, "file_name": original.File.Name, "project_id": original.File.ProjectID, "today": today})
	}

}
