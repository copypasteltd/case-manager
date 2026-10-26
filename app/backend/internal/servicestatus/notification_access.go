package servicestatus

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func NotificationIncident(ctx context.Context, db platform.DBTX, a core.Actor, id string) (Incident, error) {
	v, err := incidentMetadata(ctx, db, a.OrgID, id)
	if err != nil {
		return v, err
	}
	return v, incidentAccess(ctx, db, a, v.ServiceIDs, false, false)
}
