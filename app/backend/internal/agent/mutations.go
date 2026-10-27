package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"github.com/jackc/pgx/v5"
)

// MutationInput is supplied only by an authenticated human sending a new run.
// Tool calls must exactly match it; ordinary chat and citations grant no write.
type MutationInput struct {
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}
type MutationView struct {
	RunID         string          `json:"run_id"`
	Tool          string          `json:"tool"`
	ResourceType  string          `json:"resource_type"`
	ResourceID    string          `json:"resource_id"`
	BaseVersion   int64           `json:"base_version"`
	IntentSHA256  string          `json:"intent_sha256"`
	Arguments     json.RawMessage `json:"arguments"`
	Before        json.RawMessage `json:"before"`
	Status        string          `json:"status"`
	OperationID   *string         `json:"operation_id"`
	ResultVersion *int64          `json:"result_version"`
	Result        json.RawMessage `json:"result"`
}
type mutationRecord struct {
	MutationView
	OrgID, ActorID, ConversationID, ProjectID, BaseSHA256, Intent string
	SessionID                                                     string
	ResultSHA256                                                  *string
}
type mutationArguments struct {
	ID              string `json:"id"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expected_version"`
}

func mutationTool(tool string) bool {
	return slices.Contains([]string{"create_task", "create_risk", "update_risk", "create_issue", "update_issue", "create_deliverable", "update_deliverable", "create_milestone", "update_project", "update_task", "transition_task", "transition_project", "update_milestone", "transition_milestone"}, tool)
}

