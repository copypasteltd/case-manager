package files

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"strconv"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/importspec"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type ImportBatch struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	OwnerID   string    `json:"owner_id"`
	importspec.BatchInput
	AdmittedCount     int        `json:"admitted_count"`
	AdmittedBytes     string     `json:"admitted_bytes"`
	AdmittedChain     string     `json:"admitted_chain"`
	Approved          bool       `json:"approved"`
	ApprovalExpiresAt *time.Time `json:"approval_expires_at"`
}

type ImportEntry struct {
	ID        string    `json:"id"`
	BatchID   string    `json:"batch_id"`
	CreatedAt time.Time `json:"created_at"`
	importspec.Entry
	Upload             *Upload  `json:"upload"`
	FileVersion        *Version `json:"file_version"`
	VerificationState  string   `json:"verification_state"`
	MappingCurrent     bool     `json:"mapping_current"`
	MappedOwnerCanRead bool     `json:"mapped_owner_can_read"`
}

type ImportRetry struct {
	UploadID        string `json:"upload_id"`
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
}

type ImportService struct{ Uploads *UploadService }

const importBatchColumns = `b.id,b.created_at,b.owner_id,b.source_namespace,b.inventory_sha256,b.mapping_sha256,b.plan_sha256,b.entry_count,b.total_bytes::text,b.reason,b.admitted_count,b.admitted_bytes::text,b.admitted_chain`
const importEntryColumns = `e.id,e.batch_id,e.created_at,e.input`

func scanImportBatch(row pgx.Row) (ImportBatch, error) {
	var b ImportBatch
	err := row.Scan(&b.ID, &b.CreatedAt, &b.OwnerID, &b.SourceNamespace, &b.InventorySHA256, &b.MappingSHA256, &b.PlanSHA256, &b.EntryCount, &b.TotalBytes, &b.Reason, &b.AdmittedCount, &b.AdmittedBytes, &b.AdmittedChain)
	return b, platform.DatabaseError(err)
}

func scanImportEntry(row pgx.Row) (ImportEntry, error) {
	var e ImportEntry
	var input []byte
	err := row.Scan(&e.ID, &e.BatchID, &e.CreatedAt, &input)
	if err == nil {
		err = json.Unmarshal(input, &e.Entry)
	}
	e.VerificationState = "awaiting_upload"
	return e, platform.DatabaseError(err)
}

func (s *ImportService) actor(ctx context.Context, db platform.DBTX, a core.Actor) error {
	if err := uploadChannel(a); err != nil {
		return err
	}
	if err := currentUploadActor(ctx, db, a); err != nil {
		return err
	}
	return authz.RequireOrganization(ctx, db, a, "file.import")
}

func (s *ImportService) batch(ctx context.Context, db platform.DBTX, a core.Actor, id string, lock bool) (ImportBatch, error) {
	if err := s.actor(ctx, db, a); err != nil {
		return ImportBatch{}, err
	}
	if !core.ValidID(id) {
		return ImportBatch{}, core.Missing()
	}
	query := "SELECT " + importBatchColumns + " FROM file_import_batches b WHERE b.org_id=$1 AND b.id=$2 AND b.owner_id=$3"
	if lock {
		query += " FOR UPDATE"
	}
	b, err := scanImportBatch(db.QueryRow(ctx, query, a.OrgID, id, a.UserID))
	if err != nil {
		return b, err
	}
	err = db.QueryRow(ctx, `SELECT max(expires_at) FROM file_import_approvals WHERE org_id=$1 AND batch_id=$2 AND owner_id=$3
 AND user_auth_version=$4 AND org_auth_version=$5 AND expires_at>clock_timestamp()`, a.OrgID, id, a.UserID, a.AuthVersion, a.OrgAuthVersion).Scan(&b.ApprovalExpiresAt)
	b.Approved = b.ApprovalExpiresAt != nil
	return b, err
}

