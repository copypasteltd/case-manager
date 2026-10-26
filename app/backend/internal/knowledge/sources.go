package knowledge

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func sourceAccess(ctx context.Context, db platform.DBTX, a core.Actor, source Source) error {
	if source.Type != "file" || !core.ValidID(source.ID) || (source.VersionID != nil && !core.ValidID(*source.VersionID)) {
		return core.Invalid("source_refs", "附件须引用有效文件与可选固定版本")
	}
	var project *string
	var creator *string
	var class string
	var current *string
	err := db.QueryRow(ctx, `WITH RECURSIVE active_tree AS (
        SELECT id,project_id FROM folders WHERE org_id=$1 AND parent_id IS NULL AND state='active'
        UNION ALL SELECT f.id,f.project_id FROM folders f JOIN active_tree a ON f.parent_id=a.id
        WHERE f.org_id=$1 AND f.state='active'
    ) SELECT d.project_id,f.creator_id,f.classification,f.current_version_id FROM files f JOIN active_tree d ON d.id=f.folder_id
        WHERE f.org_id=$1 AND f.id=$2 AND f.state='active'`, a.OrgID, source.ID).Scan(&project, &creator, &class, &current)
	if err != nil {
		return platform.DatabaseError(err)
	}
	if err = authz.Require(ctx, db, a, "file.read", authz.Resource{Type: "file", ID: source.ID, ProjectID: project, OwnerID: creator, Classification: class}); err != nil {
		return err
	}
	selected := current
	if source.VersionID != nil {
		selected = source.VersionID
	}
	if selected == nil {
		return core.Missing()
	}
	var valid bool
	err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM file_versions WHERE org_id=$1 AND file_id=$2 AND id=$3 AND availability='available' AND scan_state IN ('clean','waived'))", a.OrgID, source.ID, *selected).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return core.Missing()
	}
	return nil
}
func validateSources(ctx context.Context, db platform.DBTX, a core.Actor, sources []Source) error {
	for _, source := range sources {
		if err := sourceAccess(ctx, db, a, source); err != nil {
			return err
		}
	}
	return nil
}
func filterSources(ctx context.Context, db platform.DBTX, a core.Actor, sources []Source) ([]Source, error) {
	out := []Source{}
	for _, source := range sources {
		err := sourceAccess(ctx, db, a, source)
		if err == nil {
			out = append(out, source)
			continue
		}
		public := core.PublicError(platform.DatabaseError(err))
		if public.Status >= 500 {
			return nil, err
		}
	}
	return out, nil
}
