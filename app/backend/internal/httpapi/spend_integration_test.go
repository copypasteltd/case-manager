package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/spend"
	"copypaste.hk/platform/internal/workflow"
)

func TestSpendDocumentsAndApprovals(t *testing.T) {
	u, old, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	p := func(v string) *string { return &v }
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	bind := func(user, key string) string {
		id := core.NewID()
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
		return id
	}
	for _, key := range []string{"purchase_manager", "claim_submitter", "party_manager", "request_manager"} {
		bind(f.alice, key)
	}
	reader := bind(f.bob, "purchase_reader")
	bind(f.bob, "claim_reader")
	bind(f.bob, "party_manager")
	bind(f.bob, "approval_reviewer")
	admin := f.administratorToken()
	rec, res := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic spend supplier", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: old.File.ProjectID, Classification: "project_restricted", Reason: "isolated spend fixture"}, nil)
	expect(t, rec, 201, res)
	supplier := data[party.Party](t, res)
	base := func(kind string) spend.Create {
		c := spend.Content{Title: "Exact " + kind + " fixture", Category: "spend.test", Purpose: "Necessary project tools", OwnerID: &f.alice, Currency: p("HKD"), PricingBasis: "net_plus_tax", RoundingMode: "half_up", ExpectedDeliveryDate: p("2026-11-20"), SupplierCandidates: []spend.Supplier{{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}}, QuoteVersionIDs: []string{old.Version.ID}, Lines: []spend.Line{{ID: core.NewID(), Description: "Project tools", Quantity: p("3"), Unit: "licence", UnitPrice: p("0.335"), TaxAmount: p("0.02"), DiscountAmount: p("0.01")}}}
		if kind == "claim" {
			c.PricingBasis = "gross_includes_tax"
			c.ExpectedDeliveryDate = nil
			c.SupplierCandidates = nil
			c.QuoteVersionIDs = nil
			c.PeriodStart = p("2026-10-01")
			c.PeriodEnd = p("2026-10-31")
			c.Lines = []spend.Line{{ID: core.NewID(), Description: "Employee expense", DeclaredAmount: p("12.34"), TaxAmount: p("0.34"), ExpenseDate: p("2026-10-15"), Category: "tools", Supplier: &spend.Supplier{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, ReceiptVersionIDs: []string{old.Version.ID}, Funding: "employee_advance", DuplicateReceiptReason: "Reviewed shared synthetic fixture evidence"}, {ID: core.NewID(), Description: "Company paid expense", DeclaredAmount: p("20"), ExpenseDate: p("2026-10-16"), Category: "hosting", Supplier: &spend.Supplier{PartyID: supplier.ID, RevisionID: supplier.CurrentRevisionID}, Funding: "company_paid", MissingReceiptReason: "Receipt pending supplier confirmation", DuplicateReceiptReason: "Reviewed repeated synthetic company-paid fixture"}}
		}
		return spend.Create{Content: c, ProjectID: old.File.ProjectID, Classification: "project_restricted", Reason: "isolated commercial document"}
	}
	path := func(kind string) string {
		if kind == "claim" {
			return "/v1/claims"
		}
		return "/v1/purchases"
	}
	create := func(t *testing.T, kind string, in spend.Create) spend.Record {
		t.Helper()
		r, v := f.with(t).request("POST", path(kind), f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		return data[spend.Record](t, v)
	}
	get := func(t *testing.T, kind, id string) spend.Record {
		t.Helper()
		r, v := f.with(t).request("GET", path(kind)+"/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[spend.Record](t, v)
	}
	prepare := func(t *testing.T, d spend.Record) workflow.Request {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/requests", f.aliceToken, core.NewID(), workflow.RequestCreate{Kind: d.Kind, BusinessID: d.ID, ExpectedBusinessVersion: d.Version, BusinessRevisionID: d.CurrentRevisionID, BusinessContentHash: d.ContentHash, Reason: "prepare immutable commercial evidence"}, nil)
		expect(t, r, 201, v)
		return data[workflow.Request](t, v)
	}
	submit := func(t *testing.T, r workflow.Request) workflow.Request {
		t.Helper()
		rec, res := f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		preview := data[workflow.SubmissionPreview](t, res)
		rec, res = f.with(t).request("POST", "/v1/requests/"+r.ID+"/submit", f.aliceToken, core.NewID(), workflow.SubmitInput{ExpectedVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, WorkflowDefinitionID: preview.Workflow.ID, PreviewHash: preview.PreviewHash, Reason: "submit reviewed snapshot"}, nil)
		expect(t, rec, 200, res)
		return data[workflow.Request](t, res)
	}
	decide := func(t *testing.T, r workflow.Request, decision string) workflow.Request {
		t.Helper()
		step := r.Steps[0]
		rec, res := f.with(t).request("POST", "/v1/approval-steps/"+step.ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: step.Version, RequestVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, Decision: decision, Reason: "actual assigned reviewer"}, nil)
		expect(t, rec, 200, res)
		return data[workflow.Request](t, res)
	}
	for _, kind := range []string{"purchase", "claim"} {
		rec, res = f.request("POST", "/v1/workflow-definitions", admin, core.NewID(), workflow.DefinitionInput{Key: "spend." + kind, Name: "Synthetic " + kind + " policy", Kind: kind, Category: "spend.test", Currency: "HKD", MinAmount: "0", Enabled: true, SelfApprovalPolicy: "deny", WithdrawalAllowed: true, Steps: []workflow.StepInput{{Title: "Commercial review", AssignedUserID: f.bob}}, Reason: "explicit isolated policy"}, nil)
		expect(t, rec, 201, res)
	}

	t.Run("incomplete drafts remain nullable and idempotent without synthetic expenses", func(t *testing.T) {
		for _, kind := range []string{"purchase", "claim"} {
			in := spend.Create{Content: spend.Content{Title: "Incomplete " + kind}, ProjectID: old.File.ProjectID, Classification: "project_restricted", Reason: "save incomplete work"}
			key := core.NewID()
			rec, res := f.with(t).request("POST", path(kind), f.aliceToken, key, in, nil)
			expect(t, rec, 201, res)
			d := data[spend.Record](t, res)
			if d.Amount != nil || d.EmployeeReimbursementAmount != nil || d.RequestID != nil || d.ApprovedRevisionID != nil || d.Lifecycle != "draft" || !d.CanEdit {
				t.Fatal(d)
			}
			rec, res = f.with(t).request("POST", path(kind), f.aliceToken, key, in, nil)
			expect(t, rec, 201, res)
			if data[spend.Record](t, res).ID != d.ID {
				t.Fatal("duplicate document")
			}
			rec, res = f.with(t).request("GET", path(kind)+"/"+d.ID, admin, "", nil, nil)
			expect(t, rec, 404, res)
			r := prepare(t, d)
			rec, res = f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
			expect(t, rec, 422, res)
		}
	})
	t.Run("exact purchase arithmetic and employee advance totals pin commercial revisions", func(t *testing.T) {
		for _, kind := range []string{"purchase", "claim"} {
			d := create(t, kind, base(kind))
			want := "1.020000"
			if kind == "claim" {
				want = "32.340000"
				if d.EmployeeReimbursementAmount == nil || *d.EmployeeReimbursementAmount != "12.340000" {
					t.Fatal(d)
				}
			} else if d.EmployeeReimbursementAmount != nil {
				t.Fatal("purchase manufactured employee cash")
			}
			if d.Amount == nil || *d.Amount != want || len(d.Attachments) != 1 || len(d.Suppliers) != 1 {
				t.Fatal(d)
			}
			var count int
			if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type=$1 AND owner_id=$2", kind+"_revision", d.CurrentRevisionID).Scan(&count); err != nil || count != 1 {
				t.Fatal(count, err)
			}
			rec, res := f.with(t).request("GET", path(kind)+"/"+d.ID+"/revisions", f.aliceToken, "", nil, nil)
			expect(t, rec, 200, res)
			if len(data[core.Page[spend.Revision]](t, res).Items) != 1 {
				t.Fatal("missing history")
			}
			for i := range d.LineValues {
				d.LineValues[i].DuplicateReceipt = false
			}
			raw, _ := json.Marshal(map[string]any{"schema": 1, "kind": kind, "document_id": d.ID, "project_id": d.ProjectID, "classification": d.Classification, "content": d.Content, "line_values": d.LineValues, "amount": d.Amount, "employee_reimbursement_amount": d.EmployeeReimbursementAmount})
			canonical, err := core.CanonicalJSON(raw)
			if err != nil || core.Digest(canonical) != d.ContentHash {
				t.Fatal("commercial digest did not bind exact content and totals", err)
			}
		}
	})
	t.Run("invalid commercial inputs cannot persist partial drafts", func(t *testing.T) {
		for _, change := range []func(*spend.Create){func(in *spend.Create) { in.Lines[0].Quantity = p("0") }, func(in *spend.Create) { in.Lines[0].TaxAmount = p("0.001") }, func(in *spend.Create) { in.Currency = p("ZZZ") }, func(in *spend.Create) { in.Lines[0].DiscountAmount = p("100") }, func(in *spend.Create) { in.Lines[0].DeclaredAmount = p("1") }, func(in *spend.Create) { in.Lines = append(in.Lines, in.Lines[0]) }} {
			in := base("purchase")
			change(&in)
			rec, res := f.with(t).request("POST", "/v1/purchases", f.aliceToken, core.NewID(), in, nil)
			expect(t, rec, 422, res)
		}
		in := base("claim")
		in.Lines[0].ExpenseDate = p("2026-11-01")
		rec, res := f.with(t).request("POST", "/v1/claims", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 422, res)
	})
	t.Run("concurrent editors have one winner and keep the prior commercial content", func(t *testing.T) {
		d := create(t, "purchase", base("purchase"))
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for _, note := range []string{"Editor A", "Editor B"} {
			wg.Add(1)
			go func(note string) {
				defer wg.Done()
				c := d.Content
				c.Notes = note
				r, _ := f.request("PATCH", "/v1/purchases/"+d.ID, f.aliceToken, core.NewID(), spend.Patch{Content: c, ExpectedVersion: d.Version, BaseRevisionID: d.CurrentRevisionID, Reason: "concurrent edit"}, nil)
				codes <- r.Code
			}(note)
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
		rec, res := f.with(t).request("GET", "/v1/purchases/"+d.ID+"/revisions/"+d.CurrentRevisionID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		if data[spend.Revision](t, res).Notes != "" {
			t.Fatal("prior revision rewritten")
		}
	})
	t.Run("both adapters submit exact evidence and approval never marks execution or cash paid", func(t *testing.T) {
		for _, kind := range []string{"purchase", "claim"} {
			d := create(t, kind, base(kind))
			r := submit(t, prepare(t, d))
			var c spend.Content
			if err := json.Unmarshal(r.Payload.Content, &c); err != nil || len(c.Lines) != len(d.Lines) || r.Payload.DocumentNo != d.DocumentNo || r.Payload.ContractNo != "" {
				t.Fatal(r, err)
			}
			rec, res := f.with(t).request("GET", "/v1/approval-tasks", f.bobToken, "", nil, nil)
			expect(t, rec, 200, res)
			found := false
			for _, step := range data[core.Page[workflow.ApprovalStep]](t, res).Items {
				found = found || step.RequestID == r.ID
			}
			if !found {
				t.Fatal("generic approval list omitted spend")
			}
			current := get(t, kind, d.ID)
			rec, res = f.with(t).request("PATCH", path(kind)+"/"+d.ID, f.aliceToken, core.NewID(), spend.Patch{Content: current.Content, ExpectedVersion: current.Version, BaseRevisionID: current.CurrentRevisionID, Reason: "must remain frozen"}, nil)
			expect(t, rec, 409, res)
			r = decide(t, r, "approve")
			current = get(t, kind, d.ID)
			if r.State != "approved" || current.ApprovalState != "approved" || current.Lifecycle != "requested" || current.ApprovedRevisionID == nil || *current.ApprovedRevisionID != d.CurrentRevisionID || current.CanEdit {
				t.Fatal(current, r)
			}
		}
	})
	t.Run("return revise withdrawal and resubmission retain earlier approval instances", func(t *testing.T) {
		d := create(t, "claim", base("claim"))
		r := decide(t, submit(t, prepare(t, d)), "return")
		oldRevision := r.CurrentRevisionID
		current := get(t, "claim", d.ID)
		current.Notes = "Clarified business purpose"
		rec, res := f.with(t).request("PATCH", "/v1/claims/"+d.ID, f.aliceToken, core.NewID(), spend.Patch{Content: current.Content, ExpectedVersion: current.Version, BaseRevisionID: current.CurrentRevisionID, Reason: "respond to review"}, nil)
		expect(t, rec, 200, res)
		current = data[spend.Record](t, res)
		rec, res = f.with(t).request("POST", "/v1/requests/"+r.ID+"/revisions", f.aliceToken, core.NewID(), workflow.RequestRevise{ExpectedVersion: r.Version, ExpectedBusinessVersion: current.Version, BusinessRevisionID: current.CurrentRevisionID, BusinessContentHash: current.ContentHash, Reason: "fix clarified commercial content"}, nil)
		expect(t, rec, 200, res)
		r = submit(t, data[workflow.Request](t, res))
		instance := *r.ActiveInstanceID
		rec, res = f.with(t).request("POST", "/v1/requests/"+r.ID+"/withdraw", f.aliceToken, core.NewID(), workflow.WithdrawInput{ExpectedVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, Reason: "recheck before approval"}, nil)
		expect(t, rec, 200, res)
		r = data[workflow.Request](t, res)
		if get(t, "claim", d.ID).Lifecycle != "draft" {
			t.Fatal("withdrawal left business frozen")
		}
		r = submit(t, r)
		if *r.ActiveInstanceID == instance {
			t.Fatal("reused withdrawn instance")
		}
		rec, res = f.with(t).request("GET", "/v1/requests/"+r.ID+"/revisions/"+oldRevision, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		if data[workflow.Revision](t, res).Steps[0].State != "returned" {
			t.Fatal("old decision lost")
		}
	})
	t.Run("receipt reuse requires an explicit reason and does not disclose hidden records", func(t *testing.T) {
		in := base("claim")
		in.Lines[0].DuplicateReceiptReason = ""
		d := create(t, "claim", in)
		if !d.LineValues[0].DuplicateReceipt {
			t.Fatal("shared receipt warning missing")
		}
		r := prepare(t, d)
		rec, res := f.with(t).request("GET", "/v1/requests/"+r.ID+"/submission-preview", f.aliceToken, "", nil, nil)
		expect(t, rec, 422, res)
		rec, res = f.with(t).request("GET", "/v1/claims/"+d.ID, f.memberToken, "", nil, nil)
		expect(t, rec, 404, res)
	})
	t.Run("revoked business access removes tasks and refuses the real reviewer decision", func(t *testing.T) {
		d := create(t, "purchase", base("purchase"))
		r := submit(t, prepare(t, d))
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE id=$1", reader)
		rec, res := f.with(t).request("GET", "/v1/approval-tasks", f.bobToken, "", nil, nil)
		expect(t, rec, 200, res)
		for _, s := range data[core.Page[workflow.ApprovalStep]](t, res).Items {
			if s.RequestID == r.ID {
				t.Fatal("revoked business leaked into tasks")
			}
		}
		step := r.Steps[0]
		rec, res = f.with(t).request("POST", "/v1/approval-steps/"+step.ID+"/decisions", f.bobToken, core.NewID(), workflow.DecisionInput{ExpectedVersion: step.Version, RequestVersion: r.Version, RevisionID: r.CurrentRevisionID, ContentHash: r.ContentHash, Decision: "approve"}, nil)
		expect(t, rec, 404, res)
		reader = bind(f.bob, "purchase_reader")
	})
	t.Run("database pins immutable lines suppliers and file references", func(t *testing.T) {
		d := create(t, "purchase", base("purchase"))
		for _, sql := range []string{"UPDATE spend_revisions SET reason='rewrite' WHERE id=$1", "DELETE FROM spend_lines WHERE revision_id=$1", "DELETE FROM spend_revision_suppliers WHERE revision_id=$1", "DELETE FROM spend_revision_files WHERE revision_id=$1"} {
			if _, err := f.api.DB.Exec(ctx, sql, d.CurrentRevisionID); err == nil {
				t.Fatal("mutable revision boundary", sql)
			}
		}
		tx, err := f.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "UPDATE spend_documents SET document_no='forged',version=version+1 WHERE id=$1", d.ID); err == nil {
			t.Fatal("scope identity changed")
		}
	})
	t.Run("audit failure rolls back a commercial revision and the same operation can retry", func(t *testing.T) {
		d := create(t, "purchase", base("purchase"))
		exec(`CREATE FUNCTION fail_spend_audit() RETURNS trigger LANGUAGE plpgsql AS $x$ BEGIN IF NEW.action='purchase.revised' THEN RAISE EXCEPTION 'isolated audit failure'; END IF; RETURN NEW; END $x$; CREATE TRIGGER fail_spend_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_spend_audit()`)
		key := core.NewID()
		c := d.Content
		c.Notes = "Safe retry after audit is restored"
		in := spend.Patch{Content: c, ExpectedVersion: d.Version, BaseRevisionID: d.CurrentRevisionID, Reason: "audited revision"}
		rec, res := f.with(t).request("PATCH", "/v1/purchases/"+d.ID, f.aliceToken, key, in, nil)
		expect(t, rec, 500, res)
		if get(t, "purchase", d.ID).Version != d.Version {
			t.Fatal("partial revision survived audit failure")
		}
		exec("DROP TRIGGER fail_spend_audit ON audit_events; DROP FUNCTION fail_spend_audit()")
		rec, res = f.with(t).request("PATCH", "/v1/purchases/"+d.ID, f.aliceToken, key, in, nil)
		expect(t, rec, 200, res)
	})
	t.Run("archive preserves history and is unavailable after a request is linked", func(t *testing.T) {
		d := create(t, "purchase", base("purchase"))
		rec, res := f.with(t).request("POST", "/v1/purchases/"+d.ID+"/archive", f.aliceToken, core.NewID(), spend.ArchiveInput{ExpectedVersion: d.Version, Reason: "unused draft"}, nil)
		expect(t, rec, 200, res)
		archived := data[spend.Record](t, res)
		if archived.DeletedAt == nil || archived.CanEdit {
			t.Fatal(archived)
		}
		d = create(t, "purchase", base("purchase"))
		prepare(t, d)
		d = get(t, "purchase", d.ID)
		rec, res = f.with(t).request("POST", "/v1/purchases/"+d.ID+"/archive", f.aliceToken, core.NewID(), spend.ArchiveInput{ExpectedVersion: d.Version, Reason: "must retain approval history"}, nil)
		expect(t, rec, 409, res)
	})
	t.Run("rejection stays terminal while a fresh application preserves its business history", func(t *testing.T) {
		d := create(t, "purchase", base("purchase"))
		rejected := decide(t, submit(t, prepare(t, d)), "reject")
		current := get(t, "purchase", d.ID)
		if current.CanEdit || !current.CanPrepareRequest || current.ApprovalState != "rejected" {
			t.Fatal(current)
		}
		fresh := prepare(t, current)
		if fresh.ID == rejected.ID || fresh.State != "draft" {
			t.Fatal("reused rejected request")
		}
		rec, res := f.with(t).request("GET", "/v1/requests/"+rejected.ID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		if data[workflow.Request](t, res).State != "rejected" {
			t.Fatal("rejected history changed")
		}
	})
	t.Run("cursors bind business kind and historical content needs current editing authority", func(t *testing.T) {
		rec, res := f.with(t).request("GET", "/v1/purchases?limit=1", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		page := data[core.Page[spend.Record]](t, res)
		if page.NextCursor == nil || len(page.Items) != 1 {
			t.Fatal("missing bounded purchase page")
		}
		rec, res = f.with(t).request("GET", "/v1/claims?limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, rec, 400, res)
		rec, res = f.with(t).request("GET", "/v1/purchases/"+page.Items[0].ID+"/revisions", f.bobToken, "", nil, nil)
		expect(t, rec, 404, res)
	})
	t.Run("duplicate warnings disappear when only the current claim remains visible", func(t *testing.T) {
		d := create(t, "claim", base("claim"))
		if !d.LineValues[0].DuplicateReceipt {
			t.Fatal("expected visible duplicate fixture")
		}
		exec("UPDATE role_bindings rb SET revoked_at=clock_timestamp() FROM roles r WHERE rb.org_id=$1 AND rb.user_id=$2 AND rb.role_id=r.id AND r.key='claim_submitter' AND rb.revoked_at IS NULL", f.org, f.alice)
		id := core.NewID()
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT $1,$2,$3,id,'resource',$4,'project_restricted' FROM roles WHERE org_id=$2 AND key='claim_reader'`, id, f.org, f.alice, d.ID)
		current := get(t, "claim", d.ID)
		for _, line := range current.LineValues {
			if line.DuplicateReceipt {
				t.Fatal("hidden claims influenced a disclosure")
			}
		}
		exec("UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE id=$1", id)
		bind(f.alice, "claim_submitter")
	})
	t.Run("database independently refuses forged normalized commercial amounts", func(t *testing.T) {
		d := create(t, "purchase", base("purchase"))
		revision := core.NewID()
		tx, err := f.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		statements := []string{
			`INSERT INTO spend_revisions(id,org_id,document_id,seq,base_revision_id,payload_schema_version,payload,content_hash,amount,employee_reimbursement_amount,author_id,reason)
			 SELECT $1,org_id,document_id,seq+1,id,payload_schema_version,payload,content_hash,amount+1,employee_reimbursement_amount,author_id,'forged amount fixture' FROM spend_revisions WHERE id=$2`,
			`INSERT INTO spend_lines(org_id,revision_id,position,line_id,payload,amount) SELECT org_id,$1,position,line_id,payload,amount+1 FROM spend_lines WHERE revision_id=$2`,
			`INSERT INTO spend_revision_suppliers SELECT org_id,$1,party_id,party_revision_id FROM spend_revision_suppliers WHERE revision_id=$2`,
			`INSERT INTO spend_revision_files SELECT org_id,$1,version_id FROM spend_revision_files WHERE revision_id=$2`,
		}
		for _, sql := range statements {
			if _, err = tx.Exec(ctx, sql, revision, d.CurrentRevisionID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE spend_documents SET current_revision_id=$1,version=version+1 WHERE id=$2", revision, d.ID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err == nil || !strings.Contains(err.Error(), "commercial line values") {
			t.Fatal("expected independent numeric constraint", err)
		}
		if get(t, "purchase", d.ID).CurrentRevisionID != d.CurrentRevisionID {
			t.Fatal("forged commit changed current content")
		}
	})
}