func (s *ImportService) GetBatch(ctx context.Context, a core.Actor, id string) (ImportBatch, error) {
	return s.batch(ctx, s.Uploads.Files.DB, a, id, false)
}

func importEvent(kind, id string, payload map[string]any) core.Event {
	return core.Event{Type: kind, ResourceType: "file_import", ResourceID: id, Version: 1, Payload: payload}
}

func (s *ImportService) CreateBatch(ctx context.Context, c core.Command, in importspec.BatchInput) (core.Result, error) {
	if err := importspec.ValidateBatch(in); err != nil {
		return core.Result{}, err
	}
	return s.Uploads.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { return s.actor(ctx, tx, c.Actor) }, func(tx pgx.Tx) (core.Result, error) {
		id := core.NewID()
		_, err := tx.Exec(ctx, `INSERT INTO file_import_batches(id,org_id,owner_id,source_namespace,inventory_sha256,mapping_sha256,plan_sha256,entry_count,total_bytes,reason)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(org_id,owner_id,source_namespace,plan_sha256) DO NOTHING`, id, c.Actor.OrgID, c.Actor.UserID, in.SourceNamespace, in.InventorySHA256, in.MappingSHA256, in.PlanSHA256, in.EntryCount, in.TotalBytes, in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		b, err := scanImportBatch(tx.QueryRow(ctx, "SELECT "+importBatchColumns+" FROM file_import_batches b WHERE org_id=$1 AND owner_id=$2 AND source_namespace=$3 AND plan_sha256=$4", c.Actor.OrgID, c.Actor.UserID, in.SourceNamespace, in.PlanSHA256))
		if err != nil {
			return core.Result{}, err
		}
		if b.BatchInput != in {
			return core.Result{}, core.Fail(409, "IMPORT_PLAN_CONFLICT", "相同计划已有不同来源说明，请核对原批次")
		}
		return core.Result{Status: 201, Data: b, Event: importEvent("file_import.batch.registered", b.ID, map[string]any{"plan_sha256": in.PlanSHA256, "entry_count": in.EntryCount})}, nil
	})
}

func (s *ImportService) mapping(ctx context.Context, db platform.DBTX, a core.Actor, in importspec.Entry, exact bool) (Folder, error) {
	f, err := uploadTarget(ctx, db, a, in.FolderID, in.FileID)
	if err != nil {
		return f, err
	}
	if !sameID(f.ProjectID, in.ProjectID) || f.Classification != in.Classification || (exact && f.Version != in.FolderVersion) {
		return f, core.Fail(409, "IMPORT_MAPPING_CHANGED", "导入目录、项目、分类或版本已变化，请重新核对完整映射")
	}
	var active bool
	if err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE org_id=$1 AND id=$2 AND status='active')", a.OrgID, in.MappedOwnerID).Scan(&active); err != nil {
		return f, err
	}
	if !active {
		return f, core.Missing()
	}
	return f, nil
}

