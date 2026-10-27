package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/knowledge"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/search"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFileTextSearch(t *testing.T) {
	u := newUploadFixture(t)
	configureStorageTargets(u)
	f := u.f
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	bodies := map[string][]byte{}
	cloud := &downloadCloud{}
	mode := ""
	var hook func()
	add := func(name, typ string, body []byte, base *storage.CommitResult) storage.CommitResult {
		v, store := commitContentFixture(t, u, name, typ, body, base)
		for key := range store.stored {
			bodies[key] = append([]byte(nil), body...)
		}
		return v
	}
	old := add("设计说明.md", "text/markdown", []byte("历史版本专属词，香港旧资料"), nil)
	current := add("设计说明.md", "text/markdown", []byte("当前交付约定：香港正文🙂。预算比例100%_固定\\。\n"+strings.Repeat("继续核对资料🙂\n", 900)), &old)
	var docxBytes bytes.Buffer
	z := zip.NewWriter(&docxBytes)
	for name, content := range map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>文档提取验证词🙂</w:t></w:r></w:p></w:body></w:document>`} {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(content)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	docx := add("设计说明.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", docxBytes.Bytes(), nil)
	unsupported := add("图纸.pdf", "application/pdf", []byte("%PDF fixture"), nil)
	invalid := add("无效编码.txt", "text/plain", []byte{0xff, 0xfe}, nil)
	cloud.readFn = func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
		if hook != nil {
			fn := hook
			hook = nil
			fn()
		}
		b := append([]byte(nil), bodies[source.Key]...)
		if mode == "corrupt" {
			b[len(b)-1] ^= 1
		}
		if mode == "transport" {
			return objectstore.ReadResult{}, &objectstore.Error{Code: "FIXTURE_UNAVAILABLE"}
		}
		if offset != 0 || length != int64(len(b)) {
			t.Error("unbounded or nonexact transfer")
		}
		return objectstore.ReadResult{Body: &downloadTestBody{reader: bytes.NewReader(b), onClose: func() { cloud.closed.Add(1) }}, Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
	}
	service := &storage.TextIndexService{DB: f.api.DB, Backends: map[string]*storage.AgentTextBackend{"oss": {Provider: "oss", Bucket: "fixture-bucket", Reader: cloud}}}
	q := jobqueue.Queue{DB: f.api.DB}
	run := func() *jobqueue.Job {
		t.Helper()
		j, e := q.Claim(ctx, []string{storage.TextJobKind}, storage.TextJobLane, "text-fixture", time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		if j != nil {
			if e = service.Process(ctx, *j); e != nil {
				t.Fatal(e)
			}
		}
		return j
	}
	getText := func(v storage.CommitResult, token, query string, status int) storage.TextView {
		rec, res := f.request("GET", "/v1/files/"+v.File.ID+"/versions/"+v.Version.ID+"/text"+query, token, "", nil, nil)
		expect(t, rec, status, res)
		if status != 200 {
			return storage.TextView{}
		}
		return data[storage.TextView](t, res)
	}
	find := func(token, query string, status int) core.Page[search.Hit] {
		rec, res := f.request("GET", "/v1/search?"+query, token, "", nil, nil)
		expect(t, rec, status, res)
		if status != 200 {
			return core.Page[search.Hit]{}
		}
		return data[core.Page[search.Hit]](t, res)
	}
	t.Run("pending then automatic fixed version extraction", func(t *testing.T) {
		actor, e := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if e != nil {
			t.Fatal(e)
		}
		_, e = (&storage.Service{DB: f.api.DB}).Text(ctx, actor, current.File.ID, current.Version.ID, 0, 4000)
		if e != nil {
			t.Fatal("direct text", e)
		}
		if got := getText(current, f.aliceToken, "", 200); got.State != "pending" || got.Text != "" {
			t.Fatal("unexpected pending body")
		}
		n, e := service.Schedule(ctx)
		if e != nil || n != 5 {
			t.Fatalf("schedule %d %v", n, e)
		}
		if n, e = service.Schedule(ctx); e != nil || n != 0 {
			t.Fatal("schedule duplicated")
		}
		for run() != nil {
		}
		got := getText(current, f.aliceToken, "?limit=30", 200)
		if got.State != "ready" || got.VersionID != current.Version.ID || got.SourceSHA256 != current.Version.SHA256 || got.NextOffset == nil || len([]rune(got.Text)) != 30 {
			t.Fatalf("fixed text %+v", got)
		}
		if getText(docx, f.aliceToken, "", 200).Text != "文档提取验证词🙂" {
			t.Fatal("DOCX index did not use extracted text")
		}
		if getText(old, f.aliceToken, "", 200).Text != "历史版本专属词，香港旧资料" {
			t.Fatal("history followed current")
		}
		if getText(unsupported, f.aliceToken, "", 200).State != "unsupported" || getText(invalid, f.aliceToken, "", 200).Text != "" {
			t.Fatal("unsupported content leaked")
		}
		all := ""
		offset := 0
		for {
			v := getText(current, f.aliceToken, "?limit=4000&offset="+strconv.Itoa(offset), 200)
			all += v.Text
			if v.NextOffset == nil {
				break
			}
			offset = *v.NextOffset
		}
		if core.Digest([]byte(all)) != *got.TextSHA256 {
			t.Fatal("Unicode chunks or full text digest differ")
		}
		getText(current, f.aliceToken, "?limit=8001", 422)
		getText(current, f.aliceToken, "?offset=999999", 422)
	})
	t.Run("body matching literal escaping and current head only", func(t *testing.T) {
		hits := find(f.aliceToken, "q="+url.QueryEscape("香港正文🙂")+"&type=file", 200)
		if len(hits.Items) != 1 || hits.Items[0].Resource.ID != current.File.ID || hits.Items[0].Resource.VersionID == nil || *hits.Items[0].Resource.VersionID != current.Version.ID || !strings.Contains(hits.Items[0].Snippet, "香港正文🙂") {
			t.Fatalf("body search %+v", hits)
		}
		if len(find(f.aliceToken, "q="+url.QueryEscape("历史版本专属词"), 200).Items) != 0 {
			t.Fatal("old body became current search result")
		}
		if len(find(f.aliceToken, "q="+url.QueryEscape("100%_固定\\"), 200).Items) != 1 {
			t.Fatal("literal pattern not escaped")
		}
		find(f.aliceToken, "q=x&type=contract", 422)
		find(f.aliceToken, "q=", 422)
		find(f.aliceToken, "q=x&limit=51", 422)
	})
	t.Run("names and ACL filter before page limit", func(t *testing.T) {
		hits := find(f.bobToken, "q="+url.QueryEscape("说明")+"&limit=1", 200)
		if len(hits.Items) != 1 {
			t.Fatal("member file missing")
		}
		denied := find(f.memberToken, "q="+url.QueryEscape("香港正文🙂"), 200)
		if len(denied.Items) != 0 {
			t.Fatal("unauthorized body leaked")
		}
		getText(current, f.memberToken, "", 404)
		exec("UPDATE files SET classification='confidential',version=version+1 WHERE id=$1", current.File.ID)
		if len(find(f.bobToken, "q="+url.QueryEscape("香港正文🙂"), 200).Items) != 0 {
			t.Fatal("reclassification waited for index")
		}
		getText(current, f.bobToken, "", 404)
		exec("UPDATE files SET classification='project_restricted',version=version+1 WHERE id=$1", current.File.ID)
	})
	t.Run("live lifecycle blocks cached plaintext", func(t *testing.T) {
		exec("UPDATE files SET state='trashed',trashed_at=clock_timestamp(),version=version+1 WHERE id=$1", current.File.ID)
		if len(find(f.aliceToken, "q="+url.QueryEscape("香港正文🙂"), 200).Items) != 0 {
			t.Fatal("trash visible in search")
		}
		getText(current, f.aliceToken, "", 409)
		exec("UPDATE files SET state='active',trashed_at=NULL,version=version+1 WHERE id=$1", current.File.ID)
		exec("UPDATE file_versions SET scan_state='quarantined',availability='quarantined' WHERE id=$1", current.Version.ID)
		if len(find(f.aliceToken, "q="+url.QueryEscape("香港正文🙂"), 200).Items) != 0 {
			t.Fatal("quarantined text visible")
		}
		getText(current, f.aliceToken, "", 409)
		exec("UPDATE file_versions SET scan_state='clean',availability='available' WHERE id=$1", current.Version.ID)
	})
	t.Run("selected knowledge revision and cursor binding", func(t *testing.T) {
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'text_reader','Read only knowledge fixture',false)", role, f.org)
		exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'post.read')", f.org, role)
		exec("DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.bob, role)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		f.aliceToken = f.login("alice", "android").AccessToken
		f.bobToken = f.login("bob", "android").AccessToken
		f.memberToken = f.login("member", "android").AccessToken

		project := *current.File.ProjectID
		rec, res := f.request("POST", "/v1/posts", f.aliceToken, core.NewID(), map[string]any{"project_id": project, "type": "article", "title": "交接记录", "body": "可公开的版本搜索词", "classification": "project_restricted", "tags": []string{"检索标签"}, "source_refs": []any{}}, nil)
		expect(t, rec, 201, res)
		post := data[knowledge.Post](t, res)
		rec, res = f.request("POST", "/v1/posts/"+post.ID+"/publish", f.aliceToken, core.NewID(), map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "action": "publish", "reason": "发布供成员查阅"}, nil)
		expect(t, rec, 200, res)
		post = data[knowledge.Post](t, res)
		rec, res = f.request("PATCH", "/v1/posts/"+post.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "body": "仅编辑者可见的草稿关键词"}, nil)
		expect(t, rec, 200, res)
		if len(find(f.bobToken, "q="+url.QueryEscape("仅编辑者可见"), 200).Items) != 0 || len(find(f.bobToken, "q="+url.QueryEscape("可公开的版本搜索词"), 200).Items) != 1 {
			t.Fatal("reader searched unpublished revision")
		}
		if len(find(f.aliceToken, "q="+url.QueryEscape("仅编辑者可见"), 200).Items) != 1 {
			t.Fatal("editor current revision missing")
		}
		page := find(f.aliceToken, "q="+url.QueryEscape("说明")+"&limit=1", 200)
		if page.NextCursor == nil {
			t.Fatal("second filename must create a real cursor")
		}
		next := find(f.aliceToken, "q="+url.QueryEscape("说明")+"&limit=1&cursor="+url.QueryEscape(*page.NextCursor), 200)
		if len(next.Items) != 1 || next.Items[0].Resource.ID == page.Items[0].Resource.ID {
			t.Fatal("keyset duplicated or omitted result")
		}
		find(f.aliceToken, "q=different&limit=1&cursor="+url.QueryEscape(*page.NextCursor), 400)
		find(f.memberToken, "q="+url.QueryEscape("说明")+"&limit=1&cursor="+url.QueryEscape(*page.NextCursor), 400)
	})
	t.Run("corrupt bytes never produce a ready index", func(t *testing.T) {
		v := add("坏副本.txt", "text/plain", []byte("不得索引的错误副本"), nil)
		mode = "corrupt"
		defer func() { mode = "" }()
		if _, e := service.Schedule(ctx); e != nil {
			t.Fatal(e)
		}
		for run() != nil {
		}
		got := getText(v, f.aliceToken, "", 200)
		if got.State != "failed" || got.Text != "" || got.ErrorCode == nil || *got.ErrorCode != "FILE_TEXT_CONTENT_INVALID" {
			t.Fatal("corrupt bytes indexed")
		}
	})
	t.Run("source changed during I O fails closed", func(t *testing.T) {
		v := add("来源变化.txt", "text/plain", []byte("此内容不应成为检索结果"), nil)
		hook = func() {
			exec("UPDATE files SET state='trashed',trashed_at=clock_timestamp(),version=version+1 WHERE id=$1", v.File.ID)
		}
		if _, e := service.Schedule(ctx); e != nil {
			t.Fatal(e)
		}
		for run() != nil {
		}
		var state string
		if e := f.owner.QueryRow(ctx, "SELECT state FROM file_text_derivations WHERE version_id=$1", v.Version.ID).Scan(&state); e != nil || state != "blocked" {
			t.Fatal("stale extraction published", state, e)
		}
	})
	t.Run("retry and stale fencing", func(t *testing.T) {
		v := add("暂时离线.txt", "text/plain", []byte("传输恢复后正文🙂"), nil)
		if _, e := service.Schedule(ctx); e != nil {
			t.Fatal(e)
		}
		j, e := q.Claim(ctx, []string{storage.TextJobKind}, storage.TextJobLane, "text-fixture", time.Minute)
		if e != nil || j == nil {
			t.Fatal("claim", e)
		}
		mode = "transport"
		e = service.Process(ctx, *j)
		mode = ""
		if e == nil {
			t.Fatal("transport failure ignored")
		}
		if e = service.Fail(ctx, *j, e); e != nil {
			t.Fatal(e)
		}
		if getText(v, f.aliceToken, "", 200).State != "pending" {
			t.Fatal("transient failure prematurely final")
		}
		exec("UPDATE jobs SET available_at=clock_timestamp()-interval '1 second' WHERE id=$1", j.ID)
		next, e := q.Claim(ctx, []string{storage.TextJobKind}, storage.TextJobLane, "text-fixture-2", time.Minute)
		if e != nil || next == nil || next.ID != j.ID || next.FencingToken <= j.FencingToken {
			t.Fatal("retry fence", e)
		}
		if e = service.Process(ctx, *j); e == nil || core.PublicError(e).Code != "JOB_LEASE_LOST" {
			t.Fatal("stale worker published", e)
		}
		if e = service.Process(ctx, *next); e != nil {
			t.Fatal(e)
		}
		if getText(v, f.aliceToken, "", 200).State != "ready" {
			t.Fatal("retry never completed")
		}
	})
	t.Run("terminal derivation is immutable", func(t *testing.T) {
		if _, e := f.owner.Exec(ctx, "UPDATE file_text_derivations SET body='forged' WHERE version_id=$1", current.Version.ID); e == nil {
			t.Fatal("derived evidence changed")
		}
	})
	if os.Getenv("CP_BROWSER_FILE_SEARCH_FIXTURE") == "1" && !t.Failed() {
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'file_search_mfa','Actionless MFA fixture',true)", role, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, role)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"file_search_fixture": true, "file_id": current.File.ID, "version_id": current.Version.ID, "docx_file_id": docx.File.ID, "project_id": *current.File.ProjectID, "old_version_id": old.Version.ID})
	}

}
