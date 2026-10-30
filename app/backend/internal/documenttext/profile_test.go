package documenttext

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureProfile(t *testing.T) (Profile, string) {
	t.Helper()
	root := t.TempDir()
	p := Profile{Version: 1, Root: root, TempRoot: filepath.Join(root, "work"), Tessdata: "models", Tools: map[string]Artifact{}}
	if e := os.Mkdir(filepath.Join(root, "models"), 0700); e != nil {
		t.Fatal(e)
	}
	put := func(name string) Artifact {
		t.Helper()
		body := []byte("synthetic pinned bytes " + name)
		if e := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), body, 0600); e != nil {
			t.Fatal(e)
		}
		return Artifact{Path: name, SHA256: digest(body)}
	}
	for _, tool := range []string{"pdfinfo", "pdftotext", "pdftoppm", "tesseract"} {
		p.Tools[tool] = put(tool)
	}
	for _, lang := range strings.Split(Languages, "+") {
		p.Assets = append(p.Assets, put("models/"+lang+".traineddata"))
	}
	return p, filepath.Join(root, "profile.json")
}
func saveProfile(t *testing.T, p Profile, name string) {
	t.Helper()
	b, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(name, b, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestProfileIntegrity(t *testing.T) {
	for _, tc := range []string{"valid", "escape", "absolute", "hash", "missing-model", "duplicate", "missing-tool", "tool-mutated", "unknown-field"} {
		t.Run(tc, func(t *testing.T) {
			p, name := fixtureProfile(t)
			switch tc {
			case "escape":
				a := p.Tools["pdfinfo"]
				a.Path = "../outside"
				p.Tools["pdfinfo"] = a
			case "absolute":
				a := p.Tools["pdfinfo"]
				a.Path = filepath.Join(p.Root, "pdfinfo")
				p.Tools["pdfinfo"] = a
			case "hash":
				a := p.Tools["pdfinfo"]
				a.SHA256 = strings.Repeat("0", 64)
				p.Tools["pdfinfo"] = a
			case "missing-model":
				p.Assets = p.Assets[:2]
			case "duplicate":
				p.Assets = append(p.Assets, p.Assets[0])
			case "missing-tool":
				delete(p.Tools, "tesseract")
			}
			saveProfile(t, p, name)
			if tc == "unknown-field" {
				b, _ := os.ReadFile(name)
				b = append([]byte(`{"unknown":true,`), b[1:]...)
				_ = os.WriteFile(name, b, 0600)
			}
			e, err := Load(context.Background(), name)
			if tc != "valid" && tc != "tool-mutated" {
				if err == nil {
					t.Fatal("invalid profile accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc == "tool-mutated" {
				if err = os.WriteFile(filepath.Join(p.Root, "pdfinfo"), []byte("modified"), 0600); err != nil {
					t.Fatal(err)
				}
				err = e.verify(context.Background())
				var typed *Error
				if !errors.As(err, &typed) || typed.Code != "FILE_TEXT_ENGINE_CHANGED" {
					t.Fatalf("mutated tool accepted: %v", err)
				}
			} else if len(e.Fingerprint()) != 64 {
				t.Fatal("missing profile digest")
			}
		})
	}
}
func TestBoundedOutputAndText(t *testing.T) {
	cancelled := false
	b := boundedOutput{max: 3, cancel: func() { cancelled = true }}
	if _, e := b.Write([]byte("abcd")); e == nil || !cancelled || !b.overflow || b.Len() != 0 {
		t.Fatal("oversized output retained")
	}
	for _, body := range [][]byte{{0}, {0xff}, make([]byte, MaxTextBytes+1)} {
		if _, e := cleanText(body); e == nil {
			t.Fatal("invalid text accepted")
		}
	}
	if got, e := cleanText([]byte("a\r\nb\r")); e != nil || got != "a\nb" {
		t.Fatal(got, e)
	}
	if e := imageDimensions([]byte("not an image"), "png"); e == nil {
		t.Fatal("invalid image accepted")
	}
}