func (s *ImportService) AddEntry(ctx context.Context, c core.Command, batchID string, in importspec.Entry) (core.Result, error) {
	return s.Uploads.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		b, err := s.batch(ctx, tx, c.Actor, batchID, false)
		if err != nil {
			return err
		}
		if err = importspec.ValidateEntry(b.SourceNamespace, in); err != nil {
			return err
		}
		media, params, err := mime.ParseMediaType(in.MediaType)
		if err != nil || media != in.MediaType || len(params) > 0 || len(media) > 200 {
			return core.Invalid("media_type", "请提供明确无参数的媒体类型")
		}
		_, err = s.mapping(ctx, tx, c.Actor, in, true)
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		b, err := s.batch(ctx, tx, c.Actor, batchID, true)
		if err != nil {
			return core.Result{}, err
		}
		current, err := scanImportEntry(tx.QueryRow(ctx, "SELECT "+importEntryColumns+" FROM file_import_entries e WHERE org_id=$1 AND batch_id=$2 AND ordinal=$3", c.Actor.OrgID, b.ID, in.Ordinal))
		if err == nil {
			if importspec.EntryDigest(current.Entry) != importspec.EntryDigest(in) {
				return core.Result{}, core.Fail(409, "IMPORT_ENTRY_CONFLICT", "原序号已登记不同来源或映射")
			}
			return core.Result{Data: current, Event: importEvent("file_import.entry.reconciled", current.ID, map[string]any{"batch_id": b.ID})}, nil
		}
		var missing *core.Error
		if !errors.As(err, &missing) || missing.Status != 404 {
			return core.Result{}, err
		}
		if in.Ordinal != b.AdmittedCount+1 || in.Ordinal > b.EntryCount {
			return core.Result{}, core.Fail(409, "IMPORT_SEQUENCE_CONFLICT", "请按原计划顺序继续登记，不跳过或重复来源")
		}
		n, _ := importspec.Bytes(in.SizeBytes)
		total, _ := importspec.Bytes(b.TotalBytes)
		admitted, _ := strconv.ParseInt(b.AdmittedBytes, 10, 64)
		chain := importspec.Chain(b.AdmittedChain, importspec.EntryDigest(in))
		if admitted+n > total || (in.Ordinal == b.EntryCount && (admitted+n != total || chain != b.PlanSHA256)) {
			return core.Result{}, core.Fail(409, "IMPORT_MANIFEST_MISMATCH", "完整清单数量、字节或摘要与计划不符")
		}
		data, _ := json.Marshal(in)
		id := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO file_import_entries(id,org_id,batch_id,ordinal,source_namespace,source_key,source_sha256,size_bytes,folder_id,folder_version,project_id,classification,mapped_owner_id,input,entry_sha256)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, id, c.Actor.OrgID, b.ID, in.Ordinal, b.SourceNamespace, in.SourceKey, in.SourceSHA256, n, in.FolderID, in.FolderVersion, in.ProjectID, in.Classification, in.MappedOwnerID, data, importspec.EntryDigest(in))
		if err != nil {
			return core.Result{}, err
		}
		e, err := scanImportEntry(tx.QueryRow(ctx, "SELECT "+importEntryColumns+" FROM file_import_entries e WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id))
		return core.Result{Status: 201, Data: e, Event: importEvent("file_import.entry.registered", id, map[string]any{"batch_id": b.ID, "ordinal": in.Ordinal, "source_key": in.SourceKey})}, err
	})
}

func (s *ImportService) Approve(ctx context.Context, c core.Command, id string, in importspec.ApprovalInput) (core.Result, error) {
	if !importspec.Hash(in.PlanSHA256) || !in.ReviewedMappings || !in.SourcesReadOnly || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("approval", "请核对完整计划、账号／权限映射与来源只读窗口，并说明原因")
	}
	return s.Uploads.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if err := interactiveMFA(c.Actor); err != nil {
			return err
		}
		_, err := s.batch(ctx, tx, c.Actor, id, false)
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		b, err := s.batch(ctx, tx, c.Actor, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if b.PlanSHA256 != in.PlanSHA256 || b.AdmittedCount != b.EntryCount || b.AdmittedBytes != b.TotalBytes || b.AdmittedChain != b.PlanSHA256 {
			return core.Result{}, core.Fail(409, "IMPORT_MANIFEST_INCOMPLETE", "完整计划尚未全部登记或摘要不一致")
		}
		_, err = tx.Exec(ctx, `INSERT INTO file_import_approvals(id,org_id,batch_id,owner_id,org_auth_version,user_auth_version,reason) VALUES($1,$2,$3,$4,$5,$6,$7)`, core.NewID(), c.Actor.OrgID, b.ID, c.Actor.UserID, c.Actor.OrgAuthVersion, c.Actor.AuthVersion, in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		b, err = s.batch(ctx, tx, c.Actor, id, false)
		return core.Result{Data: b, Event: importEvent("file_import.batch.approved", id, map[string]any{"plan_sha256": in.PlanSHA256, "reason": in.Reason})}, err
	})
}

