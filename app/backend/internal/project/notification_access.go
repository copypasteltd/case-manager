package project

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

// NotificationTask applies the same current business read rule inside the
// caller's authorization snapshot. Notification delivery grants no new access.
func NotificationTask(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Task, error) {
	t, err := getTask(ctx, db, a.OrgID, id, false)
	if err != nil {
		return t, err
	}
	return t, authorizeTask(ctx, db, a, id, "task.read")
}
