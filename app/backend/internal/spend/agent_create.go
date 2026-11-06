package spend

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func PrepareAgentCreate(ctx context.Context, db platform.DBTX, a core.Actor, kind string, in Create) (Create, error) {
	if a.RunID == "" || !ValidKind(kind) || core.Name(in.Reason) != nil || !core.Classification(in.Classification) || in.Classification == "confidential" || (in.ProjectID != nil && !core.ValidID(*in.ProjectID)) {
		return in, core.Invalid("business_draft", "请明确采购或报销草稿范围、分类和依据")
	}
	d := Record{ID: core.NewID(), Kind: kind, CreatorID: a.UserID, ProjectID: in.ProjectID, Classification: in.Classification}
	for _, action := range []string{kind + ".read", kind + ".create"} {
		if e := authz.Require(ctx, db, a, action, resource(d)); e != nil {
			return in, e
		}
	}
	if e := projectWritable(ctx, db, a, d, false); e != nil {
		return in, e
	}
	v, _, _, _, e := normalize(ctx, db, a, d, in.Content, nil, false)
	in.Content = v
	return in, e
}
