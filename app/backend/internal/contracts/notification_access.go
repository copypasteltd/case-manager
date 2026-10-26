package contracts

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func NotificationContract(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Contract, error) {
	d, err := metadata(ctx, db, a.OrgID, id)
	if err != nil {
		return d, err
	}
	return present(ctx, db, a, d)
}