// Keep the original field mask, including absent versus explicit null dates.
// Marshaling Nullable patch structs would incorrectly widen that field mask.
func parseMutation(in MutationInput) (mutationArguments, string, json.RawMessage, error) {
	var base mutationArguments
	if !mutationTool(in.Tool) {
		return base, "", nil, core.Invalid("mutation", "此更新工具尚未开放")
	}
	canonical, err := core.CanonicalJSON(in.Arguments)
	if err != nil || !json.Valid(in.Arguments) || len(canonical) > 65536 {
		return base, "", nil, core.Invalid("mutation", "请提供有限的准确更新内容")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(canonical, &fields) != nil || fields == nil {
		return base, "", nil, core.Invalid("mutation", "更新内容必须是对象")
	}
	rawBase, _ := json.Marshal(map[string]json.RawMessage{"id": fields["id"], "reason": fields["reason"], "expected_version": fields["expected_version"]})
	if strictArguments(rawBase, &base) != nil || !core.ValidID(base.ID) || base.ID != strings.ToLower(base.ID) || core.RequireVersion(base.ExpectedVersion) != nil || core.Name(base.Reason) != nil {
		return base, "", nil, core.Invalid("mutation", "请核对对象、基础版本和更新原因")
	}
	if in.Tool == "create_task" {
		return parseTaskCreation(base, canonical, fields)
	}
	if concernMutationTool(in.Tool) {
		return parseConcernMutation(in.Tool, base, canonical, fields)
	}
	allowed := []string{"id", "expected_version", "reason"}
	kind := "task"
	switch in.Tool {
	case "create_milestone":
		// id/version identify the parent project; the business command allocates
		// the new milestone ID. No model-supplied result ID is accepted.
		kind = "project"
		allowed = append(allowed, "title", "owner_id", "due_at")
		var typed struct {
			mutationArguments
			project.MilestoneCreate
		}
		err = strictArguments(canonical, &typed)
		if err == nil && (core.Name(typed.Title) != nil || !exactOAID(typed.OwnerID) || typed.DueAt.IsZero()) {
			err = core.Invalid("milestone", "请明确填写里程碑标题、项目成员负责人和含时区的目标时刻")
		}
	case "create_deliverable", "update_deliverable":
		kind = "deliverable"
		if in.Tool == "create_deliverable" {
			kind = "project"
		}
		allowed = append(allowed, "name", "acceptance_criteria", "owner_id", "milestone_id")
		var typed struct {
			ID string `json:"id"`
			project.DeliverablePatch
		}
		err = strictArguments(canonical, &typed)
		for _, field := range []string{"name", "acceptance_criteria", "owner_id", "milestone_id"} {
			if _, present := fields[field]; !present {
				err = core.Invalid("mutation", "交付更新须完整核对名称、验收准则、负责人和明确的里程碑关联")
			}
		}
		if err == nil && (core.Name(typed.Name) != nil || strings.TrimSpace(typed.AcceptanceCriteria) == "" || len([]rune(typed.AcceptanceCriteria)) > 20000 || !exactOAID(typed.OwnerID) || typed.MilestoneID != nil && !exactOAID(*typed.MilestoneID)) {
			err = core.Invalid("deliverable", "请完整核对交付资料、实际项目成员和同项目里程碑")
		}
	case "update_project":
		kind = "project"
		allowed = append(allowed, "name", "scope", "type", "start_date", "due_date")
		var typed struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
			project.Patch
		}
		err = strictArguments(canonical, &typed)
	case "update_task":
		allowed = append(allowed, "title", "body", "priority", "assignee_id", "parent_id", "milestone_id", "start_at", "due_at")
		var typed struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
			project.TaskPatch
		}
		err = strictArguments(canonical, &typed)
	case "transition_task":
		allowed = append(allowed, "action")
		var typed struct {
			ID string `json:"id"`
			project.Transition
		}
		err = strictArguments(canonical, &typed)
		if err == nil && !slices.Contains([]string{"start", "review", "return", "complete", "block", "resume", "cancel", "reopen"}, typed.Action) {
			err = core.Invalid("action", "任务动作无效")
		}
	case "update_milestone":
		kind = "milestone"
		allowed = append(allowed, "title", "owner_id", "due_at")
		var typed struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
			project.MilestonePatch
		}
		err = strictArguments(canonical, &typed)
		if _, ok := fields["due_at"]; ok && string(fields["due_at"]) == "null" {
			err = core.Invalid("due_at", "里程碑日期不能清空")
		}
		if typed.OwnerID != nil && !exactOAID(*typed.OwnerID) {
			err = core.Invalid("owner_id", "请选择准确的项目负责人")
		}
	case "transition_project", "transition_milestone":
		kind = "milestone"
		actions := []string{"achieve", "miss", "cancel", "reopen"}
		if in.Tool == "transition_project" {
			kind = "project"
			actions = []string{"activate", "hold", "resume", "close", "cancel", "reopen"}
		}
		allowed = append(allowed, "action")
		var typed struct {
			ID string `json:"id"`
			project.Transition
		}
		err = strictArguments(canonical, &typed)
		if err == nil && !slices.Contains(actions, typed.Action) {
			err = core.Invalid("action", "请使用真实阶段动作；项目归档须填写原结项复盘")
		}
	}
	if err != nil {
		return base, "", nil, err
	}
	for name, value := range fields {
		if !slices.Contains(allowed, name) {
			return base, "", nil, core.Invalid("mutation", "更新包含未开放的字段；权限、分类及验收要求请走原业务流程")
		}
		if string(value) == "null" && !slices.Contains([]string{"start_date", "due_date", "assignee_id", "parent_id", "milestone_id", "start_at", "due_at"}, name) {
			return base, "", nil, core.Invalid("mutation", "此字段不允许为空")
		}
	}
	if len(fields) < 4 {
		return base, "", nil, core.Invalid("mutation", "请明确选择至少一项修改")
	}
	return base, kind, canonical, nil
}

