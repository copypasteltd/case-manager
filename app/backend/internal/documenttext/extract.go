package documenttext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type Result struct {
	Text          string
	PageCount     int
	OCRPageCount  int
	ProfileSHA256 string
}

type boundedOutput struct {
	bytes.Buffer
	max      int
	cancel   context.CancelFunc
	overflow bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		b.overflow = true
		b.cancel()
		return 0, errors.New("bounded native output exceeded")
	}
	return b.Buffer.Write(p)
}

func (e *Engine) command(ctx context.Context, work, tool string, max int, args ...string) ([]byte, error) {
	if err := verifyFile(ctx, e.profile.Root, e.profile.Tools[tool]); err != nil {
		return nil, err
	}
	local, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	exe, err := pinnedPath(e.profile.Root, e.profile.Tools[tool].Path)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(local, exe, args...)
	cmd.Dir = work
	cmd.WaitDelay = 2 * time.Second
	paths := []string{filepath.Dir(exe)}
	if runtime.GOOS == "windows" {
		if system := os.Getenv("SystemRoot"); system != "" {
			cmd.Env = append(cmd.Env, "SystemRoot="+system, "WINDIR="+system)
			paths = append(paths, filepath.Join(system, "System32"))
		}
	} else {
		paths = append(paths, "/usr/bin", "/bin")
	}
	data := filepath.Join(e.profile.Root, filepath.FromSlash(e.profile.Tessdata))
	cmd.Env = append(cmd.Env, "PATH="+strings.Join(paths, string(os.PathListSeparator)), "TEMP="+work, "TMP="+work, "TMPDIR="+work, "LC_ALL=C.UTF-8", "TZ=UTC", "OMP_THREAD_LIMIT=1", "OPENBLAS_NUM_THREADS=1", "TESSDATA_PREFIX="+data)
	stdout := &boundedOutput{max: max, cancel: cancel}
	stderr := &boundedOutput{max: 32 << 10, cancel: cancel}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if stdout.overflow || stderr.overflow {
		return nil, code("OUTPUT_LIMIT")
	}
	if local.Err() != nil {
		return nil, code("NATIVE_TIMEOUT")
	}
	if err != nil {
		return nil, code("NATIVE_FAILED")
	}
	return stdout.Bytes(), nil
}

func cleanText(body []byte) (string, error) {
	if len(body) > MaxTextBytes || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return "", code("NATIVE_OUTPUT_INVALID")
	}
	value := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(string(body), "\r\n", "\n"), "\r", "\n"))
	return value, nil
}

func imageDimensions(body []byte, format string) error {
	config, kind, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || kind != format || config.Width < 1 || config.Height < 1 {
		return code("DOCUMENT_INVALID")
	}
	if config.Width > 16384 || config.Height > 16384 || int64(config.Width)*int64(config.Height) > MaxImagePixels {
		return code("IMAGE_LIMIT")
	}
	return nil
}

var pagesPattern = regexp.MustCompile(`(?m)^Pages:\s+([0-9]+)\s*$`)
var encryptedPattern = regexp.MustCompile(`(?m)^Encrypted:\s+yes(?:\s|$)`)

