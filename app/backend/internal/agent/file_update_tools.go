package agent

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type executionIO struct {
	FileRead *fileReadPrepared
	Draft    *fileDraftPrepared
}
type fileDraftPrepared struct {
	Base      fileReadPrepared
	Edits     []files.AgentTextEdit
	Text      string
	Diff      files.AgentTextDiff
	Upload    files.Upload
	Multipart objectstore.Multipart
	Part      objectstore.Part
	Phase     int
	Finalized bool
}
type fileDraftArguments struct {
	FileID        string                `json:"file_id"`
	BaseVersionID string                `json:"base_version_id"`
	Edits         []files.AgentTextEdit `json:"edits"`
	FolderID      string                `json:"folder_id,omitempty"`
	Name          string                `json:"name,omitempty"`
	Content       *string               `json:"content,omitempty"`
}
type fileProposalArguments struct {
	FileID        string `json:"file_id"`
	BaseVersionID string `json:"base_version_id"`
	DraftID       string `json:"draft_id"`
	PlanHash      string `json:"plan_hash,omitempty"`
	FolderID      string `json:"folder_id,omitempty"`
	Name          string `json:"name,omitempty"`
}

func fileUpdateTool(tool string) bool {
	return tool == "create_file_draft" || tool == "propose_file_update" || tool == "commit_file_update" || tool == "stage_file_create" || tool == "propose_file_create" || tool == "commit_file_create"
}
func fileUpdateSource(i *fileIntentRecord) SourceSnapshot {
	return SourceSnapshot{SourceReference: SourceReference{Type: fileIntentSourceType(i), ID: i.RunID}, ResourceVersion: 1, SHA256: i.IntentSHA256}
}
func fileToolResult(i *fileIntentRecord, view FileUpdateView, draftID string, status, summary, next string) map[string]any {
	refs := []ResultReference{}
	if draftID != "" {
		refs = append(refs, ResultReference{Type: "agent_file_draft", ID: draftID})
	}
	if i.OperationID != nil {
		var c files.CommitResult
		_ = json.Unmarshal(i.Result, &c)
		refs = []ResultReference{{Type: "file", ID: c.File.ID, VersionID: &c.Version.ID}}
	}
	var content any = view
	if i.Mode == "create" {
		content = fileCreateView(view)
	}
	return map[string]any{"status": status, "resource_refs": refs, "source_refs": []ResultReference{reference(fileUpdateSource(i))}, "operation_id": i.OperationID, "committed": i.OperationID != nil, "safe_summary": summary, "retryable": false, "next_action": next, "content": content}
}

