package files

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/objectstore"
	"github.com/jackc/pgx/v5"
)

type OfficeCreate struct {
	ExpectedVersion int64          `json:"expected_version"`
	Name            string         `json:"name"`
	Document        OfficeDocument `json:"document"`
}
type OfficeEdit struct {
	ExpectedVersion int64          `json:"expected_version"`
	BaseVersionID   string         `json:"base_version_id"`
	SourceSHA256    string         `json:"source_sha256"`
	WorkspaceSHA256 string         `json:"workspace_sha256"`
	Changes         []OfficeChange `json:"changes"`
}
type OfficeRead struct {
	File         File            `json:"file"`
	VersionID    string          `json:"version_id"`
	SourceSHA256 string          `json:"source_sha256"`
	Workspace    OfficeWorkspace `json:"workspace"`
}
type OfficeArtifact struct {
	Name          string          `json:"name"`
	MediaType     string          `json:"media_type"`
	SizeBytes     string          `json:"size_bytes"`
	SHA256        string          `json:"sha256"`
	ContentBase64 string          `json:"content_base64"`
	Upload        UploadCreate    `json:"upload"`
	Workspace     OfficeWorkspace `json:"workspace"`
}
type OfficeService struct {
	Files    *Service
	Backends map[string]*DownloadBackend
}
type officeBasis struct {
	file        File
	version     downloadVersionRow
	replica     replicaRow
	source      objectstore.Source
	fingerprint string
	reader      DownloadReader
}

