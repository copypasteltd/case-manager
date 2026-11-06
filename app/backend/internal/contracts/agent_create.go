package contracts

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

// Validate a complete human-selected draft before any content enters a model.
// Actual creation still uses Service.Create and its ordinary atomic command.
func PrepareAgentCreate(ctx context.Context, db pgx.Tx, a core.Actor, in Create) (Create, error) {
	if a.RunID == "" || core.Name(in.Reason) != nil || !core.Classification(in.Classification) || in.Classification == "confidential" || (in.ProjectID != nil && !core.ValidID(*in.ProjectID)) {
		return in, core.Invalid("business_draft", "请明确普通合同草稿范围、分类和依据")
	}
	d := Contract{ID: core.NewID(), ProjectID: in.ProjectID, CreatorID: a.UserID, ParentContractID: in.ParentContractID, Classification: in.Classification}
	for _, action := range []string{"contract.read", "contract.create"} {
		if e := authz.Require(ctx, db, a, action, resource(d)); e != nil {
			return in, e
		}
	}
	if e := projectWritable(ctx, db, a.OrgID, d.ProjectID, d.Classification, false); e != nil {
		return in, e
	}
	if e := validateParent(ctx, db, a, d); e != nil {
		return in, e
	}
	v, e := normalize(ctx, db, a, d, in.Content, nil)
	in.Content = v
	return in, e
}
