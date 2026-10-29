package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
)

func TestFileTextComparison(t *testing.T) {
	u := newUploadFixture(t)
	configureStorageTargets(u)
	f, ctx := u.f, context.Background()
	current := t
	exec := func(query string, args ...any) {
		current.Helper()
		if _, err := f.owner.Exec(ctx, query, args...); err != nil {
			current.Fatal(err)
		}
	}
	run := func(name string, fn func()) { t.Run(name, func(next *testing.T) { current = next; fn() }); current = t }
	bodies := map[string][]byte{}
	add := func(name, media string, body []byte, base *storage.CommitResult) storage.CommitResult {
		v, objects := commitContentFixture(t, u, name, media, body, base)
		for key := range objects.stored {
			bodies[key] = append([]byte(nil), body...)
		}
		return v
	}
	before := "\ufeff版本比较🙂\r\n" + strings.Repeat("共同正文\r\n", 85) + "旧条款\r\n末段"
	after := "版本比较🙂\n" + strings.Repeat("共同正文\n", 85) + "新条款 <script>window.__comparisonExecuted=true</script>\n末段\n"
	plainBefore := strings.ReplaceAll(strings.TrimPrefix(before, "\ufeff"), "\r\n", "\n")
	base := add("版本核对.md", "text/markdown", []byte(before), nil)
	target := add("版本核对.md", "text/markdown", []byte(after), &base)
	normalized := add("版本核对.md", "text/markdown", []byte(plainBefore), &target)
	foreign := add("其他资料.txt", "text/plain", []byte("不能混入其他文件的正文"), nil)
	pdf := add("不支持提取.pdf", "application/pdf", []byte("%PDF original remains unchanged"), nil)
	cloud := &downloadCloud{}
	cloud.readFn = func(_ context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
		body := bodies[source.Key]
		if offset != 0 || length != int64(len(body)) {
			t.Fatal("index did not read its bounded exact original")
		}
		return objectstore.ReadResult{Body: &downloadTestBody{reader: bytes.NewReader(body), onClose: func() { cloud.closed.Add(1) }}, Size: length, ETag: source.ETag, VersionID: source.VersionID}, nil
	}
	index := &storage.TextIndexService{DB: f.api.DB, Backends: map[string]*storage.AgentTextBackend{"oss": {Provider: "oss", Bucket: "fixture-bucket", Reader: cloud}}}
	queue := jobqueue.Queue{DB: f.api.DB}
	indexAll := func() {
		current.Helper()
		if _, err := index.Schedule(ctx); err != nil {
			current.Fatal(err)
		}
		for {
			j, err := queue.Claim(ctx, []string{storage.TextJobKind}, storage.TextJobLane, "comparison-index", time.Minute)
			if err != nil {
				current.Fatal(err)
			}
			if j == nil {
				return
			}
			if err = index.Process(ctx, *j); err != nil {
				current.Fatal(err)
			}
		}
	}
	get := func(file, left, right, token, extra string, status int) storage.TextComparison {
		current.Helper()
		path := "/v1/files/" + file + "/text-comparison?base_version_id=" + left + "&target_version_id=" + right + extra
		rec, res := f.request("GET", path, token, "", nil, nil)
		expect(current, rec, status, res)
		if status != 200 {
			if len(res.Data) > 0 && string(res.Data) != "null" {
				current.Fatal("denied comparison returned partial content")
			}
			return storage.TextComparison{}
		}
		return data[storage.TextComparison](current, res)
	}
	joined := func(rows []storage.TextComparisonRow) (string, string) {
		current.Helper()
		var a, b strings.Builder
		x, y := 1, 1
		for _, row := range rows {
			if row.Kind == "skip" || row.Count != 1 {
				current.Fatal("full view hid content")
			}
			text := row.Text
			if row.Newline {
				text += "\n"
			}
			if row.Kind != "insert" {
				if row.BaseLine == nil || *row.BaseLine != x {
					current.Fatal("base coordinate discontinuity")
				}
				x++
				a.WriteString(text)
			}
			if row.Kind != "delete" {
				if row.TargetLine == nil || *row.TargetLine != y {
					current.Fatal("target coordinate discontinuity")
				}
				y++
				b.WriteString(text)
			}
		}
		return a.String(), b.String()
	}
	run("pending is not an empty successful comparison", func() {
		get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "", 409)
	})
	indexAll()
	run("full actual indexed bodies paginate with exact source and no current-head substitution", func() {
		var rows []storage.TextComparisonRow
		offset, identity := 0, ""
		for {
			extra := "&view=all&limit=17&offset=" + strconv.Itoa(offset)
			if identity != "" {
				extra += "&comparison_sha256=" + identity
			}
			page := get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, extra, 200)
			if page.Base.VersionID != base.Version.ID || page.Target.VersionID != target.Version.ID || page.Target.VersionID == normalized.Version.ID || page.Base.SourceSHA256 != base.Version.SHA256 || !page.Complete || page.TextEqual || page.SourceEqual || page.Offset != offset {
				current.Fatal("comparison evidence changed")
			}
			if identity != "" && page.ComparisonSHA256 != identity {
				current.Fatal("page identity changed")
			}
			identity = page.ComparisonSHA256
			rows = append(rows, page.Rows...)
			if page.NextOffset == nil {
				break
			}
			if *page.NextOffset != len(rows) {
				current.Fatal("page skipped rows")
			}
			offset = *page.NextOffset
		}
		a, b := joined(rows)
		if a != plainBefore || b != after {
			current.Fatal("actual full text or final newline was lost")
		}
	})
	run("context omits only explicitly counted equal lines and direction can reverse", func() {
		v := get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "", 200)
		if len(v.Rows) >= 60 || v.Rows[0].Kind != "skip" || v.Rows[0].Count < 70 || v.AddedLines != 2 || v.RemovedLines != 2 {
			current.Fatal("context or newline accounting", v)
		}
		reverse := get(base.File.ID, target.Version.ID, base.Version.ID, f.aliceToken, "", 200)
		if reverse.AddedLines != v.RemovedLines || reverse.RemovedLines != v.AddedLines || reverse.ComparisonSHA256 == v.ComparisonSHA256 {
			current.Fatal("direction not pinned")
		}
	})
	run("normalized equality distinguishes original bytes and exact same-version reads", func() {
		v := get(base.File.ID, base.Version.ID, normalized.Version.ID, f.aliceToken, "", 200)
		if !v.TextEqual || v.SourceEqual || v.AddedLines != 0 || v.RemovedLines != 0 || len(v.Rows) != 1 || v.Rows[0].Kind != "skip" {
			current.Fatal("normalized text falsely claims equal originals")
		}
		v = get(base.File.ID, base.Version.ID, base.Version.ID, f.aliceToken, "", 200)
		if !v.TextEqual || !v.SourceEqual {
			current.Fatal("same exact version differs")
		}
	})
	run("read path creates no storage upload plan or job mutations", func() {
		counts := func() string {
			var v string
			if err := f.owner.QueryRow(ctx, "SELECT json_build_array((SELECT count(*) FROM file_versions),(SELECT count(*) FROM upload_sessions),(SELECT count(*) FROM jobs),(SELECT count(*) FROM agent_plans),(SELECT count(*) FROM outbox_events))::text").Scan(&v); err != nil {
				current.Fatal(err)
			}
			return v
		}
		before := counts()
		get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "", 200)
		if counts() != before {
			current.Fatal("comparison mutated a business resource")
		}
	})
	run("invalid scope cross-file versions and unsupported derivations return no content", func() {
		get(base.File.ID, base.Version.ID, foreign.Version.ID, f.aliceToken, "", 404)
		get(base.File.ID, base.Version.ID, target.Version.ID, f.memberToken, "", 404)
		get(pdf.File.ID, pdf.Version.ID, pdf.Version.ID, f.aliceToken, "", 422)
	})
	run("query shape and page identity are strict", func() {
		for _, extra := range []string{"&limit=101", "&limit=00", "&offset=-1", "&offset=1", "&view=unknown", "&limit=1&limit=2", "&body=forged", "&base_version_id=" + base.Version.ID, "&comparison_sha256=bad"} {
			get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, extra, 422)
		}
		get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "&comparison_sha256="+strings.Repeat("0", 64), 409)
		v := get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "", 200)
		get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "&view=all&comparison_sha256="+v.ComparisonSHA256, 409)
	})
	run("late lifecycle scan and classification changes revoke indexed access", func() {
		exec("UPDATE files SET state='trashed',trashed_at=clock_timestamp(),version=version+1 WHERE id=$1", base.File.ID)
		get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "", 409)
		exec("UPDATE files SET state='active',trashed_at=NULL,version=version+1 WHERE id=$1", base.File.ID)
		exec("UPDATE file_versions SET scan_state='quarantined',availability='quarantined' WHERE id=$1", target.Version.ID)
		get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "", 409)
		exec("UPDATE file_versions SET scan_state='clean',availability='available' WHERE id=$1", target.Version.ID)
		exec("UPDATE files SET classification='confidential',version=version+1 WHERE id=$1", base.File.ID)
		get(base.File.ID, base.Version.ID, target.Version.ID, f.bobToken, "", 404)
		exec("UPDATE files SET classification='project_restricted',version=version+1 WHERE id=$1", base.File.ID)
	})
	run("file revision changes invalidate a pinned page", func() {
		v := get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "&view=all&limit=1", 200)
		exec("UPDATE files SET version=version+1 WHERE id=$1", base.File.ID)
		get(base.File.ID, base.Version.ID, target.Version.ID, f.aliceToken, "&view=all&offset=1&comparison_sha256="+v.ComparisonSHA256, 409)
	})
	run("a separately revoked human session cannot reuse a page", func() {
		token := f.login("alice", "android").AccessToken
		v := get(base.File.ID, base.Version.ID, target.Version.ID, token, "&view=all&limit=1", 200)
		actor, err := f.api.Identity.Authenticate(ctx, token)
		if err != nil {
			current.Fatal(err)
		}
		exec("UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=$1", actor.SessionID)
		get(base.File.ID, base.Version.ID, target.Version.ID, token, "&view=all&offset=1&comparison_sha256="+v.ComparisonSHA256, 401)
	})
	run("CSV JSON and DOCX use their actual complete extractor evidence", func() {
		for _, sample := range []struct{ name, media, before, after string }{{"明细.csv", "text/csv", "项目,金额\r\n香港,100\r\n", "项目,金额\n香港,120\n"}, {"配置.json", "application/json", `{"项目":"香港","值":1}`, `{"项目":"香港","值":2}`}, {"合同正文.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "原正文🙂", "新正文🙂"}} {
			body := func(text string) []byte {
				if !strings.HasSuffix(sample.name, ".docx") {
					return []byte(text)
				}
				var buffer bytes.Buffer
				z := zip.NewWriter(&buffer)
				for name, content := range map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`} {
					writer, err := z.Create(name)
					if err != nil {
						current.Fatal(err)
					}
					if _, err = writer.Write([]byte(content)); err != nil {
						current.Fatal(err)
					}
				}
				if err := z.Close(); err != nil {
					current.Fatal(err)
				}
				return buffer.Bytes()
			}
			leftBody, rightBody := body(sample.before), body(sample.after)
			left := add(sample.name, sample.media, leftBody, nil)
			right := add(sample.name, sample.media, rightBody, &left)
			indexAll()
			v := get(left.File.ID, left.Version.ID, right.Version.ID, f.aliceToken, "&view=all", 200)
			a, b := joined(v.Rows)
			expectedA, err := storage.ExtractText(ctx, sample.media, leftBody)
			if err != nil {
				current.Fatal(err)
			}
			expectedB, err := storage.ExtractText(ctx, sample.media, rightBody)
			if err != nil {
				current.Fatal(err)
			}
			if a != expectedA || b != expectedB || v.Base.SourceSHA256 != left.Version.SHA256 || v.Target.SourceSHA256 != right.Version.SHA256 {
				current.Fatal("format comparison did not retain actual exact extraction", sample.name)
			}
		}
	})
	if os.Getenv("CP_BROWSER_FILE_COMPARISON_FIXTURE") == "1" && !t.Failed() {
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'file_comparison_mfa','Actionless MFA fixture',true)", role, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, role)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"file_comparison_fixture": true, "file_id": base.File.ID, "base_version_id": base.Version.ID, "target_version_id": target.Version.ID, "normalized_version_id": normalized.Version.ID, "base_text_sha256": contentSHA([]byte(plainBefore)), "target_text_sha256": contentSHA([]byte(after)), "project_id": *base.File.ProjectID, "query_example": url.QueryEscape("版本比较🙂")})
	}
}
