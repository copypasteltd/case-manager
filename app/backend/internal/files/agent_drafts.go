package files

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// The model never receives this identity. It can write only isolated draft
// multipart bytes; the existing lifecycle worker completes and freezes them.
type AgentDraftWriter interface {
	Begin(context.Context, string, string) (objectstore.Multipart, error)
	WritePart(context.Context, objectstore.Multipart, int, io.Reader, int64) (objectstore.Part, error)
	Abort(context.Context, objectstore.Multipart) error
}
type AgentDraftBackend struct {
	Provider, Bucket string
	Writer           AgentDraftWriter
}

func AgentEditableFile(ctx context.Context, db platform.DBTX, a core.Actor, id, base string) (File, error) {
	if !core.ValidID(a.RunID) {
		return File{}, core.Forbidden()
	}
	f, err := authorizeFile(ctx, db, a, id, "file.update")
	if err != nil {
		return f, err
	}
	if f.State != "active" || !writable(ctx, db, a.OrgID, f.ProjectID) {
		return f, core.Fail(409, "FILE_READ_ONLY", "此文件或项目当前不允许更新")
	}
	if f.CurrentVersionID == nil || *f.CurrentVersionID != base {
		return f, core.Fail(409, "BASE_VERSION_CONFLICT", "文件已有新版本，请保留草稿并重新核对基础版本")
	}
	folder, err := authorizeFolder(ctx, db, a, f.FolderID, "folder.read")
	if err == nil && folder.Classification != f.Classification {
		err = core.Fail(409, "UPLOAD_TARGET_CHANGED", "文件与目录分类不一致")
	}
	return f, err
}

func (b *AgentDraftBackend) Reserve(ctx context.Context, tx pgx.Tx, a core.Actor, id, fileID, baseID string, size int64, sha, media string, expires time.Time) (Upload, error) {
	var out Upload
	f, err := AgentEditableFile(ctx, tx, a, fileID, baseID)
	if err != nil {
		return out, err
	}
	return b.reserve(ctx, tx, a, id, f.FolderID, &fileID, &baseID, f.Name, f.Classification, f.ProjectID, size, sha, media, expires)
}

func (b *AgentDraftBackend) ReserveCreate(ctx context.Context, tx pgx.Tx, a core.Actor, id, folderID string, version int64, name string, size int64, sha, media string, expires time.Time) (Upload, error) {
	f, err := AgentCreateTarget(ctx, tx, a, folderID, version, name, media, true)
	if err != nil {
		return Upload{}, err
	}
	if size > AgentCreateMaxBytes {
		return Upload{}, core.Invalid("content", "新建正文超过48KiB")
	}
	return b.reserve(ctx, tx, a, id, folderID, nil, nil, name, f.Classification, f.ProjectID, size, sha, media, expires)
}

func (b *AgentDraftBackend) reserve(ctx context.Context, tx pgx.Tx, a core.Actor, id, folderID string, fileID, baseID *string, name, classification string, projectID *string, size int64, sha, media string, expires time.Time) (Upload, error) {
	var out Upload
	if b == nil || b.Writer == nil || !core.ValidID(id) || size < 1 || size > AgentTextMaxBytes || !shaPattern.MatchString(sha) || (media != "text/plain" && media != "text/markdown") {
		return out, core.Fail(503, "AGENT_DRAFT_WRITER_UNAVAILABLE", "秘书文本草稿写入身份尚未配置或格式不受支持")
	}
	if err := namespaceLock(ctx, tx, a, projectID); err != nil {
		return out, err
	}
	if err := reserveFileCapacity(ctx, tx, a.OrgID, size); err != nil {
		return out, err
	}
	epoch, provider, err := currentEpoch(ctx, tx, a.OrgID)
	if err != nil {
		return out, err
	}
	var enabled bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM storage_targets WHERE org_id=$1 AND provider=$2 AND bucket=$3 AND enabled)", a.OrgID, b.Provider, b.Bucket).Scan(&enabled); err != nil {
		return out, err
	}
	if !enabled || provider != b.Provider {
		return out, core.Fail(409, "STORAGE_CONFIGURATION_CHANGED", "草稿身份与当前主存储目标不一致")
	}
	key, _ := objectstore.DraftKey(a.OrgID, id)
	_, err = tx.Exec(ctx, `INSERT INTO upload_sessions(id,org_id,owner_id,target_folder_id,file_id,base_version_id,name,media_type,size_bytes,source_fingerprint,part_size,part_count,draft_key,storage_epoch,state,expires_at,provider,bucket,classification,source_channel)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,1,$12,$13,'created',$14,$15,$16,$17,'agent')`, id, a.OrgID, a.UserID, folderID, fileID, baseID, name, media, size, sha, objectstore.DefaultPartBytes, key, epoch, expires, b.Provider, b.Bucket, classification)
	if err != nil {
		return out, err
	}
	return upload(ctx, tx, a.OrgID, id, false)
}

