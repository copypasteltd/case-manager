package finance

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func NotificationSubscription(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Subscription, error) {
	s, err := subscriptionMetadata(ctx, db, a.OrgID, id)
	if err != nil {
		return s, err
	}
	return s, subscriptionReadable(ctx, db, a, s)
}
