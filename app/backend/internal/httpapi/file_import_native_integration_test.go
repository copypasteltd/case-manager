package httpapi

import (
	"bytes"
	"context"

	"copypaste.hk/platform/internal/fileimport"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/importspec"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/uploadworker"
	"encoding/json"

	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFileImportNativeRoundTrip(t *testing.T) {
	f, entry, namespace := importFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	raw := []byte("real synthetic source bytes\n")
	entry.SourcePath = "native-source.txt"
	entry.SourceKey = importspec.SourceKey(namespace, entry.SourcePath)
	entry.Name = "原生迁移资料.txt"
	entry.MediaType = "text/plain"
	if err := os.WriteFile(filepath.Join(root, entry.SourcePath), raw, 0600); err != nil {
		t.Fatal(err)
	}
	var inv, manifest bytes.Buffer
	if _, err := fileimport.Inventory(ctx, root, namespace, &inv); err != nil {
		t.Fatal(err)
	}
	mapping := fileimport.Mapping{SourcePath: entry.SourcePath, SourceOwner: entry.SourceOwner, MappedOwnerID: entry.MappedOwnerID, FolderID: entry.FolderID, FolderVersion: entry.FolderVersion, ProjectID: entry.ProjectID, Classification: entry.Classification, Name: entry.Name, MediaType: entry.MediaType}
	mapped, _ := json.Marshal(mapping)
	spool, err := os.CreateTemp(t.TempDir(), "spool")
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	header, err := fileimport.BuildPlan(&inv, bytes.NewReader(append(mapped, '\n')), spool, &manifest, "actual native fixture transfer")
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(t.TempDir(), "plan.jsonl")
	os.WriteFile(planPath, manifest.Bytes(), 0600)
	draft := &contentFixtureCloud{promotionCloud: &promotionCloud{integrityCloud: &integrityCloud{uploadCloudFixture: f.cloud}}, stored: map[string]map[int][]byte{}}
	primary := &contentFixtureCloud{promotionCloud: newPromotionCloud(f), stored: map[string]map[int][]byte{}}
	secondary := &contentFixtureCloud{promotionCloud: newPromotionCloud(f), stored: map[string]map[int][]byte{}}
	puts := 0
	lost := true
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.Header.Get("Authorization") != "" {
			t.Error("invalid isolated object request")
			w.WriteHeader(400)
			return
		}
		id := r.URL.Query().Get("upload")
		n, _ := strconv.Atoi(r.URL.Query().Get("part"))
		f.cloud.mu.Lock()
		m, ok := f.cloud.uploads[id]
		f.cloud.mu.Unlock()
		if !ok {
			w.WriteHeader(404)
			return
		}
		part, e := draft.WritePart(r.Context(), m, n, r.Body, r.ContentLength)
		if e != nil {
			t.Error(e)
			w.WriteHeader(400)
			return
		}
		puts++
		if lost {
			lost = false
			w.WriteHeader(503)
			return
		} // Provider accepted bytes; client did not receive success/ETag.
		w.Header().Set("ETag", part.ETag)
		w.WriteHeader(200)
	}))
	defer cloud.Close()
	f.service.Backend.Signer = uploadSignerCall(func(ctx context.Context, m objectstore.Multipart, n int, size int64, ttl time.Duration) (objectstore.Signed, error) {
		return objectstore.Signed{URL: cloud.URL + "/part?upload=" + url.QueryEscape(m.UploadID) + "&part=" + strconv.Itoa(n), Method: "PUT", ExpiresAt: time.Now().Add(ttl)}, nil
	})
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	hosts := []*uploadworker.Runner{
		{Service: f.service, Store: draft, Owner: "native-control", Mode: "control", Logger: logger},
		{Service: f.service, Store: draft, Owner: "native-verify", Mode: "verify-small", Scanner: cleanIntegrityScanner(), Logger: logger},
		{Service: f.service, Store: draft, TargetStore: primary, Owner: "native-promote", Mode: "promote-small", Logger: logger},
	}
	handler := f.f.handler
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/file-import-entries/") {
			for _, host := range hosts {
				if _, e := host.RunOne(r.Context()); e != nil {
					t.Error("native worker", e)
				}
			}
		}
		handler.ServeHTTP(w, r)
	}))
	defer api.Close()
	client, err := fileimport.NewClient(api.URL+"/v1", f.f.aliceToken, []string{cloud.URL}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	runner := fileimport.Runner{Client: client, WaitLimit: time.Minute, PollInterval: time.Millisecond, ReviewedMappings: true, SourcesReadOnly: true}
	var log bytes.Buffer
	batch, err := runner.Run(ctx, root, planPath, header.PlanSHA256, &log)
	if err == nil || !strings.Contains(err.Error(), "RECONCILE") || puts != 1 {
		t.Fatal("uncertain accepted PUT did not stop for reconciliation", err, puts)
	}
	log.Reset()
	again, err := runner.Run(ctx, root, planPath, header.PlanSHA256, &log)
	if err != nil || again != batch || puts != 1 {
		t.Fatal("resume did not reconcile actual accepted cloud part", err, puts)
	}
	var count int
	f.f.owner.QueryRow(ctx, "SELECT count(*) FROM files WHERE org_id=$1", f.f.org).Scan(&count)
	if count != 1 {
		t.Fatal("resume duplicated logical file", count)
	}
	var report bytes.Buffer
	before, err := runner.Report(ctx, batch, &report)
	if err != nil || before.ObservedContentVerified || before.Pending != 1 || before.CutoverApproved {
		t.Fatal("primary transfer falsely counted as dual backup", err, before)
	}
	f.service.ReplicaTarget = &storage.UploadBackend{Provider: "r2", Bucket: "fixture-r2-bucket"}
	replica := &uploadworker.Runner{Service: f.service, Store: primary, TargetStore: secondary, Owner: "native-replica", Mode: "replica-small", Logger: logger}
	if worked, e := replica.RunOne(ctx); e != nil || !worked {
		t.Fatal("real synthetic replica read/write verification", e)
	}
	report.Reset()
	after, err := runner.Report(ctx, batch, &report)
	if err != nil || !after.ObservedContentVerified || after.FullyVerified != 1 || after.CutoverApproved {
		t.Fatal("verified synthetic replica receipt incomplete", err, after)
	}
	log.Reset()
	if _, err = runner.Run(ctx, root, planPath, header.PlanSHA256, &log); err != nil || puts != 1 {
		t.Fatal("committed resume performed another PUT", err)
	}
	f.f.owner.QueryRow(ctx, "SELECT count(*) FROM upload_sessions WHERE org_id=$1", f.f.org).Scan(&count)
	if count != 1 {
		t.Fatal("resume created duplicate session", count)
	}
	if err = os.WriteFile(filepath.Join(root, entry.SourcePath), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	log.Reset()
	if _, err = runner.Run(ctx, root, planPath, header.PlanSHA256, &log); err == nil || puts != 1 {
		t.Fatal("changed source was silently reused", err)
	}
	var current storage.ImportBatch
	if err = client.Request(ctx, "GET", "/file-imports/"+batch, "", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.PlanSHA256 != header.PlanSHA256 {
		t.Fatal("plan changed")
	}
	if path := os.Getenv("CP_FILE_IMPORT_NATIVE_EVIDENCE"); path != "" {
		// This opt-in evidence contains only this test's synthetic data and no tokens.
		artifact := map[string]any{"kind": "native_http_round_trip", "synthetic": true, "real_cloud": false, "large_file_transport": false, "source_bytes": len(raw), "put_requests": puts, "accepted_put_without_receipt_reconciled": true, "logical_files": 1, "upload_sessions": 1, "before_replica": before, "after_replica": after, "source_mutation_rejected": true, "cutover_approved": false}
		body, _ := json.MarshalIndent(artifact, "", "  ")
		if e := os.WriteFile(path, append(body, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}

}
