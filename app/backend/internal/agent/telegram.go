package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const TelegramDispatchWindow = 30 * time.Minute

type telegramRequest struct {
	ID, ChannelID, BindingID, Skill, Message, Label string
	ChannelVersion                                  int64
	Actor                                           core.Actor
	ProjectID                                       *string
	ReceivedAt                                      time.Time
}

func telegramConversationAuthority(ctx context.Context, db platform.DBTX, a core.Actor, c Conversation) error {
	var valid bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_conversation_scopes s
  JOIN telegram_channels ch ON ch.org_id=s.org_id AND ch.id=s.channel_id
  JOIN telegram_bindings b ON b.org_id=s.org_id AND b.id=s.binding_id
  JOIN users u ON u.org_id=s.org_id AND u.id=s.actor_id JOIN organizations o ON o.id=s.org_id
  WHERE s.org_id=$1 AND s.conversation_id=$2 AND s.actor_id=$3 AND s.actor_auth_version=$4
   AND s.organization_auth_version=$5 AND o.authorization_version=s.organization_auth_version
   AND ch.version=s.channel_version AND ch.state='active' AND b.state='active'
   AND b.bot_id=ch.bot_id AND b.user_id=s.actor_id AND u.status='active' AND u.auth_version=s.actor_auth_version)`, a.OrgID, c.ID, a.UserID, a.AuthVersion, a.OrgAuthVersion).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return runtimeFailure("TELEGRAM_SCOPE_CHANGED")
	}
	return nil
}

func telegramExecutionAuthority(ctx context.Context, tx pgx.Tx, st executionState) error {
	if st.Actor.Channel != "telegram" {
		if st.Conversation.Type == "telegram" {
			return runtimeFailure("TELEGRAM_ORIGIN_INVALID")
		}
		return nil
	}
	if st.Conversation.Type != "telegram" {
		return runtimeFailure("TELEGRAM_ORIGIN_INVALID")
	}
	var valid bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_run_links l
  JOIN telegram_inbox i ON i.org_id=l.org_id AND i.id=l.inbox_id
  JOIN telegram_channels c ON c.org_id=i.org_id AND c.id=i.channel_id
  JOIN agent_runtime_profiles p ON p.org_id=i.org_id AND p.id=$4
  WHERE l.org_id=$1 AND l.run_id=$2 AND l.conversation_id=$3 AND i.state='dispatched'
   AND p.skill_id=i.requested_skill_id AND i.requested_skill_id=ANY(c.allowed_skill_ids)
   AND p.skill_id IN ('project-briefing','incident-summary'))`, st.Actor.OrgID, st.Run.ID, st.Conversation.ID, st.Run.ProfileID).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return runtimeFailure("TELEGRAM_SCOPE_CHANGED")
	}
	return nil
}

