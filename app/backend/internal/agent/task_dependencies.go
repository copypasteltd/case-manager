package agent

import (
	"context"
	"encoding/json"
	"slices"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"github.com/jackc/pgx/v5"
)

func parseTaskDependency(base mutationArguments, canonical json.RawMessage, fields map[string]json.RawMessage) (mutationArguments, string, json.RawMessage, error) {
	required := []string{"id", "expected_version", "reason", "predecessor_id", "expected_predecessor_version", "action"}
	var in struct {
		mutationArguments
		project.ExactDependency
	}
	if len(fields) != len(required) || strictArguments(canonical, &in) != nil {
		return base, "", nil, core.Invalid("mutation", "请完整核对后续任务、前置任务的版本、依赖动作和原因")
	}
	for key, value := range fields {
		if !slices.Contains(required, key) || string(value) == "null" {
			return base, "", nil, core.Invalid("mutation", "依赖更新不得省略、清空或增加字段")
		}
	}
	if !exactOAID(in.PredecessorID) || in.PredecessorID == base.ID || core.RequireVersion(in.ExpectedPredecessorVersion) != nil || !slices.Contains([]string{"add", "remove"}, in.Action) {
		return base, "", nil, core.Invalid("mutation", "请选择不同的同项目任务、准确版本及新增或解除动作")
	}
	return base, "task", canonical, nil
}

func taskDependencyAudience(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, m mutationRecord, members []string) error {
	if m.Tool != "update_task_dependency" {
		return nil
	}
	var in project.ExactDependency
	if err := json.Unmarshal(m.Arguments, &in); err != nil {
		return err
	}
	doc, err := contextDocumentRead(ctx, tx, a, c, ContextSource{Type: "task", ID: in.PredecessorID}, members)
	if err != nil {
		return err
	}
	return resourceAudience(ctx, tx, a, c, doc, members)
}

func dependencyPresent(ctx context.Context, tx pgx.Tx, org, successor, predecessor string) (bool, error) {
	var present bool
	err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM task_dependencies WHERE org_id=$1 AND successor_id=$2 AND predecessor_id=$3)", org, successor, predecessor).Scan(&present)
	return present, err
}

func prepareTaskDependency(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, m *mutationRecord) error {
	if m.Tool != "update_task_dependency" {
		return nil
	}
	var in project.ExactDependency
	_ = json.Unmarshal(m.Arguments, &in)
	var locked string
	if err := tx.QueryRow(ctx, "SELECT id FROM tasks WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, in.PredecessorID).Scan(&locked); err != nil {
		return err
	}
	doc, err := contextDocumentRead(ctx, tx, a, c, ContextSource{Type: "task", ID: in.PredecessorID}, nil)
	if err != nil {
		return err
	}
	if doc.ResourceVersion != in.ExpectedPredecessorVersion {
		return core.Conflict()
	}
	var taskState, projectState string
	if err = tx.QueryRow(ctx, "SELECT t.state,p.state FROM tasks t JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id WHERE t.org_id=$1 AND t.id=$2 AND p.id=$3", a.OrgID, m.ResourceID, m.ProjectID).Scan(&taskState, &projectState); err != nil {
		return err
	}
	if slices.Contains([]string{"archived", "cancelled", "closing"}, projectState) {
		return core.Fail(409, "PROJECT_READ_ONLY", "当前项目阶段不允许普通业务写入")
	}
	if slices.Contains([]string{"done", "cancelled"}, taskState) {
		return core.Fail(409, "TASK_READ_ONLY", "请重开任务后修改依赖")
	}
	present, err := dependencyPresent(ctx, tx, a.OrgID, m.ResourceID, in.PredecessorID)
	if err != nil {
		return err
	}
	if present == (in.Action == "add") {
		return core.Fail(409, "DEPENDENCY_UNCHANGED", "请重新核对当前依赖，本次操作不会改变关系")
	}
	hash, err := documentDigest(doc)
	if err != nil {
		return err
	}
	var before map[string]any
	_ = json.Unmarshal(m.Before, &before)
	var title, state string
	if err = tx.QueryRow(ctx, "SELECT title,state FROM tasks WHERE org_id=$1 AND id=$2", a.OrgID, in.PredecessorID).Scan(&title, &state); err != nil {
		return err
	}
	before["predecessor_id"], before["predecessor_version"], before["predecessor_title"], before["predecessor_state"] = in.PredecessorID, in.ExpectedPredecessorVersion, title, state
	before["predecessor_sha256"], before["dependency_present"] = hash, present
	before["project_id"] = m.ProjectID
	m.Before, err = json.Marshal(before)
	return err
}

func executionTaskDependencyState(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, m mutationRecord) error {
	if m.Tool != "update_task_dependency" {
		return nil
	}
	var in project.ExactDependency
	var before struct {
		SHA256  string `json:"predecessor_sha256"`
		Present bool   `json:"dependency_present"`
	}
	_ = json.Unmarshal(m.Arguments, &in)
	_ = json.Unmarshal(m.Before, &before)
	doc, err := contextDocumentRead(ctx, tx, a, c, ContextSource{Type: "task", ID: in.PredecessorID}, nil)
	if err != nil {
		return err
	}
	hash, err := documentDigest(doc)
	if err != nil {
		return err
	}
	present, err := dependencyPresent(ctx, tx, a.OrgID, m.ResourceID, in.PredecessorID)
	if err != nil {
		return err
	}
	wanted := before.Present
	if m.OperationID != nil {
		wanted = in.Action == "add"
	}
	if doc.ResourceVersion != in.ExpectedPredecessorVersion || hash != before.SHA256 || present != wanted {
		return runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return nil
}

func taskDependencyReceipt(m mutationRecord, raw json.RawMessage) bool {
	var in project.ExactDependency
	var actual project.DependencyReceipt
	if json.Unmarshal(m.Arguments, &in) != nil || json.Unmarshal(raw, &actual) != nil {
		return false
	}
	return actual.ID == m.ResourceID && actual.ProjectID == m.ProjectID && actual.Version == m.BaseVersion+1 && actual.PredecessorID == in.PredecessorID && actual.PredecessorVersion == in.ExpectedPredecessorVersion && actual.DependencyAction == in.Action && actual.DependencyPresent == (in.Action == "add")
}