func (b *AgentDraftBackend) Stage(ctx context.Context, u Upload, text string) (objectstore.Multipart, objectstore.Part, error) {
	var m objectstore.Multipart
	var p objectstore.Part
	if b == nil || b.Writer == nil || u.SourceChannel == nil || *u.SourceChannel != "agent" || u.Provider == nil || *u.Provider != b.Provider || u.Bucket == nil || *u.Bucket != b.Bucket || int64(len(text)) != u.layout().Size || core.Digest([]byte(text)) != u.SourceFingerprint {
		return m, p, core.Forbidden()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var err error
	m, err = b.Writer.Begin(ctx, u.DraftKey, u.MediaType)
	if err != nil {
		return m, p, core.Fail(503, "AGENT_DRAFT_STAGE_FAILED", "草稿隔离上传暂时不可用")
	}
	if m.Key != u.DraftKey || m.UploadID == "" {
		return m, p, core.Fail(502, "AGENT_DRAFT_STAGE_INVALID", "草稿上传返回了不一致的对象标识")
	}
	p, err = b.Writer.WritePart(ctx, m, 1, strings.NewReader(text), int64(len(text)))
	if err != nil || p.Number != 1 || p.Size != int64(len(text)) || !validOpaque(p.ETag) {
		b.Abandon(m)
		return m, p, core.Fail(503, "AGENT_DRAFT_STAGE_FAILED", "草稿字节未获准确上传回执")
	}
	return m, p, nil
}
func (b *AgentDraftBackend) Abandon(m objectstore.Multipart) {
	if b == nil || b.Writer == nil || m.Key == "" || m.UploadID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = b.Writer.Abort(ctx, m)
}

func (b *AgentDraftBackend) Finish(ctx context.Context, tx pgx.Tx, a core.Actor, id string, m objectstore.Multipart, p objectstore.Part) error {
	u, err := agentUpload(ctx, tx, a, id)
	if err != nil {
		return err
	}
	if u.State != "created" || u.DraftKey != m.Key || p.Number != 1 || p.Size != u.layout().Size || m.UploadID == "" {
		return core.Fail(409, "AGENT_DRAFT_STATE_CHANGED", "草稿上传状态已变化")
	}
	if u.FileID != nil {
		if u.BaseVersionID == nil {
			return core.Forbidden()
		}
		if _, err = AgentEditableFile(ctx, tx, a, *u.FileID, *u.BaseVersionID); err != nil {
			return err
		}
	}
	epoch, provider, err := currentEpoch(ctx, tx, a.OrgID)
	if err != nil {
		return err
	}
	if u.Provider == nil || *u.Provider != provider || u.StorageEpoch != epoch || b == nil || b.Provider != provider || u.Bucket == nil || *u.Bucket != b.Bucket {
		return core.Fail(409, "STORAGE_EPOCH_CHANGED", "草稿存储目标已变化")
	}
	if _, err = tx.Exec(ctx, "UPDATE upload_sessions SET provider_upload_id=$2,state='uploading',version=version+1,updated_at=clock_timestamp() WHERE id=$1", id, m.UploadID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO upload_parts(session_id,part_no,size_bytes,provider_etag) VALUES($1,1,$2,$3)", id, p.Size, p.ETag); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE upload_sessions SET state='finalizing',version=version+1,updated_at=clock_timestamp() WHERE id=$1", id); err != nil {
		return err
	}
	u, err = upload(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return err
	}
	return publishAgentDraft(ctx, tx, a, u)
}

// A stage is a trusted application event, not a claimed storage job. Keep its
// publication separate from workerEvent, whose non-null job_id has a job FK.
func publishAgentDraft(ctx context.Context, tx pgx.Tx, a core.Actor, u Upload) error {
	requestID := "agent-draft:" + u.ID
	payload, _ := json.Marshal(map[string]any{"session_id": u.ID, "storage_epoch": u.StorageEpoch})
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result)
 VALUES($1,$2,$3,$3,$4,$5,'agent.file.staged','upload',$6,$7,$8,'succeeded')`, core.NewID(), a.OrgID, a.UserID, a.Channel, requestID, u.ID, u.Version, digest(u)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
 VALUES($1,$2,'upload.finalize.requested','upload',$3,$4,$5,$6)`, core.NewID(), a.OrgID, u.ID, u.Version, payload, requestID)
	return err
}

func agentUpload(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Upload, error) {
	var out Upload
	if !core.ValidID(a.RunID) {
		return out, core.Forbidden()
	}
	var bound bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_file_drafts d JOIN agent_runs r ON r.org_id=d.org_id AND r.id=d.run_id
 WHERE d.org_id=$1 AND d.id=$2 AND d.run_id=$3 AND r.actor_id=$4 AND r.state='running' AND r.cancel_requested_at IS NULL
 AND r.actor_auth_version=$5 AND r.organization_auth_version=$6 AND r.channel=$7 AND r.expires_at>clock_timestamp())`, a.OrgID, id, a.RunID, a.UserID, a.AuthVersion, a.OrgAuthVersion, a.Channel).Scan(&bound)
	if err != nil {
		return out, err
	}
	if !bound {
		return out, core.Missing()
	}
	out, err = upload(ctx, db, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	if out.SourceChannel == nil || *out.SourceChannel != "agent" || (out.FileID == nil) != (out.BaseVersionID == nil) || out.OwnerID != a.UserID {
		return Upload{}, core.Missing()
	}
	_, err = uploadTarget(ctx, db, a, out.FolderID, out.FileID)
	return out, err
}

func (s *UploadService) authorizeAgent(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Upload, Folder, error) {
	u, err := agentUpload(ctx, db, a, id)
	if err != nil {
		return u, Folder{}, err
	}
	f, err := uploadTarget(ctx, db, a, u.FolderID, u.FileID)
	return u, f, err
}