// DispatchTelegramOnce performs only a short database transaction. A crash cannot
// separate receipt consumption, context creation, task creation and job enqueue.
// Its bot identity is supplied by the operator, never by a model or portal input.
func (s *Service) DispatchTelegramOnce(ctx context.Context, org, bot string) (bool, error) {
	n, err := strconv.ParseInt(bot, 10, 64)
	if !core.ValidID(org) || err != nil || n <= 0 || n > (1<<52)-1 || strconv.FormatInt(n, 10) != bot {
		return false, core.Invalid("bot", "请配置准确组织和数字Bot")
	}
	var pending bool
	if err = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_inbox WHERE org_id=$1 AND bot_id=$2 AND kind='request' AND state='awaiting_executor' AND dispatch_after<=clock_timestamp())`, org, bot).Scan(&pending); err != nil || !pending {
		return false, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	in := telegramRequest{Actor: core.Actor{OrgID: org, Channel: "telegram"}}
	if err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR UPDATE", org).Scan(&in.Actor.OrgAuthVersion); err != nil {
		return false, err
	}
	err = tx.QueryRow(ctx, `SELECT id,requester_id,actor_auth_version,channel_id,channel_version,binding_id,requested_skill_id,request_text,received_at
  FROM telegram_inbox WHERE org_id=$1 AND bot_id=$2 AND kind='request' AND state='awaiting_executor'
   AND dispatch_after<=clock_timestamp() ORDER BY received_at,id LIMIT 1 FOR UPDATE`, org, bot).Scan(&in.ID, &in.Actor.UserID, &in.Actor.AuthVersion, &in.ChannelID, &in.ChannelVersion, &in.BindingID, &in.Skill, &in.Message, &in.ReceivedAt)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_inbox i JOIN telegram_channels c ON c.org_id=i.org_id AND c.id=i.channel_id
  JOIN telegram_bindings b ON b.org_id=i.org_id AND b.id=i.binding_id JOIN users u ON u.org_id=i.org_id AND u.id=i.requester_id
  WHERE i.org_id=$1 AND i.id=$2 AND c.version=i.channel_version AND c.state='active'
   AND c.bot_id=i.bot_id AND c.chat_id=i.chat_id AND c.topic_id=i.topic_id
   AND i.requested_skill_id=ANY(c.allowed_skill_ids) AND b.state='active'
   AND b.bot_id=i.bot_id AND b.telegram_user_id=i.telegram_user_id AND b.user_id=i.requester_id
   AND u.status='active' AND u.auth_version=i.actor_auth_version)`, org, in.ID).Scan(&valid); err != nil {
		return true, err
	}
	reason := ""
	if !valid {
		reason = "TELEGRAM_SCOPE_CHANGED"
	} else if time.Now().After(in.ReceivedAt.Add(TelegramDispatchWindow)) {
		reason = "TELEGRAM_REQUEST_EXPIRED"
	}
	if reason != "" {
		if _, err = tx.Exec(ctx, "UPDATE telegram_inbox SET state='withdrawn',reason_code=$3 WHERE org_id=$1 AND id=$2", org, in.ID, reason); err != nil {
			return true, err
		}
		return true, tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, "SAVEPOINT telegram_dispatch"); err != nil {
		return true, err
	}
	err = s.dispatchTelegram(ctx, tx, in)
	if err != nil {
		public := core.PublicError(err)
		retry := public.Code == "RUNTIME_UNAVAILABLE" || public.Code == "TELEGRAM_SKILL_UNAVAILABLE" || public.Code == "RUN_CAPACITY_EXCEEDED" || public.Code == "CONVERSATION_RUN_ACTIVE"
		if public.Status >= 500 && !retry {
			return true, err
		}
		if _, rollback := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT telegram_dispatch"); rollback != nil {
			return true, rollback
		}
		state := "withdrawn"
		if retry {
			state = "awaiting_executor"
		}
		if _, err = tx.Exec(ctx, `UPDATE telegram_inbox SET state=$3,reason_code=$4,dispatch_after=clock_timestamp()+interval '30 seconds' WHERE org_id=$1 AND id=$2`, org, in.ID, state, public.Code); err != nil {
			return true, err
		}
	}
	return true, platform.DatabaseError(tx.Commit(ctx))
}

