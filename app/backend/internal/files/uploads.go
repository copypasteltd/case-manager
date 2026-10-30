package files

import (
	"context"
	"encoding/json"
	"mime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type PartSigner interface {
	SignPart(context.Context, objectstore.Multipart, int, int64, time.Duration) (objectstore.Signed, error)
}
type PartLister interface {
	Parts(context.Context, objectstore.Multipart, string) (objectstore.PartPage, error)
}

// UploadBackend contains no cloud credentials. Signer must use a distinct
// draft-part-only cloud identity; workers receive their own operation identity.
type UploadBackend struct {
	Provider, Bucket string
	Signer           PartSigner
	Lister           PartLister
}
type UploadService struct {
	Files         *Service
	Queue         jobqueue.Queue
	Backend       *UploadBackend
	ReplicaTarget *UploadBackend
}
type UploadCreate struct {
	FolderID          string  `json:"folder_id"`
	Name              string  `json:"name"`
	SizeBytes         string  `json:"size_bytes"`
	MediaType         string  `json:"media_type"`
	FileID            *string `json:"file_id,omitempty"`
	BaseVersionID     *string `json:"base_version_id,omitempty"`
	SourceFingerprint string  `json:"source_fingerprint"`
}
type Upload struct {
	ID                                                        string          `json:"id"`
	Version                                                   int64           `json:"version"`
	CreatedAt                                                 time.Time       `json:"created_at"`
	UpdatedAt                                                 time.Time       `json:"updated_at"`
	FolderID                                                  string          `json:"folder_id"`
	FileID                                                    *string         `json:"file_id"`
	BaseVersionID                                             *string         `json:"base_version_id"`
	Name                                                      string          `json:"name"`
	MediaType                                                 string          `json:"media_type"`
	SourceFingerprint                                         string          `json:"source_fingerprint"`
	SizeBytes                                                 string          `json:"size_bytes"`
	PartBytes                                                 int64           `json:"part_bytes"`
	PartCount                                                 int             `json:"part_count"`
	ConfirmedParts                                            []int           `json:"confirmed_parts"`
	State                                                     string          `json:"state"`
	StorageEpoch                                              int64           `json:"storage_epoch"`
	ExpiresAt                                                 time.Time       `json:"expires_at"`
	SHA256                                                    *string         `json:"sha256"`
	ResultVersionID                                           *string         `json:"result_version_id"`
	ResultFileID                                              *string         `json:"result_file_id"`
	ErrorCode                                                 *string         `json:"error_code"`
	VerifiedBytes                                             string          `json:"verified_bytes"`
	ScanState                                                 string          `json:"scan_state"`
	ScanResultID                                              *string         `json:"scan_result_id"`
	ScanWaiverRef                                             *string         `json:"scan_waiver_ref"`
	OwnerID                                                   string          `json:"-"`
	Provider, Bucket, Classification                          *string         `json:"-"`
	SourceChannel                                             *string         `json:"-"`
	UploadID, FrozenSourceID, FrozenETag, FrozenNativeVersion *string         `json:"-"`
	DraftKey                                                  string          `json:"-"`
	Manifest                                                  json.RawMessage `json:"-"`
}
type UploadExpected struct {
	ExpectedVersion int64 `json:"expected_version"`
}
type SignParts struct {
	ExpectedVersion int64 `json:"expected_version"`
	PartNumbers     []int `json:"part_numbers"`
}
type AckPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
	SizeBytes  string `json:"size_bytes"`
}
type AckParts struct {
	ExpectedVersion int64     `json:"expected_version"`
	Parts           []AckPart `json:"parts"`
}
type ResumeUpload struct {
	ExpectedVersion   int64  `json:"expected_version"`
	SourceFingerprint string `json:"source_fingerprint"`
}
type SignedPart struct {
	PartNumber int            `json:"part_number"`
	Method     string         `json:"method"`
	URL        string         `json:"url"`
	Headers    []SignedHeader `json:"headers"`
	ExpiresAt  time.Time      `json:"expires_at"`
}
type SignedHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type SignedParts struct {
	Items []SignedPart `json:"items"`
}

