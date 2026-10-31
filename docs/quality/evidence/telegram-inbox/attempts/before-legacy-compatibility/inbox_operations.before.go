package telegram

import (
	"context"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// These organization views never include request prose, private Telegram user
// identities, binding proofs, conversation output or free-form operator notes.
type InboxOperation struct {
	ID                    string    `json:"id"`
	Version               int64     `json:"version"`
	BotID                 string    `json:"bot_id"`
	ChannelID             string    `json:"channel_id"`
	ChannelLabel          string    `json:"channel_label"`
	ChatID                string    `json:"chat_id"`
	TopicID               int64     `json:"topic_id"`
	RequesterID           string    `json:"requester_id"`
	RequestedSkillID      *string   `json:"requested_skill_id"`
	RequestChannelVersion int64     `json:"request_channel_version"`
	CurrentChannelVersion int64     `json:"current_channel_version"`
	ChannelState          string    `json:"channel_state"`
	State                 string    `json:"state"`
	ReasonCode            string    `json:"reason_code"`
	ReceivedAt            time.Time `json:"received_at"`
	DispatchAfter         time.Time `json:"dispatch_after"`
	DispatchExpiresAt     time.Time `json:"dispatch_expires_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	RunID                 *string   `json:"run_id"`
	RunState              *string   `json:"run_state"`
	ScopeCurrent          bool      `json:"scope_current"`
	ProfileConfigured     bool      `json:"profile_configured"`
	Withdrawable          bool      `json:"withdrawable"`
}

type InboxOperationEvent struct {
	ID            string    `json:"id"`
	Version       int64     `json:"version"`
	State         string    `json:"state"`
	ReasonCode    string    `json:"reason_code"`
	DispatchAfter time.Time `json:"dispatch_after"`
	Source        string    `json:"source"`
	ActorID       *string   `json:"actor_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type InboxOperationDetails struct {
	Item    InboxOperation                 `json:"item"`
	History core.Page[InboxOperationEvent] `json:"history"`
}

type InboxWithdraw struct {
	ExpectedVersion int64  `json:"expected_version"`
	ReviewedRequest bool   `json:"reviewed_request"`
	Reason          string `json:"reason"`
}

func inboxAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, write bool) error {
	if err := identity.RequireInteractiveSession(ctx, tx, a, write); err != nil {
		return err
	}
	if err := authz.RequireOrganization(ctx, tx, a, "telegram.inbox.read"); err != nil {
		return err
	}
	if write {
		return authz.RequireOrganization(ctx, tx, a, "telegram.inbox.withdraw")
	}
	return nil
}

const inboxOperationSelect = `SELECT i.id,e.version,i.bot_id,i.channel_id,c.label,i.chat_id,i.topic_id,i.requester_id,i.requested_skill_id,
 i.channel_version,c.version,c.state,i.state,i.reason_code,i.received_at,i.dispatch_after,i.received_at+interval '30 minutes',e.created_at,l.run_id,r.state,
 (c.state='active' AND c.version=i.channel_version AND c.bot_id=i.bot_id AND c.chat_id=i.chat_id AND c.topic_id=i.topic_id
  AND i.requested_skill_id=ANY(c.allowed_skill_ids) AND b.state='active' AND b.bot_id=i.bot_id AND b.user_id=i.requester_id
  AND b.telegram_user_id=i.telegram_user_id AND u.status='active' AND u.auth_version=i.actor_auth_version),
 EXISTS(SELECT 1 FROM agent_runtime_profiles p WHERE p.org_id=i.org_id AND p.skill_id=i.requested_skill_id
  AND p.state='active' AND p.telegram_configuration_sha256 IS NOT NULL),
 (i.state='awaiting_executor' AND l.run_id IS NULL)
 FROM telegram_inbox i JOIN telegram_channels c ON c.org_id=i.org_id AND c.id=i.channel_id
 JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id JOIN users u ON u.org_id=i.org_id AND u.id=i.requester_id
 JOIN LATERAL(SELECT version,created_at FROM telegram_request_events WHERE org_id=i.org_id AND inbox_id=i.id ORDER BY version DESC LIMIT 1)e ON true
 LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id
 LEFT JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id`

func scanInboxOperation(row pgx.Row) (InboxOperation, error) {
	var v InboxOperation
	err := row.Scan(&v.ID, &v.Version, &v.BotID, &v.ChannelID, &v.ChannelLabel, &v.ChatID, &v.TopicID, &v.RequesterID, &v.RequestedSkillID,
		&v.RequestChannelVersion, &v.CurrentChannelVersion, &v.ChannelState, &v.State, &v.ReasonCode, &v.ReceivedAt, &v.DispatchAfter, &v.DispatchExpiresAt,
		&v.UpdatedAt, &v.RunID, &v.RunState, &v.ScopeCurrent, &v.ProfileConfigured, &v.Withdrawable)
	return v, platform.DatabaseError(err)
}

