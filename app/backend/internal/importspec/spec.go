// Package importspec defines the bounded, deterministic native import protocol.
package importspec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
)

const MaxEntries = 1000000
const MaxFileBytes int64 = 120000000000
const ZeroChain = "0000000000000000000000000000000000000000000000000000000000000000"

type BatchInput struct {
	SourceNamespace string `json:"source_namespace"`
	InventorySHA256 string `json:"inventory_sha256"`
	MappingSHA256   string `json:"mapping_sha256"`
	PlanSHA256      string `json:"plan_sha256"`
	EntryCount      int    `json:"entry_count"`
	TotalBytes      string `json:"total_bytes"`
	Reason          string `json:"reason"`
}

type Entry struct {
	Ordinal          int     `json:"ordinal"`
	SourcePath       string  `json:"source_path"`
	SourceKey        string  `json:"source_key"`
	SourceSHA256     string  `json:"source_sha256"`
	SizeBytes        string  `json:"size_bytes"`
	SourceModifiedAt string  `json:"source_modified_at"`
	SourceMode       string  `json:"source_mode"`
	SourceOwner      string  `json:"source_owner"`
	MappedOwnerID    string  `json:"mapped_owner_id"`
	FolderID         string  `json:"folder_id"`
	FolderVersion    int64   `json:"folder_version"`
	ProjectID        *string `json:"project_id"`
	Classification   string  `json:"classification"`
	Name             string  `json:"name"`
	MediaType        string  `json:"media_type"`
	FileID           *string `json:"file_id,omitempty"`
	BaseVersionID    *string `json:"base_version_id,omitempty"`
}

type ApprovalInput struct {
	PlanSHA256       string `json:"plan_sha256"`
	ReviewedMappings bool   `json:"reviewed_mappings"`
	SourcesReadOnly  bool   `json:"sources_read_only"`
	Reason           string `json:"reason"`
}

func Hash(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func CanonicalID(value string) bool { return core.ValidID(value) && strings.ToLower(value) == value }

func RelativePath(value string) bool {
	if !utf8.ValidString(value) || value == "" || len(value) > 4096 || len([]rune(value)) > 1000 || path.Clean(value) != value || value == "." || path.IsAbs(value) {
		return false
	}
	for _, p := range strings.Split(value, "/") {
		if p == "" || p == "." || p == ".." || strings.TrimSpace(p) != p {
			return false
		}
	}
	for _, c := range value {
		if c == '\\' || c == ':' || unicode.IsControl(c) || unicode.Is(unicode.Cf, c) {
			return false
		}
	}
	return true
}

func SourceKey(namespace, relative string) string {
	return core.Digest([]byte(namespace + "\x00" + relative))
}

func Bytes(value string) (int64, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 || strconv.FormatInt(n, 10) != value {
		return 0, fmt.Errorf("noncanonical positive byte count")
	}
	return n, nil
}

func ValidateBatch(in BatchInput) error {
	n, err := Bytes(in.TotalBytes)
	if !CanonicalID(in.SourceNamespace) || !Hash(in.InventorySHA256) || !Hash(in.MappingSHA256) || !Hash(in.PlanSHA256) || in.EntryCount < 1 || in.EntryCount > MaxEntries || err != nil || n > int64(in.EntryCount)*MaxFileBytes || core.Name(in.Reason) != nil {
		return core.Invalid("batch", "请明确来源命名空间、盘点／映射／计划摘要、数量、字节及原因")
	}
	return nil
}

func ValidateEntry(namespace string, in Entry) error {
	media, params, mediaErr := mime.ParseMediaType(in.MediaType)
	if mediaErr != nil || media != in.MediaType || len(params) != 0 || len(media) > 200 || strings.Contains(media, "*") {
		return core.Invalid("media_type", "请提供明确无参数的媒体类型")
	}
	n, err := Bytes(in.SizeBytes)
	if in.Ordinal < 1 || in.Ordinal > MaxEntries || !RelativePath(in.SourcePath) || in.SourceKey != SourceKey(namespace, in.SourcePath) || !Hash(in.SourceSHA256) || err != nil || n > MaxFileBytes || !CanonicalID(in.MappedOwnerID) || !CanonicalID(in.FolderID) || in.FolderVersion < 1 || !core.Classification(in.Classification) || (in.ProjectID != nil && !CanonicalID(*in.ProjectID)) || (in.FileID == nil) != (in.BaseVersionID == nil) || (in.FileID != nil && (!CanonicalID(*in.FileID) || !CanonicalID(*in.BaseVersionID))) {
		return core.Invalid("entry", "请核对来源、大小、摘要、目录版本、分类、归属及准确基础版本")
	}
	if _, err = core.StorageNameKey(in.Name); err != nil {
		return err
	}
	if _, err = time.Parse(time.RFC3339Nano, in.SourceModifiedAt); err != nil || len(in.SourceMode) > 32 || len([]rune(in.SourceOwner)) > 200 || strings.TrimSpace(in.SourceOwner) == "" {
		return core.Invalid("source", "请保留来源修改时间、权限及原始所有者说明")
	}
	return nil
}

func EntryDigest(in Entry) string {
	data, _ := json.Marshal(in)
	canonical, _ := core.CanonicalJSON(data)
	return core.Digest(canonical)
}

func Chain(previous, entryDigest string) string {
	a, _ := hex.DecodeString(previous)
	b, _ := hex.DecodeString(entryDigest)
	h := sha256.New()
	h.Write(a)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
