package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/party"
)

func TestContractDraftRevisions(t *testing.T) {
	u, old, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	bind := func(t *testing.T, user, key, scope string) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
        SELECT $1,$2,$3,id,$4,'project_restricted' FROM roles WHERE org_id=$2 AND key=$5`, core.NewID(), f.org, user, scope, key); err != nil {
			t.Fatal(err)
		}
	}
	bind(t, f.alice, "contract_manager", "org")
	bind(t, f.alice, "party_manager", "org")
	text := func(value string) *string { return &value }
	minimal := contracts.Create{Content: contracts.Content{Title: "Incomplete contract draft"}, ProjectID: old.File.ProjectID, Classification: "project_restricted", Reason: "isolated draft"}
	var draft, complete contracts.Contract
	var counterpart party.Party
	var submitted contracts.Patch
	var updateKey string
	get := func(t *testing.T, id string) contracts.Contract {
		t.Helper()
		rec, res := f.request("GET", "/v1/contracts/"+id, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		return data[contracts.Contract](t, res)
	}
	t.Run("reference selectors filter company scope before pagination and bind cursor", func(t *testing.T) {
		rec, res := f.request("POST", "/v1/folders", f.aliceToken, core.NewID(), map[string]any{"name": "Company selector fixtures", "classification": "internal"}, nil)
		expect(t, rec, 201, res)
		folder := data[storage.Folder](t, res)
		companyFiles := map[string]bool{}
		for _, name := range []string{"selector-a.bin", "selector-b.bin"} {
			id := core.NewID()
			companyFiles[id] = true
			// Metadata-only fixture, with no formal version or cloud availability claim.
			if _, err := f.owner.Exec(ctx, `INSERT INTO files(id,org_id,folder_id,name,name_key,classification,state,creator_id) VALUES($1,$2,$3,$4,$4,'internal','active',$5)`, id, f.org, folder.ID, name, f.alice); err != nil {
				t.Fatal(err)
			}
		}
		for _, name := range []string{"Company reference A", "Company reference B"} {
			rec, res = f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: name, Kind: "organization", Relationships: []string{"other"}}, Classification: "internal", Reason: "company selector fixture"}, nil)
			expect(t, rec, 201, res)
		}
		for _, route := range []string{"files", "parties"} {
			path := "/v1/" + route + "?company_only=true&limit=1"
			rec, res = f.request("GET", path, f.aliceToken, "", nil, nil)
			expect(t, rec, 200, res)
			page := data[core.Page[map[string]any]](t, res)
			if len(page.Items) != 1 || page.Items[0]["project_id"] != nil || page.NextCursor == nil {
				t.Fatal("company filter missing before pagination", route, page)
			}
			if route == "files" && !companyFiles[page.Items[0]["id"].(string)] {
				t.Fatal("project file leaked into selector")
			}
			rec, res = f.request("GET", path+"&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
			expect(t, rec, 200, res)
			next := data[core.Page[map[string]any]](t, res)
			if len(next.Items) != 1 || next.Items[0]["id"] == page.Items[0]["id"] || next.Items[0]["project_id"] != nil {
				t.Fatal(next)
			}
			rec, res = f.request("GET", "/v1/"+route+"?limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
			expect(t, rec, 400, res)
			rec, res = f.request("GET", path+"&project_id="+*old.File.ProjectID, f.aliceToken, "", nil, nil)
			expect(t, rec, 422, res)
			for _, value := range []string{"1", "", "false&company_only=true"} {
				rec, res = f.request("GET", "/v1/"+route+"?company_only="+value, f.aliceToken, "", nil, nil)
				expect(t, rec, 422, res)
			}
		}
		rec, res = f.request("GET", "/v1/contracts?trashed=1", f.aliceToken, "", nil, nil)
		expect(t, rec, 422, res)
	})
	t.Run("incomplete draft remains nullable and does not fabricate approval or signature", func(t *testing.T) {
		key := core.NewID()
		rec, res := f.request("POST", "/v1/contracts", f.aliceToken, key, minimal, nil)
		expect(t, rec, 201, res)
		draft = data[contracts.Contract](t, res)
		if draft.Amount != nil || draft.Currency != nil || draft.Direction != nil || draft.CounterpartyID != nil || draft.OwnerID != nil || draft.RequestID != nil || draft.ActiveRevisionID != nil || draft.ApprovalState != "not_submitted" || draft.Lifecycle != "draft" || !strings.HasPrefix(draft.ContractNo, "CP-C-") {
			t.Fatal(draft)
		}
		rec, res = f.request("POST", "/v1/contracts", f.aliceToken, key, minimal, nil)
		expect(t, rec, 201, res)
		if data[contracts.Contract](t, res).ID != draft.ID {
			t.Fatal("duplicate draft")
		}
		rec, res = f.request("GET", "/v1/contracts/"+draft.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, res)
	})
	t.Run("fixed party and file revisions register references and canonical content digest", func(t *testing.T) {
		rec, res := f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Synthetic contractual supplier", Kind: "organization", Relationships: []string{"supplier"}, Email: "original@example.invalid"}, ProjectID: old.File.ProjectID, Classification: "project_restricted", Reason: "fixed business evidence"}, nil)
		expect(t, rec, 201, res)
		counterpart = data[party.Party](t, res)
		in := minimal
		in.Title = "Fixed contract evidence"
		in.CounterpartyID = text(strings.ToUpper(counterpart.ID))
		in.CounterpartyRevisionID = text(strings.ToUpper(counterpart.CurrentRevisionID))
		in.DraftFileVersionID = text(strings.ToUpper(old.Version.ID))
		in.Currency = text("HKD")
		in.Amount = text("12.34")
		in.TaxAmount = text("0")
		in.Direction = text("payable")
		in.OwnerID = &f.alice
		in.Category = "software"
		rec, res = f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 201, res)
		complete = data[contracts.Contract](t, res)
		if complete.Counterparty == nil || complete.Counterparty.Name != counterpart.Name || complete.DraftFile == nil || complete.DraftFile.VersionID != old.Version.ID {
			t.Fatal("fixed reference presentation missing", complete)
		}
		if *complete.Amount != "12.340000" || *complete.CounterpartyID != counterpart.ID || *complete.DraftFileVersionID != old.Version.ID {
			t.Fatal(complete)
		}
		raw, _ := json.Marshal(map[string]any{"schema": complete.PayloadSchemaVersion, "contract_id": complete.ID, "project_id": complete.ProjectID, "parent_contract_id": complete.ParentContractID, "classification": complete.Classification, "content": complete.Content})
		canonical, _ := core.CanonicalJSON(raw)
		if core.Digest(canonical) != complete.ContentHash {
			t.Fatal("content digest did not bind normalized content and scope")
		}
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE org_id=$1 AND owner_type='contract_revision' AND owner_id=$2 AND version_id=$3", f.org, complete.CurrentRevisionID, old.Version.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
		before := counterpart.CurrentRevisionID
		rec, res = f.request("PATCH", "/v1/parties/"+counterpart.ID, f.aliceToken, core.NewID(), party.Patch{ExpectedVersion: counterpart.Version, BaseRevisionID: before, Content: party.Content{Name: "Renamed current supplier", Kind: counterpart.Kind, Relationships: counterpart.Relationships, Email: "changed@example.invalid"}, Reason: "later master data revision"}, nil)
		expect(t, rec, 200, res)
		if fixed := get(t, complete.ID); fixed.Counterparty == nil || fixed.Counterparty.Name != counterpart.Name {
			t.Fatal("contract displayed current party name", fixed)
		}
		if *get(t, complete.ID).CounterpartyRevisionID != before {
			t.Fatal("contract followed current party")
		}
		rec, res = f.request("GET", "/v1/parties/"+counterpart.ID+"/revisions/"+before, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		if data[party.Revision](t, res).Email != "original@example.invalid" {
			t.Fatal("fixed party evidence lost")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE contract_revisions SET title='changed' WHERE id=$1", complete.CurrentRevisionID); err == nil {
			t.Fatal("contract revision changed")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE contracts SET creator_id=$2 WHERE id=$1", complete.ID, f.bob); err == nil {
			t.Fatal("creator changed")
		}
	})
	t.Run("currency precision date range and reference pairing reject invalid draft facts", func(t *testing.T) {
		for _, change := range []func(*contracts.Create){
			func(in *contracts.Create) { in.Amount = text("1") }, func(in *contracts.Create) { in.Currency = text("HKD"); in.Amount = text("0.001") },
			func(in *contracts.Create) { in.Currency = text("JPY"); in.Amount = text("0.01") }, func(in *contracts.Create) { in.Currency = text("HKD"); in.Amount = text("1"); in.TaxAmount = text("2") },
			func(in *contracts.Create) { in.StartDate = text("2026-12-01"); in.EndDate = text("2026-11-01") }, func(in *contracts.Create) { in.CounterpartyID = &counterpart.ID },
		} {
			in := minimal
			change(&in)
			rec, res := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
			expect(t, rec, 422, res)
		}
		in := minimal
		in.DraftFileVersionID = text(core.NewID())
		rec, res := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 404, res)
	})
	t.Run("unavailable files and cross scope parties cannot become new business evidence", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, "UPDATE file_versions SET availability='missing' WHERE id=$1", old.Version.ID); err != nil {
			t.Fatal(err)
		}
		in := minimal
		in.DraftFileVersionID = &old.Version.ID
		rec, res := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 422, res)
		if get(t, complete.ID).DraftFile != nil {
			t.Fatal("unavailable evidence presented as usable")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE file_versions SET availability='available' WHERE id=$1", old.Version.ID); err != nil {
			t.Fatal(err)
		}
		other := f.createProject("Different contractual scope", nil)
		rec, res = f.request("POST", "/v1/parties", f.aliceToken, core.NewID(), party.Create{Content: party.Content{Name: "Other project party", Kind: "organization", Relationships: []string{"supplier"}}, ProjectID: &other.ID, Classification: "project_restricted", Reason: "scope isolation"}, nil)
		expect(t, rec, 201, res)
		p := data[party.Party](t, res)
		in = minimal
		in.CounterpartyID = &p.ID
		in.CounterpartyRevisionID = &p.CurrentRevisionID
		rec, res = f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 422, res)
	})
	t.Run("concurrent revisions keep prior attachment and current pointer has one successor", func(t *testing.T) {
		base := complete.CurrentRevisionID
		input := contracts.Patch{Content: complete.Content, ExpectedVersion: complete.Version, BaseRevisionID: base, Reason: "concurrent edit"}
		input.DraftFileVersionID = nil
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for _, title := range []string{"Concurrent draft A", "Concurrent draft B"} {
			wg.Add(1)
			go func(title string) {
				defer wg.Done()
				copy := input
				copy.Title = title
				rec, _ := f.request("PATCH", "/v1/contracts/"+complete.ID, f.aliceToken, core.NewID(), copy, nil)
				codes <- rec.Code
			}(title)
		}
		wg.Wait()
		close(codes)
		success := 0
		for code := range codes {
			if code == 200 {
				success++
			} else if code != 409 {
				t.Fatal(code)
			}
		}
		if success != 1 {
			t.Fatal(success)
		}
		complete = get(t, complete.ID)
		if complete.CurrentRevisionID == base || complete.DraftFileVersionID != nil {
			t.Fatal(complete)
		}
		rec, res := f.request("GET", "/v1/contracts/"+complete.ID+"/revisions/"+base, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		r := data[contracts.Revision](t, res)
		if r.DraftFileVersionID == nil || *r.DraftFileVersionID != old.Version.ID {
			t.Fatal("old attachment changed")
		}
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE owner_type='contract_revision' AND owner_id=$1", base).Scan(&count); err != nil || count != 1 {
			t.Fatal("old attachment protection lost", count, err)
		}
	})
	t.Run("parent draft trash and supplement restore serialize the dependency", func(t *testing.T) {
		in := minimal
		in.Title = "Supplement draft"
		in.ParentContractID = &draft.ID
		rec, res := f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 201, res)
		child := data[contracts.Contract](t, res)
		rec, res = f.request("POST", "/v1/contracts/"+draft.ID+"/trash", f.aliceToken, core.NewID(), contracts.TrashInput{ExpectedVersion: draft.Version, Action: "trash", Reason: "parent still referenced"}, nil)
		expect(t, rec, 409, res)
		if res.Error.Code != "CONTRACT_HAS_SUPPLEMENTS" || get(t, draft.ID).CanTrash {
			t.Fatal(res.Error)
		}
		rec, res = f.request("POST", "/v1/contracts/"+child.ID+"/trash", f.aliceToken, core.NewID(), contracts.TrashInput{ExpectedVersion: child.Version, Action: "trash", Reason: "recycle child draft"}, nil)
		expect(t, rec, 200, res)
		child = data[contracts.Contract](t, res)
		rec, res = f.request("POST", "/v1/contracts/"+draft.ID+"/trash", f.aliceToken, core.NewID(), contracts.TrashInput{ExpectedVersion: draft.Version, Action: "trash", Reason: "recycle parent draft"}, nil)
		expect(t, rec, 200, res)
		draft = data[contracts.Contract](t, res)
		rec, res = f.request("POST", "/v1/contracts/"+child.ID+"/trash", f.aliceToken, core.NewID(), contracts.TrashInput{ExpectedVersion: child.Version, Action: "restore", Reason: "parent still recycled"}, nil)
		expect(t, rec, 422, res)
		rec, res = f.request("POST", "/v1/contracts/"+draft.ID+"/trash", f.aliceToken, core.NewID(), contracts.TrashInput{ExpectedVersion: draft.Version, Action: "restore", Reason: "restore parent first"}, nil)
		expect(t, rec, 200, res)
		draft = data[contracts.Contract](t, res)
		rec, res = f.request("POST", "/v1/contracts/"+child.ID+"/trash", f.aliceToken, core.NewID(), contracts.TrashInput{ExpectedVersion: child.Version, Action: "restore", Reason: "parent available again"}, nil)
		expect(t, rec, 200, res)
	})
	t.Run("audit failure rolls back draft revisions file references and retry result", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, `CREATE FUNCTION fail_contract_audit() RETURNS trigger LANGUAGE plpgsql AS $x$
            BEGIN IF NEW.action LIKE 'contract.%' THEN RAISE EXCEPTION 'isolated contract audit failure'; END IF; RETURN NEW; END $x$;
            CREATE TRIGGER fail_contract_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_contract_audit()`); err != nil {
			t.Fatal(err)
		}
		var before, after int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM contract_revisions").Scan(&before); err != nil {
			t.Fatal(err)
		}
		submitted = contracts.Patch{Content: complete.Content, ExpectedVersion: complete.Version, BaseRevisionID: complete.CurrentRevisionID, Reason: "audit retry"}
		submitted.DraftFileVersionID = &old.Version.ID
		updateKey = core.NewID()
		rec, res := f.request("PATCH", "/v1/contracts/"+complete.ID, f.aliceToken, updateKey, submitted, nil)
		expect(t, rec, 500, res)
		rec, res = f.request("POST", "/v1/contracts", f.aliceToken, core.NewID(), minimal, nil)
		expect(t, rec, 500, res)
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM contract_revisions").Scan(&after); err != nil || before != after {
			t.Fatal(before, after, err)
		}
		if actual := get(t, complete.ID); actual.Version != complete.Version || actual.CurrentRevisionID != complete.CurrentRevisionID {
			t.Fatal("partial revision", actual)
		}
		if _, err := f.owner.Exec(ctx, "DROP TRIGGER fail_contract_audit ON audit_events; DROP FUNCTION fail_contract_audit()"); err != nil {
			t.Fatal(err)
		}
		rec, res = f.request("PATCH", "/v1/contracts/"+complete.ID, f.aliceToken, updateKey, submitted, nil)
		expect(t, rec, 200, res)
		complete = data[contracts.Contract](t, res)
	})
	t.Run("owner assignment never changes creator scope and cached references respect current access", func(t *testing.T) {
		bind(t, f.bob, "contract_manager", "own")
		in := minimal
		in.OwnerID = &f.alice
		rec, res := f.request("POST", "/v1/contracts", f.bobToken, core.NewID(), in, nil)
		expect(t, rec, 201, res)
		own := data[contracts.Contract](t, res)
		if own.CreatorID != f.bob || *own.OwnerID != f.alice {
			t.Fatal(own)
		}
		rec, res = f.request("GET", "/v1/contracts/"+complete.ID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, res)

		if _, err := f.owner.Exec(ctx, "UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='party_manager')", f.org, f.alice); err != nil {
			t.Fatal(err)
		}
		if current := get(t, complete.ID); current.Counterparty != nil || current.CounterpartyID == nil || current.DraftFile == nil {
			t.Fatal("linked party ACL did not redact only its name", current)
		}
		rec, res = f.request("PATCH", "/v1/contracts/"+complete.ID, f.aliceToken, updateKey, submitted, nil)
		expect(t, rec, 200, res)
		if cached := data[contracts.Contract](t, res); cached.Counterparty != nil {
			t.Fatal("cached response leaked revoked party name")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='contract_manager')", f.org, f.alice); err != nil {
			t.Fatal(err)
		}
		rec, res = f.request("PATCH", "/v1/contracts/"+complete.ID, f.aliceToken, updateKey, submitted, nil)
		expect(t, rec, 404, res)
	})
}
