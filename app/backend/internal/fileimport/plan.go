package fileimport

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"strconv"
	"strings"

	"copypaste.hk/platform/internal/importspec"
)

type Mapping struct {
	SourcePath     string  `json:"source_path"`
	SourceOwner    string  `json:"source_owner"`
	MappedOwnerID  string  `json:"mapped_owner_id"`
	FolderID       string  `json:"folder_id"`
	FolderVersion  int64   `json:"folder_version"`
	ProjectID      *string `json:"project_id"`
	Classification string  `json:"classification"`
	Name           string  `json:"name"`
	MediaType      string  `json:"media_type"`
	FileID         *string `json:"file_id,omitempty"`
	BaseVersionID  *string `json:"base_version_id,omitempty"`
}

type PlanHeader struct {
	Type     string `json:"type"`
	Protocol int    `json:"protocol"`
	importspec.BatchInput
}

type PlanRow struct {
	Type  string           `json:"type"`
	Entry importspec.Entry `json:"entry"`
}

func lineScanner(reader io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(reader)
	s.Buffer(make([]byte, 4096), 32768)
	return s
}
func decodeLine(data []byte, to any) error {
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(to); err != nil {
		return fmt.Errorf("INVALID_OR_UNKNOWN_JSON_FIELD")
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return fmt.Errorf("TRAILING_JSON")
	}
	return nil
}

func readInventoryHeader(s *bufio.Scanner) (InventoryHeader, error) {
	var header InventoryHeader
	if !s.Scan() {
		return header, fmt.Errorf("INVENTORY_HEADER_MISSING")
	}
	if err := decodeLine(s.Bytes(), &header); err != nil {
		return header, err
	}
	if header.Type != "inventory" || header.Protocol != 1 || !importspec.CanonicalID(header.SourceNamespace) {
		return header, fmt.Errorf("INVENTORY_PROTOCOL_INVALID")
	}
	return header, nil
}

func Template(inventory io.Reader, to io.Writer) error {
	s := lineScanner(inventory)
	_, err := readInventoryHeader(s)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(to)
	for s.Scan() {
		var kind struct {
			Type string `json:"type"`
		}
		if err = json.Unmarshal(s.Bytes(), &kind); err != nil {
			return fmt.Errorf("INVALID_INVENTORY")
		}
		if kind.Type == "summary" {
			var summary InventorySummary
			if err = decodeLine(s.Bytes(), &summary); err != nil {
				return err
			}
			if summary.Errors != 0 {
				return fmt.Errorf("INVENTORY_HAS_ERRORS")
			}
			if s.Scan() {
				return fmt.Errorf("TRAILING_INVENTORY_ROWS")
			}
			return s.Err()
		}
		var record InventoryRecord
		if err = decodeLine(s.Bytes(), &record); err != nil {
			return err
		}
		if record.Type != "file" {
			return fmt.Errorf("INVENTORY_HAS_ERRORS")
		}
		media := mime.TypeByExtension(path.Ext(record.Path))
		if value, _, e := mime.ParseMediaType(media); e == nil {
			media = value
		} else {
			media = "application/octet-stream"
		}
		if err = enc.Encode(Mapping{SourcePath: record.Path, SourceOwner: "UNREVIEWED", Name: path.Base(record.Path), MediaType: media}); err != nil {
			return err
		}
	}
	return fmt.Errorf("INVENTORY_SUMMARY_MISSING")
}

