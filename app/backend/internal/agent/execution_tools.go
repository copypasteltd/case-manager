package agent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"github.com/jackc/pgx/v5"
)

type ResultReference struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	VersionID *string `json:"version_id,omitempty"`
}
type ExecutionReceipt struct {
	RunID          string            `json:"run_id"`
	AuthorityStamp string            `json:"authority_stamp"`
	Tool           string            `json:"tool"`
	OperationID    *string           `json:"operation_id"`
	Status         string            `json:"status"`
	Committed      bool              `json:"committed"`
	ResourceRefs   []ResultReference `json:"resource_refs"`
	SourceRefs     []ResultReference `json:"source_refs"`
}

func reference(s SourceSnapshot) ResultReference {
	return ResultReference{Type: s.Type, ID: s.ID, VersionID: s.VersionID}
}
func allowedReadTool(skill, tool string) bool {
	if !slices.Contains([]string{"project-create", "project-update", "file-update", "information-organize", "oa-assist", "subscription-review", "project-briefing", "incident-summary"}, skill) {
		return false
	}
	if skill == "project-create" && (tool == "propose_project" || tool == "create_project") {
		return true
	}
	if skill == "project-create" && tool == "list_project_catalog" {
		return true
	}
	if skill == "project-update" && mutationTool(tool) {
		return true
	}
	if skill == "file-update" && fileUpdateTool(tool) {
		return true
	}
	if skill == "information-organize" && organizationTool(tool) {
		return true
	}
	if skill == "oa-assist" && oaRequestTool(tool) {
		return true
	}
	if skill == "project-briefing" && postWriteTool(tool) {
		return true
	}
	if skill == "subscription-review" && slices.Contains([]string{"read_finance_summary", "list_subscriptions", "read_subscription_review", "read_subscription_portfolio", "read_finance_record"}, tool) {
		return true
	}
	if skill == "incident-summary" && tool == "read_service_summary" {
		return true
	}
	return tool == "read_resource" || tool == "inspect_operation" || (tool == "search_resources" && (skill == "project-briefing" || skill == "project-create" || skill == "incident-summary"))
}
func allowedExecutionTool(st executionState, tool string) bool {
	if st.Actor.Channel == "telegram" && !slices.Contains([]string{"read_resource", "search_resources", "inspect_operation", "read_service_summary"}, tool) {
		return false
	}
	return allowedReadTool(st.Profile.SkillID, tool)
}
func strictArguments(raw json.RawMessage, out any) error {
	if len(raw) > 65536 {
		return core.Invalid("arguments", "工具参数过大")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return core.Invalid("arguments", "工具参数无效或含未允许字段")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return core.Invalid("arguments", "工具只接受一个参数对象")
	}
	return nil
}
func (s *Service) ExecutionTool(ctx context.Context, lease ExecutionLease, key, tool string, args json.RawMessage) (json.RawMessage, error) {
	return s.executionTool(ctx, lease, key, tool, args, nil)
}
func (s *Service) executionTool(ctx context.Context, lease ExecutionLease, key, tool string, args json.RawMessage, work *executionIO) (json.RawMessage, error) {
	if len(key) < 1 || len(key) > 200 || strings.TrimSpace(key) != key {
		return nil, core.Invalid("invocation_key", "请指定准确工具调用标识")
	}
	tx, st, err := s.executionTx(ctx, lease, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if st.Run.State != "running" || !allowedExecutionTool(st, tool) {
		return nil, runtimeFailure("SKILL_TOOL_DENIED")
	}
	canonical, err := core.CanonicalJSON(args)
	if err != nil {
		return nil, core.Invalid("arguments", "工具参数不是JSON")
	}
	inputHash := core.Digest(append([]byte(tool+"\n"), canonical...))
	var oldHash string
	var oldOutput, oldSources []byte
	err = tx.QueryRow(ctx, "SELECT input_sha256,output,source_snapshots FROM agent_tool_receipts WHERE org_id=$1 AND run_id=$2 AND invocation_key=$3", lease.OrgID, lease.RunID, key).Scan(&oldHash, &oldOutput, &oldSources)
	if err == nil {
		if oldHash != inputHash {
			return nil, core.Fail(409, "TOOL_INVOCATION_CONFLICT", "同一工具调用标识不能更改参数")
		}
		var snapshots []SourceSnapshot
		if err = json.Unmarshal(oldSources, &snapshots); err != nil {
			return nil, err
		}
		if err = executionSources(ctx, tx, st, snapshots); err != nil {
			return nil, err
		}
		return oldOutput, tx.Commit(ctx)
	}
	if err != pgx.ErrNoRows {
		return nil, err
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM agent_tool_receipts WHERE org_id=$1 AND run_id=$2", lease.OrgID, lease.RunID).Scan(&count); err != nil {
		return nil, err
	}
	if count >= st.Profile.MaxToolCalls {
		return nil, runtimeFailure("RUNTIME_TOOL_BUDGET_EXCEEDED")
	}
	_, observedSources, err := readExecutionReceipts(ctx, tx, st, lease)
	if err != nil {
		return nil, err
	}
	var content any
	var snapshots []SourceSnapshot
	var writeOutput map[string]any
	switch tool {
	case "list_project_catalog":
		content, snapshots, err = s.executionProjectCatalog(ctx, tx, st, lease, args)
	case "read_finance_summary":
		content, snapshots, err = executionFinanceSummary(ctx, tx, st, args)
	case "list_subscriptions", "read_subscription_review", "read_subscription_portfolio", "read_finance_record":
		content, snapshots, err = s.executionSubscriptionReview(ctx, tx, st, tool, args)
	case "read_service_summary":
		content, snapshots, err = executionServiceSummary(ctx, tx, st, args)
	case "draft_post", "publish_post":
		writeOutput, snapshots, err = s.executionPostWrite(ctx, tx, st, tool, args)
	case "draft_request", "submit_request", "draft_business_text":
		writeOutput, snapshots, err = s.executionOARequest(ctx, tx, st, tool, args, observedSources)
	case "list_directory", "propose_organization", "commit_organization":
		writeOutput, snapshots, err = s.executionOrganization(ctx, tx, st, tool, args)
		if tool == "list_directory" {
			content, writeOutput = writeOutput, nil
		}
	case "create_file_draft", "stage_file_create", "recover_file_draft":
		var raw json.RawMessage
		var handled bool
		writeOutput, snapshots, raw, handled, err = s.createFileDraftStep(ctx, tx, st, lease, key, tool, args, inputHash, work)
		if handled || err != nil {
			return raw, err
		}
	case "propose_file_update", "commit_file_update", "propose_file_create", "commit_file_create":
		writeOutput, snapshots, err = s.executionFileProposal(ctx, tx, st, tool, args)
	case "read_resource":
		var kind struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(args, &kind)
		if kind.Type == "file" {
			in, e := fileToolArguments(args)
			if e != nil {
				return nil, e
			}
			ai := st.Actor
			ai.RunID = st.Run.ID
			prepared, e := s.prepareFileRead(ctx, tx, ai, st.Conversation, in, st.Run.audience)
			if e != nil {
				return nil, e
			}
			if prepared.Document.TextIndex != nil {
				index := *prepared.Document.TextIndex
				part, next, total, e := files.ReadAgentIndexedText(ctx, tx, ai, in.ID, *in.VersionID, index, in.Offset, in.MaxChars)
				if e != nil {
					return nil, e
				}
				content = map[string]any{"type": "file", "id": in.ID, "version_id": in.VersionID, "resource_version": prepared.Document.ResourceVersion,
					"title": prepared.Document.Title, "state": prepared.Document.State, "text": part, "offset": in.Offset, "next_offset": next,
					"total_chars": total, "source_sha256": prepared.Hash, "content_sha256": index.SourceSHA256, "read_mode": "indexed-text",
					"text_index": index, "verified_content": true, "evidence": prepared.Document.Evidence, "untrusted_data": true}
				snapshots = []SourceSnapshot{{SourceReference: SourceReference{Type: "file", ID: in.ID, VersionID: in.VersionID}, ResourceVersion: prepared.Document.ResourceVersion, SHA256: prepared.Hash}}
				break
			}
			if work == nil || work.FileRead == nil {
				// Release authorization and business locks during provider I/O. The
				// recursive finalization repeats the lease, cancellation, budget,
				// invocation-key and complete current-audience checks exactly once.
				if e = tx.Rollback(ctx); e != nil {
					return nil, e
				}
				prepared.Text, e = s.FileText.Read(ctx, prepared.Source)
				if e != nil {
					return nil, e
				}
				return s.executionTool(ctx, lease, key, tool, args, &executionIO{FileRead: &prepared})
			}
			fileRead := work.FileRead
			if prepared.Hash != fileRead.Hash || prepared.Source.Fingerprint != fileRead.Source.Fingerprint {
				return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
			}
			part, next, total, e := fileTextSlice(fileRead.Text, in.Offset, in.MaxChars)
			if e != nil {
				return nil, e
			}
			content = map[string]any{"type": "file", "id": in.ID, "version_id": in.VersionID, "resource_version": prepared.Document.ResourceVersion, "title": prepared.Document.Title, "state": prepared.Document.State, "text": part, "offset": in.Offset, "next_offset": next, "total_chars": total, "source_sha256": prepared.Hash, "content_sha256": prepared.Source.SHA256, "content_size_bytes": prepared.Source.Size, "verified_content": true, "evidence": prepared.Document.Evidence, "untrusted_data": true}
			snapshots = []SourceSnapshot{{SourceReference: SourceReference{Type: "file", ID: in.ID, VersionID: in.VersionID}, ResourceVersion: prepared.Document.ResourceVersion, SHA256: prepared.Hash}}
		} else {
			content, snapshots, err = executionRead(ctx, tx, st, args)
		}
	case "search_resources":
		content, snapshots, err = executionSearch(ctx, tx, st, lease, args)
	case "propose_project", "create_project":
		writeOutput, snapshots, err = s.executionProjectPlan(ctx, tx, st, tool, args)
	case "update_task_dependency", "create_task", "create_risk", "update_risk", "create_issue", "update_issue", "create_deliverable", "update_deliverable", "create_milestone", "update_project", "update_task", "transition_task", "transition_project", "update_milestone", "transition_milestone":
		writeOutput, snapshots, err = s.executionMutation(ctx, tx, st, tool, args)
	case "inspect_operation":
		var in struct {
			RunID  string  `json:"run_id"`
			PlanID *string `json:"plan_id"`
		}
		if err = strictArguments(args, &in); err == nil {
			if in.RunID != st.Run.ID {
				err = core.Missing()
			} else {
				content = map[string]any{"run_id": st.Run.ID, "state": st.Run.State, "business_commit_confirmed": false}
				snapshots = []SourceSnapshot{}
				if st.Profile.SkillID == "project-briefing" {
					p, e := executionPostWriteState(ctx, tx, st)
					if e != nil {
						err = e
						break
					}
					if p != nil {
						content = map[string]any{"run_id": st.Run.ID, "state": st.Run.State, "business_commit_confirmed": p.OperationID != nil, "post_write": postWriteReceipt(p)}
						snapshots = []SourceSnapshot{postWriteSnapshot(p)}
					}
				}
				if st.Profile.SkillID == "oa-assist" {
					r, e := executionOARequestState(ctx, tx, st)
					if e != nil {
						err = e
						break
					}
					if r != nil {
						content = map[string]any{"run_id": st.Run.ID, "state": st.Run.State, "business_commit_confirmed": r.OperationID != nil, "oa_request": r.OARequestView}
						snapshots = []SourceSnapshot{oaRequestSnapshot(r)}
					}
				}
				if st.Profile.SkillID == "project-update" {
					var m *mutationRecord
					m, err = executionMutationState(ctx, tx, st)
					if err != nil {
						break
					}
					if m != nil {
						var src SourceSnapshot
						ai := st.Actor
						ai.RunID = st.Run.ID
						src, err = mutationSource(ctx, tx, ai, st.Conversation, m.RunID, st.Run.audience)
						if err != nil {
							break
						}
						content = map[string]any{"run_id": st.Run.ID, "state": st.Run.State, "business_commit_confirmed": m.OperationID != nil, "mutation": m.MutationView}
						snapshots = []SourceSnapshot{src}
					}
				}
				if st.Profile.SkillID == "file-update" {
					i, e := s.executionFileIntent(ctx, tx, st)
					if e != nil {
						err = e
						break
					}
					if i != nil {
						var view FileUpdateView
						view, err = fileUpdateViewRead(ctx, tx, i)
						if err != nil {
							break
						}
						content = map[string]any{"run_id": st.Run.ID, "state": st.Run.State, "business_commit_confirmed": i.OperationID != nil, "file_update": view}
						if i.Mode == "create" {
							content = map[string]any{"run_id": st.Run.ID, "state": st.Run.State, "business_commit_confirmed": i.OperationID != nil, "file_create": fileCreateView(view)}
						}
						snapshots = []SourceSnapshot{fileUpdateSource(i)}
					}
				}
				if in.PlanID != nil {
					if st.Profile.SkillID == "information-organize" {
						ai := st.Actor
						ai.RunID = st.Run.ID
						var src SourceSnapshot
						src, err = organizationPlanSource(ctx, tx, ai, st.Conversation, *in.PlanID)
						if err != nil {
							break
						}
						snapshots = []SourceSnapshot{src}
						var p OrganizationPlan
						p, err = organizationPlanRecord(ctx, tx, st.Actor.OrgID, *in.PlanID, false)
						if err != nil {
							break
						}
						content = map[string]any{"run_id": st.Run.ID, "state": st.Run.State, "business_commit_confirmed": p.State == "committed", "plan_id": p.ID, "plan_state": p.State, "operation_id": p.OperationID, "results": p.Results}
						break
					}
					if st.Profile.SkillID != "project-create" {
						err = runtimeFailure("SKILL_TOOL_DENIED")
						break
					}
					var p Plan
					var src SourceSnapshot
					p, src, err = executionPlan(ctx, tx, st, *in.PlanID)
					if err != nil {
						break
					}
					content = map[string]any{"run_id": st.Run.ID, "state": st.Run.State, "business_commit_confirmed": p.State == "committed", "plan": p}
					snapshots = []SourceSnapshot{src}
				}
			}
		}
	default:
		err = runtimeFailure("SKILL_TOOL_DENIED")
	}
	if err != nil {
		return nil, err
	}
	if p, e := readPostWrite(ctx, tx, st.Actor.OrgID, st.Run.ID); e != nil {
		return nil, e
	} else if p != nil {
		if e = postWriteReferences(ctx, tx, st.Actor, p, snapshots); e != nil {
			return nil, e
		}
	}
	if _, err = mergeExecutionSources(st.Sources, observedSources, snapshots); err != nil {
		return nil, err
	}
	refs := []ResultReference{}
	for _, item := range snapshots {
		refs = append(refs, reference(item))
	}
	if writeOutput == nil {
		writeOutput = map[string]any{"status": "succeeded", "resource_refs": []ResultReference{}, "source_refs": refs, "operation_id": nil, "committed": false, "safe_summary": "已按当前共同权限读取准确资料", "retryable": false, "next_action": "", "content": content}
	}
	output, err := json.Marshal(writeOutput)
	if err != nil {
		return nil, err
	}
	if len(output) > 120000 {
		return nil, runtimeFailure("RUNTIME_OUTPUT_INVALID")
	}
	rawSources, _ := json.Marshal(snapshots)
	_, err = tx.Exec(ctx, `INSERT INTO agent_tool_receipts(id,org_id,run_id,invocation_key,authority_stamp,tool,input_sha256,source_snapshots,output)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, core.NewID(), lease.OrgID, lease.RunID, key, lease.AuthorityStamp, tool, inputHash, rawSources, output)
	if err != nil {
		return nil, err
	}
	action := "agent.tool.read"
	if tool == "propose_project" || tool == "create_project" || mutationTool(tool) || fileUpdateTool(tool) || tool == "propose_organization" || tool == "commit_organization" || oaRequestTool(tool) || postWriteTool(tool) {
		action = "agent.tool.write"
	}
	if err = executionAudit(ctx, tx, st, action, map[string]any{"tool": tool, "invocation_key_sha256": core.Digest([]byte(key)), "source_count": len(snapshots)}); err != nil {
		return nil, err
	}
	err = tx.Commit(ctx)
	if err == nil && work != nil && work.Draft != nil {
		work.Draft.Finalized = true
	}
	return output, err
}

func executionRead(ctx context.Context, tx pgx.Tx, st executionState, args json.RawMessage) (any, []SourceSnapshot, error) {
	var in struct {
		Type      string  `json:"type"`
		ID        string  `json:"id"`
		VersionID *string `json:"version_id"`
		Offset    int     `json:"offset"`
		MaxChars  int     `json:"max_chars"`
	}
	if err := strictArguments(args, &in); err != nil {
		return nil, nil, err
	}
	if in.MaxChars == 0 {
		in.MaxChars = 4000
	}
	if catalogSourceType(in.Type) {
		if st.Profile.SkillID != "project-create" || in.VersionID != nil {
			return nil, nil, runtimeFailure("SKILL_TOOL_DENIED")
		}
		a := st.Actor
		a.RunID = st.Run.ID
		view, source, e := catalogText(ctx, tx, a, st.Conversation, in.Type, in.ID, in.Offset, in.MaxChars)
		return view, []SourceSnapshot{source}, e
	}
	if in.Type == "agent_post_write" {
		if st.Profile.SkillID != "project-briefing" {
			return nil, nil, core.Missing()
		}
		return executionPostWriteText(ctx, tx, st, in.ID, in.VersionID, in.Offset, in.MaxChars)
	}
	if in.Type == "agent_oa_request" {
		if st.Profile.SkillID != "oa-assist" {
			return nil, nil, runtimeFailure("SKILL_TOOL_DENIED")
		}
		return executionOARequestText(ctx, tx, st, in.ID, in.VersionID, in.Offset, in.MaxChars)
	}
	if in.Type == "agent_contract_body" {
		return executionContractBodyText(ctx, tx, st, in.ID, in.VersionID, in.Offset, in.MaxChars)
	}
	if in.Type == "agent_plan" {
		return executionPlanText(ctx, tx, st, in.ID, in.VersionID, in.Offset, in.MaxChars)
	}
	if in.Type == "agent_organization_plan" {
		return executionOrganizationText(ctx, tx, st, in.ID, in.VersionID, in.Offset, in.MaxChars)
	}
	if in.Type == "agent_mutation" {
		if st.Profile.SkillID != "project-update" {
			return nil, nil, runtimeFailure("SKILL_TOOL_DENIED")
		}
		return executionMutationText(ctx, tx, st, in.ID, in.VersionID, in.Offset, in.MaxChars)
	}
	if !slices.Contains([]string{"project", "task", "post", "deliverable", "milestone", "risk", "issue", "service", "agent_finance_report"}, in.Type) || !core.ValidID(in.ID) || in.Offset < 0 || in.Offset > 200000 || in.MaxChars < 1 || in.MaxChars > 10000 || in.VersionID != nil && (!core.ValidID(*in.VersionID) || in.Type != "post" && in.Type != "service") {
		return nil, nil, core.Invalid("arguments", "请指定准确资料及有限字符范围")
	}
	if in.Type == "task" && st.Profile.SkillID == "project-update" {
		m, e := executionMutationState(ctx, tx, st)
		if e != nil {
			return nil, nil, e
		}
		if m != nil && m.Tool == "create_task" && !mutationTargets(m, in.Type, in.ID) {
			return nil, nil, core.Missing()
		}
	}
	if in.Type == "milestone" || in.Type == "deliverable" || in.Type == "risk" || in.Type == "issue" {
		m, e := executionMutationState(ctx, tx, st)
		if e != nil {
			return nil, nil, e
		}
		if !mutationTargets(m, in.Type, in.ID) {
			return nil, nil, core.Missing()
		}
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	doc, err := contextDocumentRead(ctx, tx, ai, st.Conversation, ContextSource{Type: in.Type, ID: in.ID, VersionID: in.VersionID}, st.Run.audience)
	if err != nil {
		return nil, nil, err
	}
	if err = resourceAudience(ctx, tx, ai, st.Conversation, doc, st.Run.audience); err != nil {
		return nil, nil, err
	}
	body := []rune(doc.Body)
	if len(body) > 200000 || in.Offset > len(body) {
		return nil, nil, core.Invalid("offset", "资料或字符范围超出受控读取上限")
	}
	hash, err := documentDigest(doc)
	if err != nil {
		return nil, nil, err
	}
	end := min(in.Offset+in.MaxChars, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	snapshot := SourceSnapshot{SourceReference: SourceReference{Type: doc.Type, ID: doc.ID, VersionID: doc.VersionID}, ResourceVersion: doc.ResourceVersion, SHA256: hash}
	value := map[string]any{"type": doc.Type, "id": doc.ID, "version_id": doc.VersionID, "resource_version": doc.ResourceVersion, "title": doc.Title, "state": doc.State, "text": string(body[in.Offset:end]), "offset": in.Offset, "total_chars": len(body), "next_offset": next, "source_sha256": hash, "untrusted_data": true}
	if m, e := mutationRecordRead(ctx, tx, ai.OrgID, st.Run.ID); e != nil {
		return nil, nil, e
	} else if mutationTargets(m, doc.Type, doc.ID) {
		snapshot, e = mutationSource(ctx, tx, ai, st.Conversation, m.RunID, st.Run.audience)
		if e != nil {
			return nil, nil, e
		}
		value["source_ref"] = reference(snapshot)
		value["source_sha256"] = snapshot.SHA256
		value["observation_sha256"] = hash
		value["authorized_mutation"] = m.MutationView
	}
	return value, []SourceSnapshot{snapshot}, nil
}

type searchCursor struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Binding string `json:"binding"`
}

func executionSearch(ctx context.Context, tx pgx.Tx, st executionState, lease ExecutionLease, args json.RawMessage) (any, []SourceSnapshot, error) {
	var in struct {
		Query      string   `json:"query"`
		Types      []string `json:"resource_types"`
		PlanStates []string `json:"plan_states"`
		Cursor     string   `json:"cursor"`
		Limit      int      `json:"limit"`
	}
	if err := strictArguments(args, &in); err != nil {
		return nil, nil, err
	}
	if in.Limit == 0 {
		in.Limit = 10
	}
	if len(in.Types) == 0 {
		in.Types = []string{"project", "task", "post", "file"}
		if st.Profile.SkillID == "incident-summary" {
			in.Types = []string{"service"}
		}
	}
	in.Query = strings.TrimSpace(in.Query)
	if slices.Contains(in.Types, "agent_plan") {
		return executionPlanSearch(ctx, tx, st, lease, in.Query, in.Types, in.PlanStates, in.Cursor, in.Limit)
	}
	if len(in.PlanStates) > 0 {
		return nil, nil, core.Invalid("plan_states", "计划状态只适用于项目计划查询")
	}
	if !utf8.ValidString(in.Query) || strings.ContainsRune(in.Query, 0) || len([]rune(in.Query)) == 0 || len([]rune(in.Query)) > 200 || in.Limit < 1 || in.Limit > 20 || len(in.Types) > 5 || (len(in.Types) > 4 && st.Profile.SkillID != "incident-summary") {
		return nil, nil, core.Invalid("arguments", "查找范围无效")
	}
	slices.Sort(in.Types)
	in.Types = slices.Compact(in.Types)
	for _, kind := range in.Types {
		if !slices.Contains([]string{"project", "task", "post", "file"}, kind) && !(st.Profile.SkillID == "incident-summary" && kind == "service") {
			return nil, nil, core.Invalid("resource_types", "此工具只查找会话范围内的项目、任务、知识、文件及允许的服务资料")
		}
	}
	bound, _ := json.Marshal([]any{"body-and-indexed-file-v1", lease.RunID, lease.AuthorityStamp, in.Query, in.Types, in.Limit})
	binding := core.Digest(bound)
	if lease.credentialSHA256 == "" {
		return nil, nil, runtimeFailure("RUNTIME_CREDENTIAL_INVALID")
	}
	key := []byte(lease.credentialSHA256)
	after := searchCursor{Binding: binding}
	if in.Cursor != "" {
		parts := strings.Split(in.Cursor, ".")
		if len(parts) != 2 || len(in.Cursor) > 2048 {
			return nil, nil, core.Invalid("cursor", "查找位置无效")
		}
		raw, e := base64.RawURLEncoding.DecodeString(parts[0])
		if e != nil {
			return nil, nil, core.Invalid("cursor", "查找位置无效")
		}
		sig, e := base64.RawURLEncoding.DecodeString(parts[1])
		m := hmac.New(sha256.New, key)
		m.Write(raw)
		if e != nil || !hmac.Equal(m.Sum(nil), sig) || json.Unmarshal(raw, &after) != nil || after.Binding != binding || !core.ValidID(after.ID) || !slices.Contains(in.Types, after.Type) {
			return nil, nil, core.Invalid("cursor", "查找位置与任务或筛选不一致")
		}
	}
	// Candidate identities are never returned before the common source view and all file dependencies are checked.
	rows, err := tx.Query(ctx, `SELECT kind,id,version_id FROM (
 SELECT 'project'::text kind,id,NULL::uuid version_id FROM projects WHERE org_id=$1 AND id=$2::uuid
 UNION ALL SELECT 'task',id,NULL::uuid FROM tasks WHERE org_id=$1 AND project_id=$2::uuid
 UNION ALL SELECT 'post',id,NULL::uuid FROM posts WHERE org_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid
 UNION ALL SELECT 'service',id,NULL::uuid FROM services WHERE org_id=$1 AND project_id IS NOT DISTINCT FROM $2::uuid
 UNION ALL SELECT 'file',f.id,f.current_version_id FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id
 WHERE f.org_id=$1 AND d.project_id IS NOT DISTINCT FROM $2::uuid AND f.state='active' AND f.current_version_id IS NOT NULL
 ) x WHERE kind=ANY($3::text[]) AND ($4='' OR (kind,id) > ($4,$5::uuid)) ORDER BY kind,id LIMIT 10001`, st.Actor.OrgID, st.Conversation.ProjectID, in.Types, after.Type, func() any {
		if after.ID == "" {
			return nil
		}
		return after.ID
	}())
	if err != nil {
		return nil, nil, err
	}
	type candidate struct {
		kind, id string
		version  *string
	}
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.kind, &c.id, &c.version); err != nil {
			rows.Close()
			return nil, nil, err
		}
		candidates = append(candidates, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(candidates) > 10000 {
		return nil, nil, runtimeFailure("RUNTIME_SEARCH_LIMIT")
	}
	items := []map[string]any{}
	snapshots := []SourceSnapshot{}
	var last candidate
	more := false
	ai := st.Actor
	ai.RunID = st.Run.ID
	for _, c := range candidates {
		doc, e := contextDocumentRead(ctx, tx, ai, st.Conversation, ContextSource{Type: c.kind, ID: c.id, VersionID: c.version}, st.Run.audience)
		if e == nil {
			e = resourceAudience(ctx, tx, ai, st.Conversation, doc, st.Run.audience)
		}
		if e != nil {
			status := core.PublicError(e).Status
			if status == 403 || status == 404 || status == 409 || (c.kind == "file" && status == 422) {
				continue
			}
			return nil, nil, e
		}
		matchedTitle := strings.Contains(strings.ToLower(doc.Title), strings.ToLower(in.Query))
		body, index, e := executionSearchBody(ctx, tx, ai, doc, in.Query)
		if e != nil {
			return nil, nil, e
		}
		matchedBody := body != ""
		if !matchedTitle && !matchedBody {
			continue
		}
		if len(items) == in.Limit {
			more = true
			break
		}
		hash, e := documentDigest(doc)
		if e != nil {
			return nil, nil, e
		}
		snapshots = append(snapshots, SourceSnapshot{SourceReference: SourceReference{Type: doc.Type, ID: doc.ID, VersionID: doc.VersionID}, ResourceVersion: doc.ResourceVersion, SHA256: hash})
		matchTypes := []string{}
		if matchedTitle {
			matchTypes = append(matchTypes, "title")
		}
		if matchedBody {
			matchTypes = append(matchTypes, "body")
		}
		items = append(items, map[string]any{"type": doc.Type, "id": doc.ID, "version_id": doc.VersionID, "title": doc.Title,
			"state": doc.State, "resource_version": doc.ResourceVersion, "source_sha256": hash, "untrusted_data": true,
			"snippet": body, "match_types": matchTypes, "text_index": index})
		last = c
	}
	var next *string
	if more {
		raw, _ := json.Marshal(searchCursor{Type: last.kind, ID: last.id, Binding: binding})
		mac := hmac.New(sha256.New, key)
		mac.Write(raw)
		value := base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
		next = &value
	}
	return map[string]any{"items": items, "next_cursor": next, "untrusted_data": true}, snapshots, nil
}

func (s *Service) ExecutionReceipts(ctx context.Context, lease ExecutionLease) ([]ExecutionReceipt, error) {
	tx, st, err := s.executionTx(ctx, lease, true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	receipts, _, err := readExecutionReceipts(ctx, tx, st, lease)
	if err != nil {
		return nil, err
	}
	return receipts, tx.Commit(ctx)
}
func readExecutionReceipts(ctx context.Context, tx pgx.Tx, st executionState, lease ExecutionLease) ([]ExecutionReceipt, []SourceSnapshot, error) {
	rows, err := tx.Query(ctx, "SELECT tool,source_snapshots,output FROM agent_tool_receipts WHERE org_id=$1 AND run_id=$2 AND authority_stamp=$3 ORDER BY created_at,id", lease.OrgID, lease.RunID, lease.AuthorityStamp)
	if err != nil {
		return nil, nil, err
	}
	type item struct {
		tool    string
		sources []SourceSnapshot
		output  ExecutionReceipt
	}
	items := []item{}
	for rows.Next() {
		var tool string
		var raw, output []byte
		if err = rows.Scan(&tool, &raw, &output); err != nil {
			rows.Close()
			return nil, nil, err
		}
		var sources []SourceSnapshot
		if err = json.Unmarshal(raw, &sources); err != nil {
			rows.Close()
			return nil, nil, err
		}
		var receipt ExecutionReceipt
		if err = json.Unmarshal(output, &receipt); err != nil {
			rows.Close()
			return nil, nil, err
		}
		items = append(items, item{tool: tool, sources: sources, output: receipt})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	out := []ExecutionReceipt{}
	all := []SourceSnapshot{}
	for _, item := range items {
		if err = executionSources(ctx, tx, st, item.sources); err != nil {
			return nil, nil, err
		}
		all, err = mergeExecutionSources(all, item.sources)
		if err != nil {
			return nil, nil, err
		}
		receipt := item.output
		receipt.RunID = lease.RunID
		receipt.AuthorityStamp = lease.AuthorityStamp
		receipt.Tool = item.tool
		out = append(out, receipt)
	}
	if len(all) > 20 {
		return nil, nil, runtimeFailure("RUNTIME_CONTEXT_LIMIT")
	}
	return out, all, nil
}

// Mutable business heads have one observed revision per turn. Formal file
// versions are independently immutable and may be compared in the same turn.
func mergeExecutionSources(groups ...[]SourceSnapshot) ([]SourceSnapshot, error) {
	out := []SourceSnapshot{}
	seen := map[string]SourceSnapshot{}
	for _, group := range groups {
		for _, src := range group {
			key := src.Type + ":" + src.ID
			if (src.Type == "file" || src.Type == "service") && src.VersionID != nil {
				key += ":" + *src.VersionID
			}
			if old, ok := seen[key]; ok {
				a, _ := json.Marshal(old)
				b, _ := json.Marshal(src)
				if !bytes.Equal(a, b) {
					return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
				}
			} else {
				seen[key] = src
				out = append(out, src)
			}
		}
	}
	if len(out) > 20 {
		return nil, runtimeFailure("RUNTIME_CONTEXT_LIMIT")
	}
	return out, nil
}