func (s *ImportService) entry(ctx context.Context, db platform.DBTX, a core.Actor, id string) (ImportEntry, ImportBatch, error) {
	if err := s.actor(ctx, db, a); err != nil {
		return ImportEntry{}, ImportBatch{}, err
	}
	if !core.ValidID(id) {
		return ImportEntry{}, ImportBatch{}, core.Missing()
	}
	e, err := scanImportEntry(db.QueryRow(ctx, "SELECT "+importEntryColumns+" FROM file_import_entries e WHERE org_id=$1 AND id=$2", a.OrgID, id))
	if err != nil {
		return e, ImportBatch{}, err
	}
	b, err := s.batch(ctx, db, a, e.BatchID, false)
	if err == nil {
		_, err = s.mapping(ctx, db, a, e.Entry, false)
	}
	return e, b, err
}

func (s *ImportService) present(ctx context.Context, db platform.DBTX, a core.Actor, e ImportEntry) (ImportEntry, error) {
	var id string
	err := db.QueryRow(ctx, `SELECT upload_id FROM file_import_attempts WHERE org_id=$1 AND entry_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, a.OrgID, e.ID).Scan(&id)
	if err == pgx.ErrNoRows {
		return e, nil
	}
	if err != nil {
		return e, err
	}
	u, _, err := s.Uploads.authorize(ctx, db, a, id)
	if err != nil {
		return e, err
	}
	if err = confirmed(ctx, db, &u); err != nil {
		return e, err
	}
	e.Upload = &u
	e.VerificationState = u.State
	if u.State != "committed" || u.ResultVersionID == nil || u.ResultFileID == nil {
		return e, nil
	}
	v, err := s.Uploads.Files.Version(ctx, a, *u.ResultFileID, *u.ResultVersionID)
	if err != nil {
		return e, err
	}
	e.FileVersion = &v
	f, err := authorizeFile(ctx, db, a, *u.ResultFileID, "file.read")
	if err != nil {
		return e, err
	}
	folder, err := authorizeFolder(ctx, db, a, e.FolderID, "folder.read")
	if err != nil {
		return e, err
	}
	e.MappingCurrent = f.FolderID == e.FolderID && sameID(f.ProjectID, e.ProjectID) && f.Classification == e.Classification && f.Name == e.Name && folder.Version == e.FolderVersion && f.CurrentVersionID != nil && *f.CurrentVersionID == v.ID
	if err = db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM unnest(ARRAY['web','android','wechat','douyin']) channel WHERE can_access($1,$2,'file.read','file',$3,$4,$5,$6,channel,false))`, a.OrgID, e.MappedOwnerID, f.ID, f.ProjectID, f.CreatorID, f.Classification).Scan(&e.MappedOwnerCanRead); err != nil {
		return e, err
	}
	e.VerificationState = "primary_verified"
	if v.SHA256 != e.SourceSHA256 || v.SizeBytes != e.SizeBytes {
		e.VerificationState = "content_mismatch"
		return e, nil
	}
	if !e.MappingCurrent || !e.MappedOwnerCanRead {
		e.VerificationState = "mapping_changed"
		return e, nil
	}
	seen := map[string]bool{}
	for _, r := range v.Replicas {
		if r.State == "verified" && r.ActualSHA256 != nil && *r.ActualSHA256 == e.SourceSHA256 && r.VerifiedAt != nil {
			seen[r.Provider] = true
		}
	}
	if seen["oss"] && seen["r2"] {
		e.VerificationState = "replicas_verified"
	}
	return e, nil
}

