package contracts

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// Ordinary draft authority and normalization remain in the owning domain.
func PrepareAgentRevision(ctx context.Context, tx pgx.Tx, a core.Actor, id string, in Patch) (Patch, Contract, error) {
	d, e := metadata(ctx, tx, a.OrgID, id)
	if e != nil {
		return in, d, e
	}
	if a.RunID == "" || !editable(d) {
		return in, d, core.Forbidden()
	}
	for _, action := range []string{"contract.read", "contract.update"} {
		if e = authz.Require(ctx, tx, a, action, resource(d)); e != nil {
			return in, d, e
		}
	}
	if d.Version != in.ExpectedVersion || d.CurrentRevisionID != in.BaseRevisionID {
		return in, d, core.Conflict()
	}
	if core.Name(in.Reason) != nil {
		return in, d, core.Invalid("reason", "请填写准确草稿修订依据")
	}
	if e = projectWritable(ctx, tx, a.OrgID, d.ProjectID, d.Classification, false); e != nil {
		return in, d, e
	}
	in.Content, e = normalize(ctx, tx, a, d, in.Content, &d.Content)
	if e != nil {
		return in, d, e
	}
	d, e = present(ctx, tx, a, d)
	return in, d, e
}
