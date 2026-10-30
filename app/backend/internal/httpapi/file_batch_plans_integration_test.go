package httpapi

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
)

// Real PG namespace/history metadata with explicitly synthetic content references.
// No uploaded bytes, cloud access or production durability is asserted here.
func fileBatchFixture(t *testing.T) (*fixture, string, storage.Folder, func(string) storage.File) {
	t.Helper()
	f := setup(t)
	ctx := context.Background()
	// Explicit fixture policy: ordinary project authority, with MFA required;
	// no administrator privileges or production role defaults are added.
	if _, e := f.owner.Exec(ctx, "UPDATE roles SET requires_mfa=true WHERE org_id=$1 AND key='project_manager'", f.org); e != nil {
		t.Fatal(e)
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	p := f.createProject("批量文件整理演示", []string{f.bob})
	var root string
	if e := f.owner.QueryRow(ctx, "SELECT id FROM folders WHERE org_id=$1 AND project_id=$2 AND parent_id IS NULL", f.org, p.ID).Scan(&root); e != nil {
		t.Fatal(e)
	}
	if _, e := f.owner.Exec(ctx, "INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence) VALUES($1,1,'oss',$2,'isolated file batch metadata only')", f.org, f.admin); e != nil {
		t.Fatal(e)
	}
	r, v := f.request("POST", "/v1/folders", f.aliceToken, core.NewID(), map[string]any{"parent_id": root, "name": "已核对归档", "classification": "project_restricted"}, nil)
	expect(t, r, 201, v)
	target := data[storage.Folder](t, v)

	create := func(name string) storage.File {
		t.Helper()
		id := core.NewID()
		key, e := core.StorageNameKey(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.owner.Exec(ctx, "INSERT INTO files(id,org_id,folder_id,name,name_key,classification,state,creator_id) VALUES($1,$2,$3,$4,$5,'project_restricted','active',$6)", id, f.org, root, name, key, f.alice); e != nil {
			t.Fatal(e)
		}
		head := ""
		for seq := 1; seq <= 2; seq++ {
			head = core.NewID()
			if _, e = f.owner.Exec(ctx, `INSERT INTO file_versions(id,org_id,file_id,seq,object_key,provider_source_id,storage_epoch,size_bytes,sha256,media_type,source,author_id,executor_id,scan_state,availability,original_name)
   VALUES($1,$2,$3,$4,$5,'batch-metadata-only',1,16,$6,'text/plain','metadata-test',$7,$7,'clean','available',$8)`, head, f.org, id, seq, "synthetic/"+head, strings.Repeat("a", 64), f.alice, name); e != nil {
				t.Fatal(e)
			}
		}
		if _, e = f.owner.Exec(ctx, "UPDATE files SET current_version_id=$3 WHERE org_id=$1 AND id=$2", f.org, id, head); e != nil {
			t.Fatal(e)
		}
		r, v := f.request("GET", "/v1/files/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[storage.File](t, v)
	}
	return f, root, target, create
}
func batchInputFor(action string, target *storage.Folder, files ...storage.File) storage.FileBatchCreate {
	in := storage.FileBatchCreate{Action: action, Reason: "人工核对实际文件清单后整理"}
	for _, f := range files {
		in.Files = append(in.Files, storage.FileBatchSelection{ID: f.ID, ExpectedVersion: f.Version, CurrentVersionID: f.CurrentVersionID})
	}
	if target != nil {
		in.Target = &storage.FileBatchTarget{ID: target.ID, ExpectedVersion: target.Version}
	}
	return in
}
func batchDecisionFor(p storage.FileBatchPlan) storage.FileBatchDecision {
	return storage.FileBatchDecision{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "已核对全部文件与准确目标并确认本次操作", Acknowledged: true}
}

func TestFileBatchPlans(t *testing.T) {
	f, root, target, create := fileBatchFixture(t)
	ctx := context.Background()
	post := func(t *testing.T, path, key string, body any, status int) response {
		t.Helper()
		r, v := f.with(t).request("POST", path, f.aliceToken, key, body, nil)
		expect(t, r, status, v)
		return v
	}
	propose := func(t *testing.T, in storage.FileBatchCreate) storage.FileBatchPlan {
		t.Helper()
		return data[storage.FileBatchPlan](t, post(t, "/v1/file-batch-plans", core.NewID(), in, 201))
	}
	read := func(t *testing.T, id string) storage.FileBatchPlan {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/file-batch-plans/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[storage.FileBatchPlan](t, v)
	}
	get := func(t *testing.T, id string) storage.File {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/files/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[storage.File](t, v)
	}
	count := func(t *testing.T, q string, args ...any) int {
		t.Helper()
		var n int
		if e := f.owner.QueryRow(ctx, q, args...).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	exec := func(t *testing.T, q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	decide := func(t *testing.T, p storage.FileBatchPlan, action string, status int) storage.FileBatchPlan {
		t.Helper()
		v := post(t, "/v1/file-batch-plans/"+p.ID+"/"+action, core.NewID(), batchDecisionFor(p), status)
		if status == 200 {
			return data[storage.FileBatchPlan](t, v)
		}
		return storage.FileBatchPlan{}
	}
	t.Run("three ordinary atomic journeys retain both formal heads and reopen from every file", func(t *testing.T) {
		a, b := create("合同摘要.txt"), create("采购资料.txt")
		input := batchInputFor("move", &target, b, a)
		key := core.NewID()
		p := data[storage.FileBatchPlan](t, post(t, "/v1/file-batch-plans", key, input, 201))
		if !p.CanExecute || !p.CurrentBasisMatches || p.Version != 1 || len(p.Snapshot.Items) != 2 || len(p.Results) != 0 || get(t, a.ID).FolderID != root {
			t.Fatal(p)
		}
		if p.Input.Files[0].ID > p.Input.Files[1].ID || p.PlanHash == "" || !read(t, p.ID).CanExecute {
			t.Fatal("not fixed/readable")
		}
		for _, file := range []storage.File{a, b} {
			r, v := f.with(t).request("GET", "/v1/files/"+file.ID+"/batch-plans?limit=1", f.aliceToken, "", nil, nil)
			expect(t, r, 200, v)
			page := data[core.Page[storage.FileBatchPlan]](t, v)
			if len(page.Items) != 1 || page.Items[0].ID != p.ID {
				t.Fatal(page)
			}
		}
		executeKey := core.NewID()
		done := data[storage.FileBatchPlan](t, post(t, "/v1/file-batch-plans/"+p.ID+"/execute", executeKey, batchDecisionFor(p), 200))
		if done.State != "executed" || done.Version != 2 || len(done.Results) != 2 || done.CompletedAt == nil || done.CanExecute {
			t.Fatal(done)
		}
		for _, before := range []storage.File{a, b} {
			after := get(t, before.ID)
			if after.FolderID != target.ID || after.Version != before.Version+1 || after.CurrentVersionID == nil || *after.CurrentVersionID != *before.CurrentVersionID {
				t.Fatal(after)
			}
			if count(t, "SELECT count(*) FROM file_versions WHERE file_id=$1", before.ID) != 2 {
				t.Fatal("history changed")
			}
		}
		replay := post(t, "/v1/file-batch-plans/"+p.ID+"/execute", executeKey, batchDecisionFor(p), 200)
		if data[storage.FileBatchPlan](t, replay).State != "executed" {
			t.Fatal("execute replay not current")
		}
		replay = post(t, "/v1/file-batch-plans", key, input, 201)
		if data[storage.FileBatchPlan](t, replay).State != "executed" {
			t.Fatal("create replay not current")
		}
		for _, action := range []string{"trash", "restore"} {
			p = propose(t, batchInputFor(action, nil, get(t, a.ID), get(t, b.ID)))
			if !p.CanExecute {
				t.Fatal(p)
			}
			done = decide(t, p, "execute", 200)
			for _, item := range done.Results {
				want := "active"
				if action == "trash" {
					want = "trashed"
				}
				if item.State != want || get(t, item.ID).State != want {
					t.Fatal(item)
				}
			}
		}
		if count(t, "SELECT count(*) FROM audit_events WHERE org_id=$1 AND resource_type='file_batch_plan' AND result='succeeded'", f.org) != 6 || count(t, "SELECT count(*) FROM outbox_events WHERE org_id=$1 AND aggregate_type='file_batch_plan'", f.org) != 6 {
			t.Fatal("missing coordinator evidence")
		}
	})
	t.Run("cancel is durable and final while exact decision replay stays idempotent", func(t *testing.T) {
		a := create("cancel.txt")
		p := propose(t, batchInputFor("trash", nil, a))
		key := core.NewID()
		route := "/v1/file-batch-plans/" + p.ID + "/cancel"
		v := post(t, route, key, batchDecisionFor(p), 200)
		done := data[storage.FileBatchPlan](t, v)
		if done.State != "cancelled" || done.Version != 2 || len(done.Results) != 0 || get(t, a.ID).Version != a.Version {
			t.Fatal(done)
		}
		v = post(t, route, key, batchDecisionFor(p), 200)
		if data[storage.FileBatchPlan](t, v).ID != p.ID {
			t.Fatal("cancel replay missing")
		}
		decide(t, p, "execute", 409)
		if _, e := f.owner.Exec(ctx, "UPDATE file_batch_plans SET decision_reason='rewritten' WHERE id=$1", p.ID); e == nil {
			t.Fatal("final receipt mutable")
		}
		if _, e := f.owner.Exec(ctx, "DELETE FROM file_batch_plans WHERE id=$1", p.ID); e == nil {
			t.Fatal("evidence deletable")
		}
	})
	t.Run("name collisions cover folders and held files yield visible blocked proposals", func(t *testing.T) {
		a := create("collision.txt")
		r, v := f.with(t).request("POST", "/v1/folders", f.aliceToken, core.NewID(), map[string]any{"parent_id": target.ID, "name": a.Name, "classification": "project_restricted"}, nil)
		expect(t, r, 201, v)
		p := propose(t, batchInputFor("move", &target, a))
		if p.CanExecute || !slices.Contains(p.Snapshot.Items[0].BlockingReasons, "name_conflict") {
			t.Fatal(p)
		}
		decide(t, p, "execute", 409)
		b := create("held.txt")
		exec(t, "UPDATE files SET legal_hold=true WHERE id=$1", b.ID)
		p = propose(t, batchInputFor("trash", nil, b))
		if p.CanExecute || !slices.Contains(p.Snapshot.Items[0].BlockingReasons, "retention_hold") {
			t.Fatal(p)
		}
		decide(t, p, "execute", 409)
		exec(t, "UPDATE files SET legal_hold=false WHERE id=$1", b.ID)
		if read(t, p.ID).CanExecute {
			t.Fatal("removing hold revived stale snapshot")
		}
	})
	t.Run("source head target revision and ancestor changes invalidate the fixed basis", func(t *testing.T) {
		for _, mode := range []string{"version", "head", "target", "path", "hold"} {
			t.Run(mode, func(t *testing.T) {
				a := create(mode + ".txt")
				r, v := f.with(t).request("GET", "/v1/folders/"+target.ID, f.aliceToken, "", nil, nil)
				expect(t, r, 200, v)
				dest := data[storage.Folder](t, v)
				p := propose(t, batchInputFor("move", &dest, a))
				switch mode {
				case "version":
					exec(t, "UPDATE files SET version=version+1 WHERE id=$1", a.ID)
				case "head":
					exec(t, "UPDATE files SET current_version_id=(SELECT id FROM file_versions WHERE file_id=$1 AND seq=1) WHERE id=$1", a.ID)
				case "target":
					exec(t, "UPDATE folders SET version=version+1 WHERE id=$1", target.ID)
				case "path":
					exec(t, "UPDATE folders SET version=version+1 WHERE id=$1", root)
				case "hold":
					exec(t, "UPDATE files SET legal_hold=true WHERE id=$1", a.ID)
				}
				if read(t, p.ID).CanExecute {
					t.Fatal("changed source still executable")
				}
				decide(t, p, "execute", 409)
				if get(t, a.ID).FolderID != root {
					t.Fatal("partial move")
				}
			})
		}
	})
	t.Run("a real second-child failure rolls back every child and its successful evidence", func(t *testing.T) {
		a, b := create("rollback-a.txt"), create("rollback-b.txt")
		r, v := f.with(t).request("GET", "/v1/folders/"+target.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		dest := data[storage.Folder](t, v)
		p := propose(t, batchInputFor("move", &dest, a, b))
		ids := []string{a.ID, b.ID}
		sort.Strings(ids)
		exec(t, fmt.Sprintf(`CREATE FUNCTION batch_test_late_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id='%s'::uuid AND NEW.folder_id='%s'::uuid THEN RAISE EXCEPTION 'synthetic second child failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER batch_test_late_failure BEFORE UPDATE ON files FOR EACH ROW EXECUTE FUNCTION batch_test_late_failure()`, ids[1], target.ID))
		before := count(t, "SELECT count(*) FROM audit_events WHERE org_id=$1 AND result='succeeded'", f.org)
		beforeOut := count(t, "SELECT count(*) FROM outbox_events WHERE org_id=$1", f.org)
		beforeKeys := count(t, "SELECT count(*) FROM idempotency_records WHERE org_id=$1", f.org)
		r, _ = f.with(t).request("POST", "/v1/file-batch-plans/"+p.ID+"/execute", f.aliceToken, core.NewID(), batchDecisionFor(p), nil)
		if r.Code < 400 {
			t.Fatal("injected failure accepted")
		}
		exec(t, "DROP TRIGGER batch_test_late_failure ON files; DROP FUNCTION batch_test_late_failure()")
		for _, before := range []storage.File{a, b} {
			after := get(t, before.ID)
			if after.Version != before.Version || after.FolderID != root {
				t.Fatal("partial child survived", after)
			}
		}
		if read(t, p.ID).State != "pending" || count(t, "SELECT count(*) FROM audit_events WHERE org_id=$1 AND result='succeeded'", f.org) != before || count(t, "SELECT count(*) FROM outbox_events WHERE org_id=$1", f.org) != beforeOut || count(t, "SELECT count(*) FROM idempotency_records WHERE org_id=$1", f.org) != beforeKeys {
			t.Fatal("coordinator rollback leaked successful facts")
		}
		decide(t, p, "execute", 200)
	})
	t.Run("concurrent plans on the same files have exactly one winner", func(t *testing.T) {
		a, b := create("race-a.txt"), create("race-b.txt")
		one := propose(t, batchInputFor("trash", nil, a, b))
		two := propose(t, batchInputFor("trash", nil, a, b))
		codes := make(chan int, 2)
		var wait sync.WaitGroup
		for _, p := range []storage.FileBatchPlan{one, two} {
			wait.Add(1)
			go func(p storage.FileBatchPlan) {
				defer wait.Done()
				r, _ := f.request("POST", "/v1/file-batch-plans/"+p.ID+"/execute", f.aliceToken, core.NewID(), batchDecisionFor(p), nil)
				codes <- r.Code
			}(p)
		}
		wait.Wait()
		close(codes)
		got := []int{}
		for code := range codes {
			got = append(got, code)
		}
		sort.Ints(got)
		if fmt.Sprint(got) != "[200 409]" {
			t.Fatal(got)
		}
	})
	t.Run("read is owner-bound and rejects agent or forged session identities", func(t *testing.T) {
		f.bobToken = memberMFAToken(f, "bob", "android")
		a := create("owner-only.txt")
		p := propose(t, batchInputFor("trash", nil, a))
		r, v := f.with(t).request("GET", "/v1/file-batch-plans/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.with(t).request("GET", "/v1/files/"+a.ID+"/batch-plans", f.bobToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[storage.FileBatchPlan]](t, v).Items) != 0 {
			t.Fatal("other owner plan leaked")
		}
		actor, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		service := &storage.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}}
		for _, mode := range []string{"agent", "channel", "session"} {
			forged := actor
			if mode == "agent" {
				forged.RunID = core.NewID()
			} else if mode == "channel" {
				forged.Channel = "web"
			} else {
				forged.SessionID = core.NewID()
			}
			if _, e := service.FileBatchPlan(ctx, forged, p.ID); e == nil {
				t.Fatal("forged reader", mode)
			}
		}
	})
	t.Run("exact query and input shapes reject accidental broad or forged requests", func(t *testing.T) {
		a := create("strict.txt")
		in := batchInputFor("trash", nil, a)
		for _, bad := range []storage.FileBatchCreate{{Action: "trash", Reason: "empty"}, {Action: "move|trash", Files: in.Files, Reason: "bad action"}, {Action: "trash", Files: append(in.Files, in.Files...), Reason: "duplicate"}, {Action: "trash", Files: in.Files, Target: &storage.FileBatchTarget{ID: target.ID, ExpectedVersion: target.Version}, Reason: "hidden target"}} {
			post(t, "/v1/file-batch-plans", core.NewID(), bad, 422)
		}
		over := in
		over.Files = []storage.FileBatchSelection{}
		for i := 0; i < 51; i++ {
			over.Files = append(over.Files, storage.FileBatchSelection{ID: core.NewID(), ExpectedVersion: 1})
		}
		post(t, "/v1/file-batch-plans", core.NewID(), over, 422)
		p := propose(t, in)
		for _, route := range []string{"/v1/file-batch-plans/" + p.ID + "?unused=1", "/v1/files/" + a.ID + "/batch-plans?limit=1&limit=2", "/v1/files/" + a.ID + "/batch-plans?owner_id=" + f.bob} {
			r, v := f.with(t).request("GET", route, f.aliceToken, "", nil, nil)
			expect(t, r, 422, v)
		}
		decision := batchDecisionFor(p)
		decision.Acknowledged = false
		post(t, "/v1/file-batch-plans/"+p.ID+"/execute", core.NewID(), decision, 422)
		decision = batchDecisionFor(p)
		decision.PlanHash = strings.Repeat("0", 64)
		post(t, "/v1/file-batch-plans/"+p.ID+"/execute", core.NewID(), decision, 409)
		if _, e := f.owner.Exec(ctx, "UPDATE file_batch_plans SET input=input||' ' WHERE id=$1", p.ID); e == nil {
			t.Fatal("fixed source bytes mutable")
		}
	})
	t.Run("MFA expiry and changed authority are rechecked before execute and replay", func(t *testing.T) {
		a := create("revocation.txt")
		p := propose(t, batchInputFor("trash", nil, a))
		actor, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		r, _ := f.with(t).request("POST", "/v1/file-batch-plans/"+p.ID+"/execute", f.aliceToken, core.NewID(), batchDecisionFor(p), nil)
		if r.Code < 400 {
			t.Fatal("expired MFA accepted")
		}
		if get(t, a.ID).State != "active" {
			t.Fatal("expired MFA changed file")
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE id=$1", actor.SessionID)
		executeKey := core.NewID()
		r, v := f.with(t).request("POST", "/v1/file-batch-plans/"+p.ID+"/execute", f.aliceToken, executeKey, batchDecisionFor(p), nil)
		expect(t, r, 200, v)
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE id=$1", actor.SessionID)
		r, _ = f.with(t).request("POST", "/v1/file-batch-plans/"+p.ID+"/execute", f.aliceToken, executeKey, batchDecisionFor(p), nil)
		if r.Code < 400 {
			t.Fatal("cached execution skipped current MFA")
		}
		exec(t, "UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE id=$1", actor.SessionID)
		expiredSource := create("expired.txt")
		live := propose(t, batchInputFor("trash", nil, expiredSource))
		expiredID := core.NewID()
		exec(t, `INSERT INTO file_batch_plans(id,org_id,creator_id,origin_session_id,origin_channel,actor_auth_version,organization_version,input,snapshot,plan_hash,created_at,expires_at)
         SELECT $2,org_id,creator_id,origin_session_id,origin_channel,actor_auth_version,organization_version,input,snapshot,plan_hash,clock_timestamp()-interval '30 minutes',clock_timestamp()-interval '1 second' FROM file_batch_plans WHERE id=$1`, live.ID, expiredID)
		expired := read(t, expiredID)
		if expired.CanExecute {
			t.Fatal("expired plan executable")
		}
		decide(t, expired, "execute", 409)
		decide(t, expired, "cancel", 200)
		b := create("authority-change.txt")
		p = propose(t, batchInputFor("trash", nil, b))
		exec(t, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)

		if read(t, p.ID).CanExecute {
			t.Fatal("changed org revived plan")
		}
		decide(t, p, "execute", 409)
		decide(t, p, "cancel", 200)
	})
}

func TestFileBatchBrowserFixture(t *testing.T) {
	if os.Getenv("CP_BROWSER_FILE_BATCH") != "1" {
		t.Skip("explicit owned browser opt-in only")
	}
	f, root, target, create := fileBatchFixture(t)
	a, b := create("项目合同摘要.txt"), create("采购交付清单.txt")
	ids := []string{a.ID, b.ID}
	holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "human-file-batch", "file_batch_fixture": true, "project_id": a.ProjectID, "folder_id": root, "target_folder_id": target.ID, "target_folder_name": target.Name, "batch_file_ids": ids, "created_at": time.Now().UTC()})
}
