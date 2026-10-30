package project

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
)

type BusinessCapability struct {
	Kind          string  `json:"kind"`
	CanCreate     bool    `json:"can_create"`
	BlockedReason *string `json:"blocked_reason"`
}

// This is a current read of ordinary new-record authority, not a delegation or
// a reservation. Each business command still authorizes its exact input and
// locks/rechecks the project at the time of the write.
type BusinessContext struct {
	Scope        string               `json:"scope"`
	AsOf         time.Time            `json:"as_of"`
	Project      Project              `json:"project"`
	Capabilities []BusinessCapability `json:"capabilities"`
}

func (s *Service) BusinessContext(ctx context.Context, a core.Actor, id string) (BusinessContext, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := BusinessContext{Scope: "project_business", Capabilities: []BusinessCapability{}}
	if a.RunID != "" || (a.Channel != "web" && a.Channel != "wechat" && a.Channel != "douyin" && a.Channel != "android") {
		return out, core.Forbidden()
	}
	tx, err := s.deliverableSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	p, err := getProject(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	if err = authz.Require(ctx, tx, a, "project.read", resource(p)); err != nil {
		return out, err
	}
	out.Project = p
	for _, entry := range [][3]string{
		{"contracts", "contract", "contract.create"}, {"purchases", "purchase", "purchase.create"},
		{"claims", "claim", "claim.create"}, {"costs", "cost", "cost.record"},
		{"payments", "payment", "payment.record"}, {"subscriptions", "subscription", "subscription.create"},
	} {
		capability := BusinessCapability{Kind: entry[0]}
		reason := "not_authorized"
		if p.State == "closing" || p.State == "archived" || p.State == "cancelled" {
			reason = "project_read_only"
		} else if p.Classification == "confidential" {
			// Ordinary create forms do not invent object grants for strict data.
			reason = "classification_requires_grant"
		} else {
			err = tx.QueryRow(ctx, `SELECT
			 can_access($1,$2,$3,$4,NULL,$5,$2,$6,$7,false)
			 AND can_access($1,$2,$8,$4,NULL,$5,$2,$6,$7,false)`,
				a.OrgID, a.UserID, entry[2], entry[1], p.ID, p.Classification, a.Channel, entry[1]+".read").Scan(&capability.CanCreate)
			if err != nil {
				return BusinessContext{}, err
			}
		}
		if !capability.CanCreate {
			capability.BlockedReason = &reason
		}
		out.Capabilities = append(out.Capabilities, capability)
	}
	if err = tx.QueryRow(ctx, "SELECT transaction_timestamp()").Scan(&out.AsOf); err != nil {
		return BusinessContext{}, err
	}
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return BusinessContext{}, err
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
