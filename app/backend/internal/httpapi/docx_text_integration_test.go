package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/search"
)

func wordStoryFixture(t *testing.T, text string) ([]byte, string) {
	t.Helper()
	const wordNS = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	const rel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	p := map[string]string{
		"[Content_Types].xml":         `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/custom/main.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":                 `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="main" Type="` + rel + `/officeDocument" Target="custom/main.xml"/></Relationships>`,
		"custom/main.xml":             `<w:document xmlns:w="` + wordNS + `" xmlns:r="` + rel + `"><w:body><w:p><w:r><w:t>香港🙂 合同</w:t></w:r><w:ins><w:r><w:t>新增条款</w:t></w:r></w:ins><w:del><w:r><w:delText>删除条款</w:delText></w:r></w:del><w:moveFrom><w:r><w:t>移动原文</w:t></w:r></w:moveFrom><w:moveTo><w:r><w:t>移动新文</w:t></w:r></w:moveTo><w:hyperlink r:id="web"><w:r><w:t>链接显示文字</w:t></w:r></w:hyperlink></w:p><w:sdt><w:sdtPr><w:alias w:val="DO_NOT_INDEX_ALIAS"/></w:sdtPr><w:sdtContent><w:tbl><w:tr><w:tc><w:p><w:r><w:t>金额</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>HKD 1200</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:sdtContent></w:sdt><w:p><w:r><w:rPr><w:vanish/></w:rPr><w:t>隐藏条款</w:t></w:r><w:r><w:footnoteReference w:id="2"/><w:footnoteReference w:id="2"/><w:endnoteReference w:id="3"/></w:r><w:commentRangeStart w:id="5"/><w:r><w:t>待核对</w:t><w:commentReference w:id="5"/></w:r><w:commentRangeEnd w:id="5"/></w:p><w:p><w:r><w:pict><w:txbxContent><w:p><w:r><w:t>文本框交付说明</w:t></w:r></w:p></w:txbxContent></w:pict></w:r></w:p><w:p><w:fldSimple w:instr="DATE"><w:r><w:t>旧日期缓存</w:t></w:r></w:fldSimple><w:r><w:fldChar w:fldCharType="begin"/><w:instrText>INCLUDETEXT https://example.invalid/private</w:instrText><w:fldChar w:fldCharType="separate"/><w:t>外部字段旧缓存</w:t><w:fldChar w:fldCharType="end"/></w:r></w:p><w:sectPr><w:headerReference w:type="default" r:id="header"/><w:footerReference w:type="first" r:id="footer"/></w:sectPr></w:body></w:document>`,
		"custom/_rels/main.xml.rels":  `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="header" Type="` + rel + `/header" Target="stories/top.xml"/><Relationship Id="footer" Type="` + rel + `/footer" Target="stories/bottom.xml"/><Relationship Id="foot" Type="` + rel + `/footnotes" Target="stories/foot.xml"/><Relationship Id="end" Type="` + rel + `/endnotes" Target="stories/end.xml"/><Relationship Id="comments" Type="` + rel + `/comments" Target="stories/comments.xml"/><Relationship Id="web" Type="` + rel + `/hyperlink" TargetMode="External" Target="https://example.invalid/private"/></Relationships>`,
		"custom/stories/top.xml":      `<w:hdr xmlns:w="` + wordNS + `"><w:p><w:r><w:t>公司页眉</w:t></w:r></w:p></w:hdr>`,
		"custom/stories/bottom.xml":   `<w:ftr xmlns:w="` + wordNS + `"><w:p><w:r><w:t>合同页脚</w:t></w:r></w:p></w:ftr>`,
		"custom/stories/foot.xml":     `<w:footnotes xmlns:w="` + wordNS + `"><w:footnote w:id="-1" w:type="separator"><w:p><w:r><w:t>DO_NOT_INDEX_SEPARATOR</w:t></w:r></w:p></w:footnote><w:footnote w:id="2"><w:p><w:r><w:t>脚注付款依据</w:t></w:r></w:p></w:footnote><w:footnote w:id="99"><w:p><w:r><w:t>DO_NOT_INDEX_ORPHAN_NOTE</w:t></w:r></w:p></w:footnote></w:footnotes>`,
		"custom/stories/end.xml":      `<w:endnotes xmlns:w="` + wordNS + `"><w:endnote w:id="3"><w:p><w:r><w:t>尾注有效期</w:t></w:r></w:p></w:endnote></w:endnotes>`,
		"custom/stories/comments.xml": `<w:comments xmlns:w="` + wordNS + `"><w:comment w:id="5" w:author="DO_NOT_INDEX_AUTHOR"><w:p><w:r><w:t>批注需财务确认</w:t></w:r></w:p></w:comment></w:comments>`,
		"word/document.xml":           `<invalid>DO_NOT_INDEX_FILENAME_GUESS`,
		"word/header99.xml":           `DO_NOT_INDEX_ORPHAN_HEADER`,
	}
	p["custom/main.xml"] = strings.Replace(p["custom/main.xml"], "香港🙂 合同", text, 1)
	// A queued text-v1 job reads its historical conventional main-part name.
	p["word/document.xml"] = `<w:document xmlns:w="` + wordNS + `"><w:body><w:p><w:r><w:t>legacy-main-only ` + text + `</w:t></w:r></w:p></w:body></w:document>`
	return zipWordParts(t, p), "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
}