const uploadColumns = `u.id,u.version,u.created_at,u.updated_at,u.target_folder_id,u.file_id,u.base_version_id,
u.name,u.media_type,u.source_fingerprint,u.size_bytes::text,u.part_size,u.part_count,u.state,
u.storage_epoch,u.expires_at,u.final_digest,u.committed_version_id,u.last_error,u.owner_id,
u.provider,u.bucket,u.classification,u.provider_upload_id,u.draft_key,u.frozen_source_id,u.frozen_etag,u.frozen_native_version,u.finalize_manifest,u.source_channel,
u.verified_bytes::text,u.scan_state,u.scan_result_ref,u.scan_waiver_ref,
(SELECT v.file_id FROM file_versions v WHERE v.org_id=u.org_id AND v.id=u.committed_version_id)`

func upload(ctx context.Context, db platform.DBTX, org, id string, lock bool) (Upload, error) {
	u := Upload{ConfirmedParts: []int{}}
	if !core.ValidID(id) {
		return u, core.Missing()
	}
	query := "SELECT " + uploadColumns + " FROM upload_sessions u WHERE u.org_id=$1 AND u.id=$2"
	if lock {
		query += " FOR UPDATE OF u"
	}
	err := db.QueryRow(ctx, query, org, id).Scan(&u.ID, &u.Version, &u.CreatedAt, &u.UpdatedAt, &u.FolderID, &u.FileID, &u.BaseVersionID,
		&u.Name, &u.MediaType, &u.SourceFingerprint, &u.SizeBytes, &u.PartBytes, &u.PartCount, &u.State,
		&u.StorageEpoch, &u.ExpiresAt, &u.SHA256, &u.ResultVersionID, &u.ErrorCode, &u.OwnerID,
		&u.Provider, &u.Bucket, &u.Classification, &u.UploadID, &u.DraftKey, &u.FrozenSourceID, &u.FrozenETag, &u.FrozenNativeVersion, &u.Manifest, &u.SourceChannel,
		&u.VerifiedBytes, &u.ScanState, &u.ScanResultID, &u.ScanWaiverRef, &u.ResultFileID)
	if u.ScanState == "quarantined" {
		u.ScanState = "blocked"
	}
	return u, platform.DatabaseError(err)
}
func confirmed(ctx context.Context, db platform.DBTX, u *Upload) error {
	rows, err := db.Query(ctx, "SELECT part_no FROM upload_parts WHERE session_id=$1 AND cloud_confirmed ORDER BY part_no", u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		if err = rows.Scan(&n); err != nil {
			return err
		}
		u.ConfirmedParts = append(u.ConfirmedParts, n)
	}
	return rows.Err()
}
func (u Upload) layout() objectstore.Layout {
	size, _ := strconv.ParseInt(u.SizeBytes, 10, 64)
	return objectstore.Layout{Size: size, PartBytes: u.PartBytes, Count: u.PartCount}
}
func (u Upload) multipart() objectstore.Multipart {
	id := ""
	if u.UploadID != nil {
		id = *u.UploadID
	}
	return objectstore.Multipart{Key: u.DraftKey, UploadID: id}
}
func uploadChannel(a core.Actor) error {
	if (a.Channel != "web" && a.Channel != "android") || a.RunID != "" {
		return core.Fail(403, "UPLOAD_CHANNEL_UNSUPPORTED", "请使用 Web 或 Android 上传文件")
	}
	return nil
}
func uploadTarget(ctx context.Context, db platform.DBTX, a core.Actor, folderID string, fileID *string) (Folder, error) {
	f, err := authorizeFolder(ctx, db, a, folderID, "folder.read")
	if err != nil {
		return f, err
	}
	if fileID == nil {
		err = authz.Require(ctx, db, a, "file.upload", folderResource(f))
	} else {
		var target File
		target, err = authorizeFile(ctx, db, a, *fileID, "file.update")
		if err == nil && (target.FolderID != folderID || target.State != "active") {
			err = core.Fail(409, "UPLOAD_TARGET_CHANGED", "目标文件位置或状态已改变")
		}
	}
	return f, err
}
func (s *UploadService) authorize(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Upload, Folder, error) {
	if err := uploadChannel(a); err != nil {
		return Upload{}, Folder{}, err
	}
	if err := currentUploadActor(ctx, db, a); err != nil {
		return Upload{}, Folder{}, err
	}
	u, err := upload(ctx, db, a.OrgID, id, false)
	if err != nil {
		return u, Folder{}, err
	}
	if u.SourceChannel != nil && *u.SourceChannel == "agent" {
		return Upload{}, Folder{}, core.Missing()
	}
	if u.OwnerID != a.UserID {
		return Upload{}, Folder{}, core.Missing()
	}
	f, err := uploadTarget(ctx, db, a, u.FolderID, u.FileID)
	if err == nil && (u.Classification == nil || *u.Classification != f.Classification) {
		return Upload{}, Folder{}, core.Missing()
	}
	if err == nil && u.ResultFileID != nil {
		if _, err = authorizeFile(ctx, db, a, *u.ResultFileID, "file.read"); err != nil {
			return Upload{}, Folder{}, err
		}
	}
	return u, f, err
}
func currentUploadActor(ctx context.Context, db platform.DBTX, a core.Actor) error {
	if !core.ValidID(a.SessionID) {
		return core.Fail(412, "AUTHORIZATION_CHANGED", "会话或权限已改变，请重新获取授权")
	}
	var valid bool
	err := db.QueryRow(ctx, `SELECT u.status='active' AND u.auth_version=$3 AND o.authorization_version=$4
		AND s.auth_version=u.auth_version AND s.revoked_at IS NULL AND s.expires_at>now() AND s.device_type=$6
		FROM users u JOIN organizations o ON o.id=u.org_id
		JOIN sessions s ON s.org_id=u.org_id AND s.user_id=u.id AND s.id=$5
		WHERE u.org_id=$1 AND u.id=$2`, a.OrgID, a.UserID, a.AuthVersion, a.OrgAuthVersion, a.SessionID, a.Channel).Scan(&valid)
	if err != nil || !valid {
		return core.Fail(412, "AUTHORIZATION_CHANGED", "会话或权限已改变，请重新获取授权")
	}
	return nil
}
func (s *UploadService) Get(ctx context.Context, a core.Actor, id string) (Upload, error) {
	u, _, err := s.authorize(ctx, s.Files.DB, a, id)
	if err == nil {
		err = confirmed(ctx, s.Files.DB, &u)
	}
	return u, err
}
func currentEpoch(ctx context.Context, db platform.DBTX, org string) (int64, string, error) {
	var epoch int64
	var provider string
	err := db.QueryRow(ctx, "SELECT epoch,active_primary FROM storage_epochs WHERE org_id=$1 ORDER BY epoch DESC LIMIT 1", org).Scan(&epoch, &provider)
	if err == pgx.ErrNoRows {
		return 0, "", core.Fail(503, "STORAGE_NOT_CONFIGURED", "文件存储尚未配置")
	}
	return epoch, provider, err
}
func liveUpload(ctx context.Context, db platform.DBTX, a core.Actor, u Upload, expected int64, states ...string) error {
	if err := core.RequireVersion(expected); err != nil {
		return err
	}
	if u.Version != expected {
		return core.Fail(409, "VERSION_CONFLICT", "上传会话已更新，请刷新状态")
	}
	epoch, provider, err := currentEpoch(ctx, db, a.OrgID)
	if err != nil {
		return err
	}
	if u.Provider == nil || epoch != u.StorageEpoch || provider != *u.Provider {
		return core.Fail(409, "STORAGE_EPOCH_CHANGED", "存储主端已切换，请重新建立上传会话")
	}
	if !time.Now().Before(u.ExpiresAt) {
		return core.Fail(410, "UPLOAD_EXPIRED", "上传会话已过期")
	}
	for _, state := range states {
		if u.State == state {
			return nil
		}
	}
	return core.Fail(409, "UPLOAD_STATE_CONFLICT", "当前上传阶段不允许此操作")
}
func (s *UploadService) backend(u *Upload) error {
	b := s.Backend
	if b == nil || b.Signer == nil || (b.Provider != "oss" && b.Provider != "r2") || len(b.Bucket) < 3 {
		return core.Fail(503, "STORAGE_NOT_CONFIGURED", "文件存储尚未配置")
	}
	if u != nil && (u.Provider == nil || u.Bucket == nil || *u.Provider != b.Provider || *u.Bucket != b.Bucket) {
		return core.Fail(409, "STORAGE_CONFIGURATION_CHANGED", "存储配置已改变，请重新建立上传会话")
	}
	return nil
}
func uploadEvent(kind string, u Upload, extra map[string]any) core.Event {
	p := map[string]any{"session_id": u.ID, "storage_epoch": u.StorageEpoch}
	for k, v := range extra {
		p[k] = v
	}
	return core.Event{Type: kind, ResourceType: "upload", ResourceID: u.ID, Version: u.Version, Payload: p}
}
func validOpaque(s string) bool {
	if len(s) < 1 || len(s) > 200 || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}