func (e *Engine) ocr(ctx context.Context, work, input string) (string, error) {
	f, err := os.Open(input)
	if err != nil {
		return "", code("IMAGE_LIMIT")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 48<<20 {
		return "", code("IMAGE_LIMIT")
	}
	body, err := io.ReadAll(io.LimitReader(f, (48<<20)+1))
	if err != nil || len(body) > 48<<20 {
		return "", code("IMAGE_LIMIT")
	}
	format := "png"
	if strings.HasSuffix(input, ".jpg") {
		format = "jpeg"
	}
	if err = imageDimensions(body, format); err != nil {
		return "", err
	}
	data := filepath.Join(e.profile.Root, filepath.FromSlash(e.profile.Tessdata))
	raw, err := e.command(ctx, work, "tesseract", MaxTextBytes, input, "stdout", "--tessdata-dir", data, "-l", Languages, "--oem", "1", "--psm", "3")
	if err != nil {
		return "", err
	}
	return cleanText(raw)
}

// Extract emits every page or no result. Each PDF page includes its embedded
// text and a separately labelled OCR rendering, including mixed text/image pages.
// OCR never updates the original, executes a model tool or supplies financial facts.
func (e *Engine) Extract(ctx context.Context, media string, body []byte) (Result, error) {
	out := Result{}
	if e == nil {
		return out, code("ENGINE_UNAVAILABLE")
	}
	media = strings.ToLower(strings.TrimSpace(strings.SplitN(media, ";", 2)[0]))
	if media != "application/pdf" && media != "image/png" && media != "image/jpeg" {
		return out, code("FORMAT_UNSUPPORTED")
	}
	if len(body) < 1 || len(body) > MaxSourceBytes {
		return out, code("SIZE_UNSUPPORTED")
	}
	ctx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	if err := e.verify(ctx); err != nil {
		return out, err
	}
	work, err := os.MkdirTemp(e.profile.TempRoot, "extract-")
	if err != nil {
		return out, code("TEMPORARY_STORAGE_FAILED")
	}
	defer func() {
		relative, e := filepath.Rel(e.profile.TempRoot, work)
		if e == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			_ = os.RemoveAll(work)
		}
	}()
	extension := ".pdf"
	if media == "image/png" {
		extension = ".png"
	}
	if media == "image/jpeg" {
		extension = ".jpg"
	}
	input := filepath.Join(work, "source"+extension)
	if err = os.WriteFile(input, body, 0600); err != nil {
		return out, code("TEMPORARY_STORAGE_FAILED")
	}
	var text strings.Builder
	appendPart := func(value string) error {
		if len(value) > MaxTextBytes-text.Len() {
			return code("OUTPUT_LIMIT")
		}
		text.WriteString(value)
		return nil
	}
	if media == "application/pdf" {
		if !bytes.HasPrefix(body, []byte("%PDF-")) {
			return out, code("DOCUMENT_INVALID")
		}
		info, err := e.command(ctx, work, "pdfinfo", 128<<10, "-isodates", input)
		if err != nil {
			return out, err
		}
		if encryptedPattern.Match(info) {
			return out, code("ENCRYPTED_UNSUPPORTED")
		}
		matches := pagesPattern.FindAllSubmatch(info, -1)
		if len(matches) != 1 {
			return out, code("DOCUMENT_INVALID")
		}
		pages, err := strconv.Atoi(string(matches[0][1]))
		if err != nil || pages < 1 {
			return out, code("DOCUMENT_INVALID")
		}
		if pages > MaxPages {
			return out, code("PAGE_LIMIT")
		}
		for page := 1; page <= pages; page++ {
			if err = ctx.Err(); err != nil {
				return Result{}, err
			}
			number := strconv.Itoa(page)
			raw, err := e.command(ctx, work, "pdftotext", MaxTextBytes, "-f", number, "-l", number, "-enc", "UTF-8", "-eol", "unix", "-layout", "-nopgbrk", input, "-")
			if err != nil {
				return Result{}, err
			}
			embedded, err := cleanText(raw)
			if err != nil {
				return Result{}, err
			}
			prefix := filepath.Join(work, "page-"+number)
			if _, err = e.command(ctx, work, "pdftoppm", 4096, "-f", number, "-l", number, "-singlefile", "-r", "150", "-scale-to", "2200", "-png", input, prefix); err != nil {
				return Result{}, err
			}
			ocr, err := e.ocr(ctx, work, prefix+".png")
			if err != nil {
				return Result{}, err
			}
			if embedded == "" {
				embedded = "[此页未提取到内嵌文字]"
			}
			if ocr == "" {
				ocr = "[此页未识别到文字，请查原件]"
			}
			if err = appendPart(fmt.Sprintf("[PDF第%d页 · 内嵌文字]\n%s\n[PDF第%d页 · OCR识别，需核对原件]\n%s\n\n", page, embedded, page, ocr)); err != nil {
				return Result{}, err
			}
		}
		out.PageCount = pages
		out.OCRPageCount = pages
	} else {
		ocr, err := e.ocr(ctx, work, input)
		if err != nil {
			return out, err
		}
		if ocr == "" {
			ocr = "[未识别到文字，请查原件]"
		}
		if err = appendPart("[图片OCR识别，需核对原件]\n" + ocr + "\n"); err != nil {
			return out, err
		}
		out.PageCount = 1
		out.OCRPageCount = 1
	}
	if err = e.verify(ctx); err != nil {
		return Result{}, err
	}
	out.Text = text.String()
	out.ProfileSHA256 = e.fingerprint
	return out, nil
}
