package files

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const DownloadPermitTTL = 12 * time.Hour
const DownloadRangeBytes int64 = 32 << 20

type DownloadReader interface {
	Read(context.Context, objectstore.Source, int64, int64) (objectstore.ReadResult, error)
}
type DownloadBackend struct {
	Provider, Bucket string
	Signer           objectstore.DownloadSigner
	Reader           DownloadReader
}
type DownloadService struct {
	Files        *Service
	Backends     map[string]*DownloadBackend
	PublicOrigin string
}
type DownloadRequest struct {
	Disposition string `json:"disposition"`
}
type DownloadResult struct {
	ID              string         `json:"id"`
	Mode            string         `json:"mode"`
	URL             string         `json:"url"`
	Method          string         `json:"method"`
	RequiredHeaders []SignedHeader `json:"required_headers"`
	ExpiresAt       time.Time      `json:"expires_at"`
	PermitExpiresAt time.Time      `json:"permit_expires_at"`
	VersionID       string         `json:"file_version_id"`
	Filename        string         `json:"filename"`
	SizeBytes       string         `json:"size_bytes"`
	SHA256          string         `json:"sha256"`
	ScanState       string         `json:"scan_state"`
	Disposition     string         `json:"disposition"`
	MaxRangeBytes   string         `json:"max_range_bytes"`
	Warnings        []string       `json:"warnings"`
}

// DownloadPermit is private to trusted domain/HTTP code. It is not a response DTO.
type DownloadPermit struct {
	ID, FileID, VersionID, ReplicaID, Provider, Bucket, Filename, Classification, Mode, Disposition string
	Source                                                                                          objectstore.Source
	ExpiresAt                                                                                       time.Time
	Size                                                                                            int64
	SHA256, MediaType, ScanState                                                                    string
}
type downloadVersionRow struct {
	File                                                     File
	ID, Filename, SHA256, MediaType, ScanState, Availability string
	Size                                                     int64
	Waiver                                                   *string
}

