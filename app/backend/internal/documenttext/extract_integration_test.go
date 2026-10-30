package documenttext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in checks execute pinned Poppler/Tesseract against synthetic inputs.
func TestNativeDocuments(t *testing.T) {
	profile, fixtures := os.Getenv("CP_DOCUMENT_TEXT_TEST_PROFILE"), os.Getenv("CP_DOCUMENT_TEXT_TEST_FIXTURES")
	if profile == "" || fixtures == "" {
		t.Skip("explicit pinned native tools and synthetic fixtures required")
	}
	engine, err := Load(context.Background(), profile)
	if err != nil {
		t.Fatal(err)
	}
	load := func(name string) []byte {
		t.Helper()
		b, e := os.ReadFile(filepath.Join(fixtures, name))
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	for _, tc := range []struct {
		name, media string
		pages       int
		words       []string
	}{
		{"scan.png", "image/png", 1, []string{"CP128", "INVOICE", "123.45", "目"}},
		{"scan.jpg", "image/jpeg", 1, []string{"CP128", "INVOICE", "123.45"}},
		{"scan.pdf", "application/pdf", 1, []string{"CP128", "INVOICE", "123.45", "OCR识别"}},
		{"mixed.pdf", "application/pdf", 2, []string{"CP128 embedded document text", "CP128 DELIVERY READY", "INVOICE", "PDF第2页", "OCR识别"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, e := engine.Extract(context.Background(), tc.media, load(tc.name))
			if e != nil {
				t.Fatal(e)
			}
			if r.PageCount != tc.pages || r.OCRPageCount != tc.pages || r.ProfileSHA256 != engine.Fingerprint() {
				t.Fatalf("incomplete provenance: %+v", r)
			}
			for _, word := range tc.words {
				if !strings.Contains(r.Text, word) {
					t.Fatalf("missing %q in %q", word, r.Text)
				}
			}
		})
	}
	for _, tc := range []struct{ name, want string }{{"too-many-pages.pdf", "FILE_TEXT_PAGE_LIMIT"}, {"encrypted.pdf", "FILE_TEXT_ENCRYPTED_UNSUPPORTED"}, {"malformed.pdf", "FILE_TEXT_NATIVE_FAILED"}} {
		t.Run(tc.name, func(t *testing.T) {
			r, e := engine.Extract(context.Background(), "application/pdf", load(tc.name))
			var typed *Error
			if !errors.As(e, &typed) || typed.Code != tc.want || r != (Result{}) {
				t.Fatalf("result=%+v error=%v", r, e)
			}
		})
	}
	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r, e := engine.Extract(ctx, "application/pdf", load("mixed.pdf"))
		if !errors.Is(e, context.Canceled) || r != (Result{}) {
			t.Fatalf("result=%+v error=%v", r, e)
		}
	})
	entries, e := os.ReadDir(engine.profile.TempRoot)
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "extract-") {
			t.Fatalf("temporary extraction retained: %s", entry.Name())
		}
	}
}