func mutationRecordRead(ctx context.Context, tx pgx.Tx, org, id string) (*mutationRecord, error) {
	var m mutationRecord
	err := tx.QueryRow(ctx, `SELECT m.run_id,m.org_id,m.actor_id,m.conversation_id,m.project_id,m.tool,m.resource_type,m.resource_id,m.base_version,m.base_sha256,
 m.arguments,m.before_data,m.intent,m.intent_sha256,c.operation_id,c.result_version,c.result_sha256,c.result_data,coalesce(m.session_id::text,'')
 FROM agent_run_mutations m LEFT JOIN agent_mutation_commits c ON c.org_id=m.org_id AND c.run_id=m.run_id WHERE m.org_id=$1 AND m.run_id=$2`, org, id).
		Scan(&m.RunID, &m.OrgID, &m.ActorID, &m.ConversationID, &m.ProjectID, &m.Tool, &m.ResourceType, &m.ResourceID, &m.BaseVersion, &m.BaseSHA256,
			&m.Arguments, &m.Before, &m.Intent, &m.IntentSHA256, &m.OperationID, &m.ResultVersion, &m.ResultSHA256, &m.Result, &m.SessionID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.Status = "authorized"
	if m.OperationID != nil {
		m.Status = "committed"
	}
	return &m, nil
}

func mutationBefore(ctx context.Context, tx pgx.Tx, a core.Actor, kind, id string) (json.RawMessage, error) {
	var raw []byte
	query := `SELECT jsonb_build_object('id',id,'version',version,'name',name,'scope',scope,'type',type,'state',state,'start_date',start_date::text,'due_date',due_date::text) FROM projects WHERE org_id=$1 AND id=$2`
	if kind == "task" {
		query = `SELECT jsonb_build_object('id',id,'version',version,'title',title,'body',body,'priority',priority,'state',state,'assignee_id',assignee_id,'parent_id',parent_id,'milestone_id',milestone_id,'start_at',start_at,'due_at',due_at,'review_required',review_required) FROM tasks WHERE org_id=$1 AND id=$2`
	}
	if kind == "milestone" {
		query = `SELECT jsonb_build_object('id',id,'version',version,'project_id',project_id,'title',title,'owner_id',owner_id,'due_at',due_at,'state',state) FROM milestones WHERE org_id=$1 AND id=$2`
	}
	if kind == "deliverable" {
		query = `SELECT jsonb_build_object('id',id,'version',version,'project_id',project_id,'name',name,'acceptance_criteria',acceptance_criteria,'owner_id',owner_id,'milestone_id',milestone_id,'state',state,'protocol_version',protocol_version,'current_revision_id',current_revision_id) FROM deliverables WHERE org_id=$1 AND id=$2`
	}
	if kind == "risk" || kind == "issue" {
		query = "SELECT to_jsonb(r)-'org_id' FROM " + kind + "s r WHERE r.org_id=$1 AND r.id=$2"
	}
	err := tx.QueryRow(ctx, query, a.OrgID, id).Scan(&raw)
	return raw, platform.DatabaseError(err)
}

func mutationAudience(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, m mutationRecord, members []string) (contextDocument, error) {
	if m.ConversationID != c.ID || c.ProjectID == nil || *c.ProjectID != m.ProjectID {
		return contextDocument{}, core.Missing()
	}
	kind, id := m.ResourceType, m.ResourceID
	if err := taskCreationAudience(ctx, tx, a, c, m, members); err != nil {
		return contextDocument{}, err
	}
	if m.Tool == "create_task" && m.OperationID != nil {
		var created project.Task
		if !taskCreationReceipt(m, m.Result) || json.Unmarshal(m.Result, &created) != nil {
			return contextDocument{}, runtimeFailure("MUTATION_RECEIPT_INVALID")
		}
		kind, id = "task", created.ID
	}
	if concernCreationTool(m.Tool) && m.OperationID != nil {
		var created project.Concern
		if json.Unmarshal(m.Result, &created) != nil || !core.ValidID(created.ID) || created.ProjectID != m.ProjectID {
			return contextDocument{}, runtimeFailure("MUTATION_RECEIPT_INVALID")
		}
		kind, id = strings.TrimPrefix(m.Tool, "create_"), created.ID
	}
	if m.Tool == "create_milestone" && m.OperationID != nil {
		var created project.Milestone
		if json.Unmarshal(m.Result, &created) != nil || !core.ValidID(created.ID) || created.ProjectID != m.ProjectID {
			return contextDocument{}, runtimeFailure("MUTATION_RECEIPT_INVALID")
		}
		kind, id = "milestone", created.ID
	}
	if m.Tool == "create_deliverable" && m.OperationID != nil {
		var created project.DeliverableReceipt
		if json.Unmarshal(m.Result, &created) != nil || !core.ValidID(created.ID) || created.ID == m.ResourceID || created.Version != 1 || created.RevisionID != nil {
			return contextDocument{}, runtimeFailure("MUTATION_RECEIPT_INVALID")
		}
		kind, id = "deliverable", created.ID
	}
	doc, err := contextDocumentRead(ctx, tx, a, c, ContextSource{Type: kind, ID: id}, members)
	if err != nil {
		return doc, err
	}
	if err = resourceAudience(ctx, tx, a, c, doc, members); err != nil {
		return doc, err
	}
	if child := mutationRealizedIssue(&m); child != "" {
		linked, e := contextDocumentRead(ctx, tx, a, c, ContextSource{Type: "issue", ID: child}, members)
		if e != nil {
			return doc, e
		}
		if e = resourceAudience(ctx, tx, a, c, linked, members); e != nil {
			return doc, e
		}
	}
	return doc, nil
}

func mutationTargets(m *mutationRecord, kind, id string) bool {
	if m == nil {
		return false
	}
	if m.ResourceType == kind && m.ResourceID == id {
		return true
	}
	if m.Tool == "create_task" && m.OperationID != nil && kind == "task" {
		var created project.Task
		return taskCreationReceipt(*m, m.Result) && json.Unmarshal(m.Result, &created) == nil && created.ID == id
	}
	if concernCreationTool(m.Tool) && m.OperationID != nil && kind == strings.TrimPrefix(m.Tool, "create_") {
		var created project.Concern
		return json.Unmarshal(m.Result, &created) == nil && created.ID == id && created.ProjectID == m.ProjectID
	}
	if kind == "issue" && mutationRealizedIssue(m) == id {
		return true
	}
	if m.Tool == "create_milestone" && m.OperationID != nil && kind == "milestone" {
		var created project.Milestone
		return json.Unmarshal(m.Result, &created) == nil && created.ID == id && created.ProjectID == m.ProjectID
	}
	if m.Tool == "create_deliverable" && m.OperationID != nil && kind == "deliverable" {
		var created project.DeliverableReceipt
		return json.Unmarshal(m.Result, &created) == nil && created.ID == id && created.Version == 1 && created.RevisionID == nil
	}
	return false
}

func prepareMutation(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, runID string, in MutationInput, members []string) (*mutationRecord, error) {
	base, kind, args, err := parseMutation(in)
	if err != nil {
		return nil, err
	}
	if c.ProjectID == nil {
		return nil, core.Invalid("mutation", "请在对应的项目会话中发送准确更新")
	}
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return nil, err
	}
	// Capture current metadata and the display projection under the same locks.
	// Existing business commands lock project before task in this same order.
	var locked string
	if err = tx.QueryRow(ctx, "SELECT id FROM projects WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, *c.ProjectID).Scan(&locked); err != nil {
		return nil, platform.DatabaseError(err)
	}
	if kind == "task" {
		if err = tx.QueryRow(ctx, "SELECT id FROM tasks WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, base.ID).Scan(&locked); err != nil {
			return nil, platform.DatabaseError(err)
		}
	}
	if kind == "milestone" {
		if err = tx.QueryRow(ctx, "SELECT id FROM milestones WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, base.ID).Scan(&locked); err != nil {
			return nil, platform.DatabaseError(err)
		}
	}
	if kind == "deliverable" {
		if err = tx.QueryRow(ctx, "SELECT id FROM deliverables WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, base.ID).Scan(&locked); err != nil {
			return nil, platform.DatabaseError(err)
		}
	}
	if kind == "risk" || kind == "issue" {
		if err = tx.QueryRow(ctx, "SELECT id FROM "+kind+"s WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, base.ID).Scan(&locked); err != nil {
			return nil, platform.DatabaseError(err)
		}
	}
	ai := a
	ai.RunID = runID
	m := mutationRecord{MutationView: MutationView{RunID: runID, Tool: in.Tool, ResourceType: kind, ResourceID: base.ID, BaseVersion: base.ExpectedVersion, Arguments: args, Status: "authorized"}, OrgID: a.OrgID, ActorID: a.UserID, ConversationID: c.ID, ProjectID: *c.ProjectID}
	m.SessionID = a.SessionID
	doc, err := mutationAudience(ctx, tx, ai, c, m, members)
	if err != nil {
		return nil, err
	}
	if doc.ResourceVersion != base.ExpectedVersion {
		return nil, core.Conflict()
	}
	if err = authorizeMutation(ctx, tx, ai, doc, m); err != nil {
		return nil, err
	}
	m.BaseSHA256, err = documentDigest(doc)
	if err != nil {
		return nil, err
	}
	m.Before, err = mutationBefore(ctx, tx, a, kind, base.ID)
	if err != nil {
		return nil, err
	}
	intent, err := json.Marshal(map[string]any{"run_id": runID, "actor_id": a.UserID, "actor_auth_version": a.AuthVersion, "organization_auth_version": a.OrgAuthVersion,
		"conversation_id": c.ID, "conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "project_id": m.ProjectID,
		"tool": m.Tool, "resource_type": kind, "resource_id": m.ResourceID, "base_version": m.BaseVersion, "base_sha256": m.BaseSHA256, "arguments": args, "before": m.Before, "session_id": m.SessionID})
	if err != nil {
		return nil, err
	}
	if len(intent) > 100000 {
		return nil, core.Invalid("mutation", "更新与基础资料过大，请缩小本次修改范围")
	}
	m.Intent = string(intent)
	m.IntentSHA256 = core.Digest(intent)
	return &m, nil
}