func (s *Service) InboxOperations(ctx context.Context, a core.Actor, state, channel, skill, cursor string, limit int) (core.Page[InboxOperation], error) {
	out := core.Page[InboxOperation]{Items: []InboxOperation{}}
	if (state != "" && !slices.Contains([]string{"awaiting_executor", "dispatched", "withdrawn"}, state)) ||
		(channel != "" && !core.ValidID(channel)) || (skill != "" && !slices.Contains([]string{"project-briefing", "incident-summary"}, skill)) ||
		limit < 1 || limit > 100 || len(cursor) > 2048 {
		return out, core.Invalid("filters", "请核对入站状态、任务、群入口和分页条件")
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = inboxAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	binding := core.CursorBinding(a, "telegram-inbox", state, channel, skill)
	pager := platform.Pager{Key: s.Config.CursorKey}
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		p, e := pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before, beforeID = &p.UpdatedAt, &p.ID
	}
	rows, err := tx.Query(ctx, inboxOperationSelect+` WHERE i.org_id=$1 AND i.kind='request' AND ($2='' OR i.state=$2)
 AND ($3::uuid IS NULL OR i.channel_id=$3) AND ($4='' OR i.requested_skill_id=$4)
 AND ($5::timestamptz IS NULL OR (i.received_at,i.id)<($5,$6::uuid)) ORDER BY i.received_at DESC,i.id DESC LIMIT $7`, a.OrgID, state, emptyID(channel), skill, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanInboxOperation(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		next := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.ReceivedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, nil
}

func (s *Service) inboxHistory(ctx context.Context, tx pgx.Tx, a core.Actor, id, cursor string, limit int) (core.Page[InboxOperationEvent], error) {
	out := core.Page[InboxOperationEvent]{Items: []InboxOperationEvent{}}
	if limit < 1 || limit > 100 || len(cursor) > 2048 {
		return out, core.Invalid("page", "请核对历史分页参数")
	}
	pager := platform.Pager{Key: s.Config.CursorKey}
	binding := core.CursorBinding(a, "telegram-inbox-history", id)
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		p, err := pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		before, beforeID = &p.UpdatedAt, &p.ID
	}
	rows, err := tx.Query(ctx, `SELECT e.id,e.version,e.state,e.reason_code,e.dispatch_after,e.source,w.actor_id,e.created_at
 FROM telegram_request_events e LEFT JOIN telegram_request_withdrawals w ON w.org_id=e.org_id AND w.inbox_id=e.inbox_id AND w.event_version=e.version
 WHERE e.org_id=$1 AND e.inbox_id=$2 AND ($3::timestamptz IS NULL OR (e.created_at,e.id)<($3,$4::uuid))
 ORDER BY e.created_at DESC,e.id DESC LIMIT $5`, a.OrgID, id, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v InboxOperationEvent
		if err = rows.Scan(&v.ID, &v.Version, &v.State, &v.ReasonCode, &v.DispatchAfter, &v.Source, &v.ActorID, &v.CreatedAt); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		next := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, nil
}

func (s *Service) InboxOperation(ctx context.Context, a core.Actor, id string) (InboxOperationDetails, error) {
	out := InboxOperationDetails{}
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = inboxAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	// Lock the parent first; every dispatcher/channel/binding transition uses
	// this row too. The exact head and its initial history cannot disagree.
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM telegram_inbox WHERE org_id=$1 AND id=$2 AND kind='request' FOR SHARE`, a.OrgID, id).Scan(&found); err != nil {
		return out, platform.DatabaseError(err)
	}
	out.Item, err = scanInboxOperation(tx.QueryRow(ctx, inboxOperationSelect+` WHERE i.org_id=$1 AND i.id=$2`, a.OrgID, id))
	if err == nil {
		out.History, err = s.inboxHistory(ctx, tx, a, id, "", 20)
	}
	return out, err
}

func (s *Service) InboxOperationHistory(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[InboxOperationEvent], error) {
	if !core.ValidID(id) {
		return core.Page[InboxOperationEvent]{}, core.Missing()
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return core.Page[InboxOperationEvent]{}, err
	}
	defer tx.Rollback(ctx)
	if err = inboxAuthority(ctx, tx, a, false); err != nil {
		return core.Page[InboxOperationEvent]{}, err
	}
	var found string
	if err = tx.QueryRow(ctx, `SELECT id FROM telegram_inbox WHERE org_id=$1 AND id=$2 AND kind='request' FOR SHARE`, a.OrgID, id).Scan(&found); err != nil {
		return core.Page[InboxOperationEvent]{}, platform.DatabaseError(err)
	}
	return s.inboxHistory(ctx, tx, a, id, cursor, limit)
}

func (s *Service) WithdrawInboxOperation(ctx context.Context, c core.Command, id string, in InboxWithdraw) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || !in.ReviewedRequest || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("withdrawal", "请核对请求版本、撤回范围和原因")
	}
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return inboxAuthority(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		var found string
		if err := tx.QueryRow(ctx, `SELECT id FROM telegram_inbox WHERE org_id=$1 AND id=$2 AND kind='request' FOR UPDATE`, c.Actor.OrgID, id).Scan(&found); err != nil {
			return core.Result{}, platform.DatabaseError(err)
		}
		v, err := scanInboxOperation(tx.QueryRow(ctx, inboxOperationSelect+` WHERE i.org_id=$1 AND i.id=$2`, c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		if v.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "入站请求已变化，请重新核对")
		}
		if !v.Withdrawable {
			return core.Result{}, core.Fail(409, "TELEGRAM_REQUEST_ALREADY_STARTED", "仅尚未创建秘书任务的排队请求可以在此撤回")
		}
		if _, err = tx.Exec(ctx, `UPDATE telegram_inbox SET state='withdrawn',reason_code='manual_queue_withdrawn' WHERE org_id=$1 AND id=$2`, c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		reason := strings.TrimSpace(in.Reason)
		if _, err = tx.Exec(ctx, `INSERT INTO telegram_request_withdrawals(id,org_id,inbox_id,expected_version,event_version,actor_id,session_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, core.NewID(), c.Actor.OrgID, id, v.Version, v.Version+1, c.Actor.UserID, c.Actor.SessionID, reason); err != nil {
			return core.Result{}, err
		}
		v, err = scanInboxOperation(tx.QueryRow(ctx, inboxOperationSelect+` WHERE i.org_id=$1 AND i.id=$2`, c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		result := eventResult(v, "telegram.inbox_withdrawn", id, reason, v.Version, false)
		result.Event.Payload["reviewed_request"] = true
		return result, nil
	})
}