func (s *ImportService) GetEntry(ctx context.Context, a core.Actor, id string) (ImportEntry, error) {
	e, _, err := s.entry(ctx, s.Uploads.Files.DB, a, id)
	if err != nil {
		return e, err
	}
	return s.present(ctx, s.Uploads.Files.DB, a, e)
}

func (s *ImportService) Entries(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[ImportEntry], error) {
	out := core.Page[ImportEntry]{Items: []ImportEntry{}}
	if _, err := s.batch(ctx, s.Uploads.Files.DB, a, id, false); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	binding := core.CursorBinding(a, "file-import-entries", id)
	after := 0
	if cursor != "" {
		c, err := s.Uploads.Files.Pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		if err = s.Uploads.Files.DB.QueryRow(ctx, "SELECT ordinal FROM file_import_entries WHERE org_id=$1 AND batch_id=$2 AND id=$3 AND created_at=$4", a.OrgID, id, c.ID, c.UpdatedAt).Scan(&after); err != nil {
			return out, platform.DatabaseError(err)
		}
	}
	rows, err := s.Uploads.Files.DB.Query(ctx, "SELECT "+importEntryColumns+" FROM file_import_entries e WHERE org_id=$1 AND batch_id=$2 AND ordinal>$3 ORDER BY ordinal LIMIT $4", a.OrgID, id, after, limit+1)
	if err != nil {
		return out, err
	}
	var entries []ImportEntry
	for rows.Next() {
		e, err := scanImportEntry(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(entries) > limit {
		entries = entries[:limit]
		tail := entries[len(entries)-1]
		value := s.Uploads.Files.Pager.Encode(platform.Cursor{ID: tail.ID, UpdatedAt: tail.CreatedAt, Binding: binding})
		out.NextCursor = &value
	}
	for _, e := range entries {
		if _, err := s.mapping(ctx, s.Uploads.Files.DB, a, e.Entry, false); err != nil {
			return out, err
		}
		e, err = s.present(ctx, s.Uploads.Files.DB, a, e)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, e)
	}
	return out, nil
}

func (s *ImportService) Start(ctx context.Context, c core.Command, id string, retry *ImportRetry) (core.Result, error) {
	if retry != nil && (!core.ValidID(retry.UploadID) || retry.ExpectedVersion < 1 || core.Name(retry.Reason) != nil) {
		return core.Result{}, core.Invalid("retry", "请核对原上传、版本及明确的重试原因")
	}
	return s.Uploads.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		e, b, err := s.entry(ctx, tx, c.Actor, id)
		if err != nil {
			return err
		}
		if !b.Approved {
			return core.Fail(403, "IMPORT_APPROVAL_REQUIRED", "请使用近期两步验证重新核对并批准完整计划")
		}
		if retry != nil {
			if err = interactiveMFA(c.Actor); err != nil {
				return err
			}
		}
		_, err = s.mapping(ctx, tx, c.Actor, e.Entry, true)
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		e, b, err := s.entry(ctx, tx, c.Actor, id)
		if err != nil {
			return core.Result{}, err
		}
		if !b.Approved {
			return core.Result{}, core.Fail(403, "IMPORT_APPROVAL_REQUIRED", "完整计划批准已失效")
		}
		if err = namespaceLock(ctx, tx, c.Actor, e.ProjectID); err != nil {
			return core.Result{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO file_import_objects(org_id,source_namespace,source_key,owner_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, c.Actor.OrgID, b.SourceNamespace, e.SourceKey, c.Actor.UserID)
		if err != nil {
			return core.Result{}, err
		}
		var owner string
		var fileID, currentID *string
		err = tx.QueryRow(ctx, `SELECT owner_id,file_id,current_upload_id FROM file_import_objects WHERE org_id=$1 AND source_namespace=$2 AND source_key=$3 FOR UPDATE`, c.Actor.OrgID, b.SourceNamespace, e.SourceKey).Scan(&owner, &fileID, &currentID)
		if err != nil {
			return core.Result{}, err
		}
		if owner != c.Actor.UserID {
			return core.Result{}, core.Missing()
		}
		// All child commands retain the same authority, audit, outbox and transaction.
		metadata := *s.Uploads.Files
		metadata.Commands = platform.Commands{DB: metadata.DB, Tx: tx}
		uploads := *s.Uploads
		uploads.Files = &metadata
		if currentID != nil {
			u, err := upload(ctx, tx, c.Actor.OrgID, *currentID, true)
			if err != nil {
				return core.Result{}, err
			}
			var matches bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM file_import_attempts a JOIN file_import_entries e ON e.org_id=a.org_id AND e.id=a.entry_id
 WHERE a.org_id=$1 AND a.upload_id=$2 AND e.source_namespace=$3 AND e.source_key=$4 AND e.source_sha256=$5 AND e.size_bytes=$6)`, c.Actor.OrgID, u.ID, b.SourceNamespace, e.SourceKey, e.SourceSHA256, e.SizeBytes).Scan(&matches)
			if err != nil {
				return core.Result{}, err
			}
			if retry == nil && matches && u.FolderID == e.FolderID && u.Name == e.Name && u.MediaType == e.MediaType && ((u.State != "committed" && sameImportOptional(u.FileID, e.FileID) && sameImportOptional(u.BaseVersionID, e.BaseVersionID)) || (fileID != nil && u.ResultVersionID != nil && currentImportVersion(ctx, tx, c.Actor, *fileID, *u.ResultVersionID))) {
				if _, _, err = uploads.authorize(ctx, tx, c.Actor, u.ID); err != nil {
					return core.Result{}, err
				}
				if err = linkImportAttempt(ctx, tx, c.Actor, e.ID, u.ID, "复用准确来源与原上传"); err != nil {
					return core.Result{}, err
				}
				e.Upload = &u
				e.VerificationState = u.State
				return core.Result{Data: e, Event: importEvent("file_import.upload.reconciled", e.ID, map[string]any{"upload_id": u.ID})}, nil
			}
			if retry == nil && u.State != "committed" {
				return core.Result{}, core.Fail(409, "IMPORT_UPLOAD_REQUIRES_REVIEW", "此来源已有未完成或不同的上传，请核对原记录后明确重试")
			}
			if retry != nil {
				if u.ID != retry.UploadID || u.Version != retry.ExpectedVersion || !matches || u.State == "committed" || (u.State != "failed" && u.State != "aborted" && time.Now().Before(u.ExpiresAt)) {
					return core.Result{}, core.Fail(409, "IMPORT_RETRY_CONFLICT", "仅可明确重试准确失败、取消或已过期的原上传")
				}
				if u.State == "failed" && time.Now().Before(u.ExpiresAt) {
					body := Transition{ExpectedVersion: u.Version, Action: "abort", Reason: retry.Reason}
					raw, _ := json.Marshal(body)
					child := c
					child.Route = "POST /v1/uploads/" + u.ID + "/abort"
					child.Key = "import-abort/" + e.ID + "/" + u.ID
					child.Body = raw
					if _, err = uploads.Abort(ctx, child, u.ID, body); err != nil {
						return core.Result{}, err
					}
				}
			}
		} else if retry != nil {
			return core.Result{}, core.Conflict()
		}
		if !sameID(fileID, e.FileID) {
			return core.Result{}, core.Fail(409, "IMPORT_BASE_REVIEW_REQUIRED", "稳定来源已有逻辑文件，请明确核对其当前基础版本后再导入新内容")
		}
		in := UploadCreate{FolderID: e.FolderID, Name: e.Name, MediaType: e.MediaType, SizeBytes: e.SizeBytes, FileID: e.FileID, BaseVersionID: e.BaseVersionID, SourceFingerprint: core.Digest([]byte(b.SourceNamespace + "/" + e.SourceKey + "/" + e.SourceSHA256))}
		raw, _ := json.Marshal(in)
		child := c
		child.Route = "POST /v1/uploads"
		child.Key = "import-upload/" + e.ID
		if retry != nil {
			child.Key += "/after/" + retry.UploadID
		}
		child.Body = raw
		result, err := uploads.Create(ctx, child, in)
		if err != nil {
			return core.Result{}, err
		}
		var u Upload
		data, _ := json.Marshal(result.Data)
		if err = json.Unmarshal(data, &u); err != nil {
			return core.Result{}, err
		}
		reason := "首次上传准确来源"
		if retry != nil {
			reason = retry.Reason
		}
		if err = linkImportAttempt(ctx, tx, c.Actor, e.ID, u.ID, reason); err != nil {
			return core.Result{}, err
		}
		_, err = tx.Exec(ctx, "UPDATE file_import_objects SET current_upload_id=$4 WHERE org_id=$1 AND source_namespace=$2 AND source_key=$3", c.Actor.OrgID, b.SourceNamespace, e.SourceKey, u.ID)
		if err != nil {
			return core.Result{}, err
		}
		e.Upload = &u
		e.VerificationState = u.State
		return core.Result{Status: 201, Data: e, Event: importEvent("file_import.upload.reserved", e.ID, map[string]any{"batch_id": b.ID, "upload_id": u.ID, "source_key": e.SourceKey})}, nil
	})
}

func currentImportVersion(ctx context.Context, db platform.DBTX, a core.Actor, fileID, versionID string) bool {
	var matches bool
	err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM files WHERE org_id=$1 AND id=$2 AND current_version_id=$3 AND state='active')", a.OrgID, fileID, versionID).Scan(&matches)
	return err == nil && matches
}

func linkImportAttempt(ctx context.Context, tx pgx.Tx, a core.Actor, entryID, uploadID, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO file_import_attempts(id,org_id,entry_id,upload_id,reason) VALUES($1,$2,$3,$4,$5) ON CONFLICT(org_id,entry_id,upload_id) DO NOTHING`, core.NewID(), a.OrgID, entryID, uploadID, reason)
	return err
}

// The ordinary commit endpoint also enforces the approved import provenance.
// A caller cannot bypass this boundary by using /uploads directly.
func (s *UploadService) validateImportCommit(ctx context.Context, tx pgx.Tx, a core.Actor, u Upload, sha string) error {
	return s.checkImportCommit(ctx, tx, a, u, sha, true)
}

func (s *UploadService) checkImportCommit(ctx context.Context, db platform.DBTX, a core.Actor, u Upload, sha string, lock bool) error {
	var id, currentID string
	query := `SELECT e.id,o.current_upload_id FROM file_import_entries e
 JOIN file_import_attempts attempt ON attempt.org_id=e.org_id AND attempt.entry_id=e.id
 JOIN file_import_objects o ON o.org_id=e.org_id AND o.source_namespace=e.source_namespace AND o.source_key=e.source_key
 WHERE attempt.org_id=$1 AND attempt.upload_id=$2 ORDER BY attempt.created_at DESC,attempt.id DESC LIMIT 1`
	if lock {
		query += " FOR SHARE OF o"
	}
	err := db.QueryRow(ctx, query, a.OrgID, u.ID).Scan(&id, &currentID)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	service := &ImportService{Uploads: s}
	e, b, err := service.entry(ctx, db, a, id)
	if err != nil {
		return err
	}
	if !b.Approved {
		return core.Fail(403, "IMPORT_APPROVAL_REQUIRED", "导入批准已失效，请重新核对完整计划")
	}
	if currentID != u.ID || sha != e.SourceSHA256 || u.SizeBytes != e.SizeBytes {
		return core.Fail(409, "IMPORT_SOURCE_DIGEST_MISMATCH", "上传候选与批准的准确来源不同或已被替代")
	}
	_, err = service.mapping(ctx, db, a, e.Entry, true)
	return err
}

func sameImportOptional(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