func authorizeMutation(ctx context.Context, tx pgx.Tx, a core.Actor, doc contextDocument, m mutationRecord) error {
	action := m.ResourceType + ".update"
	if m.Tool == "update_deliverable" {
		action = "deliverable.create"
	}
	if m.Tool == "create_task" || m.Tool == "create_milestone" || m.Tool == "create_deliverable" || concernCreationTool(m.Tool) {
		// Authorize against the parent on every attempt, including replay. A
		// created child is read separately with the whole current audience.
		parent, err := contextDocumentRead(ctx, tx, a, Conversation{ProjectID: &m.ProjectID}, ContextSource{Type: "project", ID: m.ProjectID}, nil)
		if err != nil {
			return err
		}
		doc, action = parent, "milestone.create"
		if m.Tool == "create_task" {
			action = "task.create"
		}
		if m.Tool == "create_deliverable" {
			action = "deliverable.create"
		}
		if concernCreationTool(m.Tool) {
			action = strings.TrimPrefix(m.Tool, "create_") + ".create"
		}
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(m.Arguments, &args); err != nil {
		return err
	}
	if m.Tool == "transition_task" {
		action = "task.transition"
		var name string
		_ = json.Unmarshal(args["action"], &name)
		if name == "reopen" {
			action = "task.reopen"
		}
		// A secretary may request review, but cannot be the human accepting work.
		if name == "complete" {
			var required bool
			if err := tx.QueryRow(ctx, "SELECT review_required FROM tasks WHERE org_id=$1 AND id=$2", a.OrgID, m.ResourceID).Scan(&required); err != nil {
				return err
			}
			if required {
				return core.Fail(422, "TASK_HUMAN_REVIEW_REQUIRED", "此任务需要人工验收，请由人员在任务页面核对完成")
			}
		}
	}
	if m.Tool == "transition_project" {
		var name string
		_ = json.Unmarshal(args["action"], &name)
		if name == "reopen" {
			action = "project.reopen"
		}
	}
	if m.Tool == "update_risk" {
		var state string
		_ = json.Unmarshal(args["state"], &state)
		if state == "realized" {
			parentDoc, e := contextDocumentRead(ctx, tx, a, Conversation{ProjectID: &m.ProjectID}, ContextSource{Type: "project", ID: m.ProjectID}, nil)
			if e != nil {
				return e
			}
			parent := authz.Resource{Type: "project", ID: m.ProjectID, OwnerID: &parentDoc.Creator, ProjectID: &m.ProjectID, Classification: parentDoc.Class}
			if err := authz.Require(ctx, tx, a, "issue.create", parent); err != nil {
				return err
			}
		}
	}
	resource := authz.Resource{Type: doc.Type, ID: doc.ID, OwnerID: &doc.Creator, ProjectID: doc.ProjectID, Classification: doc.Class}
	if err := authz.Require(ctx, tx, a, action, resource); err != nil {
		return err
	}
	if _, ok := args["assignee_id"]; ok {
		if m.Tool == "create_task" {
			var assignee *string
			_ = json.Unmarshal(args["assignee_id"], &assignee)
			if assignee == nil || *assignee == a.UserID {
				return nil
			}
		}
		return authz.Require(ctx, tx, a, "task.assign", resource)
	}
	return nil
}

func insertMutation(ctx context.Context, tx pgx.Tx, m *mutationRecord) error {
	_, err := tx.Exec(ctx, `INSERT INTO agent_run_mutations(run_id,org_id,actor_id,conversation_id,project_id,tool,resource_type,resource_id,base_version,base_sha256,arguments,before_data,intent,intent_sha256,session_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, m.RunID, m.OrgID, m.ActorID, m.ConversationID, m.ProjectID, m.Tool, m.ResourceType, m.ResourceID, m.BaseVersion, m.BaseSHA256, m.Arguments, m.Before, m.Intent, m.IntentSHA256, m.SessionID)
	return err
}

func mutationSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string, members []string) (SourceSnapshot, error) {
	m, err := mutationRecordRead(ctx, tx, a.OrgID, id)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if m == nil {
		return SourceSnapshot{}, core.Missing()
	}
	var audience []string
	if err = tx.QueryRow(ctx, "SELECT audience_ids FROM agent_runs WHERE org_id=$1 AND id=$2", a.OrgID, id).Scan(&audience); err != nil {
		return SourceSnapshot{}, err
	}
	if !slices.Contains(audience, a.UserID) {
		return SourceSnapshot{}, core.Missing()
	}
	// Fixed human instructions are conversation history. A later model turn
	// cannot disclose an old instruction to people outside its original audience.
	for _, user := range members {
		if !slices.Contains(audience, user) {
			return SourceSnapshot{}, core.Missing()
		}
	}
	if _, err = mutationAudience(ctx, tx, a, c, *m, members); err != nil {
		return SourceSnapshot{}, err
	}
	if core.Digest([]byte(m.Intent)) != m.IntentSHA256 {
		return SourceSnapshot{}, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_mutation", ID: id}, ResourceVersion: 1, SHA256: m.IntentSHA256}, nil
}

// An immutable instruction remains readable after later business edits. During
// execution, however, only its exact base or its own confirmed result is valid.
func executionMutationState(ctx context.Context, tx pgx.Tx, st executionState) (*mutationRecord, error) {
	m, err := mutationRecordRead(ctx, tx, st.Actor.OrgID, st.Run.ID)
	if err != nil || m == nil {
		return m, err
	}
	if st.Profile.SkillID != "project-update" || m.ActorID != st.Actor.UserID {
		return nil, runtimeFailure("MUTATION_AUTHORITY_MISMATCH")
	}
	human := st.Actor
	human.SessionID = m.SessionID
	if err = identity.RequireInteractiveSession(ctx, tx, human, false); err != nil {
		return nil, err
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	doc, err := mutationAudience(ctx, tx, ai, st.Conversation, *m, st.Run.audience)
	if err != nil {
		return nil, err
	}
	hash, err := documentDigest(doc)
	if err != nil {
		return nil, err
	}
	version, want := m.BaseVersion, m.BaseSHA256
	if m.ResultVersion != nil {
		version, want = *m.ResultVersion, *m.ResultSHA256
	}
	if version != doc.ResourceVersion || hash != want {
		return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	if child := mutationRealizedIssue(m); child != "" {
		linked, e := contextDocumentRead(ctx, tx, ai, st.Conversation, ContextSource{Type: "issue", ID: child}, st.Run.audience)
		if e != nil {
			return nil, e
		}
		if linked.ResourceVersion != 1 {
			return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
	}
	if err = authorizeMutation(ctx, tx, ai, doc, *m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Service) executionMutation(ctx context.Context, tx pgx.Tx, st executionState, tool string, args json.RawMessage) (map[string]any, []SourceSnapshot, error) {
	m, err := executionMutationState(ctx, tx, st)
	if err != nil {
		return nil, nil, err
	}
	if m == nil {
		return nil, nil, runtimeFailure("MUTATION_INTENT_REQUIRED")
	}
	_, _, canonical, err := parseMutation(MutationInput{Tool: tool, Arguments: args})
	if err != nil {
		return nil, nil, err
	}
	expected, _ := core.CanonicalJSON(m.Arguments)
	if tool != m.Tool || string(canonical) != string(expected) {
		return nil, nil, runtimeFailure("MUTATION_INTENT_MISMATCH")
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	if m.OperationID == nil {
		command := core.Command{Actor: ai, RequestID: core.NewID(), Route: "/internal/agent-mutations/" + tool, Key: "agent-mutation:" + st.Run.ID, Body: canonical}
		business := project.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
		// Remove routing/reason fields without reserializing nullable patch structs.
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(canonical, &fields)
		delete(fields, "id")
		delete(fields, "reason")
		body, _ := json.Marshal(fields)
		var result core.Result
		switch tool {
		case "create_task":
			var input project.TaskCreate
			_ = json.Unmarshal(body, &input)
			result, err = business.CreateTask(ctx, command, m.ProjectID, input)
		case "create_risk", "create_issue":
			var input project.ConcernCreate
			_ = json.Unmarshal(body, &input)
			result, err = business.CreateConcern(ctx, command, m.ProjectID, strings.TrimPrefix(tool, "create_"), input)
		case "update_risk", "update_issue":
			var input project.ConcernPatch
			_ = json.Unmarshal(canonical, &input)
			result, err = business.UpdateConcern(ctx, command, m.ResourceID, strings.TrimPrefix(tool, "update_"), input)
		case "create_deliverable":
			var input project.DeliverableCreate
			_ = json.Unmarshal(canonical, &input)
			result, err = business.CreateDeliverable(ctx, command, m.ProjectID, input)
		case "update_deliverable":
			var input project.DeliverablePatch
			_ = json.Unmarshal(canonical, &input)
			result, err = business.UpdateDeliverable(ctx, command, m.ResourceID, input)
		case "create_milestone":
			var input project.MilestoneCreate
			_ = json.Unmarshal(body, &input)
			result, err = business.CreateMilestone(ctx, command, m.ProjectID, input)
		case "update_project":
			var patch project.Patch
			_ = json.Unmarshal(body, &patch)
			result, err = business.Update(ctx, command, m.ResourceID, patch)
		case "update_task":
			var patch project.TaskPatch
			_ = json.Unmarshal(body, &patch)
			result, err = business.UpdateTask(ctx, command, m.ResourceID, patch)
		case "transition_task":
			var transition project.Transition
			_ = json.Unmarshal(canonical, &transition)
			result, err = business.TransitionTask(ctx, command, m.ResourceID, transition)
		case "transition_project":
			var transition project.Transition
			_ = json.Unmarshal(canonical, &transition)
			result, err = business.Transition(ctx, command, m.ResourceID, transition)
		case "update_milestone":
			var patch project.MilestonePatch
			_ = json.Unmarshal(body, &patch)
			result, err = business.UpdateMilestone(ctx, command, m.ResourceID, patch)
		case "transition_milestone":
			var transition project.Transition
			_ = json.Unmarshal(canonical, &transition)
			result, err = business.TransitionMilestone(ctx, command, m.ResourceID, transition)
		}
		if err != nil {
			return nil, nil, err
		}
		raw, e := json.Marshal(result.Data)
		if e != nil {
			return nil, nil, e
		}
		var receipt struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		}
		validReceipt := json.Unmarshal(raw, &receipt) == nil && receipt.ID == m.ResourceID && receipt.Version == m.BaseVersion+1
		if tool == "create_task" {
			validReceipt = taskCreationReceipt(*m, raw)
			m.OperationID, m.Result = &result.OperationID, raw
		}
		if tool == "create_milestone" {
			var created project.Milestone
			validReceipt = json.Unmarshal(raw, &created) == nil && core.ValidID(created.ID) && created.ID != m.ResourceID && created.ProjectID == m.ProjectID && created.Version == 1 && created.State == "open"
			m.OperationID, m.Result = &result.OperationID, raw
		}
		if tool == "create_deliverable" {
			var created project.DeliverableReceipt
			validReceipt = json.Unmarshal(raw, &created) == nil && core.ValidID(created.ID) && created.ID != m.ResourceID && created.Version == 1 && created.RevisionID == nil
			m.OperationID, m.Result = &result.OperationID, raw
		}
		if concernMutationTool(tool) {
			validReceipt = concernMutationReceipt(*m, raw)
		}
		if !validReceipt || !core.ValidID(result.OperationID) {
			return nil, nil, runtimeFailure("MUTATION_RECEIPT_INVALID")
		}
		m.OperationID, m.Result = &result.OperationID, raw
		doc, e := mutationAudience(ctx, tx, ai, st.Conversation, *m, st.Run.audience)
		if e != nil {
			return nil, nil, e
		}
		hash, e := documentDigest(doc)
		if e != nil {
			return nil, nil, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO agent_mutation_commits(run_id,org_id,operation_id,result_version,result_sha256,result_data) VALUES($1,$2,$3,$4,$5,$6)`, st.Run.ID, ai.OrgID, result.OperationID, receipt.Version, hash, raw); e != nil {
			return nil, nil, e
		}
		m, err = mutationRecordRead(ctx, tx, ai.OrgID, st.Run.ID)
		if err != nil {
			return nil, nil, err
		}
	}
	src, err := mutationSource(ctx, tx, ai, st.Conversation, m.RunID, st.Run.audience)
	if err != nil {
		return nil, nil, err
	}
	kind, id := m.ResourceType, m.ResourceID
	if tool == "create_task" {
		var created project.Task
		_ = json.Unmarshal(m.Result, &created)
		kind, id = "task", created.ID
	}
	if concernCreationTool(tool) {
		var created project.Concern
		_ = json.Unmarshal(m.Result, &created)
		kind, id = strings.TrimPrefix(tool, "create_"), created.ID
	}
	if tool == "create_milestone" {
		var created project.Milestone
		_ = json.Unmarshal(m.Result, &created)
		kind, id = "milestone", created.ID
	}
	if tool == "create_deliverable" {
		var created project.DeliverableReceipt
		_ = json.Unmarshal(m.Result, &created)
		kind, id = "deliverable", created.ID
	}
	refs := []ResultReference{{Type: kind, ID: id}}
	if child := mutationRealizedIssue(m); child != "" {
		refs = append(refs, ResultReference{Type: "issue", ID: child})
	}
	return map[string]any{"status": "succeeded", "resource_refs": refs, "source_refs": []ResultReference{reference(src)}, "operation_id": m.OperationID, "committed": true,
		"safe_summary": "已按发送时明确指定的对象、基础版本和字段完成更新", "retryable": false, "next_action": "", "content": m.MutationView}, []SourceSnapshot{src}, nil
}

