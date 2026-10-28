package httpapi

import (
	"context"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/importspec"
)

func importFixture(t *testing.T) (*uploadFixture, importspec.Entry, string) {
	f := newUploadFixture(t)
	configureStorageTargets(f)
	_, err := f.f.owner.Exec(context.Background(), `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
 SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='file_importer'`, core.NewID(), f.f.org, f.f.alice)
	if err != nil {
		t.Fatal(err)
	}
	f.f.aliceToken = memberMFAToken(f.f, "alice", "android")
	rec, result := f.f.request("GET", "/v1/folders/"+f.root, f.f.aliceToken, "", nil, nil)
	expect(t, rec, 200, result)
	folder := data[storage.Folder](t, result)
	namespace := core.NewID()
	e := importspec.Entry{Ordinal: 1, SourcePath: "approved/原始资料.bin", SourceSHA256: zeroSHA(15), SizeBytes: "15", SourceModifiedAt: time.Now().UTC().Format(time.RFC3339Nano), SourceMode: "-rw-------", SourceOwner: "original owner reviewed", MappedOwnerID: f.f.alice, FolderID: folder.ID, FolderVersion: folder.Version, ProjectID: folder.ProjectID, Classification: folder.Classification, Name: "导入原件.bin", MediaType: "application/octet-stream"}
	e.SourceKey = importspec.SourceKey(namespace, e.SourcePath)
	return f, e, namespace
}

func registerImport(t *testing.T, f *uploadFixture, e importspec.Entry, namespace string) (storage.ImportBatch, storage.ImportEntry) {
	t.Helper()
	in := importspec.BatchInput{SourceNamespace: namespace, InventorySHA256: core.Digest([]byte("owned synthetic inventory")), MappingSHA256: core.Digest([]byte("owned synthetic mapping")), PlanSHA256: importspec.Chain(importspec.ZeroChain, importspec.EntryDigest(e)), EntryCount: 1, TotalBytes: e.SizeBytes, Reason: "explicit synthetic import"}
	rec, result := f.f.request("POST", "/v1/file-imports", f.f.aliceToken, core.NewID(), in, nil)
	expect(t, rec, 201, result)
	b := data[storage.ImportBatch](t, result)
	rec, result = f.f.request("POST", "/v1/file-imports/"+b.ID+"/entries", f.f.aliceToken, core.NewID(), e, nil)
	expect(t, rec, 201, result)
	entry := data[storage.ImportEntry](t, result)
	return b, entry
}

func approveImport(t *testing.T, f *uploadFixture, b storage.ImportBatch) {
	t.Helper()
	rec, result := f.f.request("POST", "/v1/file-imports/"+b.ID+"/approve", f.f.aliceToken, core.NewID(), importspec.ApprovalInput{PlanSHA256: b.PlanSHA256, ReviewedMappings: true, SourcesReadOnly: true, Reason: "reviewed exact source mapping and read-only window"}, nil)
	expect(t, rec, 200, result)
	if approved := data[storage.ImportBatch](t, result); !approved.Approved || approved.ApprovalExpiresAt == nil {
		t.Fatal("approval missing")
	}
}

func startImport(t *testing.T, f *uploadFixture, e storage.ImportEntry) storage.Upload {
	t.Helper()
	rec, result := f.f.request("POST", "/v1/file-import-entries/"+e.ID+"/uploads", f.f.aliceToken, core.NewID(), map[string]any{}, nil)
	expect(t, rec, 201, result)
	entry := data[storage.ImportEntry](t, result)
	if entry.Upload == nil {
		t.Fatal("durable upload mapping missing")
	}
	return *entry.Upload
}

func readyImport(t *testing.T, f *uploadFixture, u storage.Upload) storage.Upload {
	t.Helper()
	u = f.freezeUpload(u)
	from := scannedUpload(f, u)
	to := newPromotionCloud(f)
	if worked, err := promotionHost(f, from, to).RunOne(context.Background()); err != nil || !worked {
		t.Fatal("import promotion failed", err)
	}
	return f.get(u.ID)
}

