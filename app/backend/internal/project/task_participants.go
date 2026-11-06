package project

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"strings"
)

// Participation records collaboration only; ordinary task/project policy still controls access.
func validateParticipantIDs(ids []string) error {
	if len(ids) > 50 {
		return core.Invalid("participant_ids", "任务参与人最多50位")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		key := strings.ToLower(id)
		if !core.ValidID(id) || seen[key] {
			return core.Invalid("participant_ids", "请提供不重复的准确参与人")
		}
		seen[key] = true
	}
	return nil
}
func validTaskParticipants(ctx context.Context, db platform.DBTX, org, projectID string, ids []string) error {
	if err := validateParticipantIDs(ids); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	var count int
	err := db.QueryRow(ctx, `SELECT count(*) FROM project_members pm JOIN users u ON u.org_id=pm.org_id AND u.id=pm.user_id WHERE pm.org_id=$1 AND pm.project_id=$2 AND pm.user_id=ANY($3::uuid[]) AND u.status='active'`, org, projectID, ids).Scan(&count)
	if err != nil {
		return err
	}
	if count != len(ids) {
		return core.Invalid("participant_ids", "参与人必须是本项目当前有效成员")
	}
	return nil
}