func executionMutationText(ctx context.Context, tx pgx.Tx, st executionState, id string, version *string, offset, maxChars int) (any, []SourceSnapshot, error) {
	if !core.ValidID(id) || version != nil || offset < 0 || offset > 200000 || maxChars < 1 || maxChars > 10000 {
		return nil, nil, core.Invalid("arguments", "请指定准确更新记录和有限字符范围")
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	src, err := mutationSource(ctx, tx, ai, st.Conversation, id, st.Run.audience)
	if err != nil {
		return nil, nil, err
	}
	m, err := mutationRecordRead(ctx, tx, ai.OrgID, id)
	if err != nil {
		return nil, nil, err
	}
	// The signed intent contains the originating login; expose only the public
	// fixed instruction and business result to the model and conversation readers.
	raw, _ := json.Marshal(m.MutationView)
	body := []rune(string(raw))
	if offset > len(body) {
		return nil, nil, core.Invalid("offset", "字符范围无效")
	}
	end := min(offset+maxChars, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	return map[string]any{"type": "agent_mutation", "id": id, "resource_version": 1, "source_sha256": src.SHA256, "text": string(body[offset:end]), "offset": offset, "next_offset": next, "total_chars": len(body), "status": m.Status, "operation_id": m.OperationID, "untrusted_data": true}, []SourceSnapshot{src}, nil
}
