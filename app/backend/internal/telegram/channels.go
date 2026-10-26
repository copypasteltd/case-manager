package telegram

import (
	"context"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func validSkills(skills []string) bool {
	if len(skills) < 1 || len(skills) > 2 {
		return false
	}
	seen := map[string]bool{}
	for _, skill := range skills {
		if !slices.Contains([]string{"project-briefing", "incident-summary"}, skill) || seen[skill] {
			return false
		}
		seen[skill] = true
	}
	return true
}
func projectScope(ctx context.Context, tx pgx.Tx, a core.Actor, id *string) error {
	if id == nil {
		return nil
	}
	if !core.ValidID(*id) {
		return core.Missing()
	}
	var owner, classification string
	if err := tx.QueryRow(ctx, "SELECT creator_id,classification FROM projects WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, *id).Scan(&owner, &classification); err != nil {
		return platform.DatabaseError(err)
	}
	return authz.Require(ctx, tx, a, "project.read", authz.Resource{Type: "project", ID: *id, ProjectID: id, OwnerID: &owner, Classification: classification})
}
func channelRevision(ctx context.Context, tx pgx.Tx, org, id string, actor *string, source, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO telegram_channel_revisions(id,org_id,channel_id,version,actor_id,source,reason,snapshot) SELECT $1,c.org_id,c.id,c.version,$4,$5,$6,to_jsonb(c) FROM telegram_channels c WHERE c.org_id=$2 AND c.id=$3`, core.NewID(), org, id, actor, source, strings.TrimSpace(reason))
	return err
}
func withdrawChannel(ctx context.Context, tx pgx.Tx, org, id, reason string) error {
	_, err := tx.Exec(ctx, `UPDATE telegram_inbox SET state='withdrawn',reason_code=$3 WHERE org_id=$1 AND channel_id=$2 AND (state='awaiting_executor' OR (state='dispatched' AND EXISTS(SELECT 1 FROM telegram_run_links l JOIN agent_runs r ON r.org_id=l.org_id AND r.id=l.run_id WHERE l.org_id=telegram_inbox.org_id AND l.inbox_id=telegram_inbox.id AND r.state IN ('queued','running','awaiting_approval','committing'))))`, org, id, reason)
	return err
}
func (s *Service) CreateChannel(ctx context.Context, c core.Command, in ChannelCreate) (core.Result, error) {
	if !numericID(in.ChatID, true) || in.TopicID < 0 || in.TopicID > 2147483647 || core.Name(in.Label) != nil || core.Name(in.Reason) != nil || !validSkills(in.AllowedSkillIDs) || (in.DefaultSkillID != nil && !slices.Contains(in.AllowedSkillIDs, *in.DefaultSkillID)) {
		return core.Result{}, core.Invalid("channel", "请核对数字群号、话题、名称、允许任务和原因")
	}
	if _, ok := s.bot(c.Actor.OrgID, in.BotID); !ok {
		return core.Result{}, core.Fail(503, "TELEGRAM_NOT_CONFIGURED", "当前Bot尚未配置接收入口")
	}
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return authority(ctx, tx, c.Actor, true, true) }, func(tx pgx.Tx) (core.Result, error) {
		if err := projectScope(ctx, tx, c.Actor, in.ProjectID); err != nil {
			return core.Result{}, err
		}
		id := core.NewID()
		value, err := scanChannel(tx.QueryRow(ctx, `INSERT INTO telegram_channels(id,org_id,bot_id,chat_id,topic_id,label,project_id,allowed_skill_ids,default_skill_id,state,delivery_enabled) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'paused',$10) RETURNING `+channelColumns, id, c.Actor.OrgID, in.BotID, in.ChatID, in.TopicID, strings.TrimSpace(in.Label), in.ProjectID, in.AllowedSkillIDs, in.DefaultSkillID, in.DeliveryEnabled))
		if err != nil {
			return core.Result{}, err
		}
		if err = channelRevision(ctx, tx, c.Actor.OrgID, id, &c.Actor.UserID, "portal", in.Reason); err != nil {
			return core.Result{}, err
		}
		return eventResult(value, "telegram.channel_created", id, in.Reason, value.Version, true), nil
	})
}
func (s *Service) UpdateChannel(ctx context.Context, c core.Command, id string, in ChannelPatch) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || core.Name(in.Label) != nil || core.Name(in.Reason) != nil || !validSkills(in.AllowedSkillIDs) || (in.DefaultSkillID != nil && !slices.Contains(in.AllowedSkillIDs, *in.DefaultSkillID)) || !slices.Contains([]string{"active", "paused"}, in.State) {
		return core.Result{}, core.Invalid("channel", "请核对当前版本、范围、状态和原因")
	}
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return authority(ctx, tx, c.Actor, true, true) }, func(tx pgx.Tx) (core.Result, error) {
		value, err := scanChannel(tx.QueryRow(ctx, "SELECT "+channelColumns+" FROM telegram_channels WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		if value.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "群入口已更新，请重新读取")
		}
		if in.State == "active" {
			if _, ok := s.bot(c.Actor.OrgID, value.BotID); !ok {
				return core.Result{}, core.Fail(503, "TELEGRAM_NOT_CONFIGURED", "当前Bot尚未配置接收入口")
			}
		}
		if in.State == "active" || (in.ProjectID == nil) != (value.ProjectID == nil) || (in.ProjectID != nil && value.ProjectID != nil && *in.ProjectID != *value.ProjectID) {
			if err = projectScope(ctx, tx, c.Actor, in.ProjectID); err != nil {
				return core.Result{}, err
			}
		}
		value, err = scanChannel(tx.QueryRow(ctx, `UPDATE telegram_channels SET label=$3,project_id=$4,allowed_skill_ids=$5,state=$6,default_skill_id=$7,delivery_enabled=$8,version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING `+channelColumns, c.Actor.OrgID, id, strings.TrimSpace(in.Label), in.ProjectID, in.AllowedSkillIDs, in.State, in.DefaultSkillID, in.DeliveryEnabled))
		if err != nil {
			return core.Result{}, err
		}
		if err = withdrawChannel(ctx, tx, c.Actor.OrgID, id, "channel_changed"); err != nil {
			return core.Result{}, err
		}
		if err = channelRevision(ctx, tx, c.Actor.OrgID, id, &c.Actor.UserID, "portal", in.Reason); err != nil {
			return core.Result{}, err
		}
		return eventResult(value, "telegram.channel_updated", id, in.Reason, value.Version, false), nil
	})
}
func (s *Service) Channels(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[Channel], error) {
	out := core.Page[Channel]{Items: []Channel{}}
	tx, err := s.snapshot(ctx, a, true)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if limit < 1 || limit > 100 {
		limit = 20
	}
	pager := platform.Pager{Key: s.Config.CursorKey}
	binding := core.CursorBinding(a, "telegram-channels")
	var before *string
	if cursor != "" {
		p, e := pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before = &p.ID
	}
	rows, err := tx.Query(ctx, "SELECT "+channelColumns+" FROM telegram_channels WHERE org_id=$1 AND ($2::uuid IS NULL OR id<$2) ORDER BY id DESC LIMIT $3", a.OrgID, before, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanChannel(rows)
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
		token := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: binding})
		out.NextCursor = &token
	}
	return out, nil
}
func (s *Service) History(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[Revision], error) {
	out := core.Page[Revision]{Items: []Revision{}}
	if !core.ValidID(id) {
		return out, core.Missing()
	}
	tx, err := s.snapshot(ctx, a, true)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if _, err = scanChannel(tx.QueryRow(ctx, "SELECT "+channelColumns+" FROM telegram_channels WHERE org_id=$1 AND id=$2", a.OrgID, id)); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	pager := platform.Pager{Key: s.Config.CursorKey}
	binding := core.CursorBinding(a, "telegram-history", id)
	var before *string
	var beforeTime *time.Time
	if cursor != "" {
		p, e := pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before = &p.ID
		beforeTime = &p.UpdatedAt
	}
	rows, err := tx.Query(ctx, `SELECT id,channel_id,version,source,reason,snapshot,created_at FROM telegram_channel_revisions WHERE org_id=$1 AND channel_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`, a.OrgID, id, beforeTime, before, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v Revision
		if err = rows.Scan(&v.ID, &v.ChannelID, &v.Version, &v.Source, &v.Reason, &v.Snapshot, &v.CreatedAt); err != nil {
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
		token := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &token
	}
	return out, nil
}
