package files

import (
	"context"
	"strconv"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const fileColumns = `f.id,f.version,f.created_at,f.updated_at,d.project_id,f.folder_id,f.creator_id,f.name,f.classification,f.current_version_id,f.state,f.legal_hold`

func scanFile(row pgx.Row) (File, error) {
	var f File
	err := row.Scan(&f.ID, &f.Version, &f.CreatedAt, &f.UpdatedAt, &f.ProjectID, &f.FolderID, &f.CreatorID, &f.Name, &f.Classification, &f.CurrentVersionID, &f.State, &f.LegalHold)
	return f, platform.DatabaseError(err)
}
func fileResource(f File) authz.Resource {
	return authz.Resource{Type: "file", ID: f.ID, ProjectID: f.ProjectID, OwnerID: f.CreatorID, Classification: f.Classification}
}
func file(ctx context.Context, db platform.DBTX, org, id string, lock bool) (File, error) {
	if !core.ValidID(id) {
		return File{}, core.Missing()
	}
	query := "SELECT " + fileColumns + " FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id WHERE f.org_id=$1 AND f.id=$2"
	if lock {
		query += " FOR UPDATE OF f"
	}
	return scanFile(db.QueryRow(ctx, query, org, id))
}
func authorizeFile(ctx context.Context, db platform.DBTX, a core.Actor, id, action string) (File, error) {
	f, err := file(ctx, db, a.OrgID, id, false)
	if err != nil {
		return f, err
	}
	if err = authz.Require(ctx, db, a, action, fileResource(f)); err != nil {
		return f, err
	}
	if err = activeAncestors(ctx, db, a.OrgID, f.FolderID); err != nil {
		return f, err
	}
	return f, nil
}
func retentionHeld(ctx context.Context, db platform.DBTX, org, id string) (bool, error) {
	var held bool
	err := db.QueryRow(ctx, `SELECT f.legal_hold OR EXISTS(SELECT 1 FROM retention_holds h WHERE h.org_id=f.org_id AND h.file_id=f.id
        AND (h.valid_until IS NULL OR h.valid_until>now()) AND NOT EXISTS(SELECT 1 FROM retention_hold_releases r WHERE r.org_id=h.org_id AND r.hold_id=h.id)) FROM files f WHERE f.org_id=$1 AND f.id=$2`, org, id).Scan(&held)
	return held, platform.DatabaseError(err)
}
func presentFile(ctx context.Context, db platform.DBTX, a core.Actor, f File) (File, error) {
	canWrite := writable(ctx, db, a.OrgID, f.ProjectID)
	f.CanUpdate = canWrite && f.State == "active" && authz.Require(ctx, db, a, "file.update", fileResource(f)) == nil
	f.CanMove = canWrite && f.State == "active" && authz.Require(ctx, db, a, "file.move", fileResource(f)) == nil
	held, err := retentionHeld(ctx, db, a.OrgID, f.ID)
	if err != nil {
		return File{}, err
	}
	f.CanTrash = canWrite && !held && f.State == "active" && authz.Require(ctx, db, a, "file.trash", fileResource(f)) == nil
	f.CanRestore = canWrite && f.State == "trashed" && authz.Require(ctx, db, a, "file.restore", fileResource(f)) == nil
	f.CanViewHistoryRestores = authz.Require(ctx, db, a, "file.restore", fileResource(f)) == nil && authz.Require(ctx, db, a, "file.read", fileResource(f)) == nil
	f.CanRestoreHistory = canWrite && f.State == "active" && f.CurrentVersionID != nil && f.CanViewHistoryRestores
	return f, nil
}
func (s *Service) File(ctx context.Context, a core.Actor, id string) (File, error) {
	f, err := authorizeFile(ctx, s.DB, a, id, "file.read")
	if err != nil {
		return f, err
	}
	return presentFile(ctx, s.DB, a, f)
}
func (s *Service) Files(ctx context.Context, a core.Actor, f Filter) (core.Page[File], error) {
	out := core.Page[File]{Items: []File{}}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.State == "" {
		f.State = "active"
	}
	if f.State != "active" && f.State != "trashed" {
		return out, core.Invalid("state", "文件状态无效")
	}
	if f.CompanyOnly && f.ProjectID != "" {
		return out, core.Invalid("company_only", "公司资料与项目筛选不能同时指定")
	}
	if f.ProjectID != "" && !core.ValidID(f.ProjectID) {
		return out, core.Invalid("project_id", "项目标识无效")
	}
	if f.ParentID != "" {
		if _, err := authorizeFolder(ctx, s.DB, a, f.ParentID, "folder.read"); err != nil {
			return out, err
		}
	}
	binding := core.Digest([]byte(actorCursor(a) + "/files/" + f.ParentID + "/" + f.ProjectID + "/" + f.State + "/" + f.Query + "/" + strconv.FormatBool(f.CompanyOnly)))
	var before *time.Time
	var beforeID *string
	if f.Cursor != "" {
		c, err := s.Pager.Decode(f.Cursor, binding)
		if err != nil {
			return out, err
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	// The recursive chain excludes inaccessible tree states before pagination.
	rows, err := s.DB.Query(ctx, "SELECT "+fileColumns+` FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
        WHERE f.org_id=$1 AND f.state=$3 AND ($4='' OR f.folder_id=NULLIF($4,'')::uuid)
        AND (NOT $12 OR d.project_id IS NULL) AND ($5='' OR d.project_id=NULLIF($5,'')::uuid) AND ($6='' OR position(lower($6) IN lower(f.name))>0)
        AND can_access(f.org_id,$2,'file.read','file',f.id,d.project_id,f.creator_id,f.classification,$7,$11)
        AND (WITH RECURSIVE chain AS (
            SELECT id,parent_id,state,ARRAY[id] path FROM folders WHERE org_id=f.org_id AND id=f.folder_id
            UNION ALL SELECT p.id,p.parent_id,p.state,c.path||p.id FROM folders p JOIN chain c ON p.id=c.parent_id
            WHERE p.org_id=f.org_id AND NOT p.id=ANY(c.path)
        ) SELECT coalesce(bool_and(state='active') AND bool_or(parent_id IS NULL),false) FROM chain)
        AND ($8::timestamptz IS NULL OR (f.updated_at,f.id)<($8,$9::uuid))
        ORDER BY f.updated_at DESC,f.id DESC LIMIT $10`, a.OrgID, a.UserID, f.State, f.ParentID, f.ProjectID, f.Query, a.Channel, before, beforeID, f.Limit+1, a.RunID != "", f.CompanyOnly)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, err := scanFile(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		v := out.Items[f.Limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: v.ID, UpdatedAt: v.UpdatedAt, Binding: binding})
		out.NextCursor = &c
	}
	for i := range out.Items {
		out.Items[i], err = presentFile(ctx, s.DB, a, out.Items[i])
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *Service) MoveFile(ctx context.Context, c core.Command, id string, input Move) (core.Result, error) {
	if err := moveInput(input); err != nil {
		return core.Result{}, err
	}
	authorize := func(tx pgx.Tx) error {
		source, err := authorizeFile(ctx, tx, c.Actor, id, "file.move")
		if err != nil {
			return err
		}
		if input.TargetFolderID != nil {
			target, err := authorizeFolder(ctx, tx, c.Actor, *input.TargetFolderID, "folder.create")
			if err != nil {
				return err
			}
			if err = authz.Require(ctx, tx, c.Actor, "folder.read", folderResource(target)); err != nil {
				return err
			}
			return sameScope(Folder{ProjectID: source.ProjectID, Classification: source.Classification}, target)
		}
		return nil
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		source, err := file(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		if err = namespaceLock(ctx, tx, c.Actor, source.ProjectID); err != nil {
			return core.Result{}, err
		}
		if err = authorize(tx); err != nil {
			return core.Result{}, err
		}
		source, err = file(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if source.State != "active" {
			return core.Result{}, core.UnexpectedState(source.State, "active")
		}
		if source.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		before := digest(source)
		folderID := source.FolderID
		name := source.Name
		if input.TargetFolderID != nil {
			folderID = *input.TargetFolderID
		}
		if input.NewName != nil {
			name = *input.NewName
		}
		key, err := core.StorageNameKey(name)
		if err != nil {
			return core.Result{}, err
		}
		_, err = tx.Exec(ctx, "UPDATE files SET folder_id=$3,name=$4,name_key=$5,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, folderID, name, key)
		if err != nil {
			return core.Result{}, err
		}
		result, err := file(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		result, err = presentFile(ctx, tx, c.Actor, result)
		return core.Result{Data: result, Event: core.Event{Type: "file.moved", ResourceType: "file", ResourceID: id, Version: result.Version, BeforeDigest: before, Payload: map[string]any{"reason": input.Reason, "folder_id": folderID}}}, err
	})
}
func (s *Service) Trash(ctx context.Context, c core.Command, id string, input Transition) (core.Result, error) {
	if input.Action != "trash" {
		return core.Result{}, core.Invalid("action", "此入口仅支持移入回收站")
	}
	return s.changeState(ctx, c, id, input.ExpectedVersion, input.Reason, "trashed")
}
func (s *Service) Restore(ctx context.Context, c core.Command, id string, input Restore) (core.Result, error) {
	if input.Mode == "untrash" && input.SourceVersionID == nil {
		return s.changeState(ctx, c, id, input.ExpectedVersion, input.Reason, "active")
	}
	if input.Mode != "new_version_from_history" || input.SourceVersionID == nil || !core.ValidID(*input.SourceVersionID) {
		return core.Result{}, core.Invalid("mode", "恢复方式或来源版本无效")
	}
	if _, err := authorizeFile(ctx, s.DB, c.Actor, id, "file.restore"); err != nil {
		return core.Result{}, err
	}
	// Restoring bytes requires a new immutable candidate object, never a pointer rewind.
	return core.Result{}, core.Fail(409, "ASYNC_HISTORY_RESTORE_REQUIRED", "历史内容恢复请使用文件的历史恢复任务入口；恢复就绪后须明确提交")
}
func (s *Service) changeState(ctx context.Context, c core.Command, id string, version int64, reason, target string) (core.Result, error) {
	if err := core.RequireVersion(version); err != nil {
		return core.Result{}, err
	}
	if err := core.Name(reason); err != nil {
		return core.Result{}, core.Invalid("reason", "请填写操作原因")
	}
	action, eventType := "file.trash", "file.trashed"
	if target == "active" {
		action, eventType = "file.restore", "file.untrashed"
	}
	authorize := func(tx pgx.Tx) error { _, err := authorizeFile(ctx, tx, c.Actor, id, action); return err }
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		source, err := file(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		if err = namespaceLock(ctx, tx, c.Actor, source.ProjectID); err != nil {
			return core.Result{}, err
		}
		if err = authorize(tx); err != nil {
			return core.Result{}, err
		}
		source, err = file(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if source.Version != version {
			return core.Result{}, core.Conflict()
		}
		if source.State == target {
			return core.Result{}, core.UnexpectedState(source.State, target)
		}
		if target == "trashed" {
			held, err := retentionHeld(ctx, tx, c.Actor.OrgID, id)
			if err != nil {
				return core.Result{}, err
			}
			if held {
				return core.Result{}, core.Fail(409, "RETENTION_HOLD", "文件存在有效保留约束，不能移入回收站")
			}
		}
		before := digest(source)
		_, err = tx.Exec(ctx, `UPDATE files SET state=$3,trashed_at=CASE WHEN $3='trashed' THEN now() ELSE NULL END,
            version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, id, target)
		if err != nil {
			return core.Result{}, err
		}
		result, err := file(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		result, err = presentFile(ctx, tx, c.Actor, result)
		return core.Result{Data: result, Event: core.Event{Type: eventType, ResourceType: "file", ResourceID: id, Version: result.Version, BeforeDigest: before, Payload: map[string]any{"reason": reason}}}, err
	})
}
