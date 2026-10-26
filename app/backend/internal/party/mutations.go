package party

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

func (s *Service) mutate(ctx context.Context, c core.Command, id string, version int64, action string, fn func(pgx.Tx, Party) (core.Result, error)) (core.Result, error) {
	if err := core.RequireVersion(version); err != nil {
		return core.Result{}, err
	}
	check := func(tx pgx.Tx) (Party, error) {
		p, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return Party{}, err
		}
		if err = authz.Require(ctx, tx, c.Actor, "party.read", resource(p)); err != nil {
			return Party{}, err
		}
		return p, authz.Require(ctx, tx, c.Actor, action, resource(p))
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, err := check(tx); return err }, func(tx pgx.Tx) (core.Result, error) {
		p, err := check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(ctx, tx, c.Actor.OrgID, p.ProjectID, p.Classification, true); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "SELECT id FROM parties WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		p, err = check(tx)
		if err != nil {
			return core.Result{}, err
		}
		if p.Version != version {
			return core.Result{}, core.Conflict()
		}
		before, err := json.Marshal(p)
		if err != nil {
			return core.Result{}, err
		}
		out, err := fn(tx, p)
		out.Event.BeforeDigest = core.Digest(before)
		return out, err
	})
}
func (s *Service) Update(ctx context.Context, c core.Command, id string, in Patch) (core.Result, error) {
	if err := validate(in.Content, in.Reason); err != nil {
		return core.Result{}, err
	}
	if !core.ValidID(in.BaseRevisionID) {
		return core.Result{}, core.Invalid("base_revision_id", "请固定当前往来资料修订")
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, "party.update", func(tx pgx.Tx, p Party) (core.Result, error) {
		if p.State != "active" {
			return core.Result{}, core.Fail(409, "PARTY_READ_ONLY", "请先恢复归档资料，再新增修订")
		}
		if p.CurrentRevisionID != in.BaseRevisionID {
			return core.Result{}, core.Conflict()
		}
		var seq int64
		if err := tx.QueryRow(ctx, "SELECT seq FROM party_revisions WHERE org_id=$1 AND party_id=$2 AND id=$3", c.Actor.OrgID, id, p.CurrentRevisionID).Scan(&seq); err != nil {
			return core.Result{}, err
		}
		revision := core.NewID()
		if err := insertRevision(ctx, tx, c.Actor, id, revision, seq+1, &p.CurrentRevisionID, in.Content, in.Reason); err != nil {
			return core.Result{}, err
		}
		if _, err := tx.Exec(ctx, "UPDATE parties SET current_revision_id=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, revision); err != nil {
			return core.Result{}, err
		}
		p, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		p, err = present(ctx, tx, c.Actor, p)
		return result(p, "party.revised", 200, in.Reason), err
	})
}
func (s *Service) ChangeState(ctx context.Context, c core.Command, id string, in StateInput) (core.Result, error) {
	if (in.Action != "archive" && in.Action != "restore") || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("action", "请明确归档或恢复以及原因")
	}
	return s.mutate(ctx, c, id, in.ExpectedVersion, "party.archive", func(tx pgx.Tx, p Party) (core.Result, error) {
		state := "archived"
		event := "party.archived"
		if in.Action == "restore" {
			state = "active"
			event = "party.restored"
		}
		if p.State == state {
			return core.Result{}, core.Fail(409, "PARTY_STATE_CONFLICT", "资料状态已变化，请重新核对")
		}
		if _, err := tx.Exec(ctx, "UPDATE parties SET state=$3,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, state); err != nil {
			return core.Result{}, err
		}
		p, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		p, err = present(ctx, tx, c.Actor, p)
		return result(p, event, 200, in.Reason), err
	})
}
