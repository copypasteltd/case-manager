package files

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func NotificationFile(ctx context.Context, db platform.DBTX, a core.Actor, id string) (File, error) {
	f, err := authorizeFile(ctx, db, a, id, "file.read")
	if err != nil {
		return f, err
	}
	return presentFile(ctx, db, a, f)
}
