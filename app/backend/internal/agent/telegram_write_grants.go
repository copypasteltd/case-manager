package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// A fixed task-field update is the first direct TG write. Other task kinds keep
// their existing human portal handoff; this is never a free-text write grant.
type TelegramWriteGrantInput struct {
	ChannelID      string    `json:"channel_id"`
	ChannelVersion int64     `json:"channel_version"`
	BindingID      string    `json:"binding_id"`
	BindingVersion int64     `json:"binding_version"`
	Reason         string    `json:"reason"`
	Run            RunCreate `json:"run"`
}
type TelegramWriteGrant struct {
	ID             string    `json:"id"`
	RunID          string    `json:"run_id"`
	ConversationID string    `json:"conversation_id"`
	ChannelID      string    `json:"channel_id"`
	ChannelVersion int64     `json:"channel_version"`
	BindingID      string    `json:"binding_id"`
	BindingVersion int64     `json:"binding_version"`
	InputSHA256    string    `json:"input_sha256"`
	State          string    `json:"state"`
	Trigger        string    `json:"trigger"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
}
type telegramGrantMode struct{ RunID string }

func grantRead(ctx context.Context, db platform.DBTX, a core.Actor, id string) (TelegramWriteGrant, error) {
	var out TelegramWriteGrant
	e := db.QueryRow(ctx, `SELECT g.id,g.run_id,g.conversation_id,g.channel_id,g.channel_version,g.binding_id,g.binding_version,g.input_sha256,
 CASE WHEN a.inbox_id IS NOT NULL THEN 'consumed' WHEN r.cancel_requested_at IS NOT NULL OR r.state<>'queued' THEN 'stopped'
 WHEN g.expires_at<=clock_timestamp() THEN 'expired' ELSE 'pending_trigger' END,g.expires_at,g.created_at
 FROM telegram_write_grants g JOIN agent_runs r ON r.org_id=g.org_id AND r.id=g.run_id
 LEFT JOIN telegram_write_activations a ON a.org_id=g.org_id AND a.grant_id=g.id WHERE g.org_id=$1 AND g.id=$2 AND g.actor_id=$3`, a.OrgID, id, a.UserID).Scan(&out.ID, &out.RunID, &out.ConversationID, &out.ChannelID, &out.ChannelVersion, &out.BindingID, &out.BindingVersion, &out.InputSHA256, &out.State, &out.ExpiresAt, &out.CreatedAt)
	out.Trigger = "/task project-update authorize:" + out.ID
	return out, platform.DatabaseError(e)
}
func (s *Service) TelegramWriteGrant(ctx context.Context, a core.Actor, id string) (TelegramWriteGrant, error) {
	if !core.ValidID(id) {
		return TelegramWriteGrant{}, core.Missing()
	}
	tx, e := s.readSnapshot(ctx, a)
	if e != nil {
		return TelegramWriteGrant{}, e
	}
	defer tx.Rollback(ctx)
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return TelegramWriteGrant{}, e
	}
	out, e := grantRead(ctx, tx, a, id)
	if e != nil {
		return out, e
	}
	_, _, e = s.readableRun(ctx, tx, a, out.RunID)
	return out, e
}
func (s *Service) CreateTelegramWriteGrant(ctx context.Context, c core.Command, conversation string, in TelegramWriteGrantInput) (core.Result, error) {
	r := in.Run
	if !core.ValidID(in.ChannelID) || !core.ValidID(in.BindingID) || core.RequireVersion(in.ChannelVersion) != nil || core.RequireVersion(in.BindingVersion) != nil || core.Name(in.Reason) != nil || !core.ValidID(r.ProfileID) || !core.ValidID(r.ClientMessageID) || core.RequireVersion(r.ExpectedVersion) != nil || core.RequireVersion(r.ContextSecurityVersion) != nil || strings.TrimSpace(r.Message) == "" || len([]rune(r.Message)) > 20000 || r.Mutation == nil || r.Mutation.Tool != "update_task" || r.FileCreate != nil || r.FileUpdate != nil || r.FileRecovery != nil || r.OARequest != nil || r.PostWrite != nil || r.BusinessDraft != nil || r.BusinessRevision != nil || r.ContractBody != nil || r.TelegramHandoff != nil || r.ScopeTransfer != nil || r.Notification != nil {
		return core.Result{}, core.Invalid("grant", "请仅预授权一项完整固定的任务字段更新，其他任务继续使用人工入口")
	}
	c.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return identity.RequireInteractiveSession(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		conv, e := metadata(ctx, tx, c.Actor.OrgID, conversation, true)
		if e != nil {
			return core.Result{}, e
		}
		if conv.Type == "telegram" || conv.CreatorID != c.Actor.UserID {
			return core.Result{}, core.Forbidden()
		}
		members, e := memberIDs(ctx, tx, c.Actor.OrgID, conv)
		if e != nil {
			return core.Result{}, e
		}
		if len(members) != 1 || members[0] != c.Actor.UserID {
			return core.Result{}, core.Invalid("conversation", "TG写预授权仅用于本人独立会话")
		}
		if e = require(ctx, tx, c.Actor, conv, "agent.run"); e != nil {
			return core.Result{}, e
		}
		var valid bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_channels ch JOIN telegram_bindings b ON b.org_id=ch.org_id AND b.bot_id=ch.bot_id
   JOIN agent_runtime_profiles p ON p.org_id=ch.org_id AND p.id=$6 WHERE ch.org_id=$1 AND ch.id=$2 AND ch.version=$3 AND ch.state='active'
   AND b.version=$8 AND b.id=$4 AND b.user_id=$5 AND b.state='active' AND ch.project_id IS NOT DISTINCT FROM $7::uuid AND 'project-update'=ANY(ch.allowed_skill_ids)
   AND p.skill_id='project-update' AND p.state='active' AND p.telegram_write_configuration_sha256 IS NOT NULL AND agent_profile_release_enabled(p.org_id,p.id))`, c.Actor.OrgID, in.ChannelID, in.ChannelVersion, in.BindingID, c.Actor.UserID, r.ProfileID, conv.ProjectID, in.BindingVersion).Scan(&valid)
		if e != nil {
			return core.Result{}, e
		}
		if !valid {
			return core.Result{}, core.Fail(409, "TELEGRAM_WRITE_CONFIGURATION_REQUIRED", "准确群范围、本人绑定和单独部署的TG写配置尚未就绪")
		}
		id, run := core.NewID(), core.NewID()
		encoded, marshalError := json.Marshal(in)
		if marshalError != nil {
			return core.Result{}, marshalError
		}
		raw, canonicalError := core.CanonicalJSON(encoded)
		if canonicalError != nil {
			return core.Result{}, canonicalError
		}
		hash := core.Digest(raw)
		_, e = tx.Exec(ctx, `INSERT INTO telegram_write_grants(id,org_id,run_id,conversation_id,actor_id,actor_auth_version,organization_auth_version,session_id,origin_channel,channel_id,channel_version,binding_id,binding_version,profile_id,configuration_sha256,input,input_sha256,reason,expires_at)
   SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$17,p.id,p.telegram_write_configuration_sha256,$14,$15,$16,clock_timestamp()+least(p.timeout_seconds,600)*interval '1 second' FROM agent_runtime_profiles p WHERE p.org_id=$2 AND p.id=$13`, id, c.Actor.OrgID, run, conversation, c.Actor.UserID, c.Actor.AuthVersion, c.Actor.OrgAuthVersion, c.Actor.SessionID, c.Actor.Channel, in.ChannelID, in.ChannelVersion, in.BindingID, r.ProfileID, string(raw), hash, in.Reason, in.BindingVersion)
		if e != nil {
			return core.Result{}, e
		}
		result, e := s.createRunTx(ctx, tx, c, conversation, r, &telegramGrantMode{RunID: run})
		if e != nil {
			return core.Result{}, e
		}
		actual, ok := result.Data.(Run)
		if !ok || actual.ID != run {
			return core.Result{}, core.Fail(409, "MESSAGE_ID_CONFLICT", "该消息标识已用于其他任务，请重新核对后发起")
		}
		st := executionState{Run: actual, Actor: c.Actor, Conversation: conv}
		st.Actor.Channel = "telegram"
		st.Actor.SessionID = ""
		if e = s.telegramGrantState(ctx, tx, &st, false); e != nil {
			return core.Result{}, e
		}
		sources, e := grantSources(ctx, tx, c.Actor.OrgID, run)
		if e != nil {
			return core.Result{}, e
		}
		if e = executionSources(ctx, tx, st, sources); e != nil {
			return core.Result{}, e
		}
		if _, e = executionMutationState(ctx, tx, st); e != nil {
			return core.Result{}, e
		}
		out, e := grantRead(ctx, tx, c.Actor, id)
		return core.Result{Status: 201, Data: out, Event: core.Event{Type: "telegram.write_grant.created", ResourceType: "agent_run", ResourceID: run, Version: 1, Payload: map[string]any{"grant_id": id, "channel_id": in.ChannelID, "input_sha256": hash, "queued": false}}}, e
	})
}
func grantSources(ctx context.Context, tx pgx.Tx, org, run string) ([]SourceSnapshot, error) {
	var raw []byte
	var out []SourceSnapshot
	if e := tx.QueryRow(ctx, "SELECT source_snapshots FROM conversation_messages WHERE org_id=$1 AND run_id=$2 AND role='user'", org, run).Scan(&raw); e != nil {
		return nil, e
	}
	e := json.Unmarshal(raw, &out)
	return out, e
}

