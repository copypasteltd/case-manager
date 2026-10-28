package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
)

type ProposedResult struct {
	Status        string            `json:"status"`
	Summary       string            `json:"summary"`
	ResourceRefs  []ResultReference `json:"resource_refs"`
	SourceRefs    []ResultReference `json:"source_refs"`
	OperationIDs  []string          `json:"operation_ids"`
	MissingFields []string          `json:"missing_fields"`
	NextAction    string            `json:"next_action"`
}
type ExecutionUsage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

func referenceKey(r ResultReference) string {
	v := ""
	if r.VersionID != nil {
		v = *r.VersionID
	}
	return r.Type + ":" + r.ID + ":" + v
}
func validateProposedResult(r ProposedResult) error {
	if !slices.Contains([]string{"draft", "pending", "awaiting_input", "completed", "conflict", "denied", "failed"}, r.Status) || strings.TrimSpace(r.Summary) == "" || utf8.RuneCountInString(r.Summary) > 20000 || utf8.RuneCountInString(r.NextAction) > 2000 || r.ResourceRefs == nil || r.SourceRefs == nil || r.OperationIDs == nil || r.MissingFields == nil || len(r.ResourceRefs) > 100 || len(r.SourceRefs) > 100 || len(r.OperationIDs) > 100 || len(r.MissingFields) > 30 {
		return runtimeFailure("RUNTIME_OUTPUT_INVALID")
	}
	for _, refs := range [][]ResultReference{r.ResourceRefs, r.SourceRefs} {
		seen := map[string]bool{}
		for _, ref := range refs {
			key := referenceKey(ref)
			if !core.ValidID(ref.ID) || (ref.VersionID != nil && !core.ValidID(*ref.VersionID)) || seen[key] {
				return runtimeFailure("RUNTIME_OUTPUT_INVALID")
			}
			seen[key] = true
		}
	}
	seen := map[string]bool{}
	for _, field := range r.MissingFields {
		if strings.TrimSpace(field) == "" || utf8.RuneCountInString(field) > 200 || seen[field] {
			return runtimeFailure("RUNTIME_OUTPUT_INVALID")
		}
		seen[field] = true
	}
	seenOperations := map[string]bool{}
	for _, id := range r.OperationIDs {
		if !core.ValidID(id) || seenOperations[id] {
			return runtimeFailure("RESULT_OPERATION_UNVERIFIED")
		}
		seenOperations[id] = true
	}
	return nil
}

