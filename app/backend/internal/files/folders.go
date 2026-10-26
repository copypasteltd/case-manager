package files

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

const folderColumns = `f.id,f.version,f.created_at,f.updated_at,f.project_id,f.parent_id,f.creator_id,f.name,f.classification,f.state,f.root_scope`

func scanFolder(row pgx.Row) (Folder, error) {
	var f Folder
	err := row.Scan(&f.ID, &f.Version, &f.CreatedAt, &f.UpdatedAt, &f.ProjectID, &f.ParentID, &f.CreatorID, &f.Name, &f.Classification, &f.State, &f.RootScope)
	return f, platform.DatabaseError(err)
}
func folderResource(f Folder) authz.Resource {
	return authz.Resource{Type: "folder", ID: f.ID, ProjectID: f.ProjectID, OwnerID: f.CreatorID, Classification: f.Classification}
}
func folder(ctx context.Context, db platform.DBTX, org, id string, lock bool) (Folder, error) {
	if !core.ValidID(id) {
		return Folder{}, core.Missing()
	}
	query := "SELECT " + folderColumns + " FROM folders f WHERE f.org_id=$1 AND f.id=$2"
	if lock {
		query += " FOR UPDATE OF f"
	}
	return scanFolder(db.QueryRow(ctx, query, org, id))
}
func activeAncestors(ctx context.Context, db platform.DBTX, org, id string) error {
	var valid bool
	err := db.QueryRow(ctx, `WITH RECURSIVE chain AS (
        SELECT id,parent_id,state,ARRAY[id] AS path FROM folders WHERE org_id=$1 AND id=$2
        UNION ALL SELECT f.id,f.parent_id,f.state,c.path||f.id FROM folders f JOIN chain c ON f.id=c.parent_id
        WHERE f.org_id=$1 AND NOT f.id=ANY(c.path)
    ) SELECT coalesce(bool_and(state='active') AND bool_or(parent_id IS NULL),false) FROM chain`, org, id).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return core.Missing()
	}
	return nil
}

