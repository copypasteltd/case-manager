package files

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const scopeTransferMaxItems = 100

func scopeTransferInput(in ScopeTransferCreate) error {
	if !slices.Contains([]string{"file", "folder"}, in.Kind) || !organizationID(in.ResourceID) || !organizationID(in.TargetFolderID) || core.RequireVersion(in.ExpectedVersion) != nil || core.RequireVersion(in.ExpectedTargetVersion) != nil || core.Name(in.Reason) != nil {
		return core.Invalid("transfer", "请固定资料类型、当前版本、目标目录版本及操作原因")
	}
	if in.NewName != nil {
		_, e := core.StorageNameKey(*in.NewName)
		return e
	}
	return nil
}

func scopeTransferAuthority(ctx context.Context, db platform.DBTX, a core.Actor, mfa bool) error {
	if e := identity.RequireInteractiveSession(ctx, db, a, mfa); e != nil {
		return e
	}
	for _, action := range []string{"policy.manage", "user.read"} {
		if e := authz.RequireOrganization(ctx, db, a, action); e != nil {
			return e
		}
	}
	return nil
}

func transferClassRank(class string) int {
	return map[string]int{"internal": 1, "project_restricted": 2, "confidential": 3}[class]
}

func transferNamespaceLocks(ctx context.Context, tx pgx.Tx, a core.Actor, source, target *string) error {
	ids := []string{}
	if source == nil || target == nil {
		if e := namespaceLock(ctx, tx, a, nil); e != nil {
			return e
		}
	}
	for _, p := range []*string{source, target} {
		if p != nil {
			ids = append(ids, *p)
		}
	}
	slices.Sort(ids)
	for _, id := range slices.Compact(ids) {
		if e := namespaceLock(ctx, tx, a, &id); e != nil {
			return e
		}
	}
	return nil
}

