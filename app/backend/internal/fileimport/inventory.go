// Package fileimport implements the native import operator, without database or
// object-store credentials. Only ordinary authorized API sessions are accepted.
package fileimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/importspec"
)

type InventoryRecord struct {
	Type       string `json:"type"`
	Path       string `json:"source_path"`
	SourceKey  string `json:"source_key"`
	SizeBytes  string `json:"size_bytes"`
	SHA256     string `json:"sha256"`
	ModifiedAt string `json:"modified_at"`
	Mode       string `json:"mode"`
	ErrorCode  string `json:"error_code,omitempty"`
}

type InventoryHeader struct {
	Type            string    `json:"type"`
	Protocol        int       `json:"protocol"`
	SourceNamespace string    `json:"source_namespace"`
	CreatedAt       time.Time `json:"created_at"`
}

type InventorySummary struct {
	Type       string `json:"type"`
	Files      int    `json:"files"`
	TotalBytes string `json:"total_bytes"`
	Errors     int    `json:"errors"`
}

// Inspect checks a held source and its path before and after hashing. OpenRoot
// confines reads to the explicitly selected root. No symlink is inventoried.
func Inspect(ctx context.Context, root *os.Root, relative string) (InventoryRecord, error) {
	record := InventoryRecord{Type: "file", Path: relative}
	if !importspec.RelativePath(relative) {
		return record, fmt.Errorf("UNSUPPORTED_SOURCE_PATH")
	}
	before, err := root.Lstat(relative)
	if err != nil {
		return record, fmt.Errorf("SOURCE_UNREADABLE")
	}
	if !before.Mode().IsRegular() {
		return record, fmt.Errorf("SOURCE_NOT_REGULAR")
	}
	if before.Size() < 1 || before.Size() > importspec.MaxFileBytes {
		return record, fmt.Errorf("SOURCE_SIZE_UNSUPPORTED")
	}
	f, err := root.Open(relative)
	if err != nil {
		return record, fmt.Errorf("SOURCE_UNREADABLE")
	}
	defer f.Close()
	held, err := f.Stat()
	if err != nil || !os.SameFile(before, held) {
		return record, fmt.Errorf("SOURCE_CHANGED")
	}
	h := sha256.New()
	n, err := copyContext(ctx, h, f)
	if err != nil {
		return record, err
	}
	after, err := root.Lstat(relative)
	if err != nil || !os.SameFile(held, after) || after.Size() != held.Size() || !after.ModTime().Equal(held.ModTime()) || n != held.Size() {
		return record, fmt.Errorf("SOURCE_CHANGED")
	}
	record.SizeBytes = strconv.FormatInt(n, 10)
	record.SHA256 = hex.EncodeToString(h.Sum(nil))
	record.ModifiedAt = held.ModTime().UTC().Format(time.RFC3339Nano)
	record.Mode = held.Mode().String()
	return record, nil
}

