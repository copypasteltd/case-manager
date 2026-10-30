package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
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

// Only object transport is controlled. Formal commits, scan/replica evidence,
// jobs, extraction, authorization, derived reads, search and HTTP are real.
func officeDocumentFixture(t *testing.T, kind, text string) ([]byte, string) {
	t.Helper()
	const rel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
	const sheet = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	const pres = "http://schemas.openxmlformats.org/presentationml/2006/main"
	const draw = "http://schemas.openxmlformats.org/drawingml/2006/main"
	main, content, typ := "xl/workbook.xml", "spreadsheetml.sheet", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	p := map[string]string{}
	if kind == "xlsx" {
		p[main] = `<workbook xmlns="` + sheet + `" xmlns:r="` + strings.TrimSuffix(rel, "/") + `"><sheets><sheet name="项目预算" sheetId="1" r:id="sheet" state="hidden"/></sheets></workbook>`
		p["xl/_rels/workbook.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="sheet" Type="` + rel + `worksheet" Target="worksheets/one.xml"/></Relationships>`
		p["xl/worksheets/one.xml"] = `<worksheet xmlns="` + sheet + `"><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>` + text + `</t></is></c><c r="B1"><f>SUM(10,20)</f><v>30</v></c></row></sheetData></worksheet>`
	} else {
		main, content, typ = "ppt/presentation.xml", "presentationml.presentation", "application/vnd.openxmlformats-officedocument.presentationml.presentation"
		p[main] = `<p:presentation xmlns:p="` + pres + `" xmlns:r="` + strings.TrimSuffix(rel, "/") + `"><p:sldIdLst><p:sldId id="256" r:id="slide"/></p:sldIdLst></p:presentation>`
		p["ppt/_rels/presentation.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="slide" Type="` + rel + `slide" Target="slides/one.xml"/></Relationships>`
		p["ppt/slides/one.xml"] = `<p:sld xmlns:p="` + pres + `" xmlns:a="` + draw + `"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
		p["ppt/slides/_rels/one.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="notes" Type="` + rel + `notesSlide" Target="../notesSlides/one.xml"/></Relationships>`
		p["ppt/notesSlides/one.xml"] = `<p:notes xmlns:p="` + pres + `" xmlns:a="` + draw + `"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>讲者备注：核对当前交付安排。</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:notes>`
	}
	p["[Content_Types].xml"] = `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/` + main + `" ContentType="application/vnd.openxmlformats-officedocument.` + content + `.main+xml"/></Types>`
	p["_rels/.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="main" Type="` + rel + `officeDocument" Target="` + main + `"/></Relationships>`
	var buffer bytes.Buffer
	z := zip.NewWriter(&buffer)
	names := []string{}
	for name := range p {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(p[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes(), typ
}

func TestOfficeTextDocuments(t *testing.T) {
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
	leftBody, xlsx := officeDocumentFixture(t, "xlsx", "香港预算旧版独有词🙂")
	rightBody, _ := officeDocumentFixture(t, "xlsx", "香港Office当前预算🙂")
	presBody, pptx := officeDocumentFixture(t, "pptx", "香港Office季度交付🙂")
	left := add("项目预算.xlsx", xlsx, leftBody, nil)
	right := add("项目预算.xlsx", xlsx, rightBody, &left)
	slides := add("季度交付.pptx", pptx, presBody, nil)
	bad := add("损坏演示稿.pptx", pptx, []byte("not zip"), nil)
	plain := add("混合格式.txt", "text/plain", []byte("最初文字版"), nil)
	mixed := add("混合格式.txt", xlsx, rightBody, &plain)
	// Simulate an immutable pre-upgrade unsupported result, not a reset/backfill.
	exec(`INSERT INTO file_text_derivations(org_id,file_id,version_id,extractor,source_sha256,state,error_code,indexed_at) VALUES($1,$2,$3,'text-v1',$4,'unsupported','FILE_TEXT_FORMAT_UNSUPPORTED',clock_timestamp())`, f.org, right.File.ID, right.Version.ID, right.Version.SHA256)
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
		if v := get(right, f.aliceToken, "", 200); v.State != "pending" || v.Extractor != storage.OfficeTextExtractor {
			t.Fatal("legacy unsupported row selected")
		}
		if n, err := index.Schedule(ctx); err != nil || n != 6 {
			t.Fatal(n, err)
		}
		if n, err := index.Schedule(ctx); err != nil || n != 0 {
			t.Fatal("duplicate", n, err)
		}
		for {
			j, err := q.Claim(ctx, []string{storage.TextJobKind}, storage.TextJobLane, "office-fixture", time.Minute)
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
		var unchanged string
		if err := f.owner.QueryRow(ctx, `SELECT row_to_json(d)::text FROM file_text_derivations d WHERE version_id=$1 AND extractor='text-v1'`, right.Version.ID).Scan(&unchanged); err != nil || unchanged != oldEvidence {
			t.Fatal("legacy evidence rewritten", err)
		}
		for _, v := range []storage.CommitResult{left, right, slides, mixed} {
			view := get(v, f.aliceToken, "?limit=20", 200)
			if view.State != "ready" || view.Extractor != storage.OfficeTextExtractor || view.SourceSHA256 != v.Version.SHA256 || view.NextOffset == nil {
				t.Fatal("missing exact Office evidence")
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
		if _, err := f.owner.Exec(ctx, "UPDATE file_text_derivations SET body='changed' WHERE version_id=$1 AND extractor='office-v1'", right.Version.ID); err == nil {
			t.Fatal("terminal body changed")
		}
		if _, err := f.owner.Exec(ctx, `INSERT INTO file_text_derivations(org_id,file_id,version_id,extractor,source_sha256) VALUES($1,$2,$3,'office-v1',$4)`, f.org, plain.File.ID, plain.Version.ID, plain.Version.SHA256); err == nil {
			t.Fatal("office index on wrong MIME accepted")
		}
	})
	t.Run("search uses current version Office body without duplicate legacy rows", func(t *testing.T) {
		for term, want := range map[string]int{"香港Office": 3, "预算旧版独有词": 0, "讲者备注：核对当前交付安排": 1} {
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
		if v.Extractor != storage.OfficeTextExtractor || v.Base.Extractor != storage.OfficeTextExtractor || v.Target.Extractor != storage.OfficeTextExtractor || v.TextEqual {
			t.Fatal("wrong Office provenance")
		}
		v = compare(plain, mixed, f.aliceToken, 200)
		if v.Extractor != "mixed" || v.Base.Extractor != storage.TextExtractor || v.Target.Extractor != storage.OfficeTextExtractor {
			t.Fatal("mixed provenance lost")
		}
		compare(right, slides, f.aliceToken, 404)
	})
	var c agent.Conversation
	t.Run("secretary preview consumes exact derived Office text without object credentials", func(t *testing.T) {
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", core.NewID(), f.org, f.alice)
		exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: right.File.ProjectID, Title: "Office资料核对", MemberIDs: []string{}}, nil)
		expect(t, rec, 201, res)
		c = data[agent.Conversation](t, res)
		before := cloud.reads.Load()
		f.api.AgentFileReaders = nil
		f.handler = f.api.Handler()
		for _, v := range []storage.CommitResult{right, slides} {
			rec, res = f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/file/"+v.File.ID+"?context_security_version="+strconv.FormatInt(c.ContextSecurityVersion, 10)+"&version_id="+v.Version.ID+"&max_chars=10000", f.aliceToken, "", nil, nil)
			expect(t, rec, 200, res)
			view := data[agent.ContextView](t, res)
			if view.TextIndex == nil || view.TextIndex.Extractor != storage.OfficeTextExtractor || view.TextIndex.SourceSHA256 != v.Version.SHA256 || view.Text == "" {
				t.Fatal("Office context evidence lost")
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
	if os.Getenv("CP_BROWSER_OFFICE_TEXT_FIXTURE") == "1" && !t.Failed() {
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'office_text_mfa','Actionless MFA fixture',true)", role, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, role)
		f.aliceToken = memberMFAToken(f, "alice", "android")
		_ = f.administratorToken()
		holdBrowserBusinessFixture(t, f, map[string]any{"office_text_fixture": true, "file_id": right.File.ID, "base_version_id": left.Version.ID, "target_version_id": right.Version.ID, "presentation_file_id": slides.File.ID, "presentation_version_id": slides.Version.ID, "project_id": *right.File.ProjectID, "conversation_id": c.ID})
	}
}
