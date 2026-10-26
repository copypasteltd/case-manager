package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func snapshot(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, base *string, ids []string, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO conversation_revisions(id,org_id,conversation_id,seq,base_revision_id,title,state,member_ids,context_security_version,author_id,reason)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, c.CurrentRevisionID, a.OrgID, c.ID, c.Version, base, c.Title, c.State, ids, c.ContextSecurityVersion, a.UserID, reason)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.Exec(ctx, "INSERT INTO conversation_members(org_id,conversation_id,revision_id,user_id) VALUES($1,$2,$3,$4)", a.OrgID, c.ID, c.CurrentRevisionID, id); err != nil {
			return err
		}
	}
	return nil
}
func liveResult(ctx context.Context, s *Service, a core.Actor, result core.Result, err error) (core.Result, error) {
	if err != nil || !result.Replayed {
		return result, err
	}
	bytes, e := json.Marshal(result.Data)
	if e != nil {
		return core.Result{}, e
	}
	var stored Conversation
	if e = json.Unmarshal(bytes, &stored); e != nil {
		return core.Result{}, e
	}
	current, e := s.Get(ctx, a, stored.ID)
	if e != nil {
		return core.Result{}, e
	}
	result.Data = current
	return result, nil
}
func event(c Conversation, action, reason string) core.Event {
	return core.Event{Type: action, ResourceType: "conversation", ResourceID: c.ID, Version: c.Version,
		Payload: map[string]any{"reason": reason, "revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion}}
}
func (s *Service) Create(ctx context.Context, command core.Command, in ConversationCreate) (core.Result, error) {
	if command.Actor.RunID != "" {
		return core.Result{}, core.Forbidden()
	}
	in.Title = strings.TrimSpace(in.Title)
	if err := core.Name(in.Title); err != nil {
		return core.Result{}, err
	}
	ids, err := normalizeMembers(command.Actor, in)
	if err != nil {
		return core.Result{}, err
	}
	c := Conversation{ID: core.NewID(), Type: in.Type, ProjectID: in.ProjectID, CreatorID: command.Actor.UserID, Version: 1,
		CurrentRevisionID: core.NewID(), Title: in.Title, State: "active", ContextSecurityVersion: 1}
	result, err := s.Commands.Do(ctx, command, func(tx pgx.Tx) error {
		var e error
		c.Classification, e = projectScope(ctx, tx, command.Actor, c.ProjectID)
		if e != nil {
			return e
		}
		if c.Classification == "confidential" {
			return core.Fail(422, "CONVERSATION_EXPLICIT_GRANT_REQUIRED", "严格受限会话需要独立的安全范围流程")
		}
		return authz.Require(ctx, tx, command.Actor, "conversation.create", resource(c))
	}, func(tx pgx.Tx) (core.Result, error) {
		for _, user := range ids {
			if e := eligible(ctx, tx, command.Actor, c, user); e != nil {
				return core.Result{}, e
			}
		}
		err := tx.QueryRow(ctx, `INSERT INTO conversations(id,org_id,creator_id,type,project_id,classification,current_revision_id)
            VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING created_at,updated_at`, c.ID, command.Actor.OrgID, c.CreatorID, c.Type, c.ProjectID, c.Classification, c.CurrentRevisionID).Scan(&c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return core.Result{}, err
		}
		if err = snapshot(ctx, tx, command.Actor, c, nil, ids, "新建会话"); err != nil {
			return core.Result{}, err
		}
		c, err = present(ctx, tx, command.Actor, c)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: c, Event: event(c, "conversation.created", "新建会话")}, nil
	})
	return liveResult(ctx, s, command.Actor, result, err)
}
func validatePatch(version int64, reason string) error {
	if err := core.RequireVersion(version); err != nil {
		return err
	}
	if err := core.Name(reason); err != nil {
		return core.Invalid("reason", "请填写1至200个字符的变更原因")
	}
	return nil
}
func (s *Service) Update(ctx context.Context, command core.Command, id string, in ConversationPatch) (core.Result, error) {
	if command.Actor.RunID != "" {
		return core.Result{}, core.Forbidden()
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Reason = strings.TrimSpace(in.Reason)
	if err := validatePatch(in.ExpectedVersion, in.Reason); err != nil {
		return core.Result{}, err
	}
	if err := core.Name(in.Title); err != nil {
		return core.Result{}, err
	}
	if !slices.Contains([]string{"active", "archived"}, in.State) {
		return core.Result{}, core.Invalid("state", "会话状态无效")
	}
	result, err := s.Commands.Do(ctx, command, func(tx pgx.Tx) error {
		c, e := metadata(ctx, tx, command.Actor.OrgID, id, false)
		if e != nil {
			return e
		}
		if c.Type == "telegram" {
			return core.Forbidden()
		}
		return require(ctx, tx, command.Actor, c, "conversation.update")
	}, func(tx pgx.Tx) (core.Result, error) {
		c, e := metadata(ctx, tx, command.Actor.OrgID, id, true)
		if e != nil {
			return core.Result{}, e
		}
		if c.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "会话已被更新，请保留输入并重新核对")
		}
		if c.Title == in.Title && c.State == in.State {
			return core.Result{}, core.Fail(409, "CONVERSATION_UNCHANGED", "会话资料未发生变更")
		}
		ids, e := memberIDs(ctx, tx, command.Actor.OrgID, c)
		if e != nil {
			return core.Result{}, e
		}
		base := c.CurrentRevisionID
		c.CurrentRevisionID = core.NewID()
		c.Version++
		c.Title = in.Title
		c.State = in.State
		if e = snapshot(ctx, tx, command.Actor, c, &base, ids, in.Reason); e != nil {
			return core.Result{}, e
		}
		if e = tx.QueryRow(ctx, "UPDATE conversations SET version=$3,current_revision_id=$4,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2 RETURNING updated_at", command.Actor.OrgID, c.ID, c.Version, c.CurrentRevisionID).Scan(&c.UpdatedAt); e != nil {
			return core.Result{}, e
		}
		c, e = present(ctx, tx, command.Actor, c)
		if e != nil {
			return core.Result{}, e
		}
		return core.Result{Data: c, Event: event(c, "conversation.updated", in.Reason)}, nil
	})
	return liveResult(ctx, s, command.Actor, result, err)
}
func (s *Service) ChangeMember(ctx context.Context, command core.Command, id string, in MemberChange) (core.Result, error) {
	if command.Actor.RunID != "" {
		return core.Result{}, core.Forbidden()
	}
	in.UserID = strings.ToLower(in.UserID)
	in.Reason = strings.TrimSpace(in.Reason)
	if err := validatePatch(in.ExpectedVersion, in.Reason); err != nil {
		return core.Result{}, err
	}
	if !core.ValidID(in.UserID) || !slices.Contains([]string{"add", "remove"}, in.Action) {
		return core.Result{}, core.Invalid("user_id", "成员或操作无效")
	}
	command.AuthorizationMutation = true
	return s.Commands.Do(ctx, command, func(tx pgx.Tx) error {
		c, e := metadata(ctx, tx, command.Actor.OrgID, id, false)
		if e != nil {
			return e
		}
		return require(ctx, tx, command.Actor, c, "conversation.manage_members")
	}, func(tx pgx.Tx) (core.Result, error) {
		c, e := metadata(ctx, tx, command.Actor.OrgID, id, true)
		if e != nil {
			return core.Result{}, e
		}
		if c.Version != in.ExpectedVersion {
			return core.Result{}, core.Fail(409, "VERSION_CONFLICT", "会话成员已被更新，请重新核对")
		}
		if c.Type != "project" {
			return core.Result{}, core.Fail(422, "PRIVATE_CONVERSATION", "个人会话不能更改成员")
		}
		if c.State != "active" {
			return core.Result{}, core.Fail(409, "CONVERSATION_READ_ONLY", "请先恢复会话再调整成员")
		}
		if in.UserID == c.CreatorID {
			return core.Result{}, core.Fail(422, "CONVERSATION_CREATOR_REQUIRED", "创建人必须保留在会话成员中")
		}
		ids, e := memberIDs(ctx, tx, command.Actor.OrgID, c)
		if e != nil {
			return core.Result{}, e
		}
		exists := slices.Contains(ids, in.UserID)
		if (in.Action == "add") == exists {
			return core.Result{}, core.Fail(409, "CONVERSATION_MEMBERS_UNCHANGED", "成员名单已发生变化，请重新核对")
		}
		if in.Action == "add" {
			if len(ids) >= 100 {
				return core.Result{}, core.Invalid("user_id", "会话成员数量超出限制")
			}
			if e = eligible(ctx, tx, command.Actor, c, in.UserID); e != nil {
				return core.Result{}, e
			}
			ids = append(ids, in.UserID)
		} else {
			ids = slices.DeleteFunc(ids, func(user string) bool { return user == in.UserID })
		}
		slices.Sort(ids)
		base := c.CurrentRevisionID
		c.CurrentRevisionID = core.NewID()
		c.Version++
		c.ContextSecurityVersion++
		if e = snapshot(ctx, tx, command.Actor, c, &base, ids, in.Reason); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE conversations SET version=$3,current_revision_id=$4,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", command.Actor.OrgID, c.ID, c.Version, c.CurrentRevisionID); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1,updated_at=clock_timestamp() WHERE id=$1", command.Actor.OrgID); e != nil {
			return core.Result{}, e
		}
		change := event(c, "conversation.members_changed", in.Reason)
		change.Payload["user_id"] = in.UserID
		change.Payload["action"] = in.Action
		return core.Result{Data: AccessResult{ConversationID: c.ID, Version: c.Version, ContextSecurityVersion: c.ContextSecurityVersion, AccessLost: !slices.Contains(ids, command.Actor.UserID)}, Event: change}, nil
	})
}

