package files

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func (s *ObjectRetirementService) Receipts(ctx context.Context, a core.Actor, itemID string, filter Filter) (core.Page[CleanupReceipt], error) {
	out := core.Page[CleanupReceipt]{Items: []CleanupReceipt{}}
	if !core.ValidID(itemID) {
		return out, core.Missing()
	}
	var planID string
	if e := s.Files.DB.QueryRow(ctx, "SELECT plan_id FROM object_retirement_items WHERE org_id=$1 AND id=$2", a.OrgID, itemID).Scan(&planID); e != nil {
		return out, platform.DatabaseError(e)
	}
	if _, e := s.readPlan(ctx, s.Files.DB, a, planID, false, false); e != nil {
		return out, e
	}
	at, id, limit, binding, e := cleanupPage(s.Files, a, "object-retirement-receipts/"+itemID, filter)
	if e != nil {
		return out, e
	}
	rows, e := s.Files.DB.Query(ctx, `SELECT id,generation,attempt,outcome,error_code,created_at FROM object_retirement_receipts
  WHERE org_id=$1 AND item_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, itemID, at, id, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v CleanupReceipt
		if e = rows.Scan(&v.ID, &v.Generation, &v.Attempt, &v.Outcome, &v.ErrorCode, &v.CreatedAt); e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		cleanupNext(s.Files, &out, limit, binding, last.ID, last.CreatedAt)
	}
	return out, nil
}