func officeInteractive(a core.Actor, write bool) error {
	if err := manualDownload(a); err != nil {
		return err
	}
	if write {
		return uploadChannel(a)
	}
	return nil
}
func (s *OfficeService) basis(ctx context.Context, tx pgx.Tx, a core.Actor, id, version string, edit *OfficeEdit) (officeBasis, error) {
	var b officeBasis
	v, err := downloadVersion(ctx, tx, a, version, "attachment")
	if err != nil {
		return b, err
	}
	if v.File.ID != id {
		return b, core.Missing()
	}
	if v.ScanState != "clean" || v.Size < 1 || v.Size > OfficeAuthorMaxSource || officeAuthorKind(v.MediaType) == "" {
		return b, officeAuthorError("SOURCE", "编辑需要扫描通过、8MiB以内的DOCX、XLSX或PPTX准确版本")
	}
	if edit != nil {
		if _, err = authorizeFile(ctx, tx, a, id, "file.update"); err != nil {
			return b, err
		}
		if _, err = authorizeFolder(ctx, tx, a, v.File.FolderID, "file.upload"); err != nil {
			return b, err
		}
		if !writable(ctx, tx, a.OrgID, v.File.ProjectID) {
			return b, core.Fail(409, "PROJECT_READ_ONLY", "项目不允许更新文件")
		}
		if v.File.Version != edit.ExpectedVersion || v.File.CurrentVersionID == nil || *v.File.CurrentVersionID != version || v.SHA256 != edit.SourceSHA256 {
			return b, core.Fail(409, "OFFICE_BASIS_CHANGED", "文件已有新版本或已移动，请重新读取编辑依据")
		}
	}
	d := &DownloadService{Files: s.Files, Backends: s.Backends}
	r, source, err := d.downloadSource(ctx, tx, a, v)
	if err != nil {
		return b, err
	}
	raw, _ := json.Marshal([]any{v.File, v.ID, v.SHA256, v.ScanState, v.Size, r.ID, r.Version, source})
	b = officeBasis{v.File, v, r, source, core.Digest(raw), s.Backends[r.Provider].Reader}
	return b, nil
}
func (s *OfficeService) prepare(ctx context.Context, a core.Actor, id, version string, edit *OfficeEdit) (officeBasis, error) {
	tx, err := identity.BeginInteractiveRead(ctx, s.Files.DB, a)
	if err != nil {
		return officeBasis{}, err
	}
	defer tx.Rollback(ctx)
	return s.basis(ctx, tx, a, id, version, edit)
}
func readOfficeSource(ctx context.Context, b officeBasis) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := b.reader.Read(ctx, b.source, 0, b.version.Size)
	if err != nil {
		return nil, core.Fail(503, "OFFICE_SOURCE_UNAVAILABLE", "暂时无法读取准确版本")
	}
	if out.Body == nil {
		return nil, officeAuthorError("SOURCE", "文件来源缺少正文")
	}
	defer out.Body.Close()
	stop := context.AfterFunc(ctx, func() { _ = out.Body.Close() })
	defer stop()
	if out.Size != b.version.Size || out.ETag != b.source.ETag || b.source.VersionID != "" && out.VersionID != b.source.VersionID {
		return nil, officeAuthorError("SOURCE", "来源对象证据不一致")
	}
	body, err := io.ReadAll(io.LimitReader(out.Body, b.version.Size+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || int64(len(body)) != b.version.Size || core.Digest(body) != b.version.SHA256 {
		return nil, officeAuthorError("SOURCE", "文件长度或完整摘要不一致")
	}
	return body, nil
}
func (s *OfficeService) Read(ctx context.Context, a core.Actor, id, version string) (OfficeRead, error) {
	var out OfficeRead
	if err := officeInteractive(a, false); err != nil {
		return out, err
	}
	b, err := s.prepare(ctx, a, id, version, nil)
	if err != nil {
		return out, err
	}
	body, err := readOfficeSource(ctx, b)
	if err != nil {
		return out, err
	}
	w, err := OfficeDocumentWorkspace(ctx, officeAuthorKind(b.version.MediaType), body)
	if err != nil {
		return out, err
	}
	current, err := s.prepare(ctx, a, id, version, nil)
	if err != nil {
		return out, err
	}
	if current.fingerprint != b.fingerprint {
		return out, core.Fail(409, "OFFICE_BASIS_CHANGED", "读取期间文件或来源已改变，请重试")
	}
	f, err := presentFile(ctx, s.Files.DB, a, current.file)
	if err != nil {
		return out, err
	}
	return OfficeRead{f, version, b.version.SHA256, w}, nil
}
func officeArtifact(name string, body []byte, w OfficeWorkspace, input UploadCreate) OfficeArtifact {
	hash := core.Digest(body)
	input.Name = name
	input.SizeBytes = strconv.Itoa(len(body))
	input.MediaType = officeAuthorMedia(w.Kind)
	input.SourceFingerprint = "office-artifact-v1:" + hash
	return OfficeArtifact{name, input.MediaType, input.SizeBytes, hash, base64.StdEncoding.EncodeToString(body), input, w}
}
func (s *OfficeService) createTarget(ctx context.Context, a core.Actor, id string, in OfficeCreate) (Folder, error) {
	tx, err := identity.BeginInteractiveRead(ctx, s.Files.DB, a)
	if err != nil {
		return Folder{}, err
	}
	defer tx.Rollback(ctx)
	f, err := authorizeFolder(ctx, tx, a, id, "file.upload")
	if err != nil {
		return f, err
	}
	if f.Version != in.ExpectedVersion || !writable(ctx, tx, a.OrgID, f.ProjectID) {
		return f, core.Fail(409, "OFFICE_BASIS_CHANGED", "目录已变化或项目只读，请重新获取目录")
	}
	return f, nil
}
func (s *OfficeService) Create(ctx context.Context, a core.Actor, id string, in OfficeCreate) (OfficeArtifact, error) {
	var out OfficeArtifact
	if err := officeInteractive(a, true); err != nil {
		return out, err
	}
	if err := core.Name(in.Name); err != nil {
		return out, err
	}
	name := in.Name
	if !strings.HasSuffix(strings.ToLower(name), "."+in.Document.Kind) {
		name += "." + in.Document.Kind
	}
	if err := core.Name(name); err != nil {
		return out, err
	}
	if _, err := core.StorageNameKey(name); err != nil {
		return out, err
	}
	f, err := s.createTarget(ctx, a, id, in)
	if err != nil {
		return out, err
	}
	body, w, err := CreateOfficeDocument(ctx, in.Document)
	if err != nil {
		return out, err
	}
	current, err := s.createTarget(ctx, a, id, in)
	if err != nil {
		return out, err
	}
	if digest(f) != digest(current) {
		return out, core.Fail(409, "OFFICE_BASIS_CHANGED", "生成期间目录已改变")
	}
	return officeArtifact(name, body, w, UploadCreate{FolderID: id}), nil
}
func (s *OfficeService) Edit(ctx context.Context, a core.Actor, id string, in OfficeEdit) (OfficeArtifact, error) {
	var out OfficeArtifact
	if err := officeInteractive(a, true); err != nil {
		return out, err
	}
	b, err := s.prepare(ctx, a, id, in.BaseVersionID, &in)
	if err != nil {
		return out, err
	}
	body, err := readOfficeSource(ctx, b)
	if err != nil {
		return out, err
	}
	body, w, err := EditOfficeDocument(ctx, officeAuthorKind(b.version.MediaType), body, in.WorkspaceSHA256, in.Changes)
	if err != nil {
		return out, err
	}
	current, err := s.prepare(ctx, a, id, in.BaseVersionID, &in)
	if err != nil {
		return out, err
	}
	if current.fingerprint != b.fingerprint {
		return out, core.Fail(409, "OFFICE_BASIS_CHANGED", "编辑期间文件或来源已改变")
	}
	return officeArtifact(b.file.Name, body, w, UploadCreate{FolderID: b.file.FolderID, FileID: &id, BaseVersionID: &in.BaseVersionID}), nil
}
