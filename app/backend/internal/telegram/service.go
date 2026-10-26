package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB     *pgxpool.Pool
	Config platform.Config
	Bots   []BotConfig
}

func (s *Service) snapshot(ctx context.Context, a core.Actor, admin bool) (pgx.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", a.OrgID); err == nil {
		err = authority(ctx, tx, a, admin, false)
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func authority(ctx context.Context, tx pgx.Tx, a core.Actor, admin, mfa bool) error {
	if err := identity.RequireInteractiveSession(ctx, tx, a, mfa); err != nil {
		return err
	}
	if admin {
		return authz.RequireOrganization(ctx, tx, a, "policy.manage")
	}
	return nil
}
func ownerLock(ctx context.Context, tx pgx.Tx, org, user, bot string) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", fmt.Sprintf("telegram-owner/%s/%s/%s", org, user, bot))
	return err
}

const challengeColumns = `id,version,bot_id,telegram_user_id,expires_at,claimed_at,confirmed_at,cancelled_at,created_at`

func scanChallenge(row pgx.Row) (Challenge, error) {
	var v Challenge
	err := row.Scan(&v.ID, &v.Version, &v.BotID, &v.TelegramUserID, &v.ExpiresAt, &v.ClaimedAt, &v.ConfirmedAt, &v.CancelledAt, &v.CreatedAt)
	return v, platform.DatabaseError(err)
}

const bindingColumns = `id,version,bot_id,telegram_user_id,state,created_at,revoked_at`

func scanBinding(row pgx.Row) (Binding, error) {
	var v Binding
	err := row.Scan(&v.ID, &v.Version, &v.BotID, &v.TelegramUserID, &v.State, &v.CreatedAt, &v.RevokedAt)
	return v, platform.DatabaseError(err)
}

const channelColumns = `id,version,bot_id,chat_id,topic_id,label,project_id,allowed_skill_ids,disclosure_mode,state,created_at,updated_at,default_skill_id,delivery_enabled`

func scanChannel(row pgx.Row) (Channel, error) {
	var v Channel
	err := row.Scan(&v.ID, &v.Version, &v.BotID, &v.ChatID, &v.TopicID, &v.Label, &v.ProjectID, &v.AllowedSkillIDs, &v.DisclosureMode, &v.State, &v.CreatedAt, &v.UpdatedAt, &v.DefaultSkillID, &v.DeliveryEnabled)
	return v, platform.DatabaseError(err)
}
func (s *Service) Status(ctx context.Context, a core.Actor) (Status, error) {
	out := Status{Bots: []Bot{}, Bindings: []Binding{}, Challenges: []Challenge{}}
	tx, err := s.snapshot(ctx, a, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	for _, b := range s.Bots {
		if b.OrgID == a.OrgID {
			out.Bots = append(out.Bots, Bot{b.ID, b.Username})
		}
	}
	out.IngressConfigured = len(out.Bots) > 0
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_delivery_budgets b JOIN telegram_channels c ON c.org_id=b.org_id AND c.bot_id=b.bot_id WHERE b.org_id=$1 AND b.last_seen_at>clock_timestamp()-interval '90 seconds' AND c.state='active' AND c.delivery_enabled)`, a.OrgID).Scan(&out.DeliveryAvailable); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, "SELECT "+bindingColumns+" FROM telegram_bindings WHERE org_id=$1 AND user_id=$2 AND state='active' ORDER BY created_at,id", a.OrgID, a.UserID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanBinding(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Bindings = append(out.Bindings, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.Query(ctx, "SELECT "+challengeColumns+" FROM telegram_binding_challenges WHERE org_id=$1 AND user_id=$2 AND expires_at>clock_timestamp() AND confirmed_at IS NULL AND cancelled_at IS NULL ORDER BY created_at,id", a.OrgID, a.UserID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v, e := scanChallenge(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Challenges = append(out.Challenges, v)
	}
	err = rows.Err()
	rows.Close()
	return out, err
}
func eventResult(value any, typ, id, reason string, version int64, created bool) core.Result {
	status := 200
	if created {
		status = 201
	}
	return core.Result{Status: status, Data: value, Event: core.Event{Type: typ, ResourceType: "telegram", ResourceID: id, Version: version, Payload: map[string]any{"reason": reason}}}
}
func (s *Service) Requests(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[Request], error) {
	out := core.Page[Request]{Items: []Request{}}
	tx, err := s.snapshot(ctx, a, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	pager := platform.Pager{Key: s.Config.CursorKey}
	binding := core.CursorBinding(a, "telegram-requests")
	var before *time.Time
	var beforeID *string
	if cursor != "" {
		p, e := pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before, beforeID = &p.UpdatedAt, &p.ID
	}
	rows, err := tx.Query(ctx, `SELECT i.id,i.bot_id,i.channel_id,i.channel_version,i.state,i.reason_code,i.request_text,i.received_at,i.requested_skill_id,l.conversation_id,l.run_id,r.state FROM telegram_inbox i LEFT JOIN telegram_run_links l ON l.org_id=i.org_id AND l.inbox_id=i.id LEFT JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id WHERE i.org_id=$1 AND i.requester_id=$2 AND i.kind='request' AND ($3::timestamptz IS NULL OR (i.received_at,i.id)<($3,$4::uuid)) ORDER BY i.received_at DESC,i.id DESC LIMIT $5`, a.OrgID, a.UserID, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v Request
		if err = rows.Scan(&v.ID, &v.BotID, &v.ChannelID, &v.ChannelVersion, &v.State, &v.ReasonCode, &v.Text, &v.ReceivedAt, &v.RequestedSkillID, &v.ConversationID, &v.RunID, &v.RunState); err != nil {
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
		token := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.ReceivedAt, Binding: binding})
		out.NextCursor = &token
	}
	return out, nil
}
func decodeResult[T any](value any) (T, error) {
	var out T
	raw, err := json.Marshal(value)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}