// Rechecks the fixed original human session independently of the actual TG
// business channel. Never overwrite channel or organization versions on a run.
func telegramGrantHuman(ctx context.Context, tx pgx.Tx, st executionState) (core.Actor, error) {
	human := st.Actor
	if st.Actor.Channel != "telegram" {
		return human, nil
	}
	if st.Run.TelegramWriteGrantID == nil {
		return human, core.Forbidden()
	}
	e := tx.QueryRow(ctx, `SELECT session_id,origin_channel FROM telegram_write_grants WHERE org_id=$1 AND id=$2 AND run_id=$3 AND actor_id=$4 AND actor_auth_version=$5 AND organization_auth_version=$6 AND expires_at>clock_timestamp()`, st.Actor.OrgID, *st.Run.TelegramWriteGrantID, st.Run.ID, st.Actor.UserID, st.Actor.AuthVersion, st.Actor.OrgAuthVersion).Scan(&human.SessionID, &human.Channel)
	if e != nil {
		return human, core.Forbidden()
	}
	human.RunID = ""
	return human, identity.RequireInteractiveSession(ctx, tx, human, true)
}
func (s *Service) telegramGrantState(ctx context.Context, tx pgx.Tx, st *executionState, activated bool) error {
	if st.Run.TelegramWriteGrantID == nil {
		return runtimeFailure("TELEGRAM_GRANT_REQUIRED")
	}
	var valid bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_write_grants g JOIN telegram_channels ch ON ch.org_id=g.org_id AND ch.id=g.channel_id
 JOIN telegram_bindings b ON b.org_id=g.org_id AND b.id=g.binding_id JOIN agent_runtime_profiles p ON p.org_id=g.org_id AND p.id=g.profile_id
 WHERE g.org_id=$1 AND g.id=$2 AND g.run_id=$3 AND g.actor_id=$4 AND g.actor_auth_version=$5 AND g.organization_auth_version=$6
 AND g.expires_at>clock_timestamp() AND ch.state='active' AND ch.version=g.channel_version AND ch.project_id IS NOT DISTINCT FROM $7::uuid
 AND b.state='active' AND b.version=g.binding_version AND b.bot_id=ch.bot_id AND b.user_id=g.actor_id AND 'project-update'=ANY(ch.allowed_skill_ids)
 AND p.state='active' AND p.skill_id='project-update' AND p.telegram_write_configuration_sha256=g.configuration_sha256 AND agent_profile_release_enabled(p.org_id,p.id)
 AND (NOT $8 OR EXISTS(SELECT 1 FROM telegram_write_activations a JOIN telegram_inbox i ON i.org_id=a.org_id AND i.id=a.inbox_id
 WHERE a.org_id=g.org_id AND a.grant_id=g.id AND i.state='dispatched' AND i.requester_id=g.actor_id AND i.binding_id=g.binding_id AND i.channel_id=g.channel_id
 AND i.channel_version=g.channel_version AND i.bot_id=ch.bot_id AND i.chat_id=ch.chat_id AND i.topic_id=ch.topic_id AND i.telegram_user_id=b.telegram_user_id)))`, st.Actor.OrgID, *st.Run.TelegramWriteGrantID, st.Run.ID, st.Actor.UserID, st.Actor.AuthVersion, st.Actor.OrgAuthVersion, st.Conversation.ProjectID, activated).Scan(&valid)
	if e != nil {
		return e
	}
	if !valid {
		return runtimeFailure("TELEGRAM_GRANT_CHANGED")
	}
	if _, e = telegramGrantHuman(ctx, tx, *st); e != nil {
		return e
	}
	members, e := telegramGrantAudience(ctx, tx, st.Actor, st.Conversation)
	if e != nil {
		return e
	}
	if len(members) != 1 || members[0] != st.Actor.UserID || len(st.Run.audience) != 1 || st.Run.audience[0] != st.Actor.UserID {
		return runtimeFailure("TELEGRAM_GRANT_CHANGED")
	}
	st.Profile, e = scanProfile(tx.QueryRow(ctx, "SELECT "+profileColumns+" FROM agent_runtime_profiles WHERE org_id=$1 AND id=$2", st.Actor.OrgID, st.Run.ProfileID))
	return e
}
func telegramGrantAudience(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation) ([]string, error) {
	members, e := memberIDs(ctx, tx, a.OrgID, c)
	if e != nil {
		return nil, e
	}
	ai := a
	ai.RunID = core.NewID()
	for _, id := range members {
		ai.UserID = id
		if e = require(ctx, tx, ai, c, "conversation.read"); e != nil {
			return nil, e
		}
	}
	return members, nil
}
func (s *Service) dispatchTelegramGrant(ctx context.Context, tx pgx.Tx, in telegramRequest) error {
	fields := strings.Fields(in.Message)
	if len(fields) != 3 || strings.Split(strings.ToLower(fields[0]), "@")[0] != "/task" || fields[1] != "project-update" || !strings.HasPrefix(fields[2], "authorize:") {
		return core.Invalid("grant", "请使用本人预授权页面生成的完整固定指令")
	}
	id := strings.TrimPrefix(fields[2], "authorize:")
	if !core.ValidID(id) {
		return core.Invalid("grant", "预授权标识无效")
	}
	// The dispatcher already holds the organization FOR UPDATE lock. Grants are
	// immutable INSERT/SELECT evidence; row locking would require forbidden UPDATE
	// privileges. The organization lock plus the activation primary key serialize use.
	var run string
	e := tx.QueryRow(ctx, `SELECT run_id FROM telegram_write_grants g WHERE org_id=$1 AND id=$2 AND actor_id=$3 AND actor_auth_version=$4 AND channel_id=$5 AND channel_version=$6 AND binding_id=$7 AND NOT EXISTS(SELECT 1 FROM telegram_write_activations a WHERE a.org_id=g.org_id AND a.grant_id=g.id)`, in.Actor.OrgID, id, in.Actor.UserID, in.Actor.AuthVersion, in.ChannelID, in.ChannelVersion, in.BindingID).Scan(&run)
	if e == pgx.ErrNoRows {
		return core.Forbidden()
	}
	if e != nil {
		return e
	}
	r, e := runRecord(ctx, tx, in.Actor.OrgID, run, true)
	if e != nil {
		return e
	}
	if r.State != "queued" || r.CancelRequestedAt != nil || time.Now().After(r.ExpiresAt) {
		return runtimeFailure("TELEGRAM_GRANT_CHANGED")
	}
	conv, e := metadata(ctx, tx, in.Actor.OrgID, r.ConversationID, true)
	if e != nil {
		return e
	}
	var same bool
	if e = tx.QueryRow(ctx, "SELECT conversation_revision_id=$3 AND context_security_version=$4 FROM agent_runs WHERE org_id=$1 AND id=$2", in.Actor.OrgID, r.ID, conv.CurrentRevisionID, conv.ContextSecurityVersion).Scan(&same); e != nil {
		return e
	}
	if !same {
		return runtimeFailure("TELEGRAM_GRANT_CHANGED")
	}
	st := executionState{Run: r, Actor: in.Actor, Conversation: conv}
	if e = s.telegramGrantState(ctx, tx, &st, false); e != nil {
		return e
	}
	sources, e := grantSources(ctx, tx, in.Actor.OrgID, run)
	if e != nil {
		return e
	}
	if e = executionSources(ctx, tx, st, sources); e != nil {
		return e
	}
	if _, e = executionMutationState(ctx, tx, st); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO telegram_write_activations(org_id,grant_id,inbox_id,run_id) VALUES($1,$2,$3,$4)", in.Actor.OrgID, id, in.ID, run); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "UPDATE telegram_inbox SET state='dispatched',reason_code='authorized_run_queued' WHERE org_id=$1 AND id=$2", in.Actor.OrgID, in.ID); e != nil {
		return e
	}
	if e = s.enqueueRunJob(ctx, tx, in.Actor.OrgID, run); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,run_id,action,resource_type,resource_id,resource_version,after_digest,result) VALUES($1::uuid,$2,$3,$3,'telegram',$1::text,$4::uuid,'telegram.write_grant.consumed','agent_run',$4::uuid,1,$5,'succeeded')`, core.NewID(), in.Actor.OrgID, in.Actor.UserID, run, core.Digest([]byte(id)))
	return e
}