func (s *UploadService) Create(ctx context.Context, c core.Command, in UploadCreate) (core.Result, error) {
	if err := uploadChannel(c.Actor); err != nil {
		return core.Result{}, err
	}
	if _, err := core.StorageNameKey(in.Name); err != nil {
		return core.Result{}, err
	}
	size, err := strconv.ParseInt(in.SizeBytes, 10, 64)
	if err != nil || strconv.FormatInt(size, 10) != in.SizeBytes {
		return core.Result{}, core.Invalid("size_bytes", "文件大小需为十进制整数字符串")
	}
	layout, err := objectstore.Plan(size, 0)
	if err != nil {
		return core.Result{}, core.Invalid("size_bytes", "文件大小需为1至120,000,000,000字节")
	}
	media, params, err := mime.ParseMediaType(in.MediaType)
	if err != nil || len(params) > 0 || media != in.MediaType || len(media) > 200 || strings.Contains(media, "*") {
		return core.Result{}, core.Invalid("media_type", "请提供不含参数的有效媒体类型")
	}
	if !validOpaque(in.SourceFingerprint) {
		return core.Result{}, core.Invalid("source_fingerprint", "源文件指纹无效")
	}
	if strings.HasPrefix(in.SourceFingerprint, "office-artifact-v1:") && (size > OfficeAuthorMaxOutput || !shaPattern.MatchString(strings.TrimPrefix(in.SourceFingerprint, "office-artifact-v1:"))) {
		return core.Result{}, core.Invalid("source_fingerprint", "Office候选要求12MiB以内且具有完整SHA-256")
	}
	if (in.FileID == nil) != (in.BaseVersionID == nil) {
		return core.Result{}, core.Invalid("file_id", "更新文件必须同时指定目标文件和基础版本")
	}
	var target Folder
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		var err error
		target, err = uploadTarget(ctx, tx, c.Actor, in.FolderID, in.FileID)
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		if err := s.backend(nil); err != nil {
			return core.Result{}, err
		}
		if err := namespaceLock(ctx, tx, c.Actor, target.ProjectID); err != nil {
			return core.Result{}, err
		}
		target, err = uploadTarget(ctx, tx, c.Actor, in.FolderID, in.FileID)
		if err != nil {
			return core.Result{}, err
		}
		if in.FileID != nil {
			f, err := file(ctx, tx, c.Actor.OrgID, *in.FileID, true)
			if err != nil {
				return core.Result{}, err
			}
			if !sameID(f.CurrentVersionID, in.BaseVersionID) {
				return core.Result{}, core.Fail(409, "BASE_VERSION_CONFLICT", "目标文件已有新版本")
			}
			if f.Classification != target.Classification {
				return core.Result{}, core.Fail(409, "PERMISSION_CHANGE_PLAN_REQUIRED", "文件与目录分类不一致，需先处理权限变更")
			}
		} else {
			nameKey, _ := core.StorageNameKey(in.Name)
			var collision bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM storage_entries WHERE org_id=$1 AND namespace_key=$2 AND name_key=$3)", c.Actor.OrgID, "folder/"+in.FolderID, nameKey).Scan(&collision); err != nil {
				return core.Result{}, err
			}
			if collision {
				return core.Result{}, core.Fail(409, "DUPLICATE_RESOURCE", "目标目录已有同名文件或目录")
			}
		}
		if err := reserveFileCapacity(ctx, tx, c.Actor.OrgID, size); err != nil {
			return core.Result{}, err
		}
		epoch, provider, err := currentEpoch(ctx, tx, c.Actor.OrgID)
		if err != nil {
			return core.Result{}, err
		}
		if provider != s.Backend.Provider {
			return core.Result{}, core.Fail(503, "STORAGE_CONFIGURATION_CHANGED", "存储配置与当前主端不一致")
		}
		id := core.NewID()
		key, _ := objectstore.DraftKey(c.Actor.OrgID, id)
		_, err = tx.Exec(ctx, `INSERT INTO upload_sessions(id,org_id,owner_id,target_folder_id,file_id,base_version_id,name,media_type,size_bytes,
            source_fingerprint,part_size,part_count,draft_key,storage_epoch,state,expires_at,provider,bucket,classification,source_channel)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'created',now()+interval '48 hours',$15,$16,$17,$18)`,
			id, c.Actor.OrgID, c.Actor.UserID, in.FolderID, in.FileID, in.BaseVersionID, in.Name, in.MediaType, size, in.SourceFingerprint, layout.PartBytes, layout.Count, key, epoch, provider, s.Backend.Bucket, target.Classification, c.Actor.Channel)
		if err != nil {
			return core.Result{}, err
		}
		u, err := upload(ctx, tx, c.Actor.OrgID, id, false)
		return core.Result{Status: 201, Data: u, Event: uploadEvent("upload.created", u, nil)}, err
	})
}

