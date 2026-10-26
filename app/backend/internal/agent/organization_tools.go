package agent

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func organizationTool(tool string) bool {
	return slices.Contains([]string{"list_directory", "propose_organization", "commit_organization"}, tool)
}
func organizationPlanSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string) (SourceSnapshot, error) {
	p, err := organizationPlanRecord(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if err = authz.RequireOrganization(ctx, tx, a, "agent.plan.read"); err != nil {
		return SourceSnapshot{}, err
	}
	if err = organizationReadable(ctx, tx, a, c, p); err != nil {
		return SourceSnapshot{}, err
	}
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_organization_plan", ID: p.ID}, ResourceVersion: 1, SHA256: p.PlanHash}, nil
}
func directorySnapshotSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string) (SourceSnapshot, error) {
	var listing files.DirectoryListing
	var payload []byte
	var sha, actor, conversation string
	err := tx.QueryRow(ctx, `SELECT d.payload,d.sha256,r.actor_id,r.conversation_id FROM agent_directory_snapshots d JOIN agent_runs r ON r.org_id=d.org_id AND r.id=d.run_id WHERE d.org_id=$1 AND d.id=$2`, a.OrgID, id).Scan(&payload, &sha, &actor, &conversation)
	if err != nil {
		return SourceSnapshot{}, platform.DatabaseError(err)
	}
	if actor != a.UserID || conversation != c.ID || c.Type != "private" || c.CreatorID != a.UserID {
		return SourceSnapshot{}, core.Missing()
	}
	if err = json.Unmarshal(payload, &listing); err != nil {
		return SourceSnapshot{}, err
	}
	for _, old := range append([]files.OrganizationEntry{listing.Folder}, listing.Items...) {
		current, e := files.OrganizationMetadata(ctx, tx, a, old.Kind, old.ID, old.Kind+".read")
		if e != nil {
			return SourceSnapshot{}, e
		}
		if e = organizationEntryScope(c, current); e != nil {
			return SourceSnapshot{}, e
		}
	}
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_directory", ID: id}, ResourceVersion: 1, SHA256: sha}, nil
}
func directoryInConversation(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string) (files.DirectoryListing, error) {
	var out files.DirectoryListing
	if c.Type != "private" || c.CreatorID != a.UserID {
		return out, core.Fail(409, "PLAN_SCOPE_UNSUPPORTED", "目录整理首期仅支持本人私密会话")
	}
	if a.RunID == "" {
		a.RunID = core.NewID()
	}
	out, err := files.AgentDirectory(ctx, tx, a, id)
	if err != nil {
		return out, err
	}
	if err = organizationEntryScope(c, out.Folder); err != nil {
		return files.DirectoryListing{}, err
	}
	items := []files.OrganizationEntry{}
	for _, v := range out.Items {
		if organizationEntryScope(c, v) == nil {
			items = append(items, v)
		}
	}
	out.Items = items
	return out, nil
}
func (s *Service) OrganizationDirectory(ctx context.Context, a core.Actor, conversationID, folderID string) (files.DirectoryListing, error) {
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return files.DirectoryListing{}, err
	}
	defer tx.Rollback(ctx)
	c, err := organizationConversation(ctx, tx, a, conversationID, "read", false)
	if err != nil {
		return files.DirectoryListing{}, err
	}
	out, err := directoryInConversation(ctx, tx, a, c, folderID)
	if err != nil {
		return files.DirectoryListing{}, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) executionOrganization(ctx context.Context, tx pgx.Tx, st executionState, tool string, args json.RawMessage) (map[string]any, []SourceSnapshot, error) {
	a := st.Actor
	a.RunID = st.Run.ID
	if st.Conversation.Type != "private" || st.Conversation.CreatorID != a.UserID {
		return nil, nil, core.Missing()
	}
	if tool == "list_directory" {
		var in struct {
			FolderID string `json:"folder_id"`
		}
		if err := strictArguments(args, &in); err != nil {
			return nil, nil, err
		}
		listing, err := directoryInConversation(ctx, tx, a, st.Conversation, in.FolderID)
		if err != nil {
			return nil, nil, err
		}
		id := core.NewID()
		raw, _ := json.Marshal(listing)
		var sha string
		var observed time.Time
		err = tx.QueryRow(ctx, `INSERT INTO agent_directory_snapshots(id,org_id,run_id,folder_id,payload,sha256) VALUES($1,$2,$3,$4,$5,encode(sha256(convert_to($5::jsonb::text,'UTF8')),'hex')) RETURNING sha256,created_at`, id, a.OrgID, st.Run.ID, in.FolderID, raw).Scan(&sha, &observed)
		if err != nil {
			return nil, nil, err
		}
		src := SourceSnapshot{SourceReference: SourceReference{Type: "agent_directory", ID: id}, ResourceVersion: 1, SHA256: sha}
		return map[string]any{"directory": listing, "observed_at": observed, "untrusted_data": true, "source_sha256": sha, "source_refs": []ResultReference{reference(src)}}, []SourceSnapshot{src}, nil
	}
	var p OrganizationPlan
	var err error
	if tool == "propose_organization" {
		var in struct {
			Actions []files.OrganizationAction `json:"actions"`
			Reason  string                     `json:"reason"`
		}
		if err = strictArguments(args, &in); err != nil {
			return nil, nil, err
		}
		if err = authz.RequireOrganization(ctx, tx, a, "agent.plan.propose"); err != nil {
			return nil, nil, err
		}
		p, err = proposeOrganization(ctx, tx, a, st.Conversation, OrganizationPlanCreate{ExpectedVersion: st.Conversation.Version, ContextSecurityVersion: st.Conversation.ContextSecurityVersion, Actions: in.Actions, Reason: in.Reason})
	} else {
		var in struct {
			PlanID string `json:"plan_id"`
			PlanAction
		}
		if err = strictArguments(args, &in); err != nil {
			return nil, nil, err
		}
		if core.Name(in.Reason) != nil {
			return nil, nil, core.Invalid("reason", "请填写整理提交原因")
		}
		p, err = organizationPlanRecord(ctx, tx, a.OrgID, in.PlanID, true)
		if err != nil {
			return nil, nil, err
		}
		if err = authz.RequireOrganization(ctx, tx, a, "agent.plan.commit"); err != nil {
			return nil, nil, err
		}
		p, err = s.commitOrganization(ctx, tx, a, st.Conversation, p, in.PlanAction, core.NewID())
	}
	if err != nil {
		return nil, nil, err
	}
	if err = executionAudit(ctx, tx, st, "agent.organization."+p.State, map[string]any{"plan_id": p.ID, "plan_hash": p.PlanHash, "operation_id": p.OperationID}); err != nil {
		return nil, nil, err
	}
	src, err := organizationPlanSource(ctx, tx, a, st.Conversation, p.ID)
	if err != nil {
		return nil, nil, err
	}
	refs := []ResultReference{{Type: "agent_organization_plan", ID: p.ID}}
	status, summary, next := "pending", "整理计划已拟定，资料尚未变更", "请本人核对逐项差异，并通过两步验证批准准确计划"
	if p.State == "committed" {
		status, summary, next = "succeeded", "已按本人批准的准确计划整批整理资料，并保存逐项业务回执", ""
		for i, r := range p.Results {
			var value struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(r.Data, &value)
			kind := "folder"
			if p.Preview[i].Action.Kind == "file.move" {
				kind = "file"
			}
			refs = append(refs, ResultReference{Type: kind, ID: value.ID})
		}
	}
	view, err := presentOrganizationPlan(ctx, tx, a, st.Conversation, p)
	if err != nil {
		return nil, nil, err
	}
	return map[string]any{"status": status, "resource_refs": refs, "source_refs": []ResultReference{reference(src)}, "operation_id": p.OperationID, "committed": p.State == "committed", "safe_summary": summary, "retryable": false, "next_action": next, "content": view}, []SourceSnapshot{src}, nil
}
func executionOrganizationText(ctx context.Context, tx pgx.Tx, st executionState, id string, version *string, offset, limit int) (any, []SourceSnapshot, error) {
	if st.Profile.SkillID != "information-organize" || version != nil {
		return nil, nil, core.Missing()
	}
	if !core.ValidID(id) || offset < 0 || offset > 200000 || limit < 1 || limit > 10000 {
		return nil, nil, core.Invalid("arguments", "请指定准确计划及有限字符范围")
	}
	a := st.Actor
	a.RunID = st.Run.ID
	src, err := organizationPlanSource(ctx, tx, a, st.Conversation, id)
	if err != nil {
		return nil, nil, err
	}
	p, err := organizationPlanRecord(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return nil, nil, err
	}
	p, err = presentOrganizationPlan(ctx, tx, a, st.Conversation, p)
	if err != nil {
		return nil, nil, err
	}
	var raw string
	if err = tx.QueryRow(ctx, "SELECT intent FROM agent_organization_plans WHERE org_id=$1 AND id=$2", a.OrgID, id).Scan(&raw); err != nil {
		return nil, nil, err
	}
	text := []rune(raw)
	if offset > len(text) {
		return nil, nil, core.Invalid("offset", "计划读取位置超出原文")
	}
	end := min(offset+limit, len(text))
	var next *int
	if end < len(text) {
		v := end
		next = &v
	}
	return map[string]any{"type": "agent_organization_plan", "id": p.ID, "version_id": nil, "resource_version": 1, "plan_version": p.Version, "plan_hash": p.PlanHash, "state": p.State, "blocked_reason": p.BlockedReason, "expires_at": p.ExpiresAt, "untrusted_data": true, "source_sha256": src.SHA256, "text": string(text[offset:end]), "offset": offset, "total_chars": len(text), "next_offset": next}, []SourceSnapshot{src}, nil
}
