package files

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// The agent may inspect only a human-owned exact ordinary plan, under both the
// original human authority and AI policies on every source/target item.
func AgentScopeTransferPreview(ctx context.Context, tx pgx.Tx, human, ai core.Actor, id string) (ScopeTransfer, error) {
	p, e := scopeTransfer(ctx, tx, human, id, false)
	if e != nil {
		return p, e
	}
	if ai.RunID == "" || human.RunID != "" || ai.UserID != human.UserID || ai.OrgID != human.OrgID || ai.Channel != human.Channel {
		return p, core.Forbidden()
	}
	if e = transferReadAuthority(ctx, tx, human, p); e != nil {
		return p, e
	}
	if p.State != "pending" || p.ActorAuthVersion != human.AuthVersion || p.OrganizationVersion != human.OrgAuthVersion {
		return p, core.Fail(409, "TRANSFER_BASIS_CHANGED", "准确计划或原授权已改变")
	}
	current, e := scopeSnapshot(ctx, tx, human, p.Input)
	if e != nil {
		return p, e
	}
	if scopePlanHash(p.Input, current) != p.PlanHash || transferBlocked(current) {
		return p, core.Fail(409, "TRANSFER_BASIS_CHANGED", "计划存在变化或阻塞，请重新核对")
	}
	for _, item := range p.Snapshot.Items {
		before := authz.Resource{Type: item.Kind, ID: item.ID, ProjectID: item.ProjectID, OwnerID: item.CreatorID, Classification: item.Classification}
		after := before
		after.ProjectID = p.Snapshot.Target.ProjectID
		after.Classification = item.NewClassification
		for _, r := range []authz.Resource{before, after} {
			if e = authz.Require(ctx, tx, ai, item.Kind+".read", r); e != nil {
				return p, e
			}
		}
	}
	for _, entry := range append(append([]OrganizationEntry{p.Snapshot.Source, p.Snapshot.Target}, p.Snapshot.SourcePath...), p.Snapshot.TargetPath...) {
		if _, e = OrganizationMetadata(ctx, tx, ai, entry.Kind, entry.ID, entry.Kind+".read"); e != nil {
			return p, e
		}
	}
	permission := "file.move"
	if p.Input.Kind == "folder" {
		permission = "folder.update"
	}
	if _, e = OrganizationMetadata(ctx, tx, ai, p.Input.Kind, p.Input.ResourceID, permission); e != nil {
		return p, e
	}
	if _, e = OrganizationMetadata(ctx, tx, ai, "folder", p.Input.TargetFolderID, "folder.create"); e != nil {
		return p, e
	}
	p.CurrentBasisMatches = true
	p.CanExecute = true
	return p, nil
}

// A committed scope change exposes only its current target identity to the
// secretary result reader, not the prior audience-impact inventory.
func AgentScopeTransferResultReadable(ctx context.Context, tx pgx.Tx, a core.Actor, id string) error {
	p, e := scopeTransfer(ctx, tx, a, id, false)
	if e != nil {
		return e
	}
	if p.State != "executed" {
		return core.Missing()
	}
	_, e = OrganizationMetadata(ctx, tx, a, p.Input.Kind, p.Input.ResourceID, p.Input.Kind+".read")
	return e
}