// Organization row is locked by Commands first. All project storage metadata
// uses the project lock; company directories use one transaction advisory lock.
func namespaceLock(ctx context.Context, tx pgx.Tx, a core.Actor, projectID *string) error {
	if projectID == nil {
		_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", a.OrgID+"/company-files")
		return err
	}
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM projects WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, *projectID).Scan(&state); err != nil {
		return platform.DatabaseError(err)
	}
	if state == "closing" || state == "archived" || state == "cancelled" {
		return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许修改文件目录")
	}
	return nil
}
func writable(ctx context.Context, db platform.DBTX, org string, projectID *string) bool {
	if projectID == nil {
		return true
	}
	var state string
	return db.QueryRow(ctx, "SELECT state FROM projects WHERE org_id=$1 AND id=$2", org, *projectID).Scan(&state) == nil && state != "closing" && state != "archived" && state != "cancelled"
}
func sameID(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
func actorCursor(a core.Actor) string {
	return fmt.Sprintf("%s/%s/%d/%d/%s/%s", a.OrgID, a.UserID, a.AuthVersion, a.OrgAuthVersion, a.Channel, a.RunID)
}
func digest(value any) string { b, _ := json.Marshal(value); return core.Digest(b) }
func (s *Service) presentFolder(ctx context.Context, a core.Actor, f Folder) Folder {
	canWrite := writable(ctx, s.DB, a.OrgID, f.ProjectID)
	f.CanCreate = canWrite && authz.Require(ctx, s.DB, a, "folder.create", folderResource(f)) == nil
	f.CanUpdate = canWrite && authz.Require(ctx, s.DB, a, "folder.update", folderResource(f)) == nil
	f.CanUpload = canWrite && uploadChannel(a) == nil && authz.Require(ctx, s.DB, a, "file.upload", folderResource(f)) == nil
	return f
}
func authorizeFolder(ctx context.Context, db platform.DBTX, a core.Actor, id, action string) (Folder, error) {
	f, err := folder(ctx, db, a.OrgID, id, false)
	if err != nil {
		return f, err
	}
	if err = authz.Require(ctx, db, a, action, folderResource(f)); err != nil {
		return f, err
	}
	if err = activeAncestors(ctx, db, a.OrgID, id); err != nil {
		return f, err
	}
	return f, nil
}
func (s *Service) Folder(ctx context.Context, a core.Actor, id string) (Folder, error) {
	f, err := authorizeFolder(ctx, s.DB, a, id, "folder.read")
	if err != nil {
		return f, err
	}
	return s.presentFolder(ctx, a, f), nil
}
func (s *Service) Folders(ctx context.Context, a core.Actor, f Filter) (core.Page[Folder], error) {
	out := core.Page[Folder]{Items: []Folder{}}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.ProjectID != "" && !core.ValidID(f.ProjectID) {
		return out, core.Invalid("project_id", "项目标识无效")
	}
	if f.ParentID != "" {
		if _, err := authorizeFolder(ctx, s.DB, a, f.ParentID, "folder.read"); err != nil {
			return out, err
		}
	}
	binding := core.Digest([]byte(actorCursor(a) + "/folders/" + f.ParentID + "/" + f.ProjectID + "/" + f.Query))
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
	rows, err := s.DB.Query(ctx, "SELECT "+folderColumns+` FROM folders f WHERE f.org_id=$1 AND f.state='active'
        AND f.parent_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid
        AND ($4='' OR f.project_id=NULLIF($4,'')::uuid) AND ($5='' OR position(lower($5) IN lower(f.name))>0)
        AND can_access(f.org_id,$2,'folder.read','folder',f.id,f.project_id,f.creator_id,f.classification,$6,$10)
        AND ($7::timestamptz IS NULL OR (f.updated_at,f.id)<($7,$8::uuid))
        ORDER BY f.updated_at DESC,f.id DESC LIMIT $9`, a.OrgID, a.UserID, f.ParentID, f.ProjectID, f.Query, a.Channel, before, beforeID, f.Limit+1, a.RunID != "")
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, err := scanFolder(rows)
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
		out.Items[i] = s.presentFolder(ctx, a, out.Items[i])
	}
	return out, nil
}
func (s *Service) CreateFolder(ctx context.Context, c core.Command, input FolderCreate) (core.Result, error) {
	key, err := core.StorageNameKey(input.Name)
	if err != nil {
		return core.Result{}, err
	}
	if !core.Classification(input.Classification) {
		return core.Result{}, core.Invalid("classification", "目录分类无效")
	}
	if input.ProjectID != nil && !core.ValidID(*input.ProjectID) {
		return core.Result{}, core.Invalid("project_id", "项目标识无效")
	}
	authorize := func(tx pgx.Tx) error {
		if input.Classification == "confidential" {
			return core.Fail(422, "EXPLICIT_GRANT_REQUIRED", "严格受限目录须通过分类与对象授权流程创建")
		}
		if input.ParentID != nil {
			parent, err := authorizeFolder(ctx, tx, c.Actor, *input.ParentID, "folder.create")
			if err != nil {
				return err
			}
			if err = authz.Require(ctx, tx, c.Actor, "folder.read", folderResource(parent)); err != nil {
				return err
			}
			if input.ProjectID != nil && !sameID(input.ProjectID, parent.ProjectID) {
				return core.Invalid("project_id", "目录归属必须与父目录一致")
			}
			if parent.ProjectID == nil && input.Classification != "internal" {
				return core.Invalid("classification", "项目受限目录必须属于项目")
			}
			if core.ClassificationRank(input.Classification) < core.ClassificationRank(parent.Classification) {
				return core.Invalid("classification", "目录分类不能低于父目录")
			}
			return nil
		}
		if input.ProjectID != nil || input.Classification != "internal" {
			return core.Invalid("parent_id", "请在已有项目资料根目录下新建目录")
		}
		return authz.Require(ctx, tx, c.Actor, "folder.create", authz.Resource{Type: "folder", OwnerID: &c.Actor.UserID, Classification: "internal"})
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		var parent Folder
		projectID := input.ProjectID
		scope := "company"
		if input.ParentID != nil {
			parent, err = folder(ctx, tx, c.Actor.OrgID, *input.ParentID, false)
			if err != nil {
				return core.Result{}, err
			}
			projectID = parent.ProjectID
			scope = parent.RootScope
		}
		if err = namespaceLock(ctx, tx, c.Actor, projectID); err != nil {
			return core.Result{}, err
		}
		if err = authorize(tx); err != nil {
			return core.Result{}, err
		}
		id := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO folders(id,org_id,project_id,parent_id,root_scope,name,name_key,classification,state,creator_id)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,'active',$9)`, id, c.Actor.OrgID, projectID, input.ParentID, scope, input.Name, key, input.Classification, c.Actor.UserID)
		if err != nil {
			return core.Result{}, err
		}
		result, err := folder(ctx, tx, c.Actor.OrgID, id, false)
		result.CanCreate = true
		result.CanUpload = uploadChannel(c.Actor) == nil && authz.Require(ctx, tx, c.Actor, "file.upload", folderResource(result)) == nil
		result.CanUpdate = authz.Require(ctx, tx, c.Actor, "folder.update", folderResource(result)) == nil
		return core.Result{Status: 201, Data: result, Event: core.Event{Type: "folder.created", ResourceType: "folder", ResourceID: id, Version: 1, Payload: map[string]any{"project_id": projectID, "parent_id": input.ParentID}}}, err
	})
}
func moveInput(input Move) error {
	if err := core.RequireVersion(input.ExpectedVersion); err != nil {
		return err
	}
	if err := core.Name(input.Reason); err != nil {
		return core.Invalid("reason", "请填写变更原因")
	}
	if input.NewName == nil && input.TargetFolderID == nil {
		return core.Invalid("target_folder_id", "请指定新名称或目标目录")
	}
	if input.TargetFolderID != nil && !core.ValidID(*input.TargetFolderID) {
		return core.Invalid("target_folder_id", "目标目录标识无效")
	}
	if input.NewName != nil {
		if _, err := core.StorageNameKey(*input.NewName); err != nil {
			return err
		}
	}
	if input.PlanID != nil || input.PlanHash != nil {
		return core.Fail(503, "FIXED_PLAN_UNAVAILABLE", "此操作需要接入固定计划执行服务")
	}
	return nil
}
func sameScope(source Folder, target Folder) error {
	if !sameID(source.ProjectID, target.ProjectID) || source.Classification != target.Classification {
		return core.Fail(409, "PERMISSION_CHANGE_PLAN_REQUIRED", "跨项目或不同分类的移动须先分析权限影响并生成固定计划")
	}
	return nil
}
func (s *Service) MoveFolder(ctx context.Context, c core.Command, id string, input Move) (core.Result, error) {
	if err := moveInput(input); err != nil {
		return core.Result{}, err
	}
	authorize := func(tx pgx.Tx) error {
		source, err := authorizeFolder(ctx, tx, c.Actor, id, "folder.update")
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
			return sameScope(source, target)
		}
		return nil
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		source, err := folder(ctx, tx, c.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		if err = namespaceLock(ctx, tx, c.Actor, source.ProjectID); err != nil {
			return core.Result{}, err
		}
		if err = authorize(tx); err != nil {
			return core.Result{}, err
		}
		source, err = folder(ctx, tx, c.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if source.Version != input.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		before := digest(source)
		parentID := source.ParentID
		name := source.Name
		if input.TargetFolderID != nil {
			if source.ParentID == nil && source.ProjectID != nil {
				return core.Result{}, core.Fail(409, "PROJECT_ROOT_FIXED", "项目资料根目录不能移动")
			}
			var cyclic bool
			err = tx.QueryRow(ctx, `WITH RECURSIVE chain AS (
                SELECT id,parent_id FROM folders WHERE org_id=$1 AND id=$2
                UNION SELECT f.id,f.parent_id FROM folders f JOIN chain c ON f.id=c.parent_id WHERE f.org_id=$1
            ) SELECT EXISTS(SELECT 1 FROM chain WHERE id=$3)`, c.Actor.OrgID, *input.TargetFolderID, id).Scan(&cyclic)
			if err != nil {
				return core.Result{}, err
			}
			if cyclic {
				return core.Result{}, core.Invalid("target_folder_id", "不能将目录移动到自身或后代目录")
			}
			parentID = input.TargetFolderID
		}
		if input.NewName != nil {
			name = *input.NewName
		}
		key, err := core.StorageNameKey(name)
		if err != nil {
			return core.Result{}, err
		}
		_, err = tx.Exec(ctx, "UPDATE folders SET parent_id=$3,name=$4,name_key=$5,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, parentID, name, key)
		if err != nil {
			return core.Result{}, err
		}
		result, err := folder(ctx, tx, c.Actor.OrgID, id, false)
		result.CanUpdate = true
		result.CanUpload = uploadChannel(c.Actor) == nil && authz.Require(ctx, tx, c.Actor, "file.upload", folderResource(result)) == nil
		result.CanCreate = authz.Require(ctx, tx, c.Actor, "folder.create", folderResource(result)) == nil
		return core.Result{Data: result, Event: core.Event{Type: "folder.moved", ResourceType: "folder", ResourceID: id, Version: result.Version, BeforeDigest: before, Payload: map[string]any{"reason": input.Reason, "parent_id": parentID}}}, err
	})
}