func manualDownload(a core.Actor) error {
	if a.RunID != "" || a.SessionID == "" || (a.Channel != "web" && a.Channel != "android" && a.Channel != "wechat" && a.Channel != "douyin") {
		return core.Forbidden()
	}
	return nil
}
func currentDownloadActor(ctx context.Context, db platform.DBTX, a core.Actor) error {
	if err := manualDownload(a); err != nil {
		return err
	}
	var current bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions s
        JOIN users u ON u.org_id=s.org_id AND u.id=s.user_id JOIN organizations o ON o.id=s.org_id
        WHERE s.org_id=$1 AND s.id=$2 AND s.user_id=$3 AND s.device_type=$4
        AND s.revoked_at IS NULL AND s.expires_at>clock_timestamp() AND u.status='active'
        AND s.auth_version=u.auth_version AND u.auth_version=$5 AND o.authorization_version=$6)`,
		a.OrgID, a.SessionID, a.UserID, a.Channel, a.AuthVersion, a.OrgAuthVersion).Scan(&current)
	if err != nil {
		return err
	}
	if !current {
		return core.Fail(412, "AUTHORIZATION_CHANGED", "权限或会话已变更，请重新获取下载授权")
	}
	return nil
}
func downloadVersion(ctx context.Context, db platform.DBTX, a core.Actor, id, disposition string) (downloadVersionRow, error) {
	var v downloadVersionRow
	if !core.ValidID(id) {
		return v, core.Missing()
	}
	err := db.QueryRow(ctx, `SELECT id,file_id,coalesce(original_name,''),sha256,media_type,scan_state,availability,size_bytes,scan_waiver_ref
        FROM file_versions WHERE org_id=$1 AND id=$2`, a.OrgID, id).Scan(&v.ID, &v.File.ID, &v.Filename, &v.SHA256, &v.MediaType, &v.ScanState, &v.Availability, &v.Size, &v.Waiver)
	if err != nil {
		return v, platform.DatabaseError(err)
	}
	v.File, err = authorizeFile(ctx, db, a, v.File.ID, "file.read")
	if err != nil {
		return v, err
	}
	if v.File.State != "active" {
		return v, core.Fail(409, "FILE_NOT_ACTIVE", "文件已在回收站，请先按流程恢复")
	}
	if v.Availability != "available" || (v.ScanState != "clean" && (v.ScanState != "waived" || v.Waiver == nil)) {
		return v, core.Fail(409, "FILE_NOT_DOWNLOADABLE", "此版本尚未获得可用内容和扫描或放行证据")
	}
	if disposition != "attachment" && disposition != "inline" {
		return v, core.Invalid("disposition", "下载方式无效")
	}
	if disposition == "inline" && (v.ScanState != "clean" || !objectstore.InlineMediaType(v.MediaType)) {
		return v, core.Fail(422, "INLINE_PREVIEW_UNAVAILABLE", "此格式或扫描状态不支持直接预览，请下载后按公司流程查阅")
	}
	if v.Filename == "" {
		v.Filename = v.File.Name
	}
	if _, _, err = (objectstore.ReadOptions{Filename: v.Filename, Disposition: disposition, MediaType: v.MediaType}).Headers(); err != nil {
		return v, core.Fail(409, "DOWNLOAD_METADATA_INVALID", "文件下载元信息不完整")
	}
	return v, nil
}
func (s *DownloadService) downloadSource(ctx context.Context, db platform.DBTX, a core.Actor, v downloadVersionRow) (replicaRow, objectstore.Source, error) {
	var chosen replicaRow
	var source objectstore.Source
	rows, err := db.Query(ctx, `SELECT r.id FROM file_replicas r
        LEFT JOIN storage_epochs e ON e.org_id=r.org_id AND e.epoch=(SELECT max(epoch) FROM storage_epochs WHERE org_id=r.org_id)
        WHERE r.org_id=$1 AND r.version_id=$2 AND r.state='verified' AND r.expected_sha256=$3 AND r.actual_sha256=$3
        AND r.verified_at IS NOT NULL AND r.source_id IS NOT NULL
        ORDER BY (r.provider=e.active_primary) DESC NULLS LAST,r.provider,r.id`, a.OrgID, v.ID, v.SHA256)
	if err != nil {
		return chosen, source, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return chosen, source, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return chosen, source, err
	}
	for _, id := range ids {
		r, er := replica(ctx, db, a.OrgID, id, false)
		if er != nil {
			return chosen, source, er
		}
		backend := s.Backends[r.Provider]
		if backend == nil || backend.Provider != r.Provider || backend.Bucket != r.Bucket || backend.Reader == nil || backend.Signer == nil {
			continue
		}
		var enabled bool
		if er = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM storage_targets WHERE org_id=$1 AND provider=$2 AND bucket=$3 AND enabled)", a.OrgID, r.Provider, r.Bucket).Scan(&enabled); er != nil {
			return chosen, source, er
		}
		if !enabled || !downloadSourceEvidence(a.OrgID, v, r, &source) {
			continue
		}
		return r, source, nil
	}
	return chosen, source, core.Fail(503, "DOWNLOAD_SOURCE_UNAVAILABLE", "没有已配置且有完整校验证据的下载副本")
}
func downloadSourceEvidence(org string, v downloadVersionRow, r replicaRow, source *objectstore.Source) bool {
	key, err := objectstore.VersionKey(org, v.File.ID, v.ID)
	if err != nil || r.State != "verified" || r.Expected != v.SHA256 || r.ActualSHA256 == nil || *r.ActualSHA256 != v.SHA256 || r.VerifiedAt == nil ||
		r.SourceRef == nil || json.Unmarshal([]byte(*r.SourceRef), source) != nil || source.Key != r.Key || source.Size != v.Size || objectstore.ValidateDownloadSource(*source) != nil {
		return false
	}
	// A verified row still cannot redirect a grant into a draft or another version.
	prefix := strings.TrimSuffix(key, "original") + "replicas/"
	return r.Key == key || (strings.HasPrefix(r.Key, prefix) && strings.HasSuffix(r.Key, "/original"))
}
func (s *DownloadService) Create(ctx context.Context, c core.Command, id string, in DownloadRequest) (core.Result, error) {
	if err := manualDownload(c.Actor); err != nil {
		return core.Result{}, err
	}
	authorize := func(tx pgx.Tx) error {
		if err := currentDownloadActor(ctx, tx, c.Actor); err != nil {
			return err
		}
		_, err := downloadVersion(ctx, tx, c.Actor, id, in.Disposition)
		return err
	}
	result, err := s.Files.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		v, err := downloadVersion(ctx, tx, c.Actor, id, in.Disposition)
		if err != nil {
			return core.Result{}, err
		}
		if err = replicaNamespace(ctx, tx, c.Actor.OrgID, v.File.ProjectID); err != nil {
			return core.Result{}, err
		}
		if err = authorize(tx); err != nil {
			return core.Result{}, err
		}
		v, err = downloadVersion(ctx, tx, c.Actor, id, in.Disposition)
		if err != nil {
			return core.Result{}, err
		}
		r, source, err := s.downloadSource(ctx, tx, c.Actor, v)
		if err != nil {
			return core.Result{}, err
		}
		mode := "signed_url"
		if v.File.Classification == "confidential" || c.Actor.Channel == "wechat" || c.Actor.Channel == "douyin" {
			mode = "authenticated_proxy"
		}
		permitID := core.NewID()
		raw, _ := json.Marshal(source)
		var family string
		if err = tx.QueryRow(ctx, "SELECT family_id FROM sessions WHERE org_id=$1 AND user_id=$2 AND id=$3", c.Actor.OrgID, c.Actor.UserID, c.Actor.SessionID).Scan(&family); err != nil {
			return core.Result{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO download_permits(id,org_id,user_id,session_id,file_id,version_id,replica_id,provider,bucket,source,filename,
            classification,mode,disposition,channel,auth_version,org_auth_version,session_family_id,expires_at)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,now()+interval '12 hours')`,
			permitID, c.Actor.OrgID, c.Actor.UserID, c.Actor.SessionID, v.File.ID, id, r.ID, r.Provider, r.Bucket, raw, v.Filename,
			v.File.Classification, mode, in.Disposition, c.Actor.Channel, c.Actor.AuthVersion, c.Actor.OrgAuthVersion, family)
		return core.Result{Data: map[string]string{"permit_id": permitID}, Event: core.Event{Type: "download.permitted", ResourceType: "file_version", ResourceID: id,
			Version: 1, Payload: map[string]any{"permit_id": permitID, "file_id": v.File.ID, "replica_id": r.ID, "mode": mode, "disposition": in.Disposition}}}, err
	})
	if err != nil {
		return core.Result{}, err
	}
	// Replays mint a fresh short signature only after live authorization. The
	// persisted command result and its audit digest contain only a permit ID.
	raw, _ := json.Marshal(result.Data)
	var ref struct {
		ID string `json:"permit_id"`
	}
	if err = json.Unmarshal(raw, &ref); err != nil || !core.ValidID(ref.ID) {
		return core.Result{}, core.Fail(500, "DOWNLOAD_PERMIT_INVALID", "下载授权记录无效")
	}
	p, err := s.Resolve(ctx, c.Actor, ref.ID)
	if err != nil {
		return core.Result{}, err
	}
	out := DownloadResult{ID: p.ID, Mode: p.Mode, Method: "GET", RequiredHeaders: []SignedHeader{}, ExpiresAt: p.ExpiresAt, PermitExpiresAt: p.ExpiresAt,
		VersionID: p.VersionID, Filename: p.Filename, SizeBytes: strconv.FormatInt(p.Size, 10), SHA256: p.SHA256, ScanState: p.ScanState,
		Disposition: p.Disposition, MaxRangeBytes: strconv.FormatInt(DownloadRangeBytes, 10), Warnings: []string{}}
	if p.ScanState == "waived" {
		out.Warnings = append(out.Warnings, "SCAN_WAIVED_NOT_CLEAN")
	}
	if p.Mode == "signed_url" {
		ttl := objectstore.DownloadSignTTL
		if remaining := time.Until(p.ExpiresAt); remaining < ttl {
			ttl = remaining
		}
		if ttl < time.Second {
			return core.Result{}, core.Fail(410, "DOWNLOAD_EXPIRED", "下载授权已过期，请重新申请")
		}
		signed, er := s.Backends[p.Provider].Signer.SignRead(ctx, p.Source, objectstore.ReadOptions{Filename: p.Filename, Disposition: p.Disposition, MediaType: p.MediaType}, ttl)
		if er != nil {
			return core.Result{}, cloudError(er)
		}
		out.URL, out.ExpiresAt = signed.URL, signed.ExpiresAt
		for name, value := range signed.Headers {
			out.RequiredHeaders = append(out.RequiredHeaders, SignedHeader{Name: name, Value: value})
		}
		out.Warnings = append(out.Warnings, "SIGNED_URL_REVOCATION_DELAY")
	} else {
		out.URL = strings.TrimRight(s.PublicOrigin, "/") + "/v1/downloads/" + p.ID + "/content"
	}
	if _, err = s.Resolve(ctx, c.Actor, p.ID); err != nil {
		return core.Result{}, err
	}
	result.Data = out
	return result, nil
}
func (s *DownloadService) Resolve(ctx context.Context, a core.Actor, id string) (DownloadPermit, error) {
	return s.resolve(ctx, s.Files.DB, a, id)
}
func (s *DownloadService) resolve(ctx context.Context, db platform.DBTX, a core.Actor, id string) (DownloadPermit, error) {
	var p DownloadPermit
	if !core.ValidID(id) {
		return p, core.Missing()
	}
	if err := currentDownloadActor(ctx, db, a); err != nil {
		return p, err
	}
	var raw []byte
	var user, family, channel string
	var userVersion, orgVersion int64
	err := db.QueryRow(ctx, `SELECT id,file_id,version_id,replica_id,provider,bucket,source,filename,classification,mode,disposition,expires_at,
        user_id,session_family_id,channel,auth_version,org_auth_version FROM download_permits WHERE org_id=$1 AND id=$2`, a.OrgID, id).Scan(
		&p.ID, &p.FileID, &p.VersionID, &p.ReplicaID, &p.Provider, &p.Bucket, &raw, &p.Filename, &p.Classification, &p.Mode, &p.Disposition, &p.ExpiresAt,
		&user, &family, &channel, &userVersion, &orgVersion)
	if err != nil {
		return p, platform.DatabaseError(err)
	}
	var currentFamily string
	if err = db.QueryRow(ctx, "SELECT family_id FROM sessions WHERE org_id=$1 AND id=$2 AND user_id=$3", a.OrgID, a.SessionID, a.UserID).Scan(&currentFamily); err != nil {
		return p, platform.DatabaseError(err)
	}
	// Normal rotation stays in one authenticated device family. A new login,
	// logout/replay revocation, another user or another channel cannot reuse it.
	if user != a.UserID || family != currentFamily || channel != a.Channel {
		return p, core.Missing()
	}
	if userVersion != a.AuthVersion || orgVersion != a.OrgAuthVersion {
		return p, core.Fail(412, "AUTHORIZATION_CHANGED", "原下载授权的权限版本已失效，请重新申请")
	}
	if !p.ExpiresAt.After(time.Now()) {
		return p, core.Fail(410, "DOWNLOAD_EXPIRED", "下载授权已过期，请重新申请")
	}
	v, err := downloadVersion(ctx, db, a, p.VersionID, p.Disposition)
	if err != nil {
		return p, err
	}
	if v.File.Classification != p.Classification || v.File.ID != p.FileID || (p.Classification == "confidential" && p.Mode != "authenticated_proxy") {
		return p, core.Fail(412, "DOWNLOAD_SCOPE_CHANGED", "文件分类或范围已变化，请重新获取下载授权")
	}
	r, err := replica(ctx, db, a.OrgID, p.ReplicaID, false)
	if err != nil {
		return p, err
	}
	var current objectstore.Source
	if json.Unmarshal(raw, &p.Source) != nil || !downloadSourceEvidence(a.OrgID, v, r, &current) || p.Source != current || r.VersionID != p.VersionID || r.Provider != p.Provider || r.Bucket != p.Bucket {
		return p, core.Fail(409, "DOWNLOAD_SOURCE_CHANGED", "固定下载副本已变化，请重新获取授权")
	}
	backend := s.Backends[p.Provider]
	if backend == nil || backend.Provider != p.Provider || backend.Bucket != p.Bucket || backend.Reader == nil || backend.Signer == nil {
		return p, core.Fail(503, "DOWNLOAD_STORAGE_UNAVAILABLE", "固定下载存储用途尚未配置")
	}
	var enabled bool
	if err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM storage_targets WHERE org_id=$1 AND provider=$2 AND bucket=$3 AND enabled)", a.OrgID, p.Provider, p.Bucket).Scan(&enabled); err != nil {
		return p, err
	}
	if !enabled {
		return p, core.Fail(503, "DOWNLOAD_STORAGE_UNAVAILABLE", "固定下载存储已停止提供访问")
	}
	p.Size, p.SHA256, p.MediaType, p.ScanState = v.Size, v.SHA256, v.MediaType, v.ScanState
	return p, nil
}

