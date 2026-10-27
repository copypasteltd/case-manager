package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"github.com/jackc/pgx/v5"
)

// Creation is complete human intent, never a patch with inferred defaults.
func parseTaskCreation(base mutationArguments, canonical json.RawMessage, fields map[string]json.RawMessage) (mutationArguments, string, json.RawMessage, error) {
	required := []string{"id", "expected_version", "reason", "title", "body", "assignee_id", "parent_id", "milestone_id", "start_at", "due_at", "priority", "review_required"}
	for _, key := range required {
		if _, present := fields[key]; !present {
			return base, "", nil, core.Invalid("mutation", "创建任务须完整核对九项资料；负责人、关联和日期须明确设置或不设置")
		}
	}
	for key, value := range fields {
		if !slices.Contains(required, key) || string(value) == "null" && !slices.Contains([]string{"assignee_id", "parent_id", "milestone_id", "start_at", "due_at"}, key) {
			return base, "", nil, core.Invalid("mutation", "任务创建包含未授权字段或不允许的空值")
		}
	}
	var input struct {
		mutationArguments
		project.TaskCreate
	}
	if strictArguments(canonical, &input) != nil || core.Name(input.Title) != nil || len([]rune(input.Body)) > 20000 ||
		!slices.Contains([]string{"low", "normal", "high", "urgent"}, input.Priority) {
		return base, "", nil, core.Invalid("mutation", "请核对完整任务标题、说明、优先级和明确的人工验收要求")
	}
	for _, id := range []*string{input.AssigneeID, input.ParentID, input.MilestoneID} {
		if id != nil && !exactOAID(*id) {
			return base, "", nil, core.Invalid("reference", "请选择实际项目成员、同项目父任务和里程碑，或明确不设置")
		}
	}
	for _, instant := range []*time.Time{input.StartAt, input.DueAt} {
		if instant != nil && (instant.IsZero() || instant.Nanosecond()%1000 != 0) {
			return base, "", nil, core.Invalid("date", "任务时刻须含时区，最多保留六位有效小数")
		}
	}
	if input.StartAt != nil && input.DueAt != nil && input.DueAt.Before(*input.StartAt) {
		return base, "", nil, core.Invalid("due_at", "截止时刻不能早于开始时刻")
	}
	return base, "project", canonical, nil
}

// Referenced objects stay subject to the entire current conversation audience.
// The original business command checks active membership and writable links.
func taskCreationAudience(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, m mutationRecord, members []string) error {
	if m.Tool != "create_task" {
		return nil
	}
	var in project.TaskCreate
	if err := json.Unmarshal(m.Arguments, &in); err != nil {
		return err
	}
	for _, ref := range []struct {
		kind string
		id   *string
	}{{"task", in.ParentID}, {"milestone", in.MilestoneID}} {
		if ref.id == nil {
			continue
		}
		doc, err := contextDocumentRead(ctx, tx, a, c, ContextSource{Type: ref.kind, ID: *ref.id}, members)
		if err != nil {
			return err
		}
		if err = resourceAudience(ctx, tx, a, c, doc, members); err != nil {
			return err
		}
	}
	return nil
}

func taskCreationReceipt(m mutationRecord, raw json.RawMessage) bool {
	var actual project.Task
	var in project.TaskCreate
	if json.Unmarshal(raw, &actual) != nil || json.Unmarshal(m.Arguments, &in) != nil || !exactOAID(actual.ID) || actual.ID == m.ResourceID ||
		actual.ProjectID != m.ProjectID || actual.Version != 1 || actual.State != "todo" || actual.CreatorID != m.ActorID ||
		actual.Title != strings.TrimSpace(in.Title) || actual.Body != in.Body || actual.Priority != in.Priority || actual.ReviewRequired != in.ReviewRequired {
		return false
	}
	for _, pair := range [][2]*string{{actual.AssigneeID, in.AssigneeID}, {actual.ParentID, in.ParentID}, {actual.MilestoneID, in.MilestoneID}} {
		if (pair[0] == nil) != (pair[1] == nil) || pair[0] != nil && *pair[0] != *pair[1] {
			return false
		}
	}
	for _, pair := range [][2]*time.Time{{actual.StartAt, in.StartAt}, {actual.DueAt, in.DueAt}} {
		if (pair[0] == nil) != (pair[1] == nil) || pair[0] != nil && !pair[0].Equal(*pair[1]) {
			return false
		}
	}
	return true
}
