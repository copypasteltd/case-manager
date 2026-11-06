package agent

import (
	"context"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// This projection never executes or enqueues work. Pending grants must still
// satisfy the original human session and the exact current business basis.
func (s *Service) refreshGrantState(ctx context.Context, tx pgx.Tx, a core.Actor, grant *TelegramWriteGrant) error {
	if grant.State != "pending_trigger" {
		return nil
	}
	run, err := runRecord(ctx, tx, a.OrgID, grant.RunID, false)
	if err != nil {
		return err
	}
	if !run.ExpiresAt.After(time.Now()) {
		grant.State = "expired"
		return nil
	}
	conversation, err := metadata(ctx, tx, a.OrgID, run.ConversationID, false)
	if err != nil {
		return err
	}
	var same bool
	err = tx.QueryRow(ctx, `SELECT conversation_revision_id=$3 AND context_security_version=$4 FROM agent_runs WHERE org_id=$1 AND id=$2`, a.OrgID, run.ID, conversation.CurrentRevisionID, conversation.ContextSecurityVersion).Scan(&same)
	if err != nil {
		return err
	}
	if !same {
		grant.State = "invalidated"
		return nil
	}
	actor := a
	actor.Channel, actor.SessionID, actor.RunID = "telegram", "", ""
	st := executionState{Run: run, Actor: actor, Conversation: conversation}
	err = s.telegramGrantState(ctx, tx, &st, false)
	if err == nil {
		var sources []SourceSnapshot
		sources, err = grantSources(ctx, tx, a.OrgID, run.ID)
		if err == nil {
			err = executionSources(ctx, tx, st, sources)
		}
	}
	if err == nil {
		_, err = executionMutationState(ctx, tx, st)
	}
	if err != nil && core.PublicError(err).Status < 500 {
		grant.State = "invalidated"
		return nil
	}
	return err
}

// Pagination advances across immutable owner candidates, including candidates
// no longer visible under ordinary authority. An empty page with a next cursor
// is not the end of the inventory; current source data is never leaked.
func (s *Service) TelegramWriteGrants(ctx context.Context, a core.Actor, state, cursor string, limit int) (core.Page[TelegramWriteGrant], error) {
	out := core.Page[TelegramWriteGrant]{Items: []TelegramWriteGrant{}}
	if !slices.Contains([]string{"", "pending_trigger", "consumed", "stopped", "expired", "invalidated"}, state) || limit < 1 || limit > 100 || len(cursor) > 2048 {
		return out, core.Invalid("filters", "请核对预授权状态与分页条件")
	}
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	binding := core.CursorBinding(a, "telegram-owner-write-grants", a.SessionID, state)
	var before *time.Time
	var after *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before, after = &v.UpdatedAt, &v.ID
	}
	type candidate struct {
		id      string
		created time.Time
	}
	rows, err := tx.Query(ctx, `SELECT id,created_at FROM telegram_write_grants WHERE org_id=$1 AND actor_id=$2
 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, a.UserID, before, after, limit+1)
	if err != nil {
		return out, err
	}
	var candidates []candidate
	for rows.Next() {
		var v candidate
		if err = rows.Scan(&v.id, &v.created); err != nil {
			rows.Close()
			return out, err
		}
		candidates = append(candidates, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
		last := candidates[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: last.id, UpdatedAt: last.created, Binding: binding})
		out.NextCursor = &next
	}
	for _, v := range candidates {
		grant, e := grantRead(ctx, tx, a, v.id)
		if e != nil {
			return out, e
		}
		if _, _, e = s.readableRun(ctx, tx, a, grant.RunID); e != nil {
			if slices.Contains([]int{403, 404, 409}, core.PublicError(e).Status) {
				continue
			}
			return out, e
		}
		if e = s.refreshGrantState(ctx, tx, a, &grant); e != nil {
			return out, e
		}
		if state == "" || state == grant.State {
			out.Items = append(out.Items, grant)
		}
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
