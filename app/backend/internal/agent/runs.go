package agent

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type RuntimeProfile struct {
	ID                  string `json:"id"`
	SkillID             string `json:"skill_id"`
	SkillVersion        string `json:"skill_version"`
	SkillSHA256         string `json:"skill_sha256"`
	RuntimeVersion      string `json:"runtime_version"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
	MaxInputTokens      int    `json:"max_input_tokens"`
	MaxOutputTokens     int    `json:"max_output_tokens"`
	MaxToolCalls        int    `json:"max_tool_calls"`
	TimeoutSeconds      int    `json:"timeout_seconds"`
}
type RuntimeOptions struct {
	Available bool             `json:"available"`
	Reason    *string          `json:"reason"`
	Profiles  []RuntimeProfile `json:"profiles"`
}
type SourceReference struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	VersionID *string `json:"version_id,omitempty"`
}
type SourceSnapshot struct {
	SourceReference
	ResourceVersion int64  `json:"resource_version"`
	SHA256          string `json:"sha256"`
}
type RunCreate struct {
	ExpectedVersion        int64              `json:"expected_version"`
	ContextSecurityVersion int64              `json:"context_security_version"`
	ProfileID              string             `json:"profile_id"`
	ClientMessageID        string             `json:"client_message_id"`
	Message                string             `json:"message"`
	SkillID                string             `json:"skill_id,omitempty"`
	ContextRefs            []SourceReference  `json:"context_refs,omitempty"`
	Mutation               *MutationInput     `json:"mutation,omitempty"`
	OARequest              *OARequestInput    `json:"oa_request,omitempty"`
	PostWrite              *PostWriteInput    `json:"post_write,omitempty"`
	FileUpdate             *FileUpdateInput   `json:"file_update,omitempty"`
	FileCreate             *FileCreateInput   `json:"file_create,omitempty"`
	FileRecovery           *FileRecoveryInput `json:"file_recovery,omitempty"`
	ContractBody           *ContractBodyInput `json:"contract_body,omitempty"`
}
type Run struct {
	ID                string            `json:"id"`
	Version           int64             `json:"version"`
	ConversationID    string            `json:"conversation_id"`
	ActorID           string            `json:"actor_id"`
	ProfileID         string            `json:"profile_id"`
	ClientMessageID   string            `json:"client_message_id"`
	State             string            `json:"state"`
	LastEventSeq      int64             `json:"last_event_seq"`
	CancelRequestedAt *time.Time        `json:"cancel_requested_at"`
	SafeErrorCode     *string           `json:"safe_error_code"`
	ExpiresAt         time.Time         `json:"expires_at"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	CanCancel         bool              `json:"can_cancel"`
	Outcome           *string           `json:"outcome"`
	Mutation          *MutationView     `json:"mutation,omitempty"`
	OARequest         *OARequestView    `json:"oa_request,omitempty"`
	PostWrite         *PostWriteView    `json:"post_write,omitempty"`
	FileUpdate        *FileUpdateView   `json:"file_update,omitempty"`
	FileCreate        *FileCreateView   `json:"file_create,omitempty"`
	ContractBody      *ContractBodyView `json:"contract_body,omitempty"`
	audience          []string
}

const runColumns = `id,version,conversation_id,actor_id,profile_id,client_message_id,state,last_event_seq,cancel_requested_at,safe_error_code,expires_at,created_at,updated_at,audience_ids,outcome`
const profileColumns = `id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds`