// BuildPlan streams matching inventory and explicit mapping rows into a bounded
// temporary spool. It publishes no partial plan after an unmapped/error row.
func BuildPlan(inventory, mapping io.Reader, spool *os.File, to io.Writer, reason string) (PlanHeader, error) {
	header := PlanHeader{Type: "plan", Protocol: 1}
	invHash, mapHash := sha256.New(), sha256.New()
	is, ms := lineScanner(hashedReader{inventory, invHash}), lineScanner(hashedReader{mapping, mapHash})
	ih, err := readInventoryHeader(is)
	if err != nil {
		return header, err
	}
	header.SourceNamespace = ih.SourceNamespace
	chain := importspec.ZeroChain
	var total int64
	count := 0
	finished := false
	enc := json.NewEncoder(spool)
	for is.Scan() {
		var kind struct {
			Type string `json:"type"`
		}
		if err = json.Unmarshal(is.Bytes(), &kind); err != nil {
			return header, fmt.Errorf("INVALID_INVENTORY")
		}
		if kind.Type == "summary" {
			var summary InventorySummary
			if err = decodeLine(is.Bytes(), &summary); err != nil {
				return header, err
			}
			if summary.Errors != 0 || summary.Files != count || summary.TotalBytes != strconv.FormatInt(total, 10) {
				return header, fmt.Errorf("INVENTORY_COUNTS_OR_ERRORS")
			}
			if is.Scan() {
				return header, fmt.Errorf("TRAILING_INVENTORY_ROWS")
			}
			finished = true
			break
		}
		var record InventoryRecord
		if err = decodeLine(is.Bytes(), &record); err != nil {
			return header, err
		}
		if record.Type != "file" || !importspec.Hash(record.SHA256) || record.ErrorCode != "" || record.SourceKey != importspec.SourceKey(ih.SourceNamespace, record.Path) {
			return header, fmt.Errorf("INVALID_OR_ERROR_INVENTORY_ROW")
		}
		if !ms.Scan() {
			return header, fmt.Errorf("MAPPING_ROW_MISSING")
		}
		var m Mapping
		if err = decodeLine(ms.Bytes(), &m); err != nil {
			return header, err
		}
		if m.SourcePath != record.Path || m.SourceOwner == "UNREVIEWED" {
			return header, fmt.Errorf("MAPPING_ORDER_OR_OWNER_UNREVIEWED")
		}
		count++
		e := importspec.Entry{Ordinal: count, SourcePath: record.Path, SourceKey: record.SourceKey, SourceSHA256: record.SHA256, SizeBytes: record.SizeBytes, SourceModifiedAt: record.ModifiedAt, SourceMode: record.Mode, SourceOwner: m.SourceOwner, MappedOwnerID: m.MappedOwnerID, FolderID: m.FolderID, FolderVersion: m.FolderVersion, ProjectID: m.ProjectID, Classification: m.Classification, Name: m.Name, MediaType: m.MediaType, FileID: m.FileID, BaseVersionID: m.BaseVersionID}
		if err = importspec.ValidateEntry(ih.SourceNamespace, e); err != nil {
			return header, fmt.Errorf("INVALID_OR_UNMAPPED_ENTRY_%d", count)
		}
		n, _ := importspec.Bytes(e.SizeBytes)
		total += n
		chain = importspec.Chain(chain, importspec.EntryDigest(e))
		if err = enc.Encode(PlanRow{Type: "entry", Entry: e}); err != nil {
			return header, err
		}
	}
	if is.Err() != nil || ms.Err() != nil {
		return header, fmt.Errorf("MANIFEST_READ_FAILED")
	}
	if !finished {
		return header, fmt.Errorf("INVENTORY_SUMMARY_MISSING")
	}
	if ms.Scan() || ms.Err() != nil {
		return header, fmt.Errorf("EXTRA_OR_INVALID_MAPPING_ROWS")
	}
	header.InventorySHA256 = hex.EncodeToString(invHash.Sum(nil))
	header.MappingSHA256 = hex.EncodeToString(mapHash.Sum(nil))
	header.PlanSHA256 = chain
	header.EntryCount = count
	header.TotalBytes = strconv.FormatInt(total, 10)
	header.Reason = reason
	if err = importspec.ValidateBatch(header.BatchInput); err != nil {
		return header, err
	}
	if _, err = spool.Seek(0, io.SeekStart); err != nil {
		return header, err
	}
	if err = json.NewEncoder(to).Encode(header); err != nil {
		return header, err
	}
	_, err = io.Copy(to, spool)
	return header, err
}

// ReadPlan validates the complete approved plan before callers perform network
// writes. Callers may reopen and stream it; no entire inventory is held in RAM.
func ReadPlan(reader io.Reader, visit func(importspec.Entry) error) (PlanHeader, error) {
	s := lineScanner(reader)
	var header PlanHeader
	if !s.Scan() {
		return header, fmt.Errorf("PLAN_HEADER_MISSING")
	}
	if err := decodeLine(s.Bytes(), &header); err != nil {
		return header, err
	}
	if header.Type != "plan" || header.Protocol != 1 {
		return header, fmt.Errorf("PLAN_PROTOCOL_INVALID")
	}
	if err := importspec.ValidateBatch(header.BatchInput); err != nil {
		return header, err
	}
	chain := importspec.ZeroChain
	var bytes int64
	count := 0
	for s.Scan() {
		var row PlanRow
		if err := decodeLine(s.Bytes(), &row); err != nil {
			return header, err
		}
		count++
		if row.Type != "entry" || row.Entry.Ordinal != count {
			return header, fmt.Errorf("PLAN_ORDER_INVALID")
		}
		if err := importspec.ValidateEntry(header.SourceNamespace, row.Entry); err != nil {
			return header, err
		}
		n, _ := importspec.Bytes(row.Entry.SizeBytes)
		bytes += n
		chain = importspec.Chain(chain, importspec.EntryDigest(row.Entry))
		if count > header.EntryCount || bytes > int64(header.EntryCount)*importspec.MaxFileBytes {
			return header, fmt.Errorf("PLAN_BOUNDS_INVALID")
		}
		if visit != nil {
			if err := visit(row.Entry); err != nil {
				return header, err
			}
		}
	}
	if s.Err() != nil {
		return header, fmt.Errorf("PLAN_READ_FAILED")
	}
	if count != header.EntryCount || strconv.FormatInt(bytes, 10) != header.TotalBytes || chain != header.PlanSHA256 {
		return header, fmt.Errorf("PLAN_DIGEST_OR_COUNTS_INVALID")
	}
	return header, nil
}
