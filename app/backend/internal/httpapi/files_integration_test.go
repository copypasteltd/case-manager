package httpapi

import (
	"context"
	"strings"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
)

func TestStorageNamespaceAndMetadata(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := f.createProject("Storage policy", []string{f.bob})
	other := f.createProject("Separate storage policy", nil)
	root := func(projectID string) string {
		var id string
		if err := f.owner.QueryRow(ctx, "SELECT id FROM folders WHERE org_id=$1 AND project_id=$2 AND parent_id IS NULL", f.org, projectID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	rootID, otherRoot := root(p.ID), root(other.ID)
	createFolder := func(parent, name, key string) (int, response) {
		rec, result := f.request("POST", "/v1/folders", f.aliceToken, key, map[string]any{"parent_id": parent, "name": name, "classification": "project_restricted"}, nil)
		return rec.Code, result
	}
	var target storage.Folder
	t.Run("shared Unicode namespace rejects cross type collisions and serializes concurrent creates", func(t *testing.T) {
		status, result := createFolder(rootID, "版本资料", core.NewID())
		if status != 201 {
			t.Fatal(status, string(result.Data), result.Error)
		}
		target = data[storage.Folder](t, result)
		if target.CreatorID == nil || *target.CreatorID != f.alice || !target.CanCreate {
			t.Fatal("directory creator or capabilities missing")
		}
		status, _ = createFolder(rootID, "Ａbc", core.NewID())
		if status != 201 {
			t.Fatal(status)
		}
		status, result = createFolder(rootID, "aBC", core.NewID())
		if status != 409 || result.Error.Code != "DUPLICATE_RESOURCE" {
			t.Fatal("normalized duplicate accepted", status, result.Error)
		}
		var group sync.WaitGroup
		statuses := make(chan int, 2)
		for _, name := range []string{"Résumé", "Re\u0301sume\u0301"} {
			group.Add(1)
			go func(name string) {
				defer group.Done()
				code, _ := createFolder(rootID, name, core.NewID())
				statuses <- code
			}(name)
		}
		group.Wait()
		close(statuses)
		success, conflict := 0, 0
		for status := range statuses {
			if status == 201 {
				success++
			}
			if status == 409 {
				conflict++
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatal("concurrent name reservation failed", success, conflict)
		}
		if _, err := f.owner.Exec(ctx, "INSERT INTO files(id,org_id,folder_id,name,name_key,classification,state,creator_id) VALUES($1,$2,$3,'版本资料','版本资料','project_restricted','active',$4)", core.NewID(), f.org, rootID, f.alice); err == nil {
			t.Fatal("file bypassed folder namespace")
		}
		rec, result := f.request("GET", "/v1/folders?parent_id="+rootID+"&limit=1", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		page := data[core.Page[storage.Folder]](t, result)
		if len(page.Items) != 1 || page.NextCursor == nil {
			t.Fatal("folder pagination missing")
		}
		rec, result = f.request("GET", "/v1/folders?parent_id="+rootID+"&cursor="+*page.NextCursor, f.bobToken, "", nil, nil)
		expect(t, rec, 400, result)
		if _, err := f.api.DB.Exec(ctx, "INSERT INTO storage_entries(org_id,namespace_key,name_key,resource_type,resource_id) VALUES($1,'x','x','folder',$2)", f.org, core.NewID()); err == nil {
			t.Fatal("application may modify namespace reservations")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE folders SET creator_id=$3 WHERE org_id=$1 AND id=$2", f.org, target.ID, f.bob); err == nil {
			t.Fatal("folder creator changed")
		}
	})
	t.Run("moves enforce ancestry scope versions and immutable project root", func(t *testing.T) {
		status, result := createFolder(target.ID, "子目录", core.NewID())
		if status != 201 {
			t.Fatal(status, result.Error)
		}
		child := data[storage.Folder](t, result)
		body := map[string]any{"expected_version": target.Version, "target_folder_id": child.ID, "reason": "检验目录环"}
		rec, result := f.request("POST", "/v1/folders/"+target.ID+"/move", f.aliceToken, core.NewID(), body, nil)
		expect(t, rec, 422, result)
		body["target_folder_id"] = otherRoot
		rec, result = f.request("POST", "/v1/folders/"+target.ID+"/move", f.aliceToken, core.NewID(), body, nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "PERMISSION_CHANGE_PLAN_REQUIRED" {
			t.Fatal(result.Error)
		}
		body = map[string]any{"expected_version": target.Version, "new_name": "项目版本资料", "reason": "规范命名"}
		key := core.NewID()
		rec, result = f.request("POST", "/v1/folders/"+target.ID+"/move", f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		operation := rec.Header().Get("X-Operation-ID")
		target = data[storage.Folder](t, result)
		rec, result = f.request("POST", "/v1/folders/"+target.ID+"/move", f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		if rec.Header().Get("X-Operation-ID") != operation || rec.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("rename replay lost operation identity")
		}
		rec, result = f.request("POST", "/v1/folders/"+target.ID+"/move", f.aliceToken, core.NewID(), body, nil)
		expect(t, rec, 409, result)
		rec, result = f.request("POST", "/v1/folders/"+rootID+"/move", f.aliceToken, core.NewID(), map[string]any{"expected_version": 1, "target_folder_id": target.ID, "reason": "移动项目根目录"}, nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "PROJECT_ROOT_FIXED" {
			t.Fatal(result.Error)
		}
	})
	// Metadata-only fixture. It is not an object upload or a cloud verification.
	fileID, versionID := core.NewID(), core.NewID()
	sha := strings.Repeat("a", 64)
	if _, err := f.owner.Exec(ctx, "INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence) VALUES($1,1,'oss',$2,'metadata test fixture; not a cloud fence')", f.org, f.admin); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO files(id,org_id,folder_id,name,name_key,classification,state,creator_id) VALUES($1,$2,$3,'source.bin','source.bin','project_restricted','active',$4)", fileID, f.org, target.ID, f.alice); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO file_versions(id,org_id,file_id,seq,object_key,provider_source_id,storage_epoch,size_bytes,sha256,media_type,source,author_id,executor_id,scan_state,availability,original_name) VALUES($1,$2,$3,1,$4,'fixture-source',1,120000000000,$5,'application/octet-stream','metadata-test',$6,$6,'clean','available','source.bin')", versionID, f.org, fileID, "fixture/"+versionID, sha, f.alice); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "UPDATE files SET current_version_id=$3 WHERE org_id=$1 AND id=$2", f.org, fileID, versionID); err != nil {
		t.Fatal(err)
	}
	t.Run("file metadata preserves version provenance and reports replica evidence separately", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("GET", "/v1/files?folder_id="+target.ID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		listed := data[core.Page[storage.File]](t, result)
		if len(listed.Items) != 1 || listed.Items[0].ID != fileID {
			t.Fatal("file list wrong")
		}
		rec, result = f.request("GET", "/v1/files/"+fileID+"/versions", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		v := data[core.Page[storage.Version]](t, result).Items[0]
		if v.SizeBytes != "120000000000" || v.ReplicaState != nil || len(v.Replicas) != 0 {
			t.Fatal("missing replicas fabricated or byte size rounded")
		}
		for _, provider := range []string{"oss", "r2"} {
			if _, err := f.owner.Exec(ctx, "INSERT INTO file_replicas(id,org_id,version_id,provider,bucket,object_key,state,expected_sha256,actual_sha256,verified_at,last_error) VALUES($1,$2,$3,$4,'private-fixture',$5,'verified',$6,$6,now(),'secret should never leave server')", core.NewID(), f.org, versionID, provider, "private/"+versionID, sha); err != nil {
				t.Fatal(err)
			}
		}
		rec, result = f.request("GET", "/v1/files/"+fileID+"/versions", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		v = data[core.Page[storage.Version]](t, result).Items[0]
		if v.ReplicaState == nil || *v.ReplicaState != "verified" || len(v.Replicas) != 2 {
			t.Fatal("independent replica evidence lost")
		}
		if strings.Contains(rec.Body.String(), "private-fixture") || strings.Contains(rec.Body.String(), "secret should") {
			t.Fatal("storage internals leaked")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE file_replicas SET expected_sha256=$3,actual_sha256=$3 WHERE org_id=$1 AND version_id=$2 AND provider='r2'", f.org, versionID, strings.Repeat("b", 64)); err == nil {
			t.Fatal("database allowed changing the replica's immutable expected digest")
		}
		// A historical invalid import must still be presented as failed. The
		// privileged fixture replaces only its synthetic row; applications
		// cannot mutate the now-immutable replica identity to create this case.
		if _, err := f.owner.Exec(ctx, "DELETE FROM file_replicas WHERE org_id=$1 AND version_id=$2 AND provider='r2'", f.org, versionID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.owner.Exec(ctx, `INSERT INTO file_replicas(id,org_id,version_id,provider,bucket,object_key,state,expected_sha256,actual_sha256,verified_at)
            VALUES($1,$2,$3,'r2','private-fixture',$4,'verified',$5,$5,now())`, core.NewID(), f.org, versionID, "private/"+versionID, strings.Repeat("b", 64)); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("GET", "/v1/files/"+fileID+"/versions", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		v = data[core.Page[storage.Version]](t, result).Items[0]
		if v.ReplicaState == nil || *v.ReplicaState != "failed" {
			t.Fatal("wrong expected digest still reported verified")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE file_replicas SET actual_sha256=NULL WHERE org_id=$1 AND version_id=$2", f.org, versionID); err == nil {
			t.Fatal("database accepted evidence-free verified replica")
		}
		rec, result = f.request("POST", "/v1/files/"+fileID+"/move", f.aliceToken, core.NewID(), map[string]any{"expected_version": 1, "new_name": "renamed.bin", "reason": "新规范名称"}, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("GET", "/v1/files/"+fileID+"/versions", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		v = data[core.Page[storage.Version]](t, result).Items[0]
		if v.OriginalName == nil || *v.OriginalName != "source.bin" || v.ID != versionID || v.SHA256 != sha {
			t.Fatal("rename changed immutable original or bytes")
		}
		rec, result = f.request("GET", "/v1/files/"+fileID, f.memberToken, "", nil, nil)
		expect(t, rec, 404, result)
	})
	t.Run("trash holds restore name conflicts and archived projects preserve immutable bytes", func(t *testing.T) {
		f := f.with(t)
		if _, err := f.owner.Exec(ctx, "UPDATE files SET legal_hold=true WHERE org_id=$1 AND id=$2", f.org, fileID); err != nil {
			t.Fatal(err)
		}
		body := map[string]any{"expected_version": 2, "action": "trash", "reason": "测试软删除"}
		rec, result := f.request("POST", "/v1/files/"+fileID+"/trash", f.aliceToken, core.NewID(), body, nil)
		expect(t, rec, 409, result)
		if _, err := f.owner.Exec(ctx, "UPDATE files SET legal_hold=false WHERE org_id=$1 AND id=$2", f.org, fileID); err != nil {
			t.Fatal(err)
		}
		key := core.NewID()
		rec, result = f.request("POST", "/v1/files/"+fileID+"/trash", f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("POST", "/v1/files/"+fileID+"/trash", f.aliceToken, key, body, nil)
		expect(t, rec, 200, result)
		if rec.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("trash replay failed after state changed")
		}
		status, created := createFolder(target.ID, "renamed.bin", core.NewID())
		if status != 201 {
			t.Fatal(status, created.Error)
		}
		collision := data[storage.Folder](t, created)
		restore := map[string]any{"expected_version": 3, "mode": "untrash", "reason": "恢复至原目录"}
		rec, result = f.request("POST", "/v1/files/"+fileID+"/restore", f.aliceToken, core.NewID(), restore, nil)
		expect(t, rec, 409, result)
		rec, result = f.request("POST", "/v1/folders/"+collision.ID+"/move", f.aliceToken, core.NewID(), map[string]any{"expected_version": 1, "new_name": "占位目录", "reason": "释放旧名称"}, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("POST", "/v1/files/"+fileID+"/restore", f.aliceToken, core.NewID(), restore, nil)
		expect(t, rec, 200, result)
		restored := data[storage.File](t, result)
		if restored.Version != 4 || restored.CurrentVersionID == nil || *restored.CurrentVersionID != versionID {
			t.Fatal("untrash changed immutable current content")
		}
		rec, result = f.request("POST", "/v1/files/"+fileID+"/restore", f.aliceToken, core.NewID(), map[string]any{"expected_version": 4, "mode": "new_version_from_history", "source_version_id": versionID, "reason": "恢复历史内容"}, nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "ASYNC_HISTORY_RESTORE_REQUIRED" {
			t.Fatal(result.Error)
		}
		if _, err := f.owner.Exec(ctx, "UPDATE project_members SET project_role='viewer' WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, p.ID, f.bob); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("GET", "/v1/files/"+fileID, f.bobToken, "", nil, nil)
		expect(t, rec, 200, result)
		visible := data[storage.File](t, result)
		if visible.CanMove || visible.CanTrash || visible.CanUpdate {
			t.Fatal("viewer received write capabilities")
		}
		rec, result = f.request("POST", "/v1/files/"+fileID+"/move", f.bobToken, core.NewID(), map[string]any{"expected_version": 4, "new_name": "forbidden.bin", "reason": "只读用户修改"}, nil)
		expect(t, rec, 404, result)
		archiveProjectFixture(t, f, p.ID)
		rec, result = f.request("POST", "/v1/files/"+fileID+"/move", f.aliceToken, core.NewID(), map[string]any{"expected_version": 4, "new_name": "forbidden.bin", "reason": "归档后修改"}, nil)
		expect(t, rec, 409, result)
	})
	t.Run("audit failure rolls back namespace business state and idempotency together", func(t *testing.T) {
		f := f.with(t)
		_, err := f.owner.Exec(ctx, `CREATE FUNCTION reject_storage_audit() RETURNS trigger LANGUAGE plpgsql AS $$
            BEGIN IF NEW.action='folder.created' THEN RAISE EXCEPTION 'storage audit fixture failure'; END IF; RETURN NEW; END; $$;
            CREATE TRIGGER reject_storage_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_storage_audit();`)
		if err != nil {
			t.Fatal(err)
		}
		key := core.NewID()
		status, result := createFolder(otherRoot, "不得部分提交", key)
		if status != 500 {
			t.Fatal(status, result.Error)
		}
		var count int
		err = f.owner.QueryRow(ctx, `SELECT
            (SELECT count(*) FROM folders WHERE org_id=$1 AND name='不得部分提交')+
            (SELECT count(*) FROM storage_entries WHERE org_id=$1 AND name_key='不得部分提交')+
            (SELECT count(*) FROM idempotency_records WHERE org_id=$1 AND idempotency_key=$2)`, f.org, key).Scan(&count)
		if err != nil || count != 0 {
			t.Fatal("partial storage mutation survived rollback", count, err)
		}
		if _, err = f.owner.Exec(ctx, "DROP TRIGGER reject_storage_audit ON audit_events; DROP FUNCTION reject_storage_audit()"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("AI policy applies before file listing and cursors bind the channel", func(t *testing.T) {
		a, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		service := storage.Service{DB: f.api.DB, Pager: platform.Pager{Key: f.api.Config.CursorKey}}
		page, err := service.Folders(ctx, a, storage.Filter{ParentID: rootID, Limit: 1})
		if err != nil || page.NextCursor == nil {
			t.Fatal("missing fixture page", err)
		}
		changed := a
		changed.Channel = "web"
		if _, err = service.Folders(ctx, changed, storage.Filter{ParentID: rootID, Cursor: *page.NextCursor, Limit: 1}); core.PublicError(err).Code != "INVALID_CURSOR" {
			t.Fatal("cursor crossed channel", err)
		}
		agent := a
		agent.RunID = core.NewID()
		folders, err := service.Folders(ctx, agent, storage.Filter{})
		if err != nil || len(folders.Items) != 0 {
			t.Fatal("AI-disallowed directory leaked through root listing", err)
		}
		files, err := service.Files(ctx, agent, storage.Filter{})
		if err != nil || len(files.Items) != 0 {
			t.Fatal("AI-disallowed file leaked through listing", err)
		}
		if _, err = service.File(ctx, agent, fileID); core.PublicError(err).Status != 404 {
			t.Fatal("AI-disallowed metadata leaked", err)
		}
	})
	t.Run("own scope filters by immutable file creator before listing and history", func(t *testing.T) {
		f := f.with(t)
		if _, err := f.owner.Exec(ctx, "UPDATE role_bindings SET scope_type='own',scope_id=NULL WHERE org_id=$1 AND user_id=$2", f.org, f.bob); err != nil {
			t.Fatal(err)
		}
		rec, result := f.request("GET", "/v1/files/"+fileID, f.bobToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.request("GET", "/v1/files/"+fileID+"/versions", f.bobToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.request("GET", "/v1/files?q=renamed", f.bobToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[storage.File]](t, result).Items) != 0 {
			t.Fatal("own scope listed another creator's file")
		}
		rec, result = f.request("GET", "/v1/files/"+fileID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
	})
}
