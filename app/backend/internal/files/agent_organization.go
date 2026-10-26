package files

import (
	"context"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// OrganizationAction only changes namespace metadata. New directories inherit
// their parent's classification; no content, grants or retention state changes.
type OrganizationAction struct {
	Kind                  string  `json:"kind"`
	ID                    string  `json:"id"`
	ExpectedVersion       int64   `json:"expected_version"`
	Name                  *string `json:"name,omitempty"`
	TargetFolderID        *string `json:"target_folder_id,omitempty"`
	ExpectedTargetVersion *int64  `json:"expected_target_version,omitempty"`
}
type OrganizationEntry struct {
	Kind           string  `json:"kind"`
	ID             string  `json:"id"`
	Version        int64   `json:"version"`
	Name           string  `json:"name"`
	ParentID       *string `json:"parent_id"`
	ProjectID      *string `json:"project_id"`
	Classification string  `json:"classification"`
}
type OrganizationPreview struct {
	Action     OrganizationAction  `json:"action"`
	Source     OrganizationEntry   `json:"source"`
	Path       []OrganizationEntry `json:"path"`
	Target     *OrganizationEntry  `json:"target"`
	TargetPath []OrganizationEntry `json:"target_path"`
}
type DirectoryListing struct {
	Folder  OrganizationEntry   `json:"folder"`
	Items   []OrganizationEntry `json:"items"`
	HasMore bool                `json:"has_more"`
}

func organizationID(id string) bool { return core.ValidID(id) && id == strings.ToLower(id) }

func OrganizationMetadata(ctx context.Context, tx pgx.Tx, a core.Actor, kind, id, action string) (OrganizationEntry, error) {
	var out OrganizationEntry
	if !organizationID(id) {
		return out, core.Missing()
	}
	if kind == "folder" {
		f, err := authorizeFolder(ctx, tx, a, id, action)
		if err != nil {
			return out, err
		}
		if err = authz.Require(ctx, tx, a, "folder.read", folderResource(f)); err != nil {
			return out, err
		}
		return OrganizationEntry{Kind: kind, ID: f.ID, Version: f.Version, Name: f.Name, ParentID: f.ParentID, ProjectID: f.ProjectID, Classification: f.Classification}, nil
	}
	if kind != "file" {
		return out, core.Missing()
	}
	f, err := authorizeFile(ctx, tx, a, id, action)
	if err != nil {
		return out, err
	}
	if f.State != "active" {
		return out, core.Missing()
	}
	if err = authz.Require(ctx, tx, a, "file.read", fileResource(f)); err != nil {
		return out, err
	}
	return OrganizationEntry{Kind: kind, ID: f.ID, Version: f.Version, Name: f.Name, ParentID: &f.FolderID, ProjectID: f.ProjectID, Classification: f.Classification}, nil
}
func organizationPath(ctx context.Context, tx pgx.Tx, a core.Actor, parent *string) ([]OrganizationEntry, error) {
	out := []OrganizationEntry{}
	seen := map[string]bool{}
	for parent != nil {
		if len(out) >= 32 || seen[*parent] {
			return nil, core.Invalid("directory", "目录层级超过整理工具上限")
		}
		v, err := OrganizationMetadata(ctx, tx, a, "folder", *parent, "folder.read")
		if err != nil {
			return nil, err
		}
		seen[v.ID] = true
		out = append(out, v)
		parent = v.ParentID
	}
	return out, nil
}

// A namespace lock serializes validation and the later business commands.
// The caller already owns the organization authorization lock.
func PreviewOrganization(ctx context.Context, tx pgx.Tx, a core.Actor, projectID *string, actions []OrganizationAction, lock bool) ([]OrganizationPreview, error) {
	if len(actions) < 1 || len(actions) > 10 {
		return nil, core.Invalid("actions", "每份整理计划需要1至10项准确变更")
	}
	if lock {
		if err := namespaceLock(ctx, tx, a, projectID); err != nil {
			return nil, err
		}
	} else if !writable(ctx, tx, a.OrgID, projectID) {
		return nil, core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许整理资料")
	}
	out := []OrganizationPreview{}
	seen := map[string]bool{}
	for _, act := range actions {
		key := act.ID
		if act.Kind == "folder.create" && act.Name != nil {
			key = "create/" + act.ID + "/" + *act.Name
		}
		if !organizationID(act.ID) || core.RequireVersion(act.ExpectedVersion) != nil || seen[key] {
			return nil, core.Invalid("actions", "同一对象只可出现一次，请指定准确版本")
		}
		seen[key] = true
		if act.Name != nil {
			if _, err := core.StorageNameKey(*act.Name); err != nil {
				return nil, err
			}
		}
		kind, permission := "folder", "folder.update"
		switch act.Kind {
		case "folder.create":
			permission = "folder.create"
			if act.Name == nil || act.TargetFolderID != nil || act.ExpectedTargetVersion != nil {
				return nil, core.Invalid("actions", "新建目录须指定已有父目录及名称")
			}
		case "folder.move", "file.move":
			if act.Kind == "file.move" {
				kind, permission = "file", "file.move"
			}
			if act.Name == nil && act.TargetFolderID == nil {
				return nil, core.Invalid("actions", "请指定新名称或目标目录")
			}
			if (act.TargetFolderID == nil) != (act.ExpectedTargetVersion == nil) {
				return nil, core.Invalid("actions", "目标目录和准确版本必须一起提供")
			}
		default:
			return nil, core.Invalid("actions", "此整理动作尚未开放")
		}
		source, err := OrganizationMetadata(ctx, tx, a, kind, act.ID, permission)
		if err != nil {
			return nil, err
		}
		if !sameID(projectID, source.ProjectID) {
			return nil, core.Missing()
		}
		if source.Version != act.ExpectedVersion {
			return nil, core.Conflict()
		}
		path, err := organizationPath(ctx, tx, a, source.ParentID)
		if err != nil {
			return nil, err
		}
		v := OrganizationPreview{Action: act, Source: source, Path: path, TargetPath: []OrganizationEntry{}}
		if act.TargetFolderID != nil {
			if !organizationID(*act.TargetFolderID) || core.RequireVersion(*act.ExpectedTargetVersion) != nil {
				return nil, core.Invalid("actions", "目标目录版本无效")
			}
			target, err := OrganizationMetadata(ctx, tx, a, "folder", *act.TargetFolderID, "folder.create")
			if err != nil {
				return nil, err
			}
			if !sameID(source.ProjectID, target.ProjectID) || source.Classification != target.Classification {
				return nil, core.Fail(409, "PERMISSION_CHANGE_PLAN_REQUIRED", "当前整理仅支持同项目、同分类移动")
			}
			if target.Version != *act.ExpectedTargetVersion {
				return nil, core.Conflict()
			}
			v.Target, v.TargetPath = &target, []OrganizationEntry{}
			v.TargetPath, err = organizationPath(ctx, tx, a, target.ParentID)
			if err != nil {
				return nil, err
			}
			if kind == "folder" {
				if source.ProjectID != nil && source.ParentID == nil {
					return nil, core.Fail(409, "PROJECT_ROOT_FIXED", "项目资料根目录不能移动")
				}
				for _, e := range append([]OrganizationEntry{target}, v.TargetPath...) {
					if e.ID == source.ID {
						return nil, core.Invalid("target_folder_id", "不能将目录移动到自身或后代目录")
					}
				}
			}
		}
		out = append(out, v)
	}
	// Keep all displayed source/target paths stable within a batch. A follow-up
	// plan can organize descendants after its parent move has been committed.
	for i, v := range out {
		for j, other := range out {
			if v.Action.Kind != "folder.move" || i == j {
				continue
			}
			if other.Source.ID == v.Source.ID {
				return nil, core.Invalid("actions", "父目录调整与依赖该目录的变更须分开拟定")
			}
			deps := append(append([]OrganizationEntry{}, other.Path...), other.TargetPath...)
			if other.Target != nil {
				deps = append(deps, *other.Target)
			}
			for _, e := range deps {
				if e.ID == v.Source.ID {
					return nil, core.Invalid("actions", "父目录调整与依赖该目录的变更须分开拟定")
				}
			}
		}
	}
	return out, nil
}

// List one explicitly chosen directory. The fixed bound and stable ordering
// avoid recursive enumeration or a model-controlled expansion of scope.
func AgentDirectory(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (DirectoryListing, error) {
	out := DirectoryListing{Items: []OrganizationEntry{}}
	var err error
	out.Folder, err = OrganizationMetadata(ctx, tx, a, "folder", id, "folder.read")
	if err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT kind,id FROM (
 SELECT 'folder'::text kind,f.id FROM folders f WHERE f.org_id=$1 AND f.parent_id=$2 AND f.state='active'
 AND can_access(f.org_id,$3,'folder.read','folder',f.id,f.project_id,f.creator_id,f.classification,$4,true)
 UNION ALL SELECT 'file',f.id FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id WHERE f.org_id=$1 AND f.folder_id=$2 AND f.state='active'
 AND can_access(f.org_id,$3,'file.read','file',f.id,d.project_id,f.creator_id,f.classification,$4,true)
 ) x ORDER BY kind,id LIMIT 51`, a.OrgID, id, a.UserID, a.Channel)
	if err != nil {
		return out, err
	}
	refs := [][2]string{}
	for rows.Next() {
		var r [2]string
		if err = rows.Scan(&r[0], &r[1]); err != nil {
			rows.Close()
			return out, err
		}
		refs = append(refs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	out.HasMore = len(refs) > 50
	if out.HasMore {
		refs = refs[:50]
	}
	for _, r := range refs {
		v, err := OrganizationMetadata(ctx, tx, a, r[0], r[1], r[0]+".read")
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}
