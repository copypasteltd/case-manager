package httpapi

import (
	"context"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
)

func TestPinnedFileVersionMetadata(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := f.createProject("Pinned version metadata", []string{f.bob})
	var root string
	if err := f.owner.QueryRow(ctx, "SELECT id FROM folders WHERE org_id=$1 AND project_id=$2 AND parent_id IS NULL", f.org, p.ID).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence) VALUES($1,1,'oss',$2,'metadata-only fixture')", f.org, f.admin); err != nil {
		t.Fatal(err)
	}
	fileID, otherFile, otherVersion := core.NewID(), core.NewID(), core.NewID()
	for index, id := range []string{fileID, otherFile} {
		if _, err := f.owner.Exec(ctx, "INSERT INTO files(id,org_id,folder_id,name,name_key,classification,state,creator_id) VALUES($1,$2,$3,$4,$4,'project_restricted','active',$5)", id, f.org, root, []string{"pinned.bin", "separate.bin"}[index], f.alice); err != nil {
			t.Fatal(err)
		}
	}
	sha := strings.Repeat("a", 64)
	insert := func(file, id string, seq int) {
		// Exact metadata, no uploaded bytes and no real provider verification.
		if _, err := f.owner.Exec(ctx, `INSERT INTO file_versions(id,org_id,file_id,seq,object_key,provider_source_id,storage_epoch,size_bytes,sha256,media_type,source,author_id,executor_id,scan_state,availability,original_name)
            VALUES($1,$2,$3,$4,$5,'metadata-fixture',1,120000000000,$6,'application/octet-stream','metadata-test',$7,$7,'clean','available','original.bin')`, id, f.org, file, seq, "fixture/"+id, sha, f.alice); err != nil {
			t.Fatal(err)
		}
	}
	versions := make([]string, 22)
	for index := range versions {
		versions[index] = core.NewID()
		insert(fileID, versions[index], index+1)
	}
	insert(otherFile, otherVersion, 1)
	if _, err := f.owner.Exec(ctx, "UPDATE files SET current_version_id=$3 WHERE org_id=$1 AND id=$2", f.org, fileID, versions[21]); err != nil {
		t.Fatal(err)
	}
	route := "/v1/files/" + fileID + "/versions/" + versions[0]
	globalRoute := "/v1/file-versions/" + versions[0]
	t.Run("exact history beyond the first page remains pinned after a rename", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("GET", "/v1/files/"+fileID+"/versions?limit=20", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		page := data[core.Page[storage.Version]](t, result)
		if len(page.Items) != 20 || page.NextCursor == nil || page.Items[0].ID != versions[21] {
			t.Fatal("history fixture does not span pages")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE files SET name='renamed.bin',name_key='renamed.bin' WHERE org_id=$1 AND id=$2", f.org, fileID); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("GET", route, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		value := data[storage.Version](t, result)
		if value.ID != versions[0] || value.Seq != 1 || value.SizeBytes != "120000000000" || value.SHA256 != sha || value.OriginalName == nil || *value.OriginalName != "original.bin" {
			t.Fatal("requested history was substituted", value)
		}
	})
	t.Run("attachment version IDs resolve exactly with no current-version substitution", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("GET", globalRoute, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		value := data[storage.Version](t, result)
		if value.ID != versions[0] || value.FileID != fileID || value.Seq != 1 || value.SHA256 != sha {
			t.Fatal("attachment pin changed", value)
		}
		for _, id := range []string{core.NewID(), "not-a-version"} {
			rec, result = f.request("GET", "/v1/file-versions/"+id, f.aliceToken, "", nil, nil)
			expect(t, rec, 404, result)
		}
		rec, result = f.request("GET", globalRoute, "", "", nil, nil)
		expect(t, rec, 401, result)
	})
	t.Run("wrong-file unknown and malformed versions cannot resolve", func(t *testing.T) {
		f := f.with(t)
		for _, id := range []string{otherVersion, core.NewID(), "not-a-version"} {
			rec, result := f.request("GET", "/v1/files/"+fileID+"/versions/"+id, f.aliceToken, "", nil, nil)
			expect(t, rec, 404, result)
		}
	})
	t.Run("current replica evidence is projected without private object coordinates", func(t *testing.T) {
		f := f.with(t)
		for _, provider := range []string{"oss", "r2"} {
			digest := sha
			if provider == "r2" {
				digest = strings.Repeat("b", 64)
			}
			if _, err := f.owner.Exec(ctx, `INSERT INTO file_replicas(id,org_id,version_id,provider,bucket,object_key,state,expected_sha256,actual_sha256,verified_at,last_error)
                VALUES($1,$2,$3,$4,'private-pinned-fixture',$5,'verified',$6,$6,now(),'private-fixture-error')`, core.NewID(), f.org, versions[0], provider, "private/"+versions[0], digest); err != nil {
				t.Fatal(err)
			}
		}
		rec, result := f.request("GET", route, f.bobToken, "", nil, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("GET", globalRoute, f.bobToken, "", nil, nil)
		expect(t, rec, 200, result)
		value := data[storage.Version](t, result)
		if value.ReplicaState == nil || *value.ReplicaState != "failed" || len(value.Replicas) != 2 {
			t.Fatal("invalid replica proof reported as verified", value)
		}
		for _, private := range []string{"private-pinned-fixture", "private-fixture-error", "private/"} {
			if strings.Contains(rec.Body.String(), private) {
				t.Fatal("private storage coordinates leaked")
			}
		}
	})
	t.Run("current membership is required on every pinned read", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("GET", route, f.memberToken, "", nil, nil)
		expect(t, rec, 404, result)
		if _, err := f.owner.Exec(ctx, "DELETE FROM project_members WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, p.ID, f.bob); err != nil {
			t.Fatal(err)
		}
		rec, result = f.request("GET", route, f.bobToken, "", nil, nil)
		expect(t, rec, 404, result)
		for _, token := range []string{f.memberToken, f.bobToken} {
			rec, result = f.request("GET", globalRoute, token, "", nil, nil)
			expect(t, rec, 404, result)
		}
	})
}