func (s *Service) createFileDraftStep(ctx context.Context, tx pgx.Tx, st executionState, lease ExecutionLease, key, tool string, args json.RawMessage, inputHash string, work *executionIO) (map[string]any, []SourceSnapshot, json.RawMessage, bool, error) {
	var in fileDraftArguments
	if err := strictArguments(args, &in); err != nil {
		return nil, nil, nil, false, err
	}
	i, err := s.executionFileIntent(ctx, tx, st)
	if err != nil {
		return nil, nil, nil, false, err
	}
	if i == nil || i.OperationID != nil {
		return nil, nil, nil, false, core.Fail(409, "FILE_UPDATE_SCOPE_REQUIRED", "草稿必须匹配本次发送的准确文件更新范围")
	}
	creating := i.Mode == "create"
	if creating && (tool != "stage_file_create" || in.FolderID != i.Target.FolderID || in.Name != i.Target.Name || in.Content == nil || len(in.Edits) != 0 || in.FileID != "" || in.BaseVersionID != "") || !creating && (tool != "create_file_draft" || in.FileID != i.FileID || in.BaseVersionID != i.BaseVersionID || in.Content != nil || in.FolderID != "" || in.Name != "") {
		return nil, nil, nil, false, core.Fail(409, "FILE_UPDATE_SCOPE_REQUIRED", "草稿类型、位置和名称必须匹配本次发送范围")
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	var prepared fileReadPrepared
	if creating {
		prepared.Hash = i.BaseSHA256
		prepared.Source.Fingerprint = i.BaseSHA256
	} else {
		prepared, err = s.prepareFileRead(ctx, tx, ai, st.Conversation, ContextSource{Type: "file", ID: i.FileID, VersionID: &i.BaseVersionID}, st.Run.audience)
	}
	if err != nil {
		return nil, nil, nil, false, err
	}
	if prepared.Hash != i.BaseSHA256 {
		return nil, nil, nil, false, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	if work == nil || work.Draft == nil {
		if err = tx.Rollback(ctx); err != nil {
			return nil, nil, nil, false, err
		}
		var after string
		var diff files.AgentTextDiff
		var e error
		if creating {
			after = *in.Content
			diff, e = files.AgentCreateText(after)
			in.Edits = []files.AgentTextEdit{{Text: after}}
		} else {
			var text string
			text, e = s.FileText.Read(ctx, prepared.Source)
			if e == nil {
				after, diff, e = files.ApplyAgentTextEdits(text, in.Edits)
			}
		}
		if e != nil {
			return nil, nil, nil, false, e
		}
		p := &fileDraftPrepared{Base: prepared, Edits: in.Edits, Text: after, Diff: diff, Phase: 1}
		raw, e := s.executionTool(ctx, lease, key, tool, args, &executionIO{Draft: p})
		return nil, nil, raw, true, e
	}
	p := work.Draft
	if prepared.Hash != p.Base.Hash || prepared.Source.Fingerprint != p.Base.Source.Fingerprint {
		return nil, nil, nil, false, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	if p.Phase == 1 {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM agent_file_drafts WHERE org_id=$1 AND run_id=$2", ai.OrgID, st.Run.ID).Scan(&count); err != nil {
			return nil, nil, nil, false, err
		}
		if count >= 3 {
			return nil, nil, nil, false, core.Fail(409, "FILE_DRAFT_LIMIT", "每次准确更新最多准备3份草稿，请先核对已有草稿")
		}
		id := core.NewID()
		sha := core.Digest([]byte(p.Text))
		edits, _ := json.Marshal(p.Edits)
		diff, _ := json.Marshal(p.Diff)
		_, err = tx.Exec(ctx, `INSERT INTO agent_file_drafts(id,org_id,run_id,invocation_key,input_sha256,edits,content_sha256,size_bytes,diff,source_fingerprint)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, ai.OrgID, st.Run.ID, key, inputHash, edits, sha, len([]byte(p.Text)), diff, p.Base.Source.Fingerprint)
		if err != nil {
			return nil, nil, nil, false, err
		}
		if creating {
			p.Upload, err = s.FileDrafts.ReserveCreate(ctx, tx, ai, id, i.Target.FolderID, i.Target.FolderVersion, i.Target.Name, int64(len(p.Text)), sha, i.Target.MediaType, st.Run.ExpiresAt)
		} else {
			p.Upload, err = s.FileDrafts.Reserve(ctx, tx, ai, id, i.FileID, i.BaseVersionID, int64(len(p.Text)), sha, i.Target.MediaType, st.Run.ExpiresAt)
		}
		if err != nil {
			return nil, nil, nil, false, err
		}
		if err = tx.Commit(ctx); err != nil {
			return nil, nil, nil, false, err
		}
		p.Multipart, p.Part, err = s.FileDrafts.Stage(ctx, p.Upload, p.Text)
		if err != nil {
			return nil, nil, nil, false, err
		}
		defer func() {
			if !p.Finalized {
				s.FileDrafts.Abandon(p.Multipart)
			}
		}()
		p.Phase = 2
		raw, e := s.executionTool(ctx, lease, key, tool, args, work)
		return nil, nil, raw, true, e
	}
	if p.Phase != 2 {
		return nil, nil, nil, false, core.Forbidden()
	}
	if err = s.FileDrafts.Finish(ctx, tx, ai, p.Upload.ID, p.Multipart, p.Part); err != nil {
		return nil, nil, nil, false, err
	}
	source, _ := json.Marshal(map[string]any{"Key": p.Multipart.Key, "UploadID": p.Multipart.UploadID, "ETag": p.Part.ETag, "Size": p.Part.Size})
	if _, err = tx.Exec(ctx, "INSERT INTO agent_file_stages(draft_id,org_id,run_id,source) VALUES($1,$2,$3,$4)", p.Upload.ID, ai.OrgID, st.Run.ID, source); err != nil {
		return nil, nil, nil, false, err
	}
	view, err := fileUpdateViewRead(ctx, tx, i)
	if err != nil {
		return nil, nil, nil, false, err
	}
	next := "使用propose_file_update核对服务端实际差异；扫描及正式候选就绪后才能提交"
	if creating {
		next = "使用propose_file_create核对新建正文；扫描及正式候选就绪后才能提交"
	}
	return fileToolResult(i, view, p.Upload.ID, "succeeded", "草稿已写入隔离区，后台继续核验与扫描", next), []SourceSnapshot{fileUpdateSource(i)}, nil, false, nil
}

func (s *Service) executionFileProposal(ctx context.Context, tx pgx.Tx, st executionState, tool string, args json.RawMessage) (map[string]any, []SourceSnapshot, error) {
	var in fileProposalArguments
	if err := strictArguments(args, &in); err != nil {
		return nil, nil, err
	}
	i, err := s.executionFileIntent(ctx, tx, st)
	if err != nil {
		return nil, nil, err
	}
	if i == nil || in.FileID != i.FileID || in.BaseVersionID != i.BaseVersionID || !core.ValidID(in.DraftID) {
		return nil, nil, core.Fail(409, "FILE_UPDATE_SCOPE_REQUIRED", "计划必须匹配本次准确文件及其草稿")
	}
	creating := i.Mode == "create"
	if creating && (tool != "propose_file_create" && tool != "commit_file_create" || in.FolderID != i.Target.FolderID || in.Name != i.Target.Name) || !creating && (tool != "propose_file_update" && tool != "commit_file_update" || in.FolderID != "" || in.Name != "") {
		return nil, nil, core.Fail(409, "FILE_UPDATE_SCOPE_REQUIRED", "提交类型、位置和名称必须匹配本次发送范围")
	}
	var sha, size string
	var diff, stage []byte
	var uploadState, scan string
	var uploadVersion int64
	err = tx.QueryRow(ctx, `SELECT d.content_sha256,d.size_bytes::text,d.diff,s.source,u.state,u.scan_state,u.version
 FROM agent_file_drafts d JOIN agent_file_stages s ON s.org_id=d.org_id AND s.draft_id=d.id JOIN upload_sessions u ON u.org_id=d.org_id AND u.id=d.id
 WHERE d.org_id=$1 AND d.run_id=$2 AND d.id=$3`, st.Actor.OrgID, st.Run.ID, in.DraftID).Scan(&sha, &size, &diff, &stage, &uploadState, &scan, &uploadVersion)
	if err != nil {
		return nil, nil, platform.DatabaseError(err)
	}
	var planHash string
	if tool == "propose_file_update" || tool == "propose_file_create" {
		if in.PlanHash != "" {
			return nil, nil, core.Invalid("plan_hash", "拟定计划时不接收调用者自报摘要")
		}
		plan, _ := json.Marshal(map[string]any{"run_id": i.RunID, "file_id": i.FileID, "base_version_id": i.BaseVersionID, "file_version": i.FileVersion, "draft_id": in.DraftID, "intent_sha256": i.IntentSHA256, "content_sha256": sha, "size_bytes": size, "diff": json.RawMessage(diff), "target": i.Target, "stage_sha256": core.Digest(stage)})
		planHash = core.Digest(plan)
		if i.OperationID == nil {
			if _, err = tx.Exec(ctx, "INSERT INTO agent_file_proposals(draft_id,org_id,run_id,plan,plan_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(draft_id) DO NOTHING", in.DraftID, i.OrgID, i.RunID, string(plan), planHash); err != nil {
				return nil, nil, err
			}
		}
	} else if tool == "commit_file_update" || tool == "commit_file_create" {
		if err = tx.QueryRow(ctx, "SELECT plan_hash FROM agent_file_proposals WHERE org_id=$1 AND run_id=$2 AND draft_id=$3", i.OrgID, i.RunID, in.DraftID).Scan(&planHash); err != nil {
			return nil, nil, platform.DatabaseError(err)
		}
		if in.PlanHash != planHash {
			return nil, nil, core.Fail(409, "PLAN_HASH_MISMATCH", "提交摘要与服务端固定草稿计划不一致")
		}
		if i.OperationID != nil {
			var committedDraft string
			if err = tx.QueryRow(ctx, "SELECT draft_id FROM agent_file_commits WHERE org_id=$1 AND run_id=$2", i.OrgID, i.RunID).Scan(&committedDraft); err != nil {
				return nil, nil, err
			}
			if committedDraft != in.DraftID {
				return nil, nil, core.Fail(409, "FILE_UPDATE_ALREADY_COMMITTED", "本次更新已提交另一份准确草稿")
			}
		} else if uploadState == "ready" && scan == "clean" {
			if s.FileDrafts == nil {
				return nil, nil, core.Fail(503, "AGENT_DRAFT_WRITER_UNAVAILABLE", "秘书草稿身份未配置")
			}
			ai := st.Actor
			ai.RunID = st.Run.ID
			business := files.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
			upload := files.UploadService{Files: &business, Backend: &files.UploadBackend{Provider: s.FileDrafts.Provider, Bucket: s.FileDrafts.Bucket}}
			var base *string
			if !creating {
				base = &i.BaseVersionID
			}
			input := files.UploadCommit{ExpectedVersion: uploadVersion, BaseVersionID: base, SHA256: sha}
			body, _ := json.Marshal(input)
			route, prefix := "/internal/agent-files/commit", "agent-file-update:"
			if creating {
				route, prefix = "/internal/agent-files/create", "agent-file-create:"
			}
			result, e := upload.CommitAgentDraft(ctx, core.Command{Actor: ai, RequestID: core.NewID(), Route: route, Key: prefix + i.RunID, Body: body}, in.DraftID, input)
			if e != nil {
				return nil, nil, e
			}
			raw, _ := json.Marshal(result.Data)
			var committed files.CommitResult
			if json.Unmarshal(raw, &committed) != nil || !committed.Committed || committed.Version.SHA256 != sha || !core.ValidID(result.OperationID) {
				return nil, nil, core.Fail(500, "FILE_COMMIT_RECEIPT_INVALID", "文件正式提交回执不完整")
			}
			if _, err = tx.Exec(ctx, "INSERT INTO agent_file_commits(run_id,org_id,draft_id,operation_id,version_id,result_data) VALUES($1,$2,$3,$4,$5,$6)", i.RunID, i.OrgID, in.DraftID, result.OperationID, committed.Version.ID, raw); err != nil {
				return nil, nil, err
			}
			i, err = fileIntentRecordRead(ctx, tx, i.OrgID, i.RunID)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	view, err := fileUpdateViewRead(ctx, tx, i)
	if err != nil {
		return nil, nil, err
	}
	status, summary, next := "succeeded", "已固定服务端差异及准确草稿计划", "校验、扫描与正式候选就绪后，按准确draft_id和plan_hash提交"
	if tool == "commit_file_update" || tool == "commit_file_create" {
		status, summary, next = "pending", "正式版本尚未提交，后台仍在核验草稿", "使用inspect_operation查看扫描和候选状态；就绪后用新的调用键重试同一准确计划"
		if uploadState == "blocked" || uploadState == "failed" || uploadState == "aborted" || uploadState == "expired" {
			status, summary, next = "failed", "草稿被阻断或已失效，未生成正式版本", "保留原版本和差异，由员工核对错误及后续处理"
		}
		if i.OperationID != nil {
			status, summary, next = "succeeded", "文件正式新版本已提交，第二存储副本继续按实际状态复制", ""
		}
	}
	return fileToolResult(i, view, in.DraftID, status, summary, next), []SourceSnapshot{fileUpdateSource(i)}, nil
}