func scanRun(row pgx.Row) (Run, error) {
	var r Run
	err := row.Scan(&r.ID, &r.Version, &r.ConversationID, &r.ActorID, &r.ProfileID, &r.ClientMessageID, &r.State, &r.LastEventSeq, &r.CancelRequestedAt, &r.SafeErrorCode, &r.ExpiresAt, &r.CreatedAt, &r.UpdatedAt, &r.audience, &r.Outcome)
	return r, platform.DatabaseError(err)
}
func scanProfile(row pgx.Row) (RuntimeProfile, error) {
	var p RuntimeProfile
	err := row.Scan(&p.ID, &p.SkillID, &p.SkillVersion, &p.SkillSHA256, &p.RuntimeVersion, &p.ConfigurationSHA256, &p.MaxInputTokens, &p.MaxOutputTokens, &p.MaxToolCalls, &p.TimeoutSeconds)
	return p, platform.DatabaseError(err)
}
func runRecord(ctx context.Context, db platform.DBTX, org, id string, lock bool) (Run, error) {
	if !core.ValidID(id) {
		return Run{}, core.Missing()
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	return scanRun(db.QueryRow(ctx, "SELECT "+runColumns+" FROM agent_runs WHERE org_id=$1 AND id=$2"+suffix, org, id))
}
func aiAudience(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation) ([]string, error) {
	if a.RunID != "" {
		return nil, core.Forbidden()
	}
	if a.Channel == "telegram" {
		if c.Type != "telegram" {
			return nil, core.Forbidden()
		}
		if err := telegramConversationAuthority(ctx, tx, a, c); err != nil {
			return nil, err
		}
	}
	members, err := memberIDs(ctx, tx, a.OrgID, c)
	if err != nil {
		return nil, err
	}
	ai := a
	ai.RunID = core.NewID()
	for _, id := range members {
		target := ai
		target.UserID = id
		if err = require(ctx, tx, target, c, "conversation.read"); err != nil {
			return nil, err
		}
	}
	return members, nil
}
func (s *Service) RuntimeOptions(ctx context.Context, a core.Actor, id string) (RuntimeOptions, error) {
	out := RuntimeOptions{Profiles: []RuntimeProfile{}}
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	c, err := metadata(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	if err = require(ctx, tx, a, c, "conversation.read"); err != nil {
		return out, err
	}
	reason := ""
	if c.Type == "telegram" {
		reason = "CHANNEL_MANAGED"
	} else if c.State != "active" {
		reason = "CONVERSATION_READ_ONLY"
	} else if err = require(ctx, tx, a, c, "agent.run"); err != nil {
		reason = "RUN_NOT_ALLOWED"
	} else if _, err = aiAudience(ctx, tx, a, c); err != nil {
		reason = "CONTEXT_UNAVAILABLE"
	}
	if reason == "" {
		rows, e := tx.Query(ctx, "SELECT "+profileColumns+" FROM agent_runtime_profiles WHERE org_id=$1 AND state='active' ORDER BY skill_id LIMIT 8", a.OrgID)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			p, e := scanProfile(rows)
			if e != nil {
				rows.Close()
				return out, e
			}
			out.Profiles = append(out.Profiles, p)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		if len(out.Profiles) == 0 {
			reason = "RUNTIME_UNAVAILABLE"
		}
	}
	out.Available = reason == ""
	if reason != "" {
		out.Reason = &reason
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func sourceSnapshots(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, refs []SourceReference, members []string, ai bool) ([]SourceSnapshot, error) {
	out := []SourceSnapshot{}
	seen := map[string]bool{}
	if len(refs) > 20 {
		return nil, core.Invalid("context_refs", "每次最多选择20项准确资料")
	}
	if ai {
		a.RunID = core.NewID()
	}
	for _, ref := range refs {
		if ref.Type == "agent_contract_body" {
			if !exactOAID(ref.ID) || ref.VersionID != nil || seen[ref.Type+":"+ref.ID] {
				return nil, core.Missing()
			}
			seen[ref.Type+":"+ref.ID] = true
			item, err := contractBodySource(ctx, tx, a, c, ref.ID, members)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
			continue
		}
		if ref.Type == "agent_post_write" {
			if !core.ValidID(ref.ID) || ref.VersionID != nil || seen[ref.Type+":"+ref.ID] {
				return nil, core.Missing()
			}
			seen[ref.Type+":"+ref.ID] = true
			item, err := postWriteSource(ctx, tx, a, c, ref.ID, members)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
			continue
		}
		if ref.Type == "agent_oa_request" {
			if !core.ValidID(ref.ID) || ref.VersionID != nil || seen[ref.Type+":"+ref.ID] {
				return nil, core.Missing()
			}
			seen[ref.Type+":"+ref.ID] = true
			item, err := oaRequestSource(ctx, tx, a, c, ref.ID, members)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
			continue
		}
		if ref.Type == "agent_organization_plan" || ref.Type == "agent_directory" {
			if !core.ValidID(ref.ID) || ref.VersionID != nil || len(members) != 1 || members[0] != a.UserID || seen[ref.Type+":"+ref.ID] {
				return nil, core.Missing()
			}
			seen[ref.Type+":"+ref.ID] = true
			var item SourceSnapshot
			var err error
			if ref.Type == "agent_organization_plan" {
				item, err = organizationPlanSource(ctx, tx, a, c, ref.ID)
			} else {
				item, err = directorySnapshotSource(ctx, tx, a, c, ref.ID)
			}
			if err != nil {
				return nil, err
			}
			out = append(out, item)
			continue
		}
		if ref.Type == "agent_file_update" || ref.Type == "agent_file_create" {
			if !core.ValidID(ref.ID) || ref.VersionID != nil || seen[ref.Type+":"+ref.ID] {
				return nil, core.Missing()
			}
			seen[ref.Type+":"+ref.ID] = true
			item, err := fileIntentSource(ctx, tx, a, c, ref.ID, members)
			if err != nil {
				return nil, err
			}
			if item.Type != ref.Type {
				return nil, core.Missing()
			}
			out = append(out, item)
			continue
		}
		ref.ID = strings.ToLower(ref.ID)
		if ref.Type == "agent_mutation" && ref.VersionID == nil && core.ValidID(ref.ID) {
			if seen[ref.Type+":"+ref.ID] {
				return nil, core.Missing()
			}
			seen[ref.Type+":"+ref.ID] = true
			item, err := mutationSource(ctx, tx, a, c, ref.ID, members)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
			continue
		}
		if ref.Type == "agent_plan" && ref.VersionID == nil {
			if len(members) != 1 || members[0] != a.UserID || seen[ref.Type+":"+ref.ID] {
				return nil, core.Missing()
			}
			seen[ref.Type+":"+ref.ID] = true
			item, err := planSource(ctx, tx, a, c, ref.ID)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
			continue
		}
		if !slices.Contains([]string{"project", "task", "post", "file", "service", "agent_finance_report"}, ref.Type) || !core.ValidID(ref.ID) || (ref.VersionID != nil && (!core.ValidID(*ref.VersionID) || (ref.Type != "post" && ref.Type != "file" && ref.Type != "service"))) || ref.Type == "file" && ref.VersionID == nil {
			return nil, core.Invalid("context_refs", "请指定准确项目、任务、知识修订或文件版本")
		}
		key := ref.Type + ":" + ref.ID
		if ref.Type == "file" || ref.Type == "service" && ref.VersionID != nil {
			key += ":" + *ref.VersionID
		}
		if seen[key] {
			return nil, core.Invalid("context_refs", "同一资料不能重复选择")
		}
		seen[key] = true
		if ref.VersionID != nil {
			v := strings.ToLower(*ref.VersionID)
			ref.VersionID = &v
		}
		doc, err := contextDocumentRead(ctx, tx, a, c, ContextSource{Type: ref.Type, ID: ref.ID, VersionID: ref.VersionID}, members)
		if err != nil {
			return nil, err
		}
		if err = resourceAudience(ctx, tx, a, c, doc, members); err != nil {
			return nil, err
		}
		if len([]rune(doc.Body)) > 200000 {
			return nil, core.Invalid("context_refs", "资料超出受控文本准备上限，请缩小范围")
		}
		hash, err := documentDigest(doc)
		if err != nil {
			return nil, err
		}
		out = append(out, SourceSnapshot{SourceReference: SourceReference{Type: ref.Type, ID: ref.ID, VersionID: doc.VersionID}, ResourceVersion: doc.ResourceVersion, SHA256: hash})
	}
	return out, nil
}
func (s *Service) CreateRun(ctx context.Context, command core.Command, id string, in RunCreate) (core.Result, error) {
	if !core.ValidID(in.ProfileID) || !core.ValidID(in.ClientMessageID) || core.RequireVersion(in.ExpectedVersion) != nil || core.RequireVersion(in.ContextSecurityVersion) != nil || len([]rune(strings.TrimSpace(in.Message))) < 1 || len([]rune(in.Message)) > 20000 {
		return core.Result{}, core.Invalid("message", "请核对会话版本、执行配置及1至20000字消息")
	}
	authorize := func(tx pgx.Tx) error {
		c, err := metadata(ctx, tx, command.Actor.OrgID, id, false)
		if err != nil {
			return err
		}
		if command.Actor.RunID != "" || command.Actor.Channel == "telegram" || c.Type == "telegram" {
			return core.Forbidden()
		}
		if err = require(ctx, tx, command.Actor, c, "agent.run"); err != nil {
			return err
		}
		_, err = aiAudience(ctx, tx, command.Actor, c)
		return err
	}
	result, err := s.Commands.Do(ctx, command, authorize, func(tx pgx.Tx) (core.Result, error) {
		c, err := metadata(ctx, tx, command.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if c.State != "active" {
			return core.Result{}, core.Fail(409, "CONVERSATION_READ_ONLY", "请先恢复会话再发送消息")
		}
		if c.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if c.ContextSecurityVersion != in.ContextSecurityVersion {
			return core.Result{}, core.Fail(409, "CONTEXT_SECURITY_CHANGED", "会话成员范围已变化，请重新核对资料")
		}
		// Commands already holds the organization shared lock. Profile retirement
		// takes its exclusive counterpart; the application only needs SELECT.
		profile, err := scanProfile(tx.QueryRow(ctx, "SELECT "+profileColumns+" FROM agent_runtime_profiles WHERE org_id=$1 AND id=$2 AND state='active'", command.Actor.OrgID, in.ProfileID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) || core.PublicError(err).Status == 404 {
				return core.Result{}, core.Fail(503, "RUNTIME_UNAVAILABLE", "秘书执行服务尚未配置或当前配置已停用")
			}
			return core.Result{}, err
		}
		if in.SkillID != "" && in.SkillID != profile.SkillID {
			return core.Result{}, core.Invalid("skill_id", "技能与所选执行配置不一致")
		}
		if (in.FileUpdate != nil || in.FileCreate != nil || in.FileRecovery != nil) && profile.SkillID != "file-update" {
			return core.Result{}, core.Invalid("file_update", "准确文件更新需选择文件更新任务")
		}
		if in.FileCreate != nil && (in.FileUpdate != nil || in.Mutation != nil) {
			return core.Result{}, core.Invalid("file_create", "每次发送只能选择一项准确文件新建或更新")
		}
		if in.ContractBody != nil && (profile.SkillID != "file-update" || in.FileCreate == nil || in.FileUpdate != nil || in.FileRecovery != nil || in.Mutation != nil || in.OARequest != nil || in.PostWrite != nil) {
			return core.Result{}, core.Invalid("contract_body", "合同完整正文需明确绑定一项新建文件授权")
		}
		if in.FileRecovery != nil && (in.FileUpdate != nil || in.FileCreate != nil || in.Mutation != nil || in.OARequest != nil || in.PostWrite != nil) {
			return core.Result{}, core.Invalid("file_recovery", "恢复草稿需作为本次唯一的准确写入授权")
		}
		if in.PostWrite != nil {
			if profile.SkillID != "project-briefing" || in.Mutation != nil || in.FileUpdate != nil || in.FileCreate != nil || in.OARequest != nil {
				return core.Result{}, core.Invalid("post_write", "每次只向项目简报任务授权一项知识草稿或准确发布")
			}
			if err = validatePostWrite(*in.PostWrite); err != nil {
				return core.Result{}, err
			}
		}
		if in.OARequest != nil {
			if profile.SkillID != "oa-assist" || in.Mutation != nil || in.FileUpdate != nil || in.FileCreate != nil {
				return core.Result{}, core.Invalid("oa_request", "每次只向OA助手明确授权一项准确申请操作")
			}
			canonical, e := parseOARequest(*in.OARequest)
			if e != nil {
				return core.Result{}, e
			}
			in.OARequest.Arguments = canonical
		}
		var mutationBase mutationArguments
		var mutationKind string
		if in.Mutation != nil {
			if profile.SkillID != "project-update" {
				return core.Result{}, core.Invalid("mutation", "准确更新只能交给更新项目任务")
			}
			var canonical json.RawMessage
			mutationBase, mutationKind, canonical, err = parseMutation(*in.Mutation)
			if err != nil {
				return core.Result{}, err
			}
			in.Mutation.Arguments = canonical
		}
		for _, ref := range in.ContextRefs {
			if (ref.Type == "agent_organization_plan" || ref.Type == "agent_directory") && profile.SkillID != "information-organize" {
				return core.Result{}, core.Invalid("context_refs", "整理计划与目录观察只能交给资料整理任务核对")
			}
			if ref.Type == "agent_contract_body" || ref.Type == "agent_mutation" || ref.Type == "agent_file_update" || ref.Type == "agent_file_create" || ref.Type == "agent_oa_request" || ref.Type == "agent_post_write" {
				return core.Result{}, core.Invalid("context_refs", "更新授权由本次发送建立，历史引用不能代替授权")
			}
			if in.Mutation != nil && ref.Type == mutationKind && ref.ID == mutationBase.ID {
				return core.Result{}, core.Invalid("context_refs", "更新对象已包含在本次固定更新中，请移除重复资料引用")
			}
			if ref.Type == "agent_plan" && profile.SkillID != "project-create" {
				return core.Result{}, core.Invalid("context_refs", "项目创建计划只能交给项目创建任务核对")
			}
		}
		members, err := aiAudience(ctx, tx, command.Actor, c)
		if err != nil {
			return core.Result{}, err
		}
		snapshots, err := sourceSnapshots(ctx, tx, command.Actor, c, in.ContextRefs, members, true)
		if err != nil {
			return core.Result{}, err
		}
		inputParts := []any{in.Message, in.ProfileID, in.ExpectedVersion, in.ContextSecurityVersion, snapshots}
		if in.Mutation != nil {
			inputParts = append(inputParts, in.Mutation)
		}
		if in.FileUpdate != nil {
			inputParts = append(inputParts, in.FileUpdate)
		}
		if in.FileCreate != nil {
			inputParts = append(inputParts, in.FileCreate)
		}
		if in.FileRecovery != nil {
			inputParts = append(inputParts, in.FileRecovery)
		}
		if in.ContractBody != nil {
			inputParts = append(inputParts, in.ContractBody)
		}
		if in.OARequest != nil {
			inputParts = append(inputParts, in.OARequest)
		}
		if in.PostWrite != nil {
			inputParts = append(inputParts, in.PostWrite)
		}
		payload, _ := json.Marshal(inputParts)
		hash := core.Digest(payload)
		old, e := scanRun(tx.QueryRow(ctx, "SELECT "+runColumns+" FROM agent_runs WHERE org_id=$1 AND conversation_id=$2 AND actor_id=$3 AND client_message_id=$4", command.Actor.OrgID, id, command.Actor.UserID, in.ClientMessageID))
		if e == nil {
			var previous string
			if err = tx.QueryRow(ctx, "SELECT input_sha256 FROM agent_runs WHERE id=$1", old.ID).Scan(&previous); err != nil {
				return core.Result{}, err
			}
			if previous != hash {
				return core.Result{}, core.Fail(409, "MESSAGE_ID_CONFLICT", "同一消息标识不能用于不同内容或资料")
			}
			old.CanCancel = old.CancelRequestedAt == nil && (old.State == "queued" || old.State == "running" || old.State == "awaiting_approval")
			if b, e := readContractBody(ctx, tx, command.Actor.OrgID, old.ID); e != nil {
				return core.Result{}, e
			} else if b != nil {
				if _, e = contractBodySource(ctx, tx, command.Actor, c, old.ID, members); e != nil {
					return core.Result{}, e
				}
				view, e := contractBodyViewRead(ctx, tx, command.Actor, b)
				if e != nil {
					return core.Result{}, e
				}
				old.ContractBody = &view
			}
			if p, e := readPostWrite(ctx, tx, command.Actor.OrgID, old.ID); e != nil {
				return core.Result{}, e
			} else if p != nil {
				old.PostWrite = &p.PostWriteView
			}
			if r, e := readOARequestRecord(ctx, tx, command.Actor.OrgID, old.ID); e != nil {
				return core.Result{}, e
			} else if r != nil {
				old.OARequest = &r.OARequestView
			}
			if m, e := mutationRecordRead(ctx, tx, command.Actor.OrgID, old.ID); e != nil {
				return core.Result{}, e
			} else if m != nil {
				old.Mutation = &m.MutationView
			}
			intent, e := fileIntentRecordRead(ctx, tx, command.Actor.OrgID, old.ID)
			if e != nil {
				return core.Result{}, e
			}
			if intent != nil {
				view, e := fileUpdateViewRead(ctx, tx, intent)
				if e != nil {
					return core.Result{}, e
				}
				assignFileIntent(&old, intent, view)
			}
			return core.Result{Status: 202, Data: old, Event: core.Event{Type: "agent.run.reconciled", ResourceType: "agent_run", ResourceID: old.ID, Version: old.Version, Payload: map[string]any{"conversation_id": id}}}, nil
		}
		if core.PublicError(e).Status != 404 {
			return core.Result{}, e
		}
		if err = checkRunCapacity(ctx, tx, command.Actor.OrgID, command.Actor.UserID, id); err != nil {
			return core.Result{}, err
		}
		runID := core.NewID()
		var postWrite *postWriteRecord
		if in.PostWrite != nil {
			if len(snapshots) >= 20 {
				return core.Result{}, core.Invalid("context_refs", "知识授权需要保留一项资料位置")
			}
			postWrite, err = preparePostWrite(ctx, tx, command.Actor, c, runID, *in.PostWrite, in.Message)
			if err != nil {
				return core.Result{}, err
			}
			if err = postWriteReferences(ctx, tx, command.Actor, postWrite, snapshots); err != nil {
				return core.Result{}, err
			}
			snapshots = append(snapshots, postWriteSnapshot(postWrite))
		}
		var oaRequest *oaRequestRecord
		if in.OARequest != nil {
			if len(snapshots) >= 20 {
				return core.Result{}, core.Invalid("context_refs", "准确OA操作需要保留一项资料位置")
			}
			oaRequest, err = prepareOARequest(ctx, tx, command.Actor, c, runID, *in.OARequest, members, in.Message)
			if err != nil {
				return core.Result{}, err
			}
			snapshots = append(snapshots, oaRequestSnapshot(oaRequest))
		}
		var fileIntent *fileIntentRecord
		if in.FileUpdate != nil || in.FileCreate != nil || in.FileRecovery != nil {
			if len(snapshots) >= 20 {
				return core.Result{}, core.Invalid("context_refs", "准确文件更新需要保留一项资料位置")
			}
			if in.FileRecovery != nil {
				fileIntent, err = s.prepareFileRecoveryIntent(ctx, tx, command.Actor, c, runID, *in.FileRecovery, members, in.Message)
			} else if in.FileCreate != nil {
				fileIntent, err = prepareFileCreateIntent(ctx, tx, command.Actor, c, runID, *in.FileCreate, members, in.Message)
			} else {
				fileIntent, err = prepareFileIntent(ctx, tx, command.Actor, c, runID, *in.FileUpdate, members, in.Message)
			}
			if err != nil {
				return core.Result{}, err
			}
			snapshots = append(snapshots, fileUpdateSource(fileIntent))
		}
		var mutation *mutationRecord
		var contractBody *contractBodyRecord
		if in.ContractBody != nil {
			if len(snapshots) >= 20 {
				return core.Result{}, core.Invalid("context_refs", "完整合同正文需要保留一项依据位置")
			}
			contractBody, err = prepareContractBody(ctx, tx, command.Actor, c, runID, *in.ContractBody, fileIntent, in.Message)
			if err != nil {
				return core.Result{}, err
			}
			snapshots = append(snapshots, contractBodySnapshot(contractBody))
		}
		if in.Mutation != nil {
			if len(snapshots) >= 20 {
				return core.Result{}, core.Invalid("context_refs", "固定更新需要保留一项资料位置")
			}
			mutation, err = prepareMutation(ctx, tx, command.Actor, c, runID, *in.Mutation, members)
			if err != nil {
				return core.Result{}, err
			}
			snapshots = append(snapshots, SourceSnapshot{SourceReference: SourceReference{Type: "agent_mutation", ID: runID}, ResourceVersion: 1, SHA256: mutation.IntentSHA256})
		}
		snapJSON, _ := json.Marshal(snapshots)
		_, err = tx.Exec(ctx, `INSERT INTO agent_runs(id,org_id,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,channel,profile_id,client_message_id,input_sha256,audience_ids,state,expires_at)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'queued',clock_timestamp()+$14::integer*interval '1 second')`, runID, command.Actor.OrgID, id, c.CurrentRevisionID, c.ContextSecurityVersion, command.Actor.UserID, command.Actor.AuthVersion, command.Actor.OrgAuthVersion, command.Actor.Channel, profile.ID, in.ClientMessageID, hash, members, profile.TimeoutSeconds)
		if err != nil {
			return core.Result{}, err
		}
		if postWrite != nil {
			if err = insertPostWrite(ctx, tx, postWrite); err != nil {
				return core.Result{}, err
			}
		}
		if mutation != nil {
			if err = insertMutation(ctx, tx, mutation); err != nil {
				return core.Result{}, err
			}
		}
		if oaRequest != nil {
			if err = insertOARequest(ctx, tx, oaRequest); err != nil {
				return core.Result{}, err
			}
		}
		if fileIntent != nil {
			if err = insertFileIntent(ctx, tx, fileIntent); err != nil {
				return core.Result{}, err
			}
		}
		if contractBody != nil {
			if err = insertContractBody(ctx, tx, contractBody); err != nil {
				return core.Result{}, err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO conversation_messages(id,org_id,conversation_id,run_id,seq,role,body,author_id,audience_ids,source_snapshots)
   SELECT $1,$2,$3,$4,coalesce(max(seq),0)+1,'user',$5,$6,$7,$8 FROM conversation_messages WHERE org_id=$2 AND conversation_id=$3`, core.NewID(), command.Actor.OrgID, id, runID, in.Message, command.Actor.UserID, members, snapJSON)
		if err != nil {
			return core.Result{}, err
		}
		if err = s.enqueueRun(ctx, tx, command.Actor.OrgID, runID); err != nil {
			return core.Result{}, err
		}
		r, err := runRecord(ctx, tx, command.Actor.OrgID, runID, false)
		if err != nil {
			return core.Result{}, err
		}
		r.CanCancel = true
		if contractBody != nil {
			r.ContractBody = &contractBody.ContractBodyView
		}
		if oaRequest != nil {
			r.OARequest = &oaRequest.OARequestView
		}
		if postWrite != nil {
			r.PostWrite = &postWrite.PostWriteView
		}
		if fileIntent != nil {
			assignFileIntent(&r, fileIntent, fileIntent.FileUpdateView)
		}
		if mutation != nil {
			r.Mutation = &mutation.MutationView
		}
		return core.Result{Status: 202, Data: r, Event: core.Event{Type: "agent.run.queued", ResourceType: "agent_run", ResourceID: runID, Version: 1, Payload: map[string]any{"conversation_id": id, "profile_id": profile.ID, "context_security_version": c.ContextSecurityVersion}}}, nil
	})
	if err == nil && result.Replayed {
		var r Run
		if e := json.Unmarshal(result.Data.(json.RawMessage), &r); e != nil {
			return core.Result{}, e
		}
		r, e := s.GetRun(ctx, command.Actor, r.ID)
		if e != nil {
			return core.Result{}, e
		}
		result.Data = r
	}
	return result, err
}
func appendRunEvent(ctx context.Context, tx pgx.Tx, org, id string, seq int64, kind, state string, code *string) error {
	_, err := tx.Exec(ctx, "INSERT INTO agent_run_events(id,org_id,run_id,seq,type,state,safe_code) VALUES($1,$2,$3,$4,$5,$6,$7)", core.NewID(), org, id, seq, kind, state, code)
	return err
}
