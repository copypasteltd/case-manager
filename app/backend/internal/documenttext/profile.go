// Package documenttext extracts bounded, already verified document bytes using
// explicitly pinned native tools. It has no database, object-store or model keys.
package documenttext

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxSourceBytes = 8 << 20
const MaxTextBytes = 2 << 20
const MaxPages = 20
const MaxImagePixels = 20_000_000
const Languages = "chi_tra+chi_sim+eng"

type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Paths below Root are relative, regular, hash-verified files. No search on the
// caller's PATH, user-selected command, remote model or document URL is accepted.
type Profile struct {
	Version  int                 `json:"version"`
	Root     string              `json:"root"`
	TempRoot string              `json:"temp_root"`
	Tessdata string              `json:"tessdata"`
	Tools    map[string]Artifact `json:"tools"`
	Assets   []Artifact          `json:"assets"`
}

type Engine struct {
	profile     Profile
	fingerprint string
}

func digest(body []byte) string { value := sha256.Sum256(body); return hex.EncodeToString(value[:]) }

func code(value string) error { return &Error{Code: "FILE_TEXT_" + value} }

type Error struct{ Code string }

func (e *Error) Error() string { return e.Code }

func pinnedPath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.ContainsAny(relative, "\x00:") {
		return "", code("ENGINE_PROFILE_INVALID")
	}
	name := filepath.Clean(filepath.FromSlash(relative))
	if name == "." || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return "", code("ENGINE_PROFILE_INVALID")
	}
	full := filepath.Join(root, name)
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil || resolved != full {
		return "", code("ENGINE_PROFILE_INVALID")
	}
	return full, nil
}

func verifyFile(ctx context.Context, root string, a Artifact) error {
	if len(a.SHA256) != 64 {
		return code("ENGINE_PROFILE_INVALID")
	}
	if _, err := hex.DecodeString(a.SHA256); err != nil || a.SHA256 != strings.ToLower(a.SHA256) {
		return code("ENGINE_PROFILE_INVALID")
	}
	name, err := pinnedPath(root, a.Path)
	if err != nil {
		return err
	}
	f, err := os.Open(name)
	if err != nil {
		return code("ENGINE_PROFILE_INVALID")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 128<<20 {
		return code("ENGINE_PROFILE_INVALID")
	}
	h := sha256.New()
	buf := make([]byte, 128<<10)
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, e := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return code("ENGINE_PROFILE_INVALID")
		}
	}
	if hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return code("ENGINE_CHANGED")
	}
	return nil
}

func Load(ctx context.Context, name string) (*Engine, error) {
	if !filepath.IsAbs(name) {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	var p Profile
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || p.Version != 1 || !filepath.IsAbs(p.Root) || !filepath.IsAbs(p.TempRoot) || len(p.Tools) != 4 || len(p.Assets) < 3 || len(p.Assets) > 4096 {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	p.Root = filepath.Clean(p.Root)
	p.TempRoot = filepath.Clean(p.TempRoot)
	if strings.HasPrefix(p.Root, `\\`) || strings.HasPrefix(p.TempRoot, `\\`) {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	resolved, err := filepath.EvalSymlinks(p.Root)
	if err != nil || resolved != p.Root {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	seen := map[string]bool{}
	for _, key := range []string{"pdfinfo", "pdftotext", "pdftoppm", "tesseract"} {
		a, ok := p.Tools[key]
		if !ok {
			return nil, code("ENGINE_PROFILE_INVALID")
		}
		lower := strings.ToLower(filepath.Clean(filepath.FromSlash(a.Path)))
		if seen[lower] {
			return nil, code("ENGINE_PROFILE_INVALID")
		}
		seen[lower] = true
		if err = verifyFile(ctx, p.Root, a); err != nil {
			return nil, err
		}
	}
	data, err := pinnedPath(p.Root, p.Tessdata)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(data)
	if err != nil || !info.IsDir() {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	models := map[string]bool{}
	for _, a := range p.Assets {
		lower := strings.ToLower(filepath.Clean(filepath.FromSlash(a.Path)))
		if seen[lower] {
			return nil, code("ENGINE_PROFILE_INVALID")
		}
		seen[lower] = true
		if err = verifyFile(ctx, p.Root, a); err != nil {
			return nil, err
		}
		for _, lang := range strings.Split(Languages, "+") {
			if filepath.Join(p.Root, filepath.FromSlash(a.Path)) == filepath.Join(data, lang+".traineddata") {
				models[lang] = true
			}
		}
	}
	if len(models) != 3 {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	if err = os.MkdirAll(p.TempRoot, 0700); err != nil {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	resolved, err = filepath.EvalSymlinks(p.TempRoot)
	if err != nil || resolved != p.TempRoot {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	canonical, err := json.Marshal(p)
	if err != nil {
		return nil, code("ENGINE_PROFILE_INVALID")
	}
	return &Engine{profile: p, fingerprint: digest(canonical)}, nil
}

func (e *Engine) Fingerprint() string {
	if e == nil {
		return ""
	}
	return e.fingerprint
}

func (e *Engine) verify(ctx context.Context) error {
	for _, a := range e.profile.Tools {
		if err := verifyFile(ctx, e.profile.Root, a); err != nil {
			return err
		}
	}
	for _, a := range e.profile.Assets {
		if err := verifyFile(ctx, e.profile.Root, a); err != nil {
			return err
		}
	}
	return nil
}
