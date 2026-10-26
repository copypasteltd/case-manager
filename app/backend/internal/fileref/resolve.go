package fileref

import (
	"context"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
)

type Evidence struct {
	FileID        string  `json:"file_id"`
	VersionID     string  `json:"version_id"`
	Seq           int64   `json:"seq"`
	Name          string  `json:"name"`
	SHA256        string  `json:"sha256"`
	SizeBytes     string  `json:"size_bytes"`
	MediaType     string  `json:"media_type"`
	ScanState     string  `json:"scan_state"`
	ScanWaiverRef *string `json:"scan_waiver_ref"`
}

func ForApproval(ctx context.Context, db platform.DBTX, a core.Actor, versionID string, project *string, classification string) (Evidence, error) {
	value, err := Resolve(ctx, db, a, versionID, project, classification)
	if err != nil {
		return Evidence{}, err
	}
	var sourceClass string
	if err = db.QueryRow(ctx, "SELECT classification FROM files WHERE org_id=$1 AND id=$2", a.OrgID, value.FileID).Scan(&sourceClass); err != nil {
		return Evidence{}, err
	}
	if err = files.VerifyBusinessEvidence(ctx, db, a.OrgID, value.FileID, value.VersionID, sourceClass); err != nil {
		return Evidence{}, err
	}
	return value, nil
}

// Resolve fixes an available formal file version inside the calling business
// transaction. It returns no object key, provider identity or signed URL.
func Resolve(ctx context.Context, db platform.DBTX, a core.Actor, versionID string, project *string, classification string) (Evidence, error) {
	if !core.ValidID(versionID) {
		return Evidence{}, core.Invalid("file_version_id", "请指定准确文件版本")
	}
	var file string
	if err := db.QueryRow(ctx, "SELECT file_id FROM file_versions WHERE org_id=$1 AND id=$2", a.OrgID, versionID).Scan(&file); err != nil {
		return Evidence{}, platform.DatabaseError(err)
	}
	// File mutations never acquire business heads. Keeping this lock through
	// reference registration prevents a concurrent trash from passing the check.
	if _, err := db.Exec(ctx, "SELECT id FROM files WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, file); err != nil {
		return Evidence{}, err
	}
	var sourceProject *string
	var creator *string
	var class string
	var e Evidence
	var acceptable bool
	err := db.QueryRow(ctx, `WITH RECURSIVE active_tree AS (
        SELECT id,project_id FROM folders WHERE org_id=$1 AND parent_id IS NULL AND state='active'
        UNION ALL SELECT f.id,f.project_id FROM folders f JOIN active_tree a ON a.id=f.parent_id WHERE f.org_id=$1 AND f.state='active'
    ) SELECT f.id,v.id,v.seq,v.original_name,v.sha256,v.size_bytes::text,v.media_type,v.scan_state,v.scan_waiver_ref,t.project_id,f.creator_id,f.classification,
        v.availability='available' AND v.scan_state IN ('clean','waived') AND classification_rank(f.classification)<=classification_rank($3)
        FROM file_versions v JOIN files f ON f.org_id=v.org_id AND f.id=v.file_id JOIN active_tree t ON t.id=f.folder_id
        WHERE v.org_id=$1 AND v.id=$2 AND f.state='active'`, a.OrgID, versionID, classification).Scan(&e.FileID, &e.VersionID, &e.Seq, &e.Name, &e.SHA256, &e.SizeBytes, &e.MediaType, &e.ScanState, &e.ScanWaiverRef, &sourceProject, &creator, &class, &acceptable)
	if err != nil {
		return Evidence{}, platform.DatabaseError(err)
	}
	if err = authz.Require(ctx, db, a, "file.read", authz.Resource{Type: "file", ID: file, ProjectID: sourceProject, OwnerID: creator, Classification: class}); err != nil {
		return Evidence{}, err
	}
	if !acceptable || (sourceProject != nil && (project == nil || *sourceProject != *project)) {
		return Evidence{}, core.Invalid("file_version_id", "附件须可用、范围匹配且分类不高于业务资料")
	}
	return e, nil
}
