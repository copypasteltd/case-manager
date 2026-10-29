package httpapi

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
)

func TestFileScopeTransfers(t *testing.T) {
	u, old, current, _, _ := restorationFixture(t, 15)
	f, ctx := u.f, context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	// Explicit synthetic admin + existing business authority; neither role is
	// assigned to real employees or silently added by a schema upgrade.
	exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
 SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='system_admin'`, core.NewID(), f.org, f.alice)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	other := f.createProject("跨范围目标项目", []string{f.member})
	var target storage.Folder
	var targetID string
	if e := f.owner.QueryRow(ctx, "SELECT id FROM folders WHERE org_id=$1 AND project_id=$2 AND parent_id IS NULL", f.org, other.ID).Scan(&targetID); e != nil {
		t.Fatal(e)
	}
	getFolder := func(id string) storage.Folder {
		t.Helper()
		r, v := f.request("GET", "/v1/folders/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[storage.Folder](t, v)
	}
	getFile := func(id string) storage.File {
		t.Helper()
		r, v := f.request("GET", "/v1/files/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[storage.File](t, v)
	}
	target = getFolder(targetID)
	input := func(kind, id string, version int64) storage.ScopeTransferCreate {
		return storage.ScopeTransferCreate{Kind: kind, ResourceID: id, ExpectedVersion: version, TargetFolderID: target.ID, ExpectedTargetVersion: target.Version, Reason: "明确核对完整权限变化后跨项目整理"}
	}
	create := func(t *testing.T, in storage.ScopeTransferCreate) storage.ScopeTransfer {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/file-scope-transfers", f.aliceToken, core.NewID(), in, nil)
		expect(t, r, 201, v)
		return data[storage.ScopeTransfer](t, v)
	}
	read := func(t *testing.T, id string) storage.ScopeTransfer {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/file-scope-transfers/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		return data[storage.ScopeTransfer](t, v)
	}
	decision := func(p storage.ScopeTransfer) storage.ScopeTransferDecision {
		return storage.ScopeTransferDecision{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "人工确认本次准确清单及全部权限影响"}
	}
	base := getFile(current.File.ID)
	var preview storage.ScopeTransfer
	t.Run("fixed full-history proposal shows actual lost and gained policy readers without moving", func(t *testing.T) {
		preview = create(t, input("file", base.ID, base.Version))
		if preview.State != "pending" || !preview.CanExecute || !preview.CurrentBasisMatches || len(preview.Snapshot.Items) != 1 || preview.Snapshot.Items[0].ContentVersions != 2 {
			t.Fatal(preview)
		}
		lost, gained := false, false
		for _, v := range preview.Snapshot.Items[0].VisibilityChanges {
			lost = lost || (v.UserID == f.bob && !v.AI && v.Channel == "web" && v.Before && !v.After)
			gained = gained || (v.UserID == f.member && !v.AI && v.Channel == "web" && !v.Before && v.After)
		}
		if !lost || !gained {
			t.Fatal("incomplete policy impact", preview.Snapshot.Items[0])
		}
		if getFile(base.ID).FolderID != base.FolderID {
			t.Fatal("proposal mutated source")
		}
		if !read(t, preview.ID).CanExecute {
			t.Fatal("fresh plan not executable")
		}
	})
	t.Run("human read rejects agent execution and forged session channel pairs", func(t *testing.T) {
		actor, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		service := &storage.Service{DB: f.api.DB}
		if _, e = service.ScopeTransfer(ctx, actor, preview.ID); e != nil {
			t.Fatal("valid human control", e)
		}
		for _, channel := range []string{"telegram", "web", "wechat", "douyin"} {
			a := actor
			a.Channel = channel
			_, e = service.ScopeTransfer(ctx, a, preview.ID)
			if e == nil {
				t.Fatal("forged channel accepted", channel)
			}
			status := core.PublicError(e).Status
			if status != 403 && status != 412 {
				t.Fatal("forged channel not rejected as identity mismatch", channel, e)
			}
		}
		actor.RunID = core.NewID()
		_, e = service.ScopeTransfer(ctx, actor, preview.ID)
		if e == nil || core.PublicError(e).Status != 403 {
			t.Fatal("agent identity not rejected", e)
		}
	})
	t.Run("strict inputs owner-only reads and ordinary move guard remain enforced", func(t *testing.T) {
		for _, token := range []string{f.bobToken, f.memberToken} {
			r, v := f.request("GET", "/v1/file-scope-transfers/"+preview.ID, token, "", nil, nil)
			expect(t, r, 404, v)
		}
		for _, query := range []string{"?actor_id=" + f.alice, "?%zz"} {
			r, v := f.request("GET", "/v1/file-scope-transfers/"+preview.ID+query, f.aliceToken, "", nil, nil)
			expect(t, r, 422, v)
		}
		bad := input("file", base.ID, base.Version)
		bad.ExpectedTargetVersion++
		r, v := f.request("POST", "/v1/file-scope-transfers", f.aliceToken, core.NewID(), bad, nil)
		expect(t, r, 409, v)
		r, v = f.request("POST", "/v1/files/"+base.ID+"/move", f.aliceToken, core.NewID(), map[string]any{"expected_version": base.Version, "target_folder_id": target.ID, "reason": "ordinary endpoint is not a permission-change authorization"}, nil)
		expect(t, r, 409, v)
		if v.Error.Code != "PERMISSION_CHANGE_PLAN_REQUIRED" {
			t.Fatal(v.Error)
		}
		bad = input("folder", u.root, getFolder(u.root).Version)
		r, v = f.request("POST", "/v1/file-scope-transfers", f.aliceToken, core.NewID(), bad, nil)
		expect(t, r, 409, v)
		if v.Error.Code != "ROOT_SCOPE_FIXED" {
			t.Fatal(v.Error)
		}
		bad = input("file", base.ID, base.Version)
		bad.TargetFolderID = u.root
		bad.ExpectedTargetVersion = getFolder(u.root).Version
		r, v = f.request("POST", "/v1/file-scope-transfers", f.aliceToken, core.NewID(), bad, nil)
		expect(t, r, 422, v)
	})
	t.Run("recent MFA hash and current basis are checked before business writes", func(t *testing.T) {
		wrong := decision(preview)
		wrong.PlanHash = strings.Repeat("f", 64)
		r, v := f.request("POST", "/v1/file-scope-transfers/"+preview.ID+"/execute", f.aliceToken, core.NewID(), wrong, nil)
		expect(t, r, 409, v)
		var original time.Time
		if e := f.owner.QueryRow(ctx, "SELECT mfa_verified_at FROM sessions WHERE user_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1", f.alice).Scan(&original); e != nil {
			t.Fatal(e)
		}
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE user_id=$1 AND revoked_at IS NULL", f.alice)
		r, v = f.request("POST", "/v1/file-scope-transfers/"+preview.ID+"/execute", f.aliceToken, core.NewID(), decision(preview), nil)
		expect(t, r, 403, v)
		exec("UPDATE sessions SET mfa_verified_at=$2 WHERE user_id=$1 AND revoked_at IS NULL", f.alice, original)
		if getFile(base.ID).Version != base.Version {
			t.Fatal("rejected approval mutated source")
		}
	})
	t.Run("same owner plan pagination binds actor filter and complete fixed inventory", func(t *testing.T) {
		_ = create(t, input("file", base.ID, base.Version))
		r, v := f.request("GET", "/v1/files/"+base.ID+"/scope-transfers?limit=1", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		p := data[core.Page[storage.ScopeTransfer]](t, v)
		if len(p.Items) != 1 || p.NextCursor == nil {
			t.Fatal(p)
		}
		r, v = f.request("GET", "/v1/files/"+base.ID+"/scope-transfers?limit=1&cursor="+*p.NextCursor, f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.request("GET", "/v1/files/"+base.ID+"/scope-transfers?limit=1&limit=2", f.aliceToken, "", nil, nil)
		expect(t, r, 422, v)
	})
	t.Run("changed target invalidates pending basis and cancel preserves immutable evidence", func(t *testing.T) {
		p := create(t, input("file", base.ID, base.Version))
		r, v := f.request("POST", "/v1/folders/"+target.ID+"/move", f.aliceToken, core.NewID(), map[string]any{"expected_version": target.Version, "new_name": "目标项目资料", "reason": "ordinary target change"}, nil)
		expect(t, r, 200, v)
		target = getFolder(target.ID)
		if read(t, p.ID).CanExecute {
			t.Fatal("stale target still executable")
		}
		r, v = f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/execute", f.aliceToken, core.NewID(), decision(p), nil)
		expect(t, r, 409, v)
		key := core.NewID()
		body := decision(p)
		r, v = f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/cancel", f.aliceToken, key, body, nil)
		expect(t, r, 200, v)
		cancelled := data[storage.ScopeTransfer](t, v)
		if cancelled.State != "cancelled" || cancelled.PlanHash != p.PlanHash || cancelled.Version != 2 || cancelled.CompletedAt == nil {
			t.Fatal(cancelled)
		}
		r, v = f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/cancel", f.aliceToken, key, body, nil)
		expect(t, r, 200, v)
		if r.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("missing cancel replay")
		}
		if _, e := f.api.DB.Exec(ctx, "UPDATE file_scope_transfers SET plan_hash=$3 WHERE org_id=$1 AND id=$2", f.org, p.ID, strings.Repeat("e", 64)); e == nil {
			t.Fatal("mutable plan evidence")
		}
	})
	t.Run("legal hold reference and in-flight upload appear as explicit blockers", func(t *testing.T) {
		hold, ref := core.NewID(), core.NewID()
		exec("INSERT INTO retention_holds(id,org_id,file_id,kind,reason,owner_id) VALUES($1,$2,$3,'legal','synthetic transfer blocker',$4)", hold, f.org, base.ID, f.alice)
		exec("INSERT INTO file_version_references(id,org_id,file_id,version_id,owner_type,owner_id) VALUES($1,$2,$3,$4,'knowledge_revision',$5)", ref, f.org, base.ID, old.Version.ID, core.NewID())
		p := create(t, input("file", base.ID, base.Version))
		codes := strings.Join(p.Snapshot.Items[0].BlockingReasons, ",")
		if p.CanExecute || !strings.Contains(codes, "RETENTION_HOLD") || !strings.Contains(codes, "BUSINESS_REFERENCE") {
			t.Fatal(p)
		}
		r, v := f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/execute", f.aliceToken, core.NewID(), decision(p), nil)
		expect(t, r, 409, v)
		exec("INSERT INTO retention_hold_releases(id,org_id,hold_id,released_by,reason) VALUES($1,$2,$3,$4,'synthetic hold completed')", core.NewID(), f.org, hold, f.alice)
		exec("INSERT INTO file_reference_releases(id,org_id,reference_id,released_by,reason) VALUES($1,$2,$3,$4,'synthetic reference completed')", core.NewID(), f.org, ref, f.alice)
		if read(t, p.ID).CurrentBasisMatches {
			t.Fatal("old blocked plan silently became executable")
		}
		r, v = f.request("POST", "/v1/uploads", f.aliceToken, core.NewID(), map[string]any{"folder_id": u.root, "name": "pending.bin", "size_bytes": "1", "media_type": "application/octet-stream", "source_fingerprint": "transfer-pending", "file_id": base.ID, "base_version_id": current.Version.ID}, nil)
		expect(t, r, 201, v)
		upload := data[storage.Upload](t, v)
		p = create(t, input("file", base.ID, base.Version))
		if p.CanExecute || !strings.Contains(strings.Join(p.Snapshot.Items[0].BlockingReasons, ","), "UPLOAD_ACTIVE") {
			t.Fatal(p)
		}
		r, v = f.request("POST", "/v1/uploads/"+upload.ID+"/abort", f.aliceToken, core.NewID(), map[string]any{"expected_version": upload.Version, "action": "abort", "reason": "complete synthetic upload before namespace transfer"}, nil)
		expect(t, r, 200, v)
		// Process the queued initiation through the real upload service;
		// later content helpers must not claim this aborted upload's old event.
		// No provider upload existed, so abort correctly emitted no provider job.
		if e := u.service.RunUploadJob(ctx, u.job(), u.cloud); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("authority and original login changes invalidate execution without hiding current review", func(t *testing.T) {
		p := create(t, input("file", base.ID, base.Version))
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		if read(t, p.ID).CanExecute {
			t.Fatal("old authorization plan remained executable")
		}
		r, v := f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/execute", f.aliceToken, core.NewID(), decision(p), nil)
		expect(t, r, 412, v)
		if getFile(base.ID).FolderID != base.FolderID {
			t.Fatal("stale authority mutated file")
		}
	})
	t.Run("database refuses an invented execution receipt without its completed inventory", func(t *testing.T) {
		p := create(t, input("file", base.ID, base.Version))
		if _, e := f.api.DB.Exec(ctx, `UPDATE file_scope_transfers SET state='executed',version=2,completed_at=clock_timestamp(),decision_reason='forged',decision_session_id=origin_session_id,decision_channel=origin_channel WHERE org_id=$1 AND id=$2`, f.org, p.ID); e == nil {
			t.Fatal("invented final receipt accepted")
		}
		if getFile(base.ID).Version != base.Version {
			t.Fatal("forged receipt mutated original")
		}
	})
	t.Run("planned permission time boundaries cap expiry and forbid late execution", func(t *testing.T) {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling,valid_until)
 SELECT $1,$2,$3,id,'org','internal',clock_timestamp()+interval '3 seconds' FROM roles WHERE org_id=$2 AND key='system_admin'`, core.NewID(), f.org, f.member)
		p := create(t, input("file", base.ID, base.Version))
		if p.ExpiresAt.Sub(p.CreatedAt) > 3*time.Second {
			t.Fatal("ignored permission expiry boundary")
		}
		if wait := time.Until(p.ExpiresAt.Add(50 * time.Millisecond)); wait > 0 {
			time.Sleep(wait)
		}
		if read(t, p.ID).CanExecute {
			t.Fatal("expired plan still executable")
		}
		r, v := f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/execute", f.aliceToken, core.NewID(), decision(p), nil)
		expect(t, r, 409, v)
		if v.Error.Code != "TRANSFER_EXPIRED" {
			t.Fatal(v.Error)
		}
	})
	t.Run("no classification downgrade and cross-type normalized name collisions", func(t *testing.T) {
		r, v := f.request("POST", "/v1/folders", f.aliceToken, core.NewID(), map[string]any{"name": "公司整理目标", "classification": "internal"}, nil)
		expect(t, r, 201, v)
		company := data[storage.Folder](t, v)
		in := input("file", base.ID, base.Version)
		in.TargetFolderID, in.ExpectedTargetVersion = company.ID, company.Version
		p := create(t, in)
		if p.CanExecute || !strings.Contains(strings.Join(p.Snapshot.Items[0].BlockingReasons, ","), "CLASSIFICATION_DOWNGRADE") {
			t.Fatal(p)
		}
		r, v = f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/execute", f.aliceToken, core.NewID(), decision(p), nil)
		expect(t, r, 409, v)
		r, v = f.request("POST", "/v1/folders", f.aliceToken, core.NewID(), map[string]any{"parent_id": target.ID, "name": base.Name, "classification": "project_restricted"}, nil)
		expect(t, r, 201, v)
		collision := data[storage.Folder](t, v)
		r, v = f.request("POST", "/v1/file-scope-transfers", f.aliceToken, core.NewID(), input("file", base.ID, base.Version), nil)
		expect(t, r, 409, v)
		if v.Error.Code != "DUPLICATE_RESOURCE" {
			t.Fatal(v.Error)
		}
		r, v = f.request("POST", "/v1/folders/"+collision.ID+"/move", f.aliceToken, core.NewID(), map[string]any{"expected_version": collision.Version, "new_name": "同名冲突已核对", "reason": "finish controlled name collision fixture"}, nil)
		expect(t, r, 200, v)
	})
	t.Run("exact human execution preserves both immutable content versions and replays one receipt", func(t *testing.T) {
		p := create(t, input("file", base.ID, base.Version))
		var before, after []byte
		if e := f.owner.QueryRow(ctx, "SELECT jsonb_agg(to_jsonb(v) ORDER BY seq) FROM file_versions v WHERE org_id=$1 AND file_id=$2", f.org, base.ID).Scan(&before); e != nil {
			t.Fatal(e)
		}
		body, key := decision(p), core.NewID()
		r, v := f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/execute", f.aliceToken, key, body, nil)
		expect(t, r, 200, v)
		done := data[storage.ScopeTransfer](t, v)
		if done.State != "executed" || done.Version != 2 || done.CompletedAt == nil || done.CanExecute {
			t.Fatal(done)
		}
		moved := getFile(base.ID)
		if moved.FolderID != target.ID || moved.ProjectID == nil || *moved.ProjectID != other.ID || moved.Version != base.Version+1 || moved.CurrentVersionID == nil || *moved.CurrentVersionID != current.Version.ID {
			t.Fatal(moved)
		}
		if e := f.owner.QueryRow(ctx, "SELECT jsonb_agg(to_jsonb(v) ORDER BY seq) FROM file_versions v WHERE org_id=$1 AND file_id=$2", f.org, base.ID).Scan(&after); e != nil {
			t.Fatal(e)
		}
		if string(before) != string(after) {
			t.Fatal("content version or object provenance changed")
		}
		r, v = f.request("GET", "/v1/files/"+base.ID, f.bobToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.request("GET", "/v1/files/"+base.ID, f.memberToken, "", nil, nil)
		expect(t, r, 200, v)
		r, v = f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/execute", f.aliceToken, key, body, nil)
		expect(t, r, 200, v)
		if r.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("missing execution replay")
		}
		var audit, outbox int
		if e := f.owner.QueryRow(ctx, "SELECT (SELECT count(*) FROM audit_events WHERE org_id=$1 AND resource_id=$2 AND action='file.scope_transfer.executed'),(SELECT count(*) FROM outbox_events WHERE org_id=$1 AND aggregate_id=$2 AND event_type='file.scope_transfer.executed')", f.org, p.ID).Scan(&audit, &outbox); e != nil {
			t.Fatal(e)
		}
		if audit != 1 || outbox != 1 {
			t.Fatal("duplicate audit/outbox", audit, outbox)
		}
		if _, e := f.api.DB.Exec(ctx, "DELETE FROM file_scope_transfers WHERE org_id=$1 AND id=$2", f.org, p.ID); e == nil {
			t.Fatal("final receipt deleted")
		}
	})
	t.Run("recursive transfer updates every descendant and rejects descendant destinations", func(t *testing.T) {
		createFolder := func(parent, name string) storage.Folder {
			t.Helper()
			r, v := f.request("POST", "/v1/folders", f.aliceToken, core.NewID(), map[string]any{"parent_id": parent, "name": name, "classification": "project_restricted"}, nil)
			expect(t, r, 201, v)
			return data[storage.Folder](t, v)
		}
		root := createFolder(u.root, "跨项目整目录")
		child := createFolder(root.ID, "嵌套资料")
		cycle := input("folder", root.ID, root.Version)
		cycle.TargetFolderID, cycle.ExpectedTargetVersion = child.ID, child.Version
		r, v := f.request("POST", "/v1/file-scope-transfers", f.aliceToken, core.NewID(), cycle, nil)
		expect(t, r, 422, v)
		content, _ := commitContentFixture(t, u, "树内.txt", "text/plain", []byte("bounded actual transfer content\n"), nil)
		r, v = f.request("POST", "/v1/files/"+content.File.ID+"/move", f.aliceToken, core.NewID(), map[string]any{"expected_version": content.File.Version, "target_folder_id": child.ID, "reason": "ordinary tree fixture placement"}, nil)
		expect(t, r, 200, v)
		placed := data[storage.File](t, v)
		p := create(t, input("folder", root.ID, root.Version))
		if len(p.Snapshot.Items) != 3 || !p.CanExecute {
			t.Fatal(p)
		}
		r, v = f.request("POST", "/v1/file-scope-transfers/"+p.ID+"/execute", f.aliceToken, core.NewID(), decision(p), nil)
		expect(t, r, 200, v)
		for _, id := range []string{root.ID, child.ID} {
			folder := getFolder(id)
			if folder.ProjectID == nil || *folder.ProjectID != other.ID || folder.Version != 2 {
				t.Fatal(folder)
			}
		}
		moved := getFile(placed.ID)
		if moved.FolderID != child.ID || moved.ProjectID == nil || *moved.ProjectID != other.ID || moved.Version != placed.Version+1 {
			t.Fatal(moved)
		}
		if getFolder(root.ID).ParentID == nil || *getFolder(root.ID).ParentID != target.ID {
			t.Fatal("root target not applied")
		}
	})
	if os.Getenv("CP_BROWSER_SCOPE_TRANSFER_FIXTURE") == "1" {
		// New ordinary objects, not a reset of executed test facts. Both browser
		// clients receive their own isolated database and current human login.
		content, _ := commitContentFixture(t, u, "跨项目整理演示.txt", "text/plain", []byte("Synthetic company project source.\nAll old content versions remain pinned.\n"), nil)
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"scope_transfer_fixture": true, "transfer_file_id": content.File.ID, "transfer_source_folder": u.root, "transfer_target_folder": target.ID, "transfer_target_project": other.ID})
	}
}
