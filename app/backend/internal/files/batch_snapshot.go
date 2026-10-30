package files

import (
	"context"
	"encoding/json"
	"sort"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func batchInput(in FileBatchCreate) (FileBatchCreate, error) {
	if (in.Action != "move" && in.Action != "trash" && in.Action != "restore") || len(in.Files) < 1 || len(in.Files) > FileBatchLimit || core.Name(in.Reason) != nil {
		return in, core.Invalid("files", "请选择1至50份准确文件及移动、移入回收站或恢复操作，并填写原因")
	}
	if (in.Action == "move") != (in.Target != nil) || in.Target != nil && (!organizationID(in.Target.ID) || core.RequireVersion(in.Target.ExpectedVersion) != nil) {
		return in, core.Invalid("target", "只有移动操作需要准确目标目录和目录版本")
	}
	in.Files = append([]FileBatchSelection{}, in.Files...)
	sort.Slice(in.Files, func(i, j int) bool { return in.Files[i].ID < in.Files[j].ID })
	for i, f := range in.Files {
		if !organizationID(f.ID) || core.RequireVersion(f.ExpectedVersion) != nil || f.CurrentVersionID != nil && !organizationID(*f.CurrentVersionID) || i > 0 && f.ID == in.Files[i-1].ID {
			return in, core.Invalid("files", "文件不得重复；请固定文件修订及准确正式内容头")
		}
	}
	return in, nil
}

func batchAction(action string) string {
	if action == "restore" {
		return "file.restore"
	}
	if action == "trash" {
		return "file.trash"
	}
	return "file.move"
}

func batchAuthority(ctx context.Context, db platform.DBTX, a core.Actor, in FileBatchCreate, mfa bool) error {
	if e := identity.RequireInteractiveSession(ctx, db, a, mfa); e != nil {
		return e
	}
	for _, item := range in.Files {
		f, e := authorizeFile(ctx, db, a, item.ID, "file.read")
		if e != nil {
			return e
		}
		if e = authz.Require(ctx, db, a, batchAction(in.Action), fileResource(f)); e != nil {
			return e
		}
	}
	if in.Target != nil {
		target, e := authorizeFolder(ctx, db, a, in.Target.ID, "folder.read")
		if e != nil {
			return e
		}
		return authz.Require(ctx, db, a, "folder.create", folderResource(target))
	}
	return nil
}

func batchSnapshot(ctx context.Context, tx pgx.Tx, a core.Actor, in FileBatchCreate) (FileBatchSnapshot, error) {
	out := FileBatchSnapshot{Items: []FileBatchItem{}}
	if e := batchAuthority(ctx, tx, a, in, false); e != nil {
		return out, e
	}
	var target *Folder
	if in.Target != nil {
		f, e := authorizeFolder(ctx, tx, a, in.Target.ID, "folder.create")
		if e != nil {
			return out, e
		}
		if f.Version != in.Target.ExpectedVersion {
			return out, core.Conflict()
		}
		path, e := organizationPath(ctx, tx, a, f.ParentID)
		if e != nil {
			return out, e
		}
		target = &f
		out.Target = &FileBatchDestination{Folder: OrganizationEntry{Kind: "folder", ID: f.ID, Version: f.Version, Name: f.Name, ParentID: f.ParentID, ProjectID: f.ProjectID, Classification: f.Classification}, Path: path}
	}
	seen := map[string]bool{}
	for _, selected := range in.Files {
		f, e := authorizeFile(ctx, tx, a, selected.ID, batchAction(in.Action))
		if e != nil {
			return out, e
		}
		if f.Version != selected.ExpectedVersion || !sameID(f.CurrentVersionID, selected.CurrentVersionID) {
			return out, core.Conflict()
		}
		path, e := organizationPath(ctx, tx, a, &f.FolderID)
		if e != nil {
			return out, e
		}
		held, e := retentionHeld(ctx, tx, a.OrgID, f.ID)
		if e != nil {
			return out, e
		}
		item := FileBatchItem{File: batchFile(f), Path: path, RetentionHeld: held, BlockingReasons: []string{}}
		expected := "active"
		if in.Action == "restore" {
			expected = "trashed"
		}
		if f.State != expected {
			item.BlockingReasons = append(item.BlockingReasons, "file_state_changed")
		}
		if !writable(ctx, tx, a.OrgID, f.ProjectID) {
			item.BlockingReasons = append(item.BlockingReasons, "project_read_only")
		}
		if in.Action == "trash" && held {
			item.BlockingReasons = append(item.BlockingReasons, "retention_hold")
		}
		destination := f.FolderID
		if target != nil {
			if e = sameScope(Folder{ProjectID: f.ProjectID, Classification: f.Classification}, *target); e != nil {
				return out, e
			}
			if !writable(ctx, tx, a.OrgID, target.ProjectID) {
				item.BlockingReasons = append(item.BlockingReasons, "project_read_only")
			}
			destination = target.ID
			if destination == f.FolderID {
				item.BlockingReasons = append(item.BlockingReasons, "already_in_target")
			}
		}
		if in.Action != "trash" {
			key, e := core.StorageNameKey(f.Name)
			if e != nil {
				return out, e
			}
			var collision bool
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM storage_entries WHERE org_id=$1 AND namespace_key='folder/'||$2 AND name_key=$3 AND resource_id<>$4)`, a.OrgID, destination, key, f.ID).Scan(&collision)
			if e != nil {
				return out, e
			}
			if collision || seen[destination+"/"+key] {
				item.BlockingReasons = append(item.BlockingReasons, "name_conflict")
			}
			seen[destination+"/"+key] = true
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func batchBytes(in FileBatchCreate, snapshot FileBatchSnapshot) ([]byte, []byte, string) {
	input, _ := json.Marshal(in)
	raw, _ := json.Marshal(snapshot)
	return input, raw, core.Digest(append(append(append([]byte{}, input...), '\n'), raw...))
}

func batchBlocked(s FileBatchSnapshot) bool {
	for _, item := range s.Items {
		if len(item.BlockingReasons) > 0 {
			return true
		}
	}
	return false
}