// A response and its authority/evidence, event, audit and queue receipt commit together.
// "succeeded" means the turn returned a verified response; outcome describes the business state.
func (s *Service) CompleteExecution(ctx context.Context, lease ExecutionLease, result ProposedResult, usage ExecutionUsage) error {
	if err := validateProposedResult(result); err != nil {
		return err
	}
	tx, st, err := s.executionTx(ctx, lease, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if st.Run.State != "running" {
		return runtimeFailure("RUNTIME_TERMINAL")
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CacheWriteInputTokens < 0 || usage.ReasoningOutputTokens < 0 || usage.CachedInputTokens < 0 || usage.CachedInputTokens > usage.InputTokens || usage.InputTokens > int64(st.Profile.MaxInputTokens) || usage.OutputTokens > int64(st.Profile.MaxOutputTokens) {
		return runtimeFailure("RUNTIME_USAGE_BUDGET_EXCEEDED")
	}
	if err = recordExecutionUsage(ctx, tx, lease.OrgID, lease.RunID, usage); err != nil {
		return err
	}
	receipts, readSources, err := readExecutionReceipts(ctx, tx, st, lease)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	knownOperations := map[string]bool{}
	projectCommitted := false
	mutationCommitted := false
	fileCommitted := false
	organizationCommitted := false
	oaCommitted := false
	postCommitted := false
	for _, receipt := range receipts {
		for _, ref := range append(receipt.SourceRefs, receipt.ResourceRefs...) {
			known[referenceKey(ref)] = true
		}
		if receipt.Status == "succeeded" && receipt.Committed && receipt.OperationID != nil {
			knownOperations[*receipt.OperationID] = true
			if receipt.Tool == "create_project" && slices.Contains(result.OperationIDs, *receipt.OperationID) {
				projectCommitted = true
			}
			if mutationTool(receipt.Tool) && slices.Contains(result.OperationIDs, *receipt.OperationID) {
				mutationCommitted = true
			}
			if (receipt.Tool == "commit_file_update" || receipt.Tool == "commit_file_create") && slices.Contains(result.OperationIDs, *receipt.OperationID) {
				fileCommitted = true
			}
			if receipt.Tool == "commit_organization" && slices.Contains(result.OperationIDs, *receipt.OperationID) {
				organizationCommitted = true
			}
			if oaRequestTool(receipt.Tool) && slices.Contains(result.OperationIDs, *receipt.OperationID) {
				oaCommitted = true
			}
			if postWriteTool(receipt.Tool) && slices.Contains(result.OperationIDs, *receipt.OperationID) {
				postCommitted = true
			}
		}
	}
	for _, id := range result.OperationIDs {
		if !knownOperations[id] {
			return runtimeFailure("RESULT_OPERATION_UNVERIFIED")
		}
	}
	for _, refs := range [][]ResultReference{result.ResourceRefs, result.SourceRefs} {
		for _, ref := range refs {
			if !known[referenceKey(ref)] {
				return runtimeFailure("RESULT_REFERENCE_UNVERIFIED")
			}
		}
	}
	if result.Status == "completed" {
		if p, e := readPostWrite(ctx, tx, st.Actor.OrgID, st.Run.ID); e != nil {
			return e
		} else if p != nil && (!postCommitted || len(result.MissingFields) > 0) {
			return runtimeFailure("RESULT_COMPLETION_UNVERIFIED")
		}
		if st.Profile.SkillID == "oa-assist" {
			if !oaCommitted || len(result.MissingFields) > 0 {
				return runtimeFailure("RESULT_COMPLETION_UNVERIFIED")
			}
		} else if st.Profile.SkillID == "information-organize" {
			if !organizationCommitted || len(result.MissingFields) > 0 {
				return runtimeFailure("RESULT_COMPLETION_UNVERIFIED")
			}
		} else if st.Profile.SkillID == "project-create" {
			if !projectCommitted || len(result.MissingFields) > 0 {
				return runtimeFailure("RESULT_COMPLETION_UNVERIFIED")
			}
		} else if st.Profile.SkillID == "project-update" {
			if !mutationCommitted || len(result.MissingFields) > 0 {
				return runtimeFailure("RESULT_COMPLETION_UNVERIFIED")
			}
		} else if st.Profile.SkillID == "file-update" {
			if !fileCommitted || len(result.MissingFields) > 0 {
				return runtimeFailure("RESULT_COMPLETION_UNVERIFIED")
			}
		} else if st.Profile.SkillID == "subscription-review" {
			verified := false
			for _, receipt := range receipts {
				if slices.Contains([]string{"read_finance_summary", "read_subscription_review", "read_finance_record"}, receipt.Tool) && receipt.Status == "succeeded" {
					for _, ref := range receipt.SourceRefs {
						if ref.Type == "agent_finance_report" {
							for _, cited := range result.SourceRefs {
								if referenceKey(ref) == referenceKey(cited) {
									verified = true
								}
							}
						}
					}
				}
			}
			if !verified {
				return runtimeFailure("RESULT_READ_UNVERIFIED")
			}
		} else if st.Profile.SkillID != "project-briefing" && st.Profile.SkillID != "incident-summary" {
			return runtimeFailure("RESULT_COMPLETION_UNVERIFIED")
		}
		if len(result.MissingFields) > 0 || (st.Profile.SkillID != "project-create" && len(result.SourceRefs) == 0) || len(receipts) == 0 {
			return runtimeFailure("RESULT_READ_UNVERIFIED")
		}
	}
	// Keep every source observed by the turn, including omitted citations. Subsequent reads can
	// withdraw this entire assistant response if any dependency or common audience changes.
	sources, err := mergeExecutionSources(st.Sources, readSources)
	if err != nil {
		return err
	}
	labels := map[string]string{"draft": "草稿", "pending": "待处理", "awaiting_input": "待补充资料", "completed": "已完成本次事项", "conflict": "需核对冲突", "denied": "权限不足", "failed": "事项未完成"}
	body := labels[result.Status] + "\n\n" + result.Summary
	if len(result.MissingFields) > 0 {
		body += "\n\n待补充：" + strings.Join(result.MissingFields, "；")
	}
	if strings.TrimSpace(result.NextAction) != "" {
		body += "\n\n下一步：" + result.NextAction
	}
	if utf8.RuneCountInString(body) > 20000 {
		return runtimeFailure("RUNTIME_OUTPUT_INVALID")
	}
	rawSources, _ := json.Marshal(sources)
	rawResult, _ := json.Marshal(result)
	rawUsage, _ := json.Marshal(usage)
	_, err = tx.Exec(ctx, `INSERT INTO conversation_messages(id,org_id,conversation_id,run_id,seq,role,body,author_id,audience_ids,source_snapshots)
 SELECT $1,$2,$3,$4,coalesce(max(seq),0)+1,'assistant',$5,$6,$7,$8 FROM conversation_messages WHERE org_id=$2 AND conversation_id=$3`, core.NewID(), lease.OrgID, st.Conversation.ID, lease.RunID, body, st.Actor.UserID, st.Run.audience, rawSources)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO agent_run_results(run_id,org_id,authority_stamp,result,usage,source_snapshots,body) VALUES($1,$2,$3,$4,$5,$6,$7)`, lease.RunID, lease.OrgID, lease.AuthorityStamp, rawResult, rawUsage, rawSources, body)
	if err != nil {
		return err
	}
	if err = executionTransition(ctx, tx, st, "succeeded", "run.completed", nil, &result.Status); err != nil {
		return err
	}
	if err = s.Queue().CompleteInTx(ctx, tx, lease.Job); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// This is controller-only reconciliation after a lost HTTP acknowledgement. It never
// interprets model text as success and never creates a second assistant message.
func (s *Service) ExecutionSettled(ctx context.Context, j jobqueue.Job) (bool, error) {
	var settled bool
	err := s.DB.QueryRow(ctx, `SELECT r.state IN ('succeeded','failed','cancelled','interrupted') AND j.state IN ('succeeded','dead','cancelled')
 FROM agent_run_jobs x JOIN agent_runs r ON r.id=x.run_id JOIN jobs j ON j.id=x.job_id
 WHERE x.org_id=$1 AND x.job_id=$2 AND j.fencing_token=$3`, j.OrgID, j.ID, j.FencingToken).Scan(&settled)
	return settled, err
}
