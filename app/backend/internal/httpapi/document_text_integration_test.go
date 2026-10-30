package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/documenttext"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/search"
)

func TestNativeDocumentText(t *testing.T) {
	profile, fixtures := os.Getenv("CP_DOCUMENT_TEXT_TEST_PROFILE"), os.Getenv("CP_DOCUMENT_TEXT_TEST_FIXTURES")
	if profile == "" || fixtures == "" {
		t.Skip("explicit pinned native tools and synthetic fixtures required")
	}
	ctx := context.Background()
	engine, err := documenttext.Load(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	u := newUploadFixture(t)
	configureStorageTargets(u)
	f := u.f
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	bodies := map[string][]byte{}
	keys := map[string]string{}
	add := func(name, media, fixture string, base *storage.CommitResult) storage.CommitResult {
		t.Helper()
		b, e := os.ReadFile(filepath.Join(fixtures, fixture))
		if e != nil {
			t.Fatal(e)
		}
		v, store := commitContentFixture(t, u, name, media, b, base)
		for key := range store.stored {
			bodies[key] = append([]byte(nil), b...)
			keys[v.Version.ID] = key
		}
		return v
	}
	left := add("扫描交付.pdf", "application/pdf", "scan.pdf", nil)
	right := add("扫描交付.pdf", "application/pdf", "mixed.pdf", &left)
	png := add("验收图片.png", "image/png", "scan.png", nil)
	jpeg := add("验收附件.jpg", "image/jpeg", "scan.jpg", nil)
	many := add("超页数.pdf", "application/pdf", "too-many-pages.pdf", nil)
	encrypted := add("加密资料.pdf", "application/pdf", "encrypted.pdf", nil)
	bad := add("损坏资料.pdf", "application/pdf", "malformed.pdf", nil)
	late := add("撤回资料.pdf", "application/pdf", "scan.pdf", nil)
	tamper := add("摘要损坏.png", "image/png", "scan.png", nil)
	exec(`INSERT INTO file_text_derivations(org_id,file_id,version_id,extractor,source_sha256,state,error_code,indexed_at) VALUES($1,$2,$3,'text-v1',$4,'unsupported','FILE_TEXT_FORMAT_UNSUPPORTED',clock_timestamp())`, f.org, right.File.ID, right.Version.ID, right.Version.SHA256)
	var oldEvidence string
	if err = f.owner.QueryRow(ctx, `SELECT row_to_json(d)::text FROM file_text_derivations d WHERE version_id=$1 AND extractor='text-v1'`, right.Version.ID).Scan(&oldEvidence); err != nil {
		t.Fatal(err)
	}
	cloud := &downloadCloud{}
	cloud.readFn = func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
		body := append([]byte(nil), bodies[source.Key]...)
		if offset != 0 || length != int64(len(body)) {
			t.Fatal("exact source range lost")
		}
		if source.Key == keys[late.Version.ID] {
			exec("UPDATE files SET state='trashed',version=version+1 WHERE id=$1", late.File.ID)
		}
		if source.Key == keys[tamper.Version.ID] {
			body[len(body)-1] ^= 1
		}
		return objectstore.ReadResult{Body: &downloadTestBody{reader: bytes.NewReader(body), onClose: func() { cloud.closed.Add(1) }}, Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
	}
	index := storage.TextIndexService{DB: f.api.DB, Backends: map[string]*storage.AgentTextBackend{"oss": {Provider: "oss", Bucket: "fixture-bucket", Reader: cloud}}}
	q := jobqueue.Queue{DB: f.api.DB}
	get := func(v storage.CommitResult, token, query string, status int) storage.TextView {
		t.Helper()
		rec, res := f.request("GET", "/v1/files/"+v.File.ID+"/versions/"+v.Version.ID+"/text"+query, token, "", nil, nil)
		expect(t, rec, status, res)
		if status != 200 {
			return storage.TextView{}
		}
		return data[storage.TextView](t, res)
	}
	compare := func(token string, status int) storage.TextComparison {
		t.Helper()
		rec, res := f.request("GET", "/v1/files/"+right.File.ID+"/text-comparison?base_version_id="+left.Version.ID+"&target_version_id="+right.Version.ID+"&view=all", token, "", nil, nil)
		expect(t, rec, status, res)
		if status != 200 {
			return storage.TextComparison{}
		}
		return data[storage.TextComparison](t, res)
	}
	t.Run("explicit engine enablement preserves legacy evidence", func(t *testing.T) {
		if n, e := index.Schedule(ctx); e != nil || n != 0 {
			t.Fatal("native work scheduled without engine", n, e)
		}
		if v := get(right, f.aliceToken, "", 200); v.State != "pending" || v.Extractor != storage.DocumentTextExtractor {
			t.Fatal("legacy terminal row selected")
		}
		index.Native = engine
		if n, e := index.Schedule(ctx); e != nil || n != 9 {
			t.Fatal(n, e)
		}
		if n, e := index.Schedule(ctx); e != nil || n != 0 {
			t.Fatal("duplicate schedule", n, e)
		}
	})
	t.Run("engine provenance cannot be silently substituted", func(t *testing.T) {
		if _, e := f.owner.Exec(ctx, "UPDATE file_text_derivations SET engine_sha256=$1 WHERE version_id=$2 AND extractor='document-ocr-v1'", strings.Repeat("0", 64), right.Version.ID); e == nil {
			t.Fatal("engine basis replaced")
		}
		if _, e := f.owner.Exec(ctx, `UPDATE file_text_derivations SET state='ready',body='partial',text_sha256=encode(sha256(convert_to('partial','UTF8')),'hex'),character_count=7,indexed_at=clock_timestamp(),page_count=2,ocr_page_count=1 WHERE version_id=$1 AND extractor='document-ocr-v1'`, right.Version.ID); e == nil {
			t.Fatal("partial page result published")
		}
		j, e := q.Claim(ctx, []string{storage.TextJobKind}, storage.TextJobLane, "native-document-fixture", 2*time.Minute)
		if e != nil || j == nil {
			t.Fatal(e)
		}
		original := append([]byte(nil), j.Payload...)
		var payload map[string]any
		if e = json.Unmarshal(j.Payload, &payload); e != nil {
			t.Fatal(e)
		}
		payload["engine_sha256"] = strings.Repeat("0", 64)
		j.Payload, _ = json.Marshal(payload)
		if e = index.Process(ctx, *j); core.PublicError(e).Code != "FILE_TEXT_ENGINE_UNAVAILABLE" {
			t.Fatal("wrong engine ran", e)
		}
		j.Payload = original
		if e = index.Process(ctx, *j); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("all pages or explicit terminal failure with current source checks", func(t *testing.T) {
		for {
			j, e := q.Claim(ctx, []string{storage.TextJobKind}, storage.TextJobLane, "native-document-fixture", 2*time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			if j == nil {
				break
			}
			if e = index.Process(ctx, *j); e != nil {
				t.Fatal(e)
			}
		}
		for _, v := range []storage.CommitResult{left, right, png, jpeg} {
			view := get(v, f.aliceToken, "", 200)
			if view.State != "ready" || view.Extractor != storage.DocumentTextExtractor || view.SourceSHA256 != v.Version.SHA256 || view.NativeEvidence == nil || view.NativeEvidence.EngineSHA256 != engine.Fingerprint() || view.NativeEvidence.PageCount != view.NativeEvidence.OCRPageCount || !strings.Contains(view.Text, "123.45") {
				t.Fatalf("incomplete native evidence: %+v", view)
			}
		}
		for _, tc := range []struct {
			v           storage.CommitResult
			state, code string
		}{{many, "failed", "FILE_TEXT_PAGE_LIMIT"}, {encrypted, "unsupported", "FILE_TEXT_ENCRYPTED_UNSUPPORTED"}, {bad, "failed", "FILE_TEXT_NATIVE_FAILED"}, {tamper, "failed", "FILE_TEXT_CONTENT_INVALID"}} {
			view := get(tc.v, f.aliceToken, "", 200)
			if view.State != tc.state || view.ErrorCode == nil || *view.ErrorCode != tc.code || view.Text != "" || view.NativeEvidence != nil {
				t.Fatalf("partial or wrong failure: %+v", view)
			}
		}
		var state string
		var body *string
		if e := f.owner.QueryRow(ctx, `SELECT state,body FROM file_text_derivations WHERE version_id=$1 AND extractor='document-ocr-v1'`, late.Version.ID).Scan(&state, &body); e != nil || state != "blocked" || body != nil {
			t.Fatal("late source withdrawal ignored", e, state)
		}
		var unchanged string
		if e := f.owner.QueryRow(ctx, `SELECT row_to_json(d)::text FROM file_text_derivations d WHERE version_id=$1 AND extractor='text-v1'`, right.Version.ID).Scan(&unchanged); e != nil || unchanged != oldEvidence {
			t.Fatal("legacy evidence rewritten", e)
		}
	})
	t.Run("exact text pagination search and comparison retain OCR basis", func(t *testing.T) {
		offset, all := 0, ""
		first := get(right, f.aliceToken, "", 200)
		for {
			p := get(right, f.aliceToken, "?limit=37&offset="+strconv.Itoa(offset), 200)
			all += p.Text
			if p.NativeEvidence == nil || p.NativeEvidence.PageCount != 2 {
				t.Fatal("page evidence lost")
			}
			if p.NextOffset == nil {
				break
			}
			offset = *p.NextOffset
		}
		if core.Digest([]byte(all)) != *first.TextSHA256 || !strings.Contains(all, "CP128 DELIVERY READY") {
			t.Fatal("incomplete body")
		}
		rec, res := f.request("GET", "/v1/search?q="+url.QueryEscape("INVOICE")+"&type=file&limit=20", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		hits := data[core.Page[search.Hit]](t, res)
		found := false
		for _, h := range hits.Items {
			if h.Resource.ID == right.File.ID {
				found = true
				if h.Resource.VersionID == nil || *h.Resource.VersionID != right.Version.ID || h.TextExtractor == nil || *h.TextExtractor != storage.DocumentTextExtractor || !strings.Contains(h.Snippet, "INVOICE") {
					t.Fatal("search lost native basis")
				}
			}
		}
		if !found {
			t.Fatal("OCR body not searchable")
		}
		v := compare(f.aliceToken, 200)
		if v.Extractor != storage.DocumentTextExtractor || v.Base.NativeEvidence == nil || v.Target.NativeEvidence == nil || v.Base.NativeEvidence.PageCount != 1 || v.Target.NativeEvidence.PageCount != 2 || v.TextEqual {
			t.Fatal("wrong comparison basis")
		}
	})
	var c agent.Conversation
	t.Run("secretary reads fixed derived text without object or model credentials", func(t *testing.T) {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, f.alice)
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: right.File.ProjectID, Title: "PDF图片资料核对", MemberIDs: []string{}}, nil)
		expect(t, rec, 201, res)
		c = data[agent.Conversation](t, res)
		before := cloud.reads.Load()
		f.api.AgentFileReaders = nil
		f.handler = f.api.Handler()
		rec, res = f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/file/"+right.File.ID+"?context_security_version="+strconv.FormatInt(c.ContextSecurityVersion, 10)+"&version_id="+right.Version.ID+"&max_chars=10000", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		v := data[agent.ContextView](t, res)
		if v.TextIndex == nil || v.TextIndex.NativeEvidence == nil || v.TextIndex.NativeEvidence.PageCount != 2 || !strings.Contains(v.Text, "OCR识别") || cloud.reads.Load() != before {
			t.Fatal("secretary lost native evidence or fetched originals")
		}
	})
	t.Run("current classification and scan still govern derived visibility", func(t *testing.T) {
		exec("UPDATE files SET classification='confidential',version=version+1 WHERE id=$1", right.File.ID)
		get(right, f.bobToken, "", 404)
		compare(f.bobToken, 404)
		exec("UPDATE files SET classification='project_restricted',version=version+1 WHERE id=$1", right.File.ID)
		exec("UPDATE file_versions SET scan_state='quarantined',availability='quarantined' WHERE id=$1", right.Version.ID)
		get(right, f.aliceToken, "", 409)
		compare(f.aliceToken, 409)
		exec("UPDATE file_versions SET scan_state='clean',availability='available' WHERE id=$1", right.Version.ID)
	})
	if os.Getenv("CP_BROWSER_DOCUMENT_TEXT_FIXTURE") == "1" && !t.Failed() {
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'document_text_mfa','Actionless MFA fixture',true)", role, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, role)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"document_text_fixture": true, "file_id": right.File.ID, "base_version_id": left.Version.ID, "target_version_id": right.Version.ID, "image_file_id": png.File.ID, "image_version_id": png.Version.ID, "project_id": *right.File.ProjectID, "conversation_id": c.ID})
	}
}
