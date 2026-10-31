package files

import (
	"context"
	"copypaste.hk/platform/internal/jobqueue"
	"github.com/jackc/pgx/v5"
	"strconv"
)

// A crashed final attempt cannot turn into an unaudited terminal item. This
// sweep records failure only; explicit human retry must reconcile cloud state.
func (s *ObjectRetirementService) sweepDead(ctx context.Context) error {
	rows, err := s.Files.DB.Query(ctx, `SELECT i.org_id,i.plan_id,i.id,j.id FROM object_retirement_items i JOIN jobs j
 ON j.org_id=i.org_id AND j.payload->>'item_id'=i.id::text AND j.payload->>'generation'=i.generation::text
 WHERE i.state IN ('queued','running','retry_wait') AND j.kind=$1 AND j.lane=$2 AND j.state='dead' AND j.attempt>0
 ORDER BY i.updated_at,i.id LIMIT 100`, RetirementJobKind, RetirementJobLane)
	if err != nil {
		return err
	}
	type target struct{ org, plan, item, job string }
	targets := []target{}
	for rows.Next() {
		var v target
		if err = rows.Scan(&v.org, &v.plan, &v.item, &v.job); err != nil {
			break
		}
		targets = append(targets, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range targets {
		if err = s.sweepDeadItem(ctx, v.org, v.plan, v.item, v.job); err != nil {
			return err
		}
	}
	return nil
}

func (s *ObjectRetirementService) sweepDeadItem(ctx context.Context, org, planID, itemID, jobID string) error {
	tx, e := s.Files.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var lock string
	if e = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", org).Scan(&lock); e != nil {
		return e
	}
	p, e := retirementPlan(ctx, tx, org, planID, true)
	if e != nil {
		return e
	}
	item, e := retirementItem(tx.QueryRow(ctx, "SELECT "+retirementItemColumns+" FROM object_retirement_items WHERE org_id=$1 AND id=$2 FOR UPDATE", org, itemID))
	if e != nil {
		return e
	}
	if item.State != "queued" && item.State != "running" && item.State != "retry_wait" {
		return tx.Commit(ctx)
	}
	j := jobqueue.Job{ID: jobID, OrgID: org}
	var state, generation string
	e = tx.QueryRow(ctx, `SELECT state,attempt,payload->>'generation' FROM jobs WHERE org_id=$1 AND id=$2
 AND kind=$3 AND lane=$4 AND payload->>'item_id'=$5 FOR UPDATE`, org, jobID, RetirementJobKind, RetirementJobLane, itemID).Scan(&state, &j.Attempt, &generation)
	if e == pgx.ErrNoRows {
		return tx.Commit(ctx)
	}
	if e != nil {
		return e
	}
	if state != "dead" || generation != strconv.Itoa(item.Generation) {
		return tx.Commit(ctx)
	}
	if e = tx.QueryRow(ctx, `UPDATE object_retirement_items SET state='failed',version=version+1,attempts=attempts+1,
 last_error='CLEANUP_ATTEMPTS_EXHAUSTED',updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING version`, org, item.ID).Scan(&item.Version); e != nil {
		return e
	}
	if e = s.receipt(ctx, tx, j, p, item, "failed", "CLEANUP_ATTEMPTS_EXHAUSTED"); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
