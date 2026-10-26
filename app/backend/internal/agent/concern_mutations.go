package agent

import (
	"encoding/json"
	"slices"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
)

func concernMutationTool(tool string) bool {
	return slices.Contains([]string{"create_risk", "update_risk", "create_issue", "update_issue"}, tool)
}

func concernCreationTool(tool string) bool { return tool == "create_risk" || tool == "create_issue" }

// Complete metadata is fixed by the employee. An omitted state preserves the
// current state; a supplied state authorizes precisely that original transition.
func parseConcernMutation(tool string, base mutationArguments, canonical json.RawMessage, fields map[string]json.RawMessage) (mutationArguments, string, json.RawMessage, error) {
	kind := "issue"
	if strings.HasSuffix(tool, "_risk") {
		kind = "risk"
	}
	required := []string{"id", "expected_version", "reason", "title", "description", "owner_id", "severity", "due_at"}
	if kind == "risk" {
		required = append(required, "probability", "mitigation")
	} else {
		required = append(required, "resolution")
	}
	allowed := slices.Clone(required)
	if !concernCreationTool(tool) {
		allowed = append(allowed, "state")
	}
	for _, field := range required {
		if _, ok := fields[field]; !ok {
			return base, "", nil, core.Invalid("mutation", "请完整核对事项资料，并明确截止时刻和风险概率是否为空")
		}
	}
	for field, value := range fields {
		if !slices.Contains(allowed, field) || string(value) == "null" && field != "due_at" && field != "probability" {
			return base, "", nil, core.Invalid("mutation", "事项包含未授权字段或不允许的空值")
		}
	}
	var input struct {
		mutationArguments
		project.ConcernCreate
		State *string `json:"state,omitempty"`
	}
	if strictArguments(canonical, &input) != nil || core.Name(input.Title) != nil || !exactOAID(input.OwnerID) ||
		!slices.Contains([]string{"low", "medium", "high", "critical"}, input.Severity) ||
		len([]rune(input.Description)) > 20000 || len([]rune(input.Mitigation)) > 20000 || len([]rune(input.Resolution)) > 20000 ||
		input.DueAt != nil && (input.DueAt.IsZero() || input.DueAt.Nanosecond()%1000 != 0) || input.Probability != nil && !slices.Contains([]string{"low", "medium", "high"}, *input.Probability) {
		return base, "", nil, core.Invalid("mutation", "请核对事项标题、完整说明、实际成员、严重程度、时区及风险概率")
	}
	if input.State != nil {
		states := []string{"open", "investigating", "resolved", "closed"}
		if kind == "risk" {
			states = []string{"open", "mitigating", "closed", "realized"}
		}
		if !slices.Contains(states, *input.State) {
			return base, "", nil, core.Invalid("state", "请核对原业务支持的事项状态")
		}
	}
	if concernCreationTool(tool) {
		kind = "project"
	}
	return base, kind, canonical, nil
}

func mutationRealizedIssue(m *mutationRecord) string {
	if m == nil || m.Tool != "update_risk" || m.OperationID == nil {
		return ""
	}
	var result project.Concern
	if json.Unmarshal(m.Result, &result) != nil || result.State != "realized" || result.IssueID == nil || !exactOAID(*result.IssueID) {
		return ""
	}
	return *result.IssueID
}

func concernMutationReceipt(m mutationRecord, raw json.RawMessage) bool {
	var result, before project.Concern
	var input struct {
		project.ConcernCreate
		State *string `json:"state,omitempty"`
	}
	if json.Unmarshal(raw, &result) != nil || json.Unmarshal(m.Arguments, &input) != nil || !exactOAID(result.ID) || result.ProjectID != m.ProjectID {
		return false
	}
	state := "open"
	if concernCreationTool(m.Tool) {
		if result.ID == m.ResourceID || result.Version != 1 || result.CreatorID != m.ActorID {
			return false
		}
	} else {
		if json.Unmarshal(m.Before, &before) != nil || result.ID != m.ResourceID || result.Version != m.BaseVersion+1 || result.CreatorID != before.CreatorID {
			return false
		}
		state = before.State
		if input.State != nil {
			state = *input.State
		}
	}
	if result.State != state || result.Title != strings.TrimSpace(input.Title) || result.Description != input.Description || result.OwnerID != input.OwnerID || result.Severity != input.Severity ||
		(result.DueAt == nil) != (input.DueAt == nil) || result.DueAt != nil && !result.DueAt.Equal(*input.DueAt) {
		return false
	}
	if strings.HasSuffix(m.Tool, "_risk") {
		if result.Mitigation != input.Mitigation || (result.Probability == nil) != (input.Probability == nil) || result.Probability != nil && *result.Probability != *input.Probability {
			return false
		}
		if state == "realized" {
			return result.IssueID != nil && exactOAID(*result.IssueID) && *result.IssueID != result.ID && *result.IssueID != m.ProjectID && before.IssueID == nil
		}
		return result.IssueID == nil
	}
	return result.Resolution == input.Resolution && result.IssueID == nil
}