func copyContext(ctx context.Context, to io.Writer, from io.Reader) (int64, error) {
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := from.Read(buffer)
		if n > 0 {
			written, e := to.Write(buffer[:n])
			total += int64(written)
			if e != nil {
				return total, e
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

// Inventory streams directory entries in bounded chunks, not a whole-tree list.
// Order is the observed native enumeration; a saved inventory is immutable input
// for resume. Cross-scan identity is namespace + exact relative UTF-8 path.
func Inventory(ctx context.Context, rootPath, namespace string, to io.Writer) (InventorySummary, error) {
	summary := InventorySummary{Type: "summary", TotalBytes: "0"}
	if !importspec.CanonicalID(namespace) {
		return summary, fmt.Errorf("CANONICAL_SOURCE_NAMESPACE_REQUIRED")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return summary, fmt.Errorf("SOURCE_ROOT_UNREADABLE")
	}
	defer root.Close()
	enc := json.NewEncoder(to)
	enc.SetEscapeHTML(true)
	if err = enc.Encode(InventoryHeader{Type: "inventory", Protocol: 1, SourceNamespace: namespace, CreatedAt: time.Now().UTC()}); err != nil {
		return summary, err
	}
	var total int64
	var walk func(string, int) error
	walk = func(relative string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 256 {
			return fmt.Errorf("SOURCE_DEPTH_LIMIT")
		}
		before, err := root.Lstat(relative)
		if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("SOURCE_DIRECTORY_UNREADABLE")
		}
		dir, err := root.Open(relative)
		if err != nil {
			return fmt.Errorf("SOURCE_DIRECTORY_UNREADABLE")
		}
		defer dir.Close()
		held, err := dir.Stat()
		if err != nil || !os.SameFile(before, held) {
			return fmt.Errorf("SOURCE_DIRECTORY_CHANGED")
		}
		for {
			items, readErr := dir.ReadDir(128)
			for _, item := range items {
				if err := ctx.Err(); err != nil {
					return err
				}
				name := item.Name()
				if relative != "." {
					name = relative + "/" + name
				}
				if item.IsDir() && item.Type()&os.ModeSymlink == 0 {
					if !importspec.RelativePath(name) {
						summary.Errors++
						if err = enc.Encode(InventoryRecord{Type: "error", Path: name, ErrorCode: "UNSUPPORTED_SOURCE_PATH"}); err != nil {
							return err
						}
						continue
					}
					if err = walk(name, depth+1); err != nil {
						return err
					}
					continue
				}
				record, e := Inspect(ctx, root, name)
				record.SourceKey = importspec.SourceKey(namespace, name)
				if e != nil {
					record.Type = "error"
					record.ErrorCode = e.Error()
					summary.Errors++
				} else {
					summary.Files++
					if summary.Files > importspec.MaxEntries {
						return fmt.Errorf("INVENTORY_ENTRY_LIMIT")
					}
					n, _ := importspec.Bytes(record.SizeBytes)
					total += n
				}
				if err = enc.Encode(record); err != nil {
					return err
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				return fmt.Errorf("SOURCE_DIRECTORY_UNREADABLE")
			}
		}
		return nil
	}
	if err = walk(".", 0); err != nil {
		return summary, err
	}
	summary.TotalBytes = strconv.FormatInt(total, 10)
	if err = enc.Encode(summary); err != nil {
		return summary, err
	}
	return summary, nil
}

type hashedReader struct {
	io.Reader
	hash hash.Hash
}

func (r hashedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.hash.Write(p[:n])
	return n, err
}

// PrivateOutput never overwrites an existing plan/report, and keeps temporary
// data beside the selected output, outside the source tree and outside Git.
func PrivateOutput(name, sourceRoot string, fn func(*os.File) error) error {
	abs, err := filepath.Abs(name)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return fmt.Errorf("OUTPUT_PARENT_UNAVAILABLE")
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	if sourceRoot != "" {
		root, err := filepath.EvalSymlinks(sourceRoot)
		if err != nil {
			return err
		}
		root, err = filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("SOURCE_ROOT_UNAVAILABLE")
		}
		rel, err := filepath.Rel(root, abs)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("OUTPUT_MUST_BE_OUTSIDE_SOURCE_ROOT")
		}
	}
	if _, err = os.Lstat(abs); !os.IsNotExist(err) {
		return fmt.Errorf("OUTPUT_ALREADY_EXISTS_OR_UNREADABLE")
	}
	tmp, err := os.CreateTemp(filepath.Dir(abs), ".copypaste-import-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err = tmp.Chmod(0600); err != nil {
		return err
	}
	if err = fn(tmp); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	// Link is atomic and refuses an existing destination. There is no overwrite
	// race between a preflight check and rename on Unix.
	if err = os.Link(tmp.Name(), abs); err != nil {
		return fmt.Errorf("OUTPUT_PUBLISH_FAILED: %w", err)
	}
	return nil
}
