package workflow

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func NotificationRequest(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Request, error) {
	r, err := requestMetadata(ctx, db, a.OrgID, id)
	if err != nil {
		return r, err
	}
	return presentRequest(ctx, db, a, r)
}