func (s *Service) MemberCandidates(ctx context.Context, a core.Actor, id string) (map[string]any, error) {
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	c, err := metadata(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return nil, err
	}
	if err = require(ctx, tx, a, c, "conversation.manage_members"); err != nil {
		return nil, err
	}
	items := []Member{}
	if c.Type != "project" || c.State != "active" {
		return map[string]any{"items": items}, platform.DatabaseError(tx.Commit(ctx))
	}
	rows, err := tx.Query(ctx, `SELECT u.id,u.display_name FROM project_members pm JOIN users u ON u.org_id=pm.org_id AND u.id=pm.user_id
        JOIN projects p ON p.org_id=pm.org_id AND p.id=pm.project_id WHERE pm.org_id=$1 AND pm.project_id=$2 AND u.status='active'
        AND NOT EXISTS(SELECT 1 FROM conversation_members cm WHERE cm.revision_id=$3 AND cm.user_id=u.id)
        AND can_access(p.org_id,u.id,'project.read','project',p.id,p.id,p.creator_id,p.classification,$4,false)
        AND can_access(p.org_id,u.id,'conversation.read','conversation',$5,p.id,$6,$7,$4,false)
        ORDER BY u.id LIMIT 101`, a.OrgID, *c.ProjectID, c.CurrentRevisionID, a.Channel, c.ID, c.CreatorID, c.Classification)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var m Member
		if err = rows.Scan(&m.UserID, &m.DisplayName); err != nil {
			return nil, err
		}
		m.Available = true
		items = append(items, m)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(items) > 100 {
		return nil, core.Fail(422, "CONVERSATION_DIRECTORY_LIMIT", "可选成员数量超出当前选择器范围")
	}
	return map[string]any{"items": items}, platform.DatabaseError(tx.Commit(ctx))
}
