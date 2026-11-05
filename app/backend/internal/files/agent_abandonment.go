package files

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func AuthorizeAgentDraftAbandonment(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Upload, error) {
	if a.RunID != "" || a.SessionID == "" {
		return Upload{}, core.Forbidden()
	}
	u, err := upload(ctx, db, a.OrgID, id, false)
	if err != nil {
		return u, err
	}
	if u.OwnerID != a.UserID || u.SourceChannel == nil || *u.SourceChannel != "agent" {
		return u, core.Missing()
	}
	if u.State == "committed" || u.ResultVersionID != nil || u.RetirementPlanID != nil {
		return u, core.Fail(409, "FILE_RECOVERY_UNAVAILABLE", "此草稿已经提交或进入清理，不能重新授权恢复")
	}
	_, err = uploadTarget(ctx, db, a, u.FolderID, u.FileID)
	return u, err
}