// mutate rechecks the target under the same namespace lock as moves/archival.
func (s *UploadService) mutate(ctx context.Context, c core.Command, id string, expected int64, states []string, fn func(pgx.Tx, Upload) (core.Result, error)) (core.Result, error) {
	return s.mutateMode(ctx, c, id, expected, states, false, fn)
}
func (s *UploadService) mutateMode(ctx context.Context, c core.Command, id string, expected int64, states []string, agent bool, fn func(pgx.Tx, Upload) (core.Result, error)) (core.Result, error) {
	var target Folder
	authorize := s.authorize
	if agent {
		authorize = s.authorizeAgent
	}
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		_, f, err := authorize(ctx, tx, c.Actor, id)
		target = f
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		if err := namespaceLock(ctx, tx, c.Actor, target.ProjectID); err != nil {
			return core.Result{}, err
		}
		_, current, err := authorize(ctx, tx, c.Actor, id)
		if err != nil {
			return core.Result{}, err
		}
		u, err := upload(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if u.Classification == nil || *u.Classification != current.Classification {
			return core.Result{}, core.Fail(409, "UPLOAD_TARGET_CHANGED", "目标目录分类已改变")
		}
		if err = liveUpload(ctx, tx, c.Actor, u, expected, states...); err != nil {
			return core.Result{}, err
		}
		return fn(tx, u)
	})
}
func (s *UploadService) Finalize(ctx context.Context, c core.Command, id string, in UploadExpected) (core.Result, error) {
	return s.mutate(ctx, c, id, in.ExpectedVersion, []string{"uploading"}, func(tx pgx.Tx, u Upload) (core.Result, error) {
		before := digest(u)
		_, err := tx.Exec(ctx, "UPDATE upload_sessions SET state='finalizing',last_error=NULL,version=version+1,updated_at=clock_timestamp() WHERE id=$1", u.ID)
		if err != nil {
			return core.Result{}, err
		}
		u, err = upload(ctx, tx, c.Actor.OrgID, id, false)
		e := uploadEvent("upload.finalize.requested", u, nil)
		e.BeforeDigest = before
		return core.Result{Status: 202, Data: u, Event: e}, err
	})
}
func (s *UploadService) Abort(ctx context.Context, c core.Command, id string, in Transition) (core.Result, error) {
	if in.Action != "abort" || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("action", "请指定 abort 并说明原因")
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, []string{"created", "uploading", "finalizing", "verifying", "blocked", "ready", "failed"}, func(tx pgx.Tx, u Upload) (core.Result, error) {
		before := digest(u)
		_, err := tx.Exec(ctx, "UPDATE upload_sessions SET state='aborted',version=version+1,updated_at=clock_timestamp() WHERE id=$1", u.ID)
		if err != nil {
			return core.Result{}, err
		}
		u, err = upload(ctx, tx, c.Actor.OrgID, id, false)
		e := uploadEvent("upload.abort.requested", u, map[string]any{"reason": in.Reason})
		e.BeforeDigest = before
		return core.Result{Data: u, Event: e}, err
	})
}
func (s *UploadService) Ack(ctx context.Context, c core.Command, id string, in AckParts) (core.Result, error) {
	if len(in.Parts) < 1 || len(in.Parts) > 100 {
		return core.Result{}, core.Invalid("parts", "每次最多回报100个分片")
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, []string{"uploading"}, func(tx pgx.Tx, u Upload) (core.Result, error) {
		seen := map[int]bool{}
		for _, p := range in.Parts {
			n, err := strconv.ParseInt(p.SizeBytes, 10, 64)
			want, e := u.layout().PartSize(p.PartNumber)
			if err != nil || e != nil || n != want || !validOpaque(p.ETag) || seen[p.PartNumber] {
				return core.Result{}, core.Invalid("parts", "分片编号、大小或 ETag 无效")
			}
			seen[p.PartNumber] = true
			if _, err = tx.Exec(ctx, `INSERT INTO upload_parts(session_id,part_no,size_bytes,provider_etag,cloud_confirmed) VALUES($1,$2,$3,$4,false)
                ON CONFLICT(session_id,part_no) DO UPDATE SET size_bytes=excluded.size_bytes,provider_etag=excluded.provider_etag,
                cloud_confirmed=upload_parts.cloud_confirmed AND upload_parts.provider_etag=excluded.provider_etag AND upload_parts.size_bytes=excluded.size_bytes,
                acknowledged_at=clock_timestamp()`, id, p.PartNumber, n, p.ETag); err != nil {
				return core.Result{}, err
			}
		}
		if err := confirmed(ctx, tx, &u); err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: u, Event: uploadEvent("upload.parts.acknowledged", u, map[string]any{"part_count": len(in.Parts)})}, nil
	})
}
func cloudParts(ctx context.Context, store PartLister, u Upload) ([]objectstore.Part, error) {
	var parts []objectstore.Part
	marker := ""
	seen := map[int]bool{}
	markers := map[string]bool{}
	for {
		page, err := store.Parts(ctx, u.multipart(), marker)
		if err != nil {
			return nil, cloudError(err)
		}
		for _, p := range page.Items {
			size, err := u.layout().PartSize(p.Number)
			if err != nil || size != p.Size || seen[p.Number] || !validOpaque(p.ETag) {
				return nil, core.Fail(409, "CLOUD_PARTS_MISMATCH", "云端分片与上传布局不一致")
			}
			seen[p.Number] = true
			parts = append(parts, p)
			if len(parts) > u.PartCount {
				return nil, core.Fail(409, "CLOUD_PARTS_MISMATCH", "云端分片超出上传布局")
			}
		}
		if page.NextMarker == "" {
			break
		}
		if page.NextMarker == marker || markers[page.NextMarker] {
			return nil, core.Fail(502, "CLOUD_PARTS_PAGINATION", "云端分片分页未能推进")
		}
		markers[page.NextMarker] = true
		marker = page.NextMarker
		if len(markers) > objectstore.MaxParts {
			return nil, core.Fail(502, "CLOUD_PARTS_PAGINATION", "云端分片分页超出上限")
		}
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].Number < parts[j].Number })
	return parts, nil
}
func (s *UploadService) checkCurrent(ctx context.Context, a core.Actor, id string, expected int64) (Upload, error) {
	u, _, err := s.authorize(ctx, s.Files.DB, a, id)
	if err == nil {
		err = liveUpload(ctx, s.Files.DB, a, u, expected, "uploading")
	}
	return u, err
}
func (s *UploadService) Sign(ctx context.Context, c core.Command, id string, in SignParts) (core.Result, error) {
	if len(in.PartNumbers) < 1 || len(in.PartNumbers) > objectstore.MaxSignBatch {
		return core.Result{}, core.Invalid("part_numbers", "每次最多签发20个分片")
	}
	permit, err := s.mutate(ctx, c, id, in.ExpectedVersion, []string{"uploading"}, func(tx pgx.Tx, u Upload) (core.Result, error) {
		if err := s.backend(&u); err != nil {
			return core.Result{}, err
		}
		seen := map[int]bool{}
		for _, n := range in.PartNumbers {
			if _, err := u.layout().PartSize(n); err != nil || seen[n] {
				return core.Result{}, core.Invalid("part_numbers", "分片编号无效或重复")
			}
			seen[n] = true
		}
		// Persist only the non-secret permit, never a bearer URL or headers.
		return core.Result{Data: map[string]any{"session_id": u.ID, "version": u.Version, "part_numbers": in.PartNumbers},
			Event: uploadEvent("upload.parts.signing_permitted", u, map[string]any{"part_count": len(in.PartNumbers)})}, nil
	})
	if err != nil {
		return core.Result{}, err
	}
	u, err := s.checkCurrent(ctx, c.Actor, id, in.ExpectedVersion)
	if err != nil {
		return core.Result{}, err
	}
	if err = s.backend(&u); err != nil {
		return core.Result{}, err
	}
	ttl := 5 * time.Minute
	if remaining := time.Until(u.ExpiresAt); remaining < ttl {
		ttl = remaining
	}
	if ttl < time.Second {
		return core.Result{}, core.Fail(410, "UPLOAD_EXPIRED", "上传会话已过期")
	}
	out := SignedParts{Items: []SignedPart{}}
	for _, n := range in.PartNumbers {
		size, err := u.layout().PartSize(n)
		if err != nil {
			return core.Result{}, core.Invalid("part_numbers", "分片编号超出布局")
		}
		signed, err := s.Backend.Signer.SignPart(ctx, u.multipart(), n, size, ttl)
		if err != nil {
			return core.Result{}, cloudError(err)
		}
		headers := []SignedHeader{}
		names := []string{}
		for name := range signed.Headers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			headers = append(headers, SignedHeader{Name: name, Value: signed.Headers[name]})
		}
		out.Items = append(out.Items, SignedPart{PartNumber: n, Method: signed.Method, URL: signed.URL, Headers: headers, ExpiresAt: signed.ExpiresAt})
	}
	// A concurrent abort/finalize or authorization change must not return a new permit.
	if _, err = s.checkCurrent(ctx, c.Actor, id, in.ExpectedVersion); err != nil {
		return core.Result{}, err
	}
	permit.Data = out
	return permit, nil
}
func (s *UploadService) Resume(ctx context.Context, c core.Command, id string, in ResumeUpload) (core.Result, error) {
	u, err := s.checkCurrent(ctx, c.Actor, id, in.ExpectedVersion)
	if err != nil {
		return core.Result{}, err
	}
	if u.SourceFingerprint != in.SourceFingerprint {
		return core.Result{}, core.Fail(409, "UPLOAD_SOURCE_MISMATCH", "所选文件与原上传源不一致")
	}
	if err = s.backend(&u); err != nil {
		return core.Result{}, err
	}
	if s.Backend.Lister == nil {
		return core.Result{}, core.Fail(503, "STORAGE_RECONCILIATION_UNAVAILABLE", "云端分片对账尚未配置")
	}
	parts, err := cloudParts(ctx, s.Backend.Lister, u)
	if err != nil {
		return core.Result{}, err
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, []string{"uploading"}, func(tx pgx.Tx, current Upload) (core.Result, error) {
		if current.UploadID == nil || u.UploadID == nil || *current.UploadID != *u.UploadID || current.DraftKey != u.DraftKey {
			return core.Result{}, core.Fail(409, "UPLOAD_SOURCE_CHANGED", "上传源已改变，请重新获取状态")
		}
		if _, err := tx.Exec(ctx, "DELETE FROM upload_parts WHERE session_id=$1", id); err != nil {
			return core.Result{}, err
		}
		for _, p := range parts {
			if _, err := tx.Exec(ctx, "INSERT INTO upload_parts(session_id,part_no,size_bytes,provider_etag,cloud_confirmed) VALUES($1,$2,$3,$4,true)", id, p.Number, p.Size, p.ETag); err != nil {
				return core.Result{}, err
			}
		}
		if err := confirmed(ctx, tx, &current); err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: current, Event: uploadEvent("upload.parts.reconciled", current, map[string]any{"part_count": len(parts)})}, nil
	})
}
func cloudError(err error) error {
	code := "STORAGE_REQUEST_FAILED"
	if e, ok := err.(*objectstore.Error); ok && e.Code != "" {
		code = e.Code
	}
	return core.Fail(502, code, "存储请求未能完成，请稍后重试或查看任务状态")
}