func zipWordParts(t *testing.T, p map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	z := zip.NewWriter(&buffer)
	names := []string{}
	for name := range p {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(p[name])); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return buffer.Bytes()
}
func TestDOCXTextDocuments(t *testing.T) {
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
	add := func(name, typ string, body []byte, base *storage.CommitResult) storage.CommitResult {
		t.Helper()
		v, store := commitContentFixture(t, u, name, typ, body, base)
		for key := range store.stored {
			bodies[key] = append([]byte(nil), body...)
		}
		return v
	}
	leftBody, xlsx := wordStoryFixture(t, "香港预算旧版独有词🙂")
	rightBody, _ := wordStoryFixture(t, "香港Word当前合同🙂")
	presBody, pptx := wordStoryFixture(t, "香港Word季度交付🙂")
	left := add("合同审阅.docx", xlsx, leftBody, nil)
	right := add("合同审阅.docx", xlsx, rightBody, &left)
	slides := add("附件审阅.docx", pptx, presBody, nil)
	bad := add("损坏合同.docx", pptx, []byte("not zip"), nil)
	plain := add("混合格式.txt", "text/plain", []byte("最初文字版"), nil)
	mixed := add("混合格式.txt", xlsx, rightBody, &plain)
	// Keep an actual immutable legacy ready result beside the new derivation.
	exec(`INSERT INTO file_text_derivations(org_id,file_id,version_id,extractor,source_sha256,state,body,text_sha256,character_count,indexed_at) VALUES($1,$2,$3,'text-v1',$4,'ready','旧版正文证据',encode(sha256(convert_to('旧版正文证据','UTF8')),'hex'),char_length('旧版正文证据'),clock_timestamp())`, f.org, right.File.ID, right.Version.ID, right.Version.SHA256)
	var oldEvidence string
	if err := f.owner.QueryRow(ctx, `SELECT row_to_json(d)::text FROM file_text_derivations d WHERE version_id=$1 AND extractor='text-v1'`, right.Version.ID).Scan(&oldEvidence); err != nil {
		t.Fatal(err)
	}
	cloud := &downloadCloud{}
	cloud.readFn = func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
		body := bodies[source.Key]
		if offset != 0 || length != int64(len(body)) {
			t.Fatal("source identity mismatch")
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
	compare := func(a, b storage.CommitResult, token string, status int) storage.TextComparison {
		t.Helper()
		rec, res := f.request("GET", "/v1/files/"+a.File.ID+"/text-comparison?base_version_id="+a.Version.ID+"&target_version_id="+b.Version.ID+"&view=all", token, "", nil, nil)
		expect(t, rec, status, res)
		if status != 200 {
			return storage.TextComparison{}
		}
		return data[storage.TextComparison](t, res)
	}
	t.Run("new identity queues once and keeps legacy terminal evidence", func(t *testing.T) {
		if v := get(right, f.aliceToken, "", 200); v.State != "pending" || v.Extractor != storage.DOCXTextExtractor {
			t.Fatal("legacy unsupported row selected")
		}
		if n, err := index.Schedule(ctx); err != nil || n != 6 {
			t.Fatal(n, err)
		}
		if n, err := index.Schedule(ctx); err != nil || n != 0 {
			t.Fatal("duplicate", n, err)
		}
		exec(`INSERT INTO file_text_derivations(org_id,file_id,version_id,extractor,source_sha256) VALUES($1,$2,$3,'text-v1',$4)`, f.org, left.File.ID, left.Version.ID, left.Version.SHA256)
		payload, _ := json.Marshal(map[string]string{"file_id": left.File.ID, "version_id": left.Version.ID, "extractor": "text-v1"})
		tx, e := f.api.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e = q.Enqueue(ctx, tx, jobqueue.Input{OrgID: f.org, Kind: storage.TextJobKind, Lane: storage.TextJobLane, DedupKey: storage.TextJobKind + "/" + left.Version.ID + "/text-v1", PayloadVersion: 1, Payload: payload, MaxAttempts: 8}); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		for {
			j, err := q.Claim(ctx, []string{storage.TextJobKind}, storage.TextJobLane, "docx-fixture", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if j == nil {
				break
			}
			if err = index.Process(ctx, *j); err != nil {
				t.Fatal(err)
			}
		}
		var legacy string
		if e := f.owner.QueryRow(ctx, `SELECT body FROM file_text_derivations WHERE version_id=$1 AND extractor='text-v1'`, left.Version.ID).Scan(&legacy); e != nil || strings.Contains(legacy, "删除条款") || strings.Contains(legacy, "脚注付款依据") {
			t.Fatal("legacy job changed its algorithm", e)
		}
		var unchanged string
		if err := f.owner.QueryRow(ctx, `SELECT row_to_json(d)::text FROM file_text_derivations d WHERE version_id=$1 AND extractor='text-v1'`, right.Version.ID).Scan(&unchanged); err != nil || unchanged != oldEvidence {
			t.Fatal("legacy evidence rewritten", err)
		}
		for _, v := range []storage.CommitResult{left, right, slides, mixed} {
			view := get(v, f.aliceToken, "?limit=20", 200)
			if view.State != "ready" || view.Extractor != storage.DOCXTextExtractor || view.SourceSHA256 != v.Version.SHA256 || view.NextOffset == nil {
				t.Fatal("missing exact Word evidence")
			}
			offset, all := 0, ""
			for {
				part := get(v, f.aliceToken, "?limit=37&offset="+strconv.Itoa(offset), 200)
				all += part.Text
				if part.NextOffset == nil {
					break
				}
				offset = *part.NextOffset
			}
			if core.Digest([]byte(all)) != *view.TextSHA256 {
				t.Fatal("incomplete Unicode body")
			}
		}
		if v := get(bad, f.aliceToken, "", 200); v.State != "failed" || v.Text != "" {
			t.Fatal("broken document indexed")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE file_text_derivations SET body='changed' WHERE version_id=$1 AND extractor='docx-v2'", right.Version.ID); err == nil {
			t.Fatal("terminal body changed")
		}
		if _, err := f.owner.Exec(ctx, `INSERT INTO file_text_derivations(org_id,file_id,version_id,extractor,source_sha256) VALUES($1,$2,$3,'docx-v2',$4)`, f.org, plain.File.ID, plain.Version.ID, plain.Version.SHA256); err == nil {
			t.Fatal("office index on wrong MIME accepted")
		}
	})
	t.Run("search uses current version Word body without duplicate legacy rows", func(t *testing.T) {
		for term, want := range map[string]int{"香港Word": 3, "预算旧版独有词": 0, "脚注付款依据": 3, "批注需财务确认": 3, "删除条款": 3, "DO_NOT_INDEX": 0} {
			rec, res := f.request("GET", "/v1/search?q="+url.QueryEscape(term)+"&type=file", f.aliceToken, "", nil, nil)
			expect(t, rec, 200, res)
			hits := data[core.Page[search.Hit]](t, res)
			if len(hits.Items) != want {
				t.Fatalf("%s: %d != %d", term, len(hits.Items), want)
			}
		}
	})
	t.Run("comparison binds office or mixed per-version provenance", func(t *testing.T) {
		v := compare(left, right, f.aliceToken, 200)
		if v.Extractor != storage.DOCXTextExtractor || v.Base.Extractor != storage.DOCXTextExtractor || v.Target.Extractor != storage.DOCXTextExtractor || v.TextEqual {
			t.Fatal("wrong Word provenance")
		}
		v = compare(plain, mixed, f.aliceToken, 200)
		if v.Extractor != "mixed" || v.Base.Extractor != storage.TextExtractor || v.Target.Extractor != storage.DOCXTextExtractor {
			t.Fatal("mixed provenance lost")
		}
		compare(right, slides, f.aliceToken, 404)
	})
	var c agent.Conversation
	t.Run("secretary preview consumes exact derived Word text without object credentials", func(t *testing.T) {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, f.alice)
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: right.File.ProjectID, Title: "Word资料核对", MemberIDs: []string{}}, nil)
		expect(t, rec, 201, res)
		c = data[agent.Conversation](t, res)
		before := cloud.reads.Load()
		f.api.AgentFileReaders = nil
		f.handler = f.api.Handler()
		for _, v := range []storage.CommitResult{right, slides} {
			rec, res = f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/file/"+v.File.ID+"?context_security_version="+strconv.FormatInt(c.ContextSecurityVersion, 10)+"&version_id="+v.Version.ID+"&max_chars=10000", f.aliceToken, "", nil, nil)
			expect(t, rec, 200, res)
			view := data[agent.ContextView](t, res)
			if view.TextIndex == nil || view.TextIndex.Extractor != storage.DOCXTextExtractor || view.TextIndex.SourceSHA256 != v.Version.SHA256 || view.Text == "" {
				t.Fatal("Word context evidence lost")
			}
		}
		if cloud.reads.Load() != before {
			t.Fatal("derived reads fetched cloud originals")
		}
	})
	t.Run("current classification and scan rechecks hide stored bodies", func(t *testing.T) {
		exec("UPDATE files SET classification='confidential',version=version+1 WHERE id=$1", right.File.ID)
		get(right, f.bobToken, "", 404)
		compare(left, right, f.bobToken, 404)
		exec("UPDATE files SET classification='project_restricted',version=version+1 WHERE id=$1", right.File.ID)
		exec("UPDATE file_versions SET scan_state='quarantined',availability='quarantined' WHERE id=$1", right.Version.ID)
		get(right, f.aliceToken, "", 409)
		compare(left, right, f.aliceToken, 409)
		exec("UPDATE file_versions SET scan_state='clean',availability='available' WHERE id=$1", right.Version.ID)
	})
	if os.Getenv("CP_BROWSER_DOCX_TEXT_FIXTURE") == "1" && !t.Failed() {
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'office_text_mfa','Actionless MFA fixture',true)", role, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, role)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"docx_text_fixture": true, "file_id": right.File.ID, "base_version_id": left.Version.ID, "target_version_id": right.Version.ID, "presentation_file_id": slides.File.ID, "presentation_version_id": slides.Version.ID, "project_id": *right.File.ProjectID, "conversation_id": c.ID})
	}
}
