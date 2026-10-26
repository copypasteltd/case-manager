package authz

import (
	"context"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

type Resource struct {
	Type           string
	ID             string
	ProjectID      *string
	OwnerID        *string
	Classification string
}

func Require(ctx context.Context, db platform.DBTX, actor core.Actor, action string, r Resource) error {
	var allowed bool
	err := db.QueryRow(ctx, "SELECT can_access($1,$2,$3,$4,NULLIF($5,'')::uuid,$6::uuid,$7::uuid,$8,$9,$10)",
		actor.OrgID, actor.UserID, action, r.Type, r.ID, r.ProjectID, r.OwnerID, r.Classification, actor.Channel, actor.RunID != "").Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return core.Missing()
	}
	return nil
}
func RequireOrganization(ctx context.Context, db platform.DBTX, actor core.Actor, action string) error {
	return Require(ctx, db, actor, action, Resource{Type: "organization", ID: actor.OrgID, Classification: "internal"})
}