// StartRead commits an audit before opening any provider stream. No transaction
// spans cloud I/O. Archived projects remain readable under current file ACL.
func (s *DownloadService) StartRead(ctx context.Context, a core.Actor, id, request string, offset, length int64) (DownloadPermit, string, error) {
	var p DownloadPermit
	tx, err := s.Files.DB.Begin(ctx)
	if err != nil {
		return p, "", err
	}
	defer tx.Rollback(ctx)
	var org string
	if err = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&org); err != nil {
		return p, "", err
	}
	p, err = s.resolve(ctx, tx, a, id)
	if err != nil {
		return p, "", err
	}
	f, err := file(ctx, tx, a.OrgID, p.FileID, false)
	if err != nil {
		return p, "", err
	}
	if err = replicaNamespace(ctx, tx, a.OrgID, f.ProjectID); err != nil {
		return p, "", err
	}
	p, err = s.resolve(ctx, tx, a, id)
	if err != nil {
		return p, "", err
	}
	if p.Mode != "authenticated_proxy" || offset < 0 || length < 0 || length > DownloadRangeBytes || offset > p.Size || length > p.Size-offset {
		return p, "", core.Invalid("Range", "读取范围无效")
	}
	readID := core.NewID()
	if err = downloadReadEvent(ctx, tx, a, p, readID, request, "started", "READ_STARTED", offset, length, 0); err != nil {
		return p, "", err
	}
	return p, readID, tx.Commit(ctx)
}
func (s *DownloadService) FinishRead(ctx context.Context, a core.Actor, p DownloadPermit, id, request, stage, code string, offset, length, written int64) error {
	tx, err := s.Files.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = downloadReadEvent(ctx, tx, a, p, id, request, stage, code, offset, length, written); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func downloadReadEvent(ctx context.Context, tx pgx.Tx, a core.Actor, p DownloadPermit, id, request, stage, code string, offset, length, written int64) error {
	if _, err := tx.Exec(ctx, `INSERT INTO download_read_events(id,org_id,permit_id,read_id,stage,range_offset,range_length,bytes_written,result_code,request_id)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, core.NewID(), a.OrgID, p.ID, id, stage, offset, length, written, code, request); err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]any{"read_id": id, "permit_id": p.ID, "stage": stage, "offset": offset, "length": length, "bytes_written": written, "code": code})
	_, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result,reason,session_id)
        VALUES($1,$2,$3,$3,$4,$5,$6,'file_version',$7,1,$8,$9,$10,$11)`,
		core.NewID(), a.OrgID, a.UserID, a.Channel, request, "download.read."+stage, p.VersionID, core.Digest(raw),
		map[bool]string{true: "succeeded", false: "failed"}[stage == "started" || stage == "completed"], code, a.SessionID)
	return err
}