func (s *Service) dispatchTelegram(ctx context.Context, tx pgx.Tx, in telegramRequest) error {
	profile, err := scanProfile(tx.QueryRow(ctx, "SELECT "+profileColumns+" FROM agent_runtime_profiles WHERE org_id=$1 AND skill_id=$2 AND state='active' AND telegram_configuration_sha256 IS NOT NULL", in.Actor.OrgID, in.Skill))
	if err != nil {
		if core.PublicError(err).Status == 404 {
			return core.Fail(503, "RUNTIME_UNAVAILABLE", "指定任务尚无有效执行配置")
		}
		return err
	}
	if err = tx.QueryRow(ctx, "SELECT project_id,label FROM telegram_channels WHERE org_id=$1 AND id=$2", in.Actor.OrgID, in.ChannelID).Scan(&in.ProjectID, &in.Label); err != nil {
		return err
	}
	ai := in.Actor
	ai.RunID = core.NewID()
	classification, err := projectScope(ctx, tx, ai, in.ProjectID)
	if err != nil {
		return err
	}
	// Reuse the existing explicit-grant boundary for confidential conversations.
	if classification == "confidential" {
		return core.Fail(422, "CONVERSATION_EXPLICIT_GRANT_REQUIRED", "此资料范围需要独立安全流程")
	}
	var conversationID string
	err = tx.QueryRow(ctx, `SELECT conversation_id FROM telegram_conversation_scopes WHERE org_id=$1 AND channel_id=$2 AND channel_version=$3 AND binding_id=$4 AND actor_auth_version=$5 AND organization_auth_version=$6`, in.Actor.OrgID, in.ChannelID, in.ChannelVersion, in.BindingID, in.Actor.AuthVersion, in.Actor.OrgAuthVersion).Scan(&conversationID)
	var c Conversation
	if err == nil {
		c, err = metadata(ctx, tx, in.Actor.OrgID, conversationID, true)
		if err != nil {
			return err
		}
		if c.State != "active" {
			return core.Fail(409, "CONVERSATION_READ_ONLY", "渠道会话已停止")
		}
	} else if err == pgx.ErrNoRows {
		label := []rune(strings.TrimSpace(in.Label))
		if len(label) > 180 {
			label = label[:180]
		}
		c = Conversation{ID: core.NewID(), Version: 1, Type: "telegram", ProjectID: in.ProjectID, Classification: classification, CreatorID: in.Actor.UserID, CurrentRevisionID: core.NewID(), Title: "TG · " + string(label), State: "active", ContextSecurityVersion: 1}
		if err = authz.Require(ctx, tx, ai, "conversation.create", resource(c)); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO conversations(id,org_id,creator_id,type,project_id,classification,current_revision_id) VALUES($1,$2,$3,'telegram',$4,$5,$6)`, c.ID, in.Actor.OrgID, c.CreatorID, c.ProjectID, c.Classification, c.CurrentRevisionID); err != nil {
			return err
		}
		if err = snapshot(ctx, tx, in.Actor, c, nil, []string{in.Actor.UserID}, "TG渠道独立上下文"); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO telegram_conversation_scopes(org_id,conversation_id,channel_id,channel_version,binding_id,actor_id,actor_auth_version,organization_auth_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, in.Actor.OrgID, c.ID, in.ChannelID, in.ChannelVersion, in.BindingID, in.Actor.UserID, in.Actor.AuthVersion, in.Actor.OrgAuthVersion); err != nil {
			return err
		}
	} else {
		return err
	}
	if err = require(ctx, tx, ai, c, "agent.run"); err != nil {
		return err
	}
	members, err := aiAudience(ctx, tx, in.Actor, c)
	if err != nil {
		return err
	}
	if err = checkRunCapacity(ctx, tx, in.Actor.OrgID, in.Actor.UserID, c.ID); err != nil {
		return err
	}
	refs := []SourceReference{}
	if c.ProjectID != nil {
		refs = append(refs, SourceReference{Type: "project", ID: *c.ProjectID})
	}
	sources, err := sourceSnapshots(ctx, tx, in.Actor, c, refs, members, true)
	if err != nil {
		return err
	}
	sourceJSON, _ := json.Marshal(sources)
	hashInput, _ := json.Marshal([]any{in.ID, in.ChannelID, in.ChannelVersion, in.BindingID, in.Actor.AuthVersion, in.Actor.OrgAuthVersion, profile.ID, in.Message, sources})
	run := core.NewID()
	if _, err = tx.Exec(ctx, `INSERT INTO agent_runs(id,org_id,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,channel,profile_id,client_message_id,input_sha256,audience_ids,state,expires_at)
  VALUES($1,$2,$3,$4,$5,$6,$7,$8,'telegram',$9,$10,$11,$12,'queued',clock_timestamp()+$13::integer*interval '1 second')`, run, in.Actor.OrgID, c.ID, c.CurrentRevisionID, c.ContextSecurityVersion, in.Actor.UserID, in.Actor.AuthVersion, in.Actor.OrgAuthVersion, profile.ID, in.ID, core.Digest(hashInput), members, profile.TimeoutSeconds); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO conversation_messages(id,org_id,conversation_id,run_id,seq,role,body,author_id,audience_ids,source_snapshots)
  SELECT $1,$2,$3,$4,coalesce(max(seq),0)+1,'user',$5,$6,$7,$8 FROM conversation_messages WHERE org_id=$2 AND conversation_id=$3`, core.NewID(), in.Actor.OrgID, c.ID, run, in.Message, in.Actor.UserID, members, sourceJSON); err != nil {
		return err
	}
	if err = s.enqueueRun(ctx, tx, in.Actor.OrgID, run); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO telegram_run_links(org_id,inbox_id,run_id,conversation_id) VALUES($1,$2,$3,$4)`, in.Actor.OrgID, in.ID, run, c.ID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE telegram_inbox SET state='dispatched',reason_code='run_queued' WHERE org_id=$1 AND id=$2", in.Actor.OrgID, in.ID)
	return err
}
