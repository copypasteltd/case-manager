package party

import (
	"context"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

// ResolveBusinessReference is internal to a business command transaction. The
// command fixes an explicit revision; the party's later edits never retarget it.
func ResolveBusinessReference(ctx context.Context, db platform.DBTX, a core.Actor, id, revisionID string, requireActive bool) (Party, Revision, error) {
	p, err := metadata(ctx, db, a.OrgID, id)
	if err != nil {
		return Party{}, Revision{}, err
	}
	if err = authz.Require(ctx, db, a, "party.read", resource(p)); err != nil {
		return Party{}, Revision{}, err
	}
	if _, err = db.Exec(ctx, "SELECT id FROM parties WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, id); err != nil {
		return Party{}, Revision{}, err
	}
	p, err = metadata(ctx, db, a.OrgID, id)
	if err != nil {
		return Party{}, Revision{}, err
	}
	if err = authz.Require(ctx, db, a, "party.read", resource(p)); err != nil {
		return Party{}, Revision{}, err
	}
	if requireActive && p.State != "active" {
		return Party{}, Revision{}, core.Fail(409, "PARTY_ARCHIVED", "新业务引用须选择使用中的往来对象")
	}
	if !core.ValidID(revisionID) {
		return Party{}, Revision{}, core.Invalid("counterparty_revision_id", "请固定相对方资料修订")
	}
	r, err := scanRevision(db.QueryRow(ctx, "SELECT "+revisionColumns+" FROM party_revisions WHERE org_id=$1 AND party_id=$2 AND id=$3", a.OrgID, id, revisionID))
	return p, r, err
}
