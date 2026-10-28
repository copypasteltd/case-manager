package fileimport

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/importspec"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sourcePlan(t *testing.T) (string, []byte, PlanHeader) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "原件.txt"), []byte("actual synthetic bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var inventory, template, plan bytes.Buffer
	if _, err := Inventory(context.Background(), root, core.NewID(), &inventory); err != nil {
		t.Fatal(err)
	}
	if err := Template(bytes.NewReader(inventory.Bytes()), &template); err != nil {
		t.Fatal(err)
	}
	var m Mapping
	if json.Unmarshal(bytes.TrimSpace(template.Bytes()), &m) != nil {
		t.Fatal("mapping template")
	}
	m.SourceOwner = "reviewed source owner"
	m.MappedOwnerID = core.NewID()
	m.FolderID = core.NewID()
	m.FolderVersion = 1
	m.Classification = "internal"
	raw, _ := json.Marshal(m)
	spool, err := os.CreateTemp(t.TempDir(), "plan-*")
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	h, err := BuildPlan(bytes.NewReader(inventory.Bytes()), bytes.NewReader(append(raw, '\n')), spool, &plan, "reviewed synthetic plan")
	if err != nil {
		t.Fatal(err)
	}
	return root, plan.Bytes(), h
}
func TestNativeImportPlan(t *testing.T) {
	root, raw, h := sourcePlan(t)
	got, err := ReadPlan(bytes.NewReader(raw), nil)
	if err != nil || got != h {
		t.Fatal("saved plan mismatch", err)
	}
	for _, change := range []func([]byte) []byte{func(b []byte) []byte { return bytes.Replace(b, []byte("原件.txt"), []byte("改名.txt"), 1) }, func(b []byte) []byte { return b[:bytes.LastIndex(bytes.TrimSpace(b), []byte("\n"))] }, func(b []byte) []byte { return append(b, []byte("{}\n")...) }} {
		if _, err := ReadPlan(bytes.NewReader(change(bytes.Clone(raw))), nil); err == nil {
			t.Fatal("altered plan accepted")
		}
	}
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var entry importspec.Entry
	ReadPlan(bytes.NewReader(raw), func(e importspec.Entry) error { entry = e; return nil })
	held, info, err := holdSource(context.Background(), r, entry)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err = os.WriteFile(filepath.Join(root, entry.SourcePath), []byte("changed after plan"), 0600); err != nil {
		t.Fatal(err)
	}
	if stableSource(r, held, info, entry.SourcePath) == nil {
		t.Fatal("held source mutation ignored")
	}
	if f, _, err := holdSource(context.Background(), r, entry); err == nil {
		f.Close()
		t.Fatal("changed source uploaded")
	}
	var out bytes.Buffer
	if _, err = Inventory(context.Background(), root, core.NewID(), &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	os.WriteFile(filepath.Join(root, "empty.bin"), nil, 0600)
	sum, err := Inventory(context.Background(), root, core.NewID(), &out)
	if err != nil || sum.Errors != 1 {
		t.Fatal("zero-size source not explicit", err)
	}
	if Template(bytes.NewReader(out.Bytes()), io.Discard) == nil {
		t.Fatal("error inventory admitted")
	}
}
func TestNativeImportOutput(t *testing.T) {
	root := t.TempDir()
	parent := t.TempDir()
	write := func(f *os.File) error { _, e := f.WriteString("private"); return e }
	for _, name := range []string{"inside", ".hidden", "..valid"} {
		if PrivateOutput(filepath.Join(root, name), root, write) == nil {
			t.Fatal("output inside source admitted")
		}
	}
	dst := filepath.Join(parent, "report.jsonl")
	if err := PrivateOutput(dst, root, write); err != nil {
		t.Fatal(err)
	}
	if PrivateOutput(dst, root, write) == nil {
		t.Fatal("existing report overwritten")
	}
	fail := filepath.Join(parent, "failed.jsonl")
	if PrivateOutput(fail, root, func(f *os.File) error { write(f); return io.ErrUnexpectedEOF }) == nil {
		t.Fatal("failed output published")
	}
	if _, err := os.Stat(fail); !os.IsNotExist(err) {
		t.Fatal("partial plan published")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	var b bytes.Buffer
	if _, err := Inventory(canceled, root, core.NewID(), &b); err == nil {
		t.Fatal("canceled inventory passed")
	}
}
func TestNativeImportTransport(t *testing.T) {
	token := strings.Repeat("t", 32)
	requests := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("API credentials reached object upload")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "bytes" || r.ContentLength != 5 {
			t.Error("part body not streamed accurately")
		}
		w.Header().Set("ETag", "actual-etag")
		w.WriteHeader(200)
	}))
	defer target.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("session missing")
		}
		if r.URL.Path == "/v1/redirect" {
			http.Redirect(w, r, target.URL, 307)
			return
		}
		w.WriteHeader(403)
		io.WriteString(w, `{"error":{"code":"secret https://bad.invalid/?token=SECRET","message":"SECRET"}}`)
	}))
	defer api.Close()
	c, err := NewClient(api.URL+"/v1", token, []string{target.URL}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := files.SignedPart{URL: target.URL + "/part?signature=private", Method: "PUT", ExpiresAt: time.Now().Add(time.Minute)}
	if etag, err := c.Put(context.Background(), p, strings.NewReader("bytes"), 5); err != nil || etag != "actual-etag" {
		t.Fatal("streamed part failed", err)
	}
	p.Headers = []files.SignedHeader{{Name: "Authorization", Value: "private"}}
	if _, err = c.Put(context.Background(), p, strings.NewReader("bytes"), 5); err == nil {
		t.Fatal("unexpected signed authorization accepted")
	}
	p.Headers = nil
	p.URL = "https://unapproved.invalid/part"
	if _, err = c.Put(context.Background(), p, strings.NewReader("bytes"), 5); err == nil {
		t.Fatal("unapproved host accepted")
	}
	if err = c.Request(context.Background(), "GET", "/reject", "", nil, nil); err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "https") {
		t.Fatal("untrusted error exposed", err)
	}
	before := requests
	if err = c.Request(context.Background(), "GET", "/redirect", "", nil, nil); err == nil || requests != before {
		t.Fatal("redirect followed")
	}
	if _, err = NewClient(api.URL+"/v1", token, nil, false); err == nil {
		t.Fatal("plain HTTP outside explicit test mode")
	}
}