func TestFileImportManifest(t *testing.T) {
	t.Run("complete exact plan required before any upload; duplicate admission and session reconcile", func(t *testing.T) {
		f, e, namespace := importFixture(t)
		b, entry := registerImport(t, f, e, namespace)
		rec, result := f.f.request("POST", "/v1/file-import-entries/"+entry.ID+"/uploads", f.f.aliceToken, core.NewID(), map[string]any{}, nil)
		expect(t, rec, 403, result)
		var count int
		f.f.owner.QueryRow(context.Background(), "SELECT count(*) FROM upload_sessions WHERE org_id=$1", f.f.org).Scan(&count)
		if count != 0 {
			t.Fatal("unapproved plan created upload")
		}
		approveImport(t, f, b)
		u := startImport(t, f, entry)
		rec, result = f.f.request("POST", "/v1/file-import-entries/"+entry.ID+"/uploads", f.f.aliceToken, core.NewID(), map[string]any{}, nil)
		expect(t, rec, 200, result)
		if got := data[storage.ImportEntry](t, result); got.Upload == nil || got.Upload.ID != u.ID {
			t.Fatal("source retry created another session")
		}
		rec, result = f.f.request("POST", "/v1/file-imports/"+b.ID+"/entries", f.f.aliceToken, core.NewID(), e, nil)
		expect(t, rec, 200, result)
		changed := e
		changed.Name = "unexpected.bin"
		rec, result = f.f.request("POST", "/v1/file-imports/"+b.ID+"/entries", f.f.aliceToken, core.NewID(), changed, nil)
		expect(t, rec, 409, result)
		if _, err := f.f.api.DB.Exec(context.Background(), "UPDATE file_import_entries SET source_sha256=$2 WHERE id=$1", entry.ID, core.Digest([]byte("forged"))); err == nil {
			t.Fatal("immutable import entry changed")
		}
		if _, err := f.f.api.DB.Exec(context.Background(), "UPDATE file_import_batches SET admitted_count=0 WHERE id=$1", b.ID); err == nil {
			t.Fatal("import counters rewritten")
		}
	})
	t.Run("manifest mismatch rolls back admission without counters or side effects", func(t *testing.T) {
		f, e, namespace := importFixture(t)
		in := importspec.BatchInput{SourceNamespace: namespace, InventorySHA256: e.SourceSHA256, MappingSHA256: e.SourceSHA256, PlanSHA256: core.Digest([]byte("different plan")), EntryCount: 1, TotalBytes: e.SizeBytes, Reason: "mismatch fixture"}
		rec, result := f.f.request("POST", "/v1/file-imports", f.f.aliceToken, core.NewID(), in, nil)
		expect(t, rec, 201, result)
		b := data[storage.ImportBatch](t, result)
		rec, result = f.f.request("POST", "/v1/file-imports/"+b.ID+"/entries", f.f.aliceToken, core.NewID(), e, nil)
		expect(t, rec, 409, result)
		rec, result = f.f.request("GET", "/v1/file-imports/"+b.ID, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if data[storage.ImportBatch](t, result).AdmittedCount != 0 {
			t.Fatal("mismatch retained partial counter")
		}
		rec, result = f.f.request("POST", "/v1/file-imports/"+b.ID+"/approve", f.f.aliceToken, core.NewID(), importspec.ApprovalInput{PlanSHA256: in.PlanSHA256, ReviewedMappings: true, SourcesReadOnly: true, Reason: "incomplete"}, nil)
		expect(t, rec, 409, result)
	})
}

func TestFileImportCommit(t *testing.T) {
	t.Run("actual scan promotion commit binds durable logical file and reports pending replica separately", func(t *testing.T) {
		f, e, namespace := importFixture(t)
		b, entry := registerImport(t, f, e, namespace)
		approveImport(t, f, b)
		u := readyImport(t, f, startImport(t, f, entry))
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
		expect(t, rec, 201, result)
		committed := data[storage.CommitResult](t, result)
		var mapped string
		if err := f.f.owner.QueryRow(context.Background(), "SELECT file_id FROM file_import_objects WHERE org_id=$1 AND source_namespace=$2 AND source_key=$3", f.f.org, namespace, e.SourceKey).Scan(&mapped); err != nil || mapped != committed.File.ID {
			t.Fatal("persistent source-to-file mapping missing", err)
		}
		rec, result = f.f.request("GET", "/v1/file-import-entries/"+entry.ID, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		view := data[storage.ImportEntry](t, result)
		if view.FileVersion == nil || view.FileVersion.ID != committed.Version.ID || view.VerificationState != "primary_verified" || !view.MappingCurrent || !view.MappedOwnerCanRead {
			t.Fatal("primary incorrectly reported as full backup", view)
		}
		rec, result = f.f.request("POST", "/v1/file-import-entries/"+entry.ID+"/uploads", f.f.aliceToken, core.NewID(), map[string]any{}, nil)
		expect(t, rec, 200, result)
		if data[storage.ImportEntry](t, result).Upload.ID != u.ID {
			t.Fatal("committed source repeated upload")
		}
	})
	t.Run("approved expected source hash prevents a valid but different candidate from becoming formal", func(t *testing.T) {
		f, e, namespace := importFixture(t)
		e.SourceSHA256 = core.Digest([]byte("different bytes"))
		b, entry := registerImport(t, f, e, namespace)
		approveImport(t, f, b)
		u := readyImport(t, f, startImport(t, f, entry))
		rec, _ := f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
		if rec.Code < 400 {
			t.Fatal("wrong source digest became formal")
		}
		var count int
		f.f.owner.QueryRow(context.Background(), "SELECT count(*) FROM file_versions WHERE org_id=$1", f.f.org).Scan(&count)
		if count != 0 {
			t.Fatal("failed digest retained formal version")
		}
	})
}

func TestFileImportAuthority(t *testing.T) {
	f, e, namespace := importFixture(t)
	b, entry := registerImport(t, f, e, namespace)
	approveImport(t, f, b)
	rec, result := f.f.request("GET", "/v1/file-imports/"+b.ID, f.f.bobToken, "", nil, nil)
	if rec.Code < 400 {
		t.Fatal("non-importer read private migration", result)
	}
	// A current metadata change requires another complete plan; there is no inferred grant.
	_, err := f.f.owner.Exec(context.Background(), "UPDATE folders SET version=version+1 WHERE id=$1", f.root)
	if err != nil {
		t.Fatal(err)
	}
	rec, result = f.f.request("POST", "/v1/file-import-entries/"+entry.ID+"/uploads", f.f.aliceToken, core.NewID(), map[string]any{}, nil)
	expect(t, rec, 409, result)
	_, err = f.f.owner.Exec(context.Background(), "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.f.org)
	if err != nil {
		t.Fatal(err)
	}
	rec, result = f.f.request("GET", "/v1/file-imports/"+b.ID, f.f.aliceToken, "", nil, nil)
	expect(t, rec, 200, result)
	if data[storage.ImportBatch](t, result).Approved {
		t.Fatal("approval survived an authority revision")
	}
	_, err = f.f.owner.Exec(context.Background(), `UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND role_id=(SELECT id FROM roles WHERE org_id=$1 AND key='file_importer')`, f.f.org, f.f.alice)
	if err != nil {
		t.Fatal(err)
	}
	rec, _ = f.f.request("GET", "/v1/file-imports/"+b.ID, f.f.aliceToken, "", nil, nil)
	if rec.Code < 400 {
		t.Fatal("revoked importer read the batch")
	}
}

func TestFileImportRetryAndDelta(t *testing.T) {
	t.Run("expired restart is explicit and stale session cannot become formal", func(t *testing.T) {
		f, e, ns := importFixture(t)
		b, entry := registerImport(t, f, e, ns)
		approveImport(t, f, b)
		old := startImport(t, f, entry)
		rec, result := f.f.request("POST", "/v1/file-import-entries/"+entry.ID+"/retry", f.f.aliceToken, core.NewID(), storage.ImportRetry{UploadID: old.ID, ExpectedVersion: old.Version, Reason: "reviewed exact failed session"}, nil)
		expect(t, rec, 409, result)
		if _, err := f.f.owner.Exec(context.Background(), "UPDATE upload_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", old.ID); err != nil {
			t.Fatal(err)
		}
		old = f.get(old.ID)
		rec, result = f.f.request("POST", "/v1/file-import-entries/"+entry.ID+"/retry", f.f.aliceToken, core.NewID(), storage.ImportRetry{UploadID: old.ID, ExpectedVersion: old.Version, Reason: "reviewed expired session; source unchanged"}, nil)
		expect(t, rec, 201, result)
		replacement := data[storage.ImportEntry](t, result)
		if replacement.Upload == nil || replacement.Upload.ID == old.ID {
			t.Fatal("restart not explicit")
		}
		rec, result = f.f.request("POST", "/v1/file-import-entries/"+entry.ID+"/uploads", f.f.aliceToken, core.NewID(), map[string]any{}, nil)
		expect(t, rec, 200, result)
		if data[storage.ImportEntry](t, result).Upload.ID != replacement.Upload.ID {
			t.Fatal("active source pointer lost")
		}
	})
	t.Run("changed source requires fixed logical file and current base; replay rechecks import authority", func(t *testing.T) {
		f, e, ns := importFixture(t)
		b, entry := registerImport(t, f, e, ns)
		approveImport(t, f, b)
		u := readyImport(t, f, startImport(t, f, entry))
		key := core.NewID()
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, key, commitInput(u), nil)
		expect(t, rec, 201, result)
		base := data[storage.CommitResult](t, result)
		if _, err := f.f.owner.Exec(context.Background(), "UPDATE upload_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", u.ID); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", "/v1/file-import-entries/"+entry.ID+"/retry", f.f.aliceToken, core.NewID(), storage.ImportRetry{UploadID: u.ID, ExpectedVersion: f.get(u.ID).Version, Reason: "committed session is never restartable"}, nil)
		expect(t, rec, 409, result)
		changed := e
		changed.SourceSHA256 = zeroSHA(16)
		changed.SizeBytes = "16"
		changed.SourceModifiedAt = time.Now().UTC().Format(time.RFC3339Nano)
		delta, deltaEntry := registerImport(t, f, changed, ns)
		approveImport(t, f, delta)
		rec, result = f.f.request("POST", "/v1/file-import-entries/"+deltaEntry.ID+"/uploads", f.f.aliceToken, core.NewID(), map[string]any{}, nil)
		expect(t, rec, 409, result)
		changed.FileID = &base.File.ID
		changed.BaseVersionID = &base.Version.ID
		delta, deltaEntry = registerImport(t, f, changed, ns)
		approveImport(t, f, delta)
		next := readyImport(t, f, startImport(t, f, deltaEntry))
		rec, result = f.f.request("POST", "/v1/uploads/"+next.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(next), nil)
		expect(t, rec, 201, result)
		final := data[storage.CommitResult](t, result)
		if final.File.ID != base.File.ID || final.Version.ID == base.Version.ID {
			t.Fatal("delta duplicated logical file")
		}
		var count int
		f.f.owner.QueryRow(context.Background(), "SELECT count(*) FROM files WHERE org_id=$1", f.f.org).Scan(&count)
		if count != 1 {
			t.Fatal("delta produced extra file")
		}
		if _, err := f.f.owner.Exec(context.Background(), `UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2 AND role_id=(SELECT id FROM roles WHERE org_id=$1 AND key='file_importer')`, f.f.org, f.f.alice); err != nil {
			t.Fatal(err)
		}
		rec, _ = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, key, commitInput(u), nil)
		if rec.Code < 400 {
			t.Fatal("cached import commit survived authority revocation")
		}
	})
}