func transferItem(ctx context.Context, db platform.DBTX, a core.Actor, kind, id string, target OrganizationEntry) (ScopeTransferItem, error) {
	out := ScopeTransferItem{Kind: kind, ID: id, VisibilityChanges: []ScopeVisibilityChange{}, BlockingReasons: []string{}}
	var r authz.Resource
	permission := "folder.update"
	if kind == "file" {
		f, e := file(ctx, db, a.OrgID, id, false)
		if e != nil {
			return out, e
		}
		out.Version, out.Name, out.State, out.ParentID, out.ProjectID, out.CreatorID = f.Version, f.Name, f.State, &f.FolderID, f.ProjectID, f.CreatorID
		out.Classification, out.CurrentVersionID = f.Classification, f.CurrentVersionID
		r, permission = fileResource(f), "file.move"
		var evidence []byte
		var held, referenced, uploading, restoring, downloading bool
		e = db.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM file_versions v WHERE v.org_id=f.org_id AND v.file_id=f.id),
 (SELECT coalesce(jsonb_agg(jsonb_build_array(v.id,v.seq,v.sha256,v.size_bytes,v.media_type,v.scan_state,v.availability,v.original_name) ORDER BY v.seq,v.id),'[]'::jsonb) FROM file_versions v WHERE v.org_id=f.org_id AND v.file_id=f.id),
 f.legal_hold OR EXISTS(SELECT 1 FROM retention_holds h WHERE h.org_id=f.org_id AND h.file_id=f.id AND (h.valid_until IS NULL OR h.valid_until>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM retention_hold_releases z WHERE z.org_id=h.org_id AND z.hold_id=h.id)),
 EXISTS(SELECT 1 FROM file_version_references r WHERE r.org_id=f.org_id AND r.file_id=f.id AND NOT EXISTS(SELECT 1 FROM file_reference_releases z WHERE z.org_id=r.org_id AND z.reference_id=r.id)),
 EXISTS(SELECT 1 FROM upload_sessions u WHERE u.org_id=f.org_id AND u.file_id=f.id AND u.state NOT IN ('committed','aborted','expired','failed')),
 EXISTS(SELECT 1 FROM file_history_restores h WHERE h.org_id=f.org_id AND h.file_id=f.id AND h.state IN ('queued','copying','verifying','retry_wait','ready')),
 EXISTS(SELECT 1 FROM download_permits d JOIN file_versions v ON v.org_id=d.org_id AND v.id=d.version_id WHERE d.org_id=f.org_id AND v.file_id=f.id AND d.expires_at>clock_timestamp())
 FROM files f WHERE f.org_id=$1 AND f.id=$2`, a.OrgID, id).Scan(&out.ContentVersions, &evidence, &held, &referenced, &uploading, &restoring, &downloading)
		if e != nil {
			return out, e
		}
		out.ContentSHA256 = core.Digest(evidence)
		for _, b := range []struct {
			active bool
			code   string
		}{{held, "RETENTION_HOLD"}, {referenced, "BUSINESS_REFERENCE"}, {uploading, "UPLOAD_ACTIVE"}, {restoring, "RESTORE_ACTIVE"}, {downloading, "DOWNLOAD_PERMIT_ACTIVE"}} {
			if b.active {
				out.BlockingReasons = append(out.BlockingReasons, b.code)
			}
		}
	} else {
		f, e := folder(ctx, db, a.OrgID, id, false)
		if e != nil {
			return out, e
		}
		out.Version, out.Name, out.State, out.ParentID, out.ProjectID, out.CreatorID = f.Version, f.Name, f.State, f.ParentID, f.ProjectID, f.CreatorID
		out.Classification, r = f.Classification, folderResource(f)
		var uploading bool
		if e = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM upload_sessions WHERE org_id=$1 AND target_folder_id=$2 AND state NOT IN ('committed','aborted','expired','failed'))", a.OrgID, id).Scan(&uploading); e != nil {
			return out, e
		}
		if uploading {
			out.BlockingReasons = append(out.BlockingReasons, "UPLOAD_ACTIVE")
		}
	}
	for _, action := range []string{kind + ".read", permission} {
		if e := authz.Require(ctx, db, a, action, r); e != nil {
			return out, e
		}
	}
	out.NewClassification = target.Classification
	if transferClassRank(target.Classification) < transferClassRank(out.Classification) {
		out.BlockingReasons = append(out.BlockingReasons, "CLASSIFICATION_DOWNGRADE")
	}
	r.ProjectID, r.Classification = target.ProjectID, target.Classification
	if e := authz.Require(ctx, db, a, kind+".read", r); e != nil {
		out.BlockingReasons = append(out.BlockingReasons, "CALLER_TARGET_READ_REQUIRED")
	}
	return out, nil
}

func scopeInventory(ctx context.Context, tx pgx.Tx, a core.Actor, in ScopeTransferCreate, target OrganizationEntry) ([]ScopeTransferItem, error) {
	refs := [][2]string{{in.Kind, in.ResourceID}}
	if in.Kind == "folder" {
		rows, e := tx.Query(ctx, `WITH RECURSIVE tree AS (
 SELECT id FROM folders WHERE org_id=$1 AND id=$2
 UNION SELECT f.id FROM folders f JOIN tree t ON f.parent_id=t.id WHERE f.org_id=$1
 ) SELECT kind,id::text FROM (
 SELECT 'folder'::text kind,id FROM tree
 UNION ALL SELECT 'file',f.id FROM files f JOIN tree t ON f.folder_id=t.id WHERE f.org_id=$1
 ) q ORDER BY kind,id LIMIT 101`, a.OrgID, in.ResourceID)
		if e != nil {
			return nil, e
		}
		refs = [][2]string{}
		for rows.Next() {
			var ref [2]string
			if e = rows.Scan(&ref[0], &ref[1]); e != nil {
				break
			}
			refs = append(refs, ref)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	if len(refs) > scopeTransferMaxItems {
		return nil, core.Fail(422, "TRANSFER_SCOPE_TOO_LARGE", "单份计划最多100个文件与目录；请先按实际子目录分批整理")
	}
	out := []ScopeTransferItem{}
	for _, ref := range refs {
		if ref[0] == "folder" && ref[1] == target.ID {
			return nil, core.Invalid("target_folder_id", "目标目录不能是来源目录或其后代")
		}
		item, e := transferItem(ctx, tx, a, ref[0], ref[1], target)
		if e != nil {
			return nil, e
		}
		out = append(out, item)
	}
	return out, nil
}

func scopeAudiences(ctx context.Context, tx pgx.Tx, a core.Actor, items []ScopeTransferItem, project *string) error {
	var count int
	if e := tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE org_id=$1 AND status='active'", a.OrgID).Scan(&count); e != nil {
		return e
	}
	if count > 100 {
		return core.Fail(422, "TRANSFER_AUDIENCE_TOO_LARGE", "当前成员数量超过单份权限分析上限100人")
	}
	raw, _ := json.Marshal(items)
	rows, e := tx.Query(ctx, `WITH items AS (
 SELECT * FROM jsonb_to_recordset($3::jsonb) AS x(kind text,id uuid,project_id uuid,creator_id uuid,classification text,new_classification text)
 ) SELECT i.kind,i.id::text,u.id::text,u.display_name,c.channel,m.ai,
 can_access($1,u.id,i.kind||'.read',i.kind,i.id,i.project_id,i.creator_id,i.classification,c.channel,m.ai),
 can_access($1,u.id,i.kind||'.read',i.kind,i.id,$2::uuid,i.creator_id,i.new_classification,c.channel,m.ai)
 FROM items i CROSS JOIN users u CROSS JOIN (VALUES('web'),('wechat'),('douyin'),('android'),('telegram')) c(channel)
 CROSS JOIN (VALUES(false),(true)) m(ai)
 WHERE u.org_id=$1 AND u.status='active' ORDER BY i.kind,i.id,u.id,c.channel,m.ai`, a.OrgID, project, raw)
	if e != nil {
		return e
	}
	defer rows.Close()
	index := map[string]int{}
	all := map[string][]ScopeVisibilityChange{}
	for i := range items {
		index[items[i].Kind+"/"+items[i].ID] = i
	}
	for rows.Next() {
		var kind, id string
		var v ScopeVisibilityChange
		if e = rows.Scan(&kind, &id, &v.UserID, &v.DisplayName, &v.Channel, &v.AI, &v.Before, &v.After); e != nil {
			return e
		}
		key := kind + "/" + id
		item := &items[index[key]]
		all[key] = append(all[key], v)
		if v.AI {
			if v.Before {
				item.AIBefore++
			}
			if v.After {
				item.AIAfter++
			}
		} else {
			if v.Before {
				item.HumanBefore++
			}
			if v.After {
				item.HumanAfter++
			}
		}
		if v.Before != v.After {
			item.VisibilityChanges = append(item.VisibilityChanges, v)
		}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	for i := range items {
		items[i].AudienceSHA256 = digest(all[items[i].Kind+"/"+items[i].ID])
	}
	return nil
}

func scopeSnapshot(ctx context.Context, tx pgx.Tx, a core.Actor, in ScopeTransferCreate) (ScopeTransferSnapshot, error) {
	out := ScopeTransferSnapshot{}
	permission := "file.move"
	if in.Kind == "folder" {
		permission = "folder.update"
	}
	var e error
	out.Source, e = OrganizationMetadata(ctx, tx, a, in.Kind, in.ResourceID, permission)
	if e != nil {
		return out, e
	}
	out.Target, e = OrganizationMetadata(ctx, tx, a, "folder", in.TargetFolderID, "folder.create")
	if e != nil {
		return out, e
	}
	if out.Source.Version != in.ExpectedVersion || out.Target.Version != in.ExpectedTargetVersion {
		return out, core.Conflict()
	}
	if sameID(out.Source.ProjectID, out.Target.ProjectID) && out.Source.Classification == out.Target.Classification {
		return out, core.Fail(422, "SAME_SCOPE_MOVE_AVAILABLE", "相同项目与分类请使用普通改名或移动")
	}
	if in.Kind == "folder" && out.Source.ParentID == nil {
		return out, core.Fail(409, "ROOT_SCOPE_FIXED", "公司与项目根目录不能跨范围移动，请选择其内的资料")
	}
	if out.Target.ProjectID == nil && out.Target.Classification == "project_restricted" {
		return out, core.Invalid("target_folder_id", "项目受限目录必须属于实际项目")
	}
	if e = transferNamespaceLocks(ctx, tx, a, out.Source.ProjectID, out.Target.ProjectID); e != nil {
		return out, e
	}
	out.SourcePath, e = organizationPath(ctx, tx, a, out.Source.ParentID)
	if e != nil {
		return out, e
	}
	out.TargetPath, e = organizationPath(ctx, tx, a, out.Target.ParentID)
	if e != nil {
		return out, e
	}
	out.NewName = out.Source.Name
	if in.NewName != nil {
		out.NewName = *in.NewName
	}
	key, e := core.StorageNameKey(out.NewName)
	if e != nil {
		return out, e
	}
	var collision bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM storage_entries WHERE org_id=$1 AND namespace_key=$2 AND name_key=$3 AND resource_id<>$4)", a.OrgID, "folder/"+out.Target.ID, key, out.Source.ID).Scan(&collision); e != nil {
		return out, e
	}
	if collision {
		return out, core.Fail(409, "DUPLICATE_RESOURCE", "目标目录已有同名文件或目录")
	}
	out.Items, e = scopeInventory(ctx, tx, a, in, out.Target)
	if e != nil {
		return out, e
	}
	for _, item := range out.Items {
		if !sameID(item.ProjectID, out.Source.ProjectID) {
			return out, core.Fail(409, "TRANSFER_SOURCE_INCONSISTENT", "来源目录树存在不一致的项目归属，须先核对原记录")
		}
	}
	e = scopeAudiences(ctx, tx, a, out.Items, out.Target.ProjectID)
	return out, e
}

func scopePlanHash(in ScopeTransferCreate, snapshot ScopeTransferSnapshot) string {
	return digest([]any{in, snapshot})
}

func transferBlocked(snapshot ScopeTransferSnapshot) bool {
	for _, item := range snapshot.Items {
		if len(item.BlockingReasons) != 0 {
			return true
		}
	}
	return false
}

func scopeHashValid(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}
