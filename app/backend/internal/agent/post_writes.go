package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/knowledge"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// PostWriteInput delegates one bounded draft or one reviewed publication.
// Generated body/summary/tags are permitted only by the draft branch.
type PostWriteInput struct {
	Tool            string             `json:"tool"`
	PostID          *string            `json:"post_id,omitempty"`
	ExpectedVersion *int64             `json:"expected_version,omitempty"`
	BaseRevisionID  *string            `json:"base_revision_id,omitempty"`
	ContentHash     *string            `json:"content_hash,omitempty"`
	Type            string             `json:"type,omitempty"`
	Title           string             `json:"title,omitempty"`
	Classification  string             `json:"classification,omitempty"`
	SourceRefs      []knowledge.Source `json:"source_refs,omitempty"`
	Reason          string             `json:"reason"`
}
type postScope struct {
	ProjectID      *string `json:"project_id"`
	Version        int64   `json:"version"`
	State          string  `json:"state"`
	Classification string  `json:"classification"`
}
type PostWriteBasis struct {
	Scope       postScope          `json:"scope"`
	Post        *knowledge.Post    `json:"post"`
	Attachments []fileref.Evidence `json:"attachments"`
}
type PostWriteView struct {
	RunID        string          `json:"run_id"`
	Input        PostWriteInput  `json:"input"`
	Before       PostWriteBasis  `json:"before"`
	IntentSHA256 string          `json:"intent_sha256"`
	Status       string          `json:"status"`
	OperationID  *string         `json:"operation_id"`
	Result       *knowledge.Post `json:"result"`
}
type postWriteRecord struct {
	PostWriteView
	OrgID, ActorID, ConversationID, SessionID, BaseSHA256, Intent string
	CommitArguments                                               json.RawMessage
}
type postWriteArguments struct {
	RunID        string    `json:"run_id"`
	IntentSHA256 string    `json:"intent_sha256"`
	Body         *string   `json:"body,omitempty"`
	Summary      *string   `json:"summary,omitempty"`
	Tags         *[]string `json:"tags,omitempty"`
}

func postWriteTool(tool string) bool { return tool == "draft_post" || tool == "publish_post" }

func validatePostWrite(in PostWriteInput) error {
	if !postWriteTool(in.Tool) || core.Name(in.Reason) != nil {
		return core.Invalid("post_write", "请指定知识操作与说明")
	}
	hasBase := in.PostID != nil && in.ExpectedVersion != nil && in.BaseRevisionID != nil && in.ContentHash != nil
	if (in.PostID != nil || in.ExpectedVersion != nil || in.BaseRevisionID != nil || in.ContentHash != nil) && !hasBase {
		return core.Invalid("post_write", "请完整核对原文档、版本、修订和摘要")
	}
	if hasBase && (!exactOAID(*in.PostID) || !exactOAID(*in.BaseRevisionID) || core.RequireVersion(*in.ExpectedVersion) != nil || !oaDigestPattern.MatchString(*in.ContentHash)) {
		return core.Invalid("post_write", "知识基础版本无效")
	}
	if in.Tool == "publish_post" {
		if !hasBase || in.Type != "" || in.Title != "" || in.Classification != "" || len(in.SourceRefs) > 0 {
			return core.Invalid("post_write", "内部发布只能引用已核对的完整修订")
		}
	} else {
		if !slices.Contains([]string{"project_log", "meeting_note", "sop", "article"}, in.Type) || core.Name(in.Title) != nil || in.Title != strings.TrimSpace(in.Title) || !slices.Contains([]string{"internal", "project_restricted"}, in.Classification) || len(in.SourceRefs) > 20 {
			return core.Invalid("post_write", "请核对文档类型、标题、分类和固定附件")
		}
	}
	seen := map[string]bool{}
	for _, ref := range in.SourceRefs {
		if ref.Type != "file" || !exactOAID(ref.ID) || ref.VersionID == nil || !exactOAID(*ref.VersionID) || seen[*ref.VersionID] {
			return core.Invalid("source_refs", "秘书知识附件须为不重复的准确正式文件版本")
		}
		seen[*ref.VersionID] = true
	}
	return nil
}
func postValue(in PostWriteInput, project *string, base *knowledge.Post) knowledge.Create {
	if in.Tool == "publish_post" && base != nil {
		return knowledge.Create{ProjectID: base.ProjectID, Type: base.Type, Title: base.Title, Classification: base.Classification, Body: base.Body, Summary: base.Summary, Tags: base.Tags, SourceRefs: base.SourceRefs}
	}
	return knowledge.Create{ProjectID: project, Type: in.Type, Title: in.Title, Classification: in.Classification, SourceRefs: in.SourceRefs}
}
func postAttachments(ctx context.Context, tx pgx.Tx, a core.Actor, project *string, class string, refs []knowledge.Source) ([]fileref.Evidence, error) {
	out := []fileref.Evidence{}
	if len(refs) > 20 {
		return nil, core.Invalid("source_refs", "本次最多20个固定附件")
	}
	if err := knowledge.AgentSources(ctx, tx, a, refs); err != nil {
		return nil, err
	}
	for _, ref := range refs {
		// Following-current references cannot silently change after authorization.
		if ref.Type != "file" || ref.VersionID == nil {
			return nil, core.Invalid("source_refs", "请先将所有附件固定到准确正式版本")
		}
		e, err := fileref.Resolve(ctx, tx, a, *ref.VersionID, project, class)
		if err != nil {
			return nil, err
		}
		if e.FileID != ref.ID {
			return nil, core.Missing()
		}
		out = append(out, e)
	}
	return out, nil
}
func postBasis(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in PostWriteInput, writable bool) (PostWriteBasis, error) {
	out := PostWriteBasis{Scope: postScope{ProjectID: c.ProjectID, Classification: "internal", State: "company"}, Attachments: []fileref.Evidence{}}
	if c.Type != "private" || c.CreatorID != a.UserID {
		return out, core.Missing()
	}
	if c.ProjectID != nil {
		var owner string
		if err := tx.QueryRow(ctx, "SELECT version,state,classification,owner_id FROM projects WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, *c.ProjectID).Scan(&out.Scope.Version, &out.Scope.State, &out.Scope.Classification, &owner); err != nil {
			return out, platform.DatabaseError(err)
		}
		if err := resourceAudience(ctx, tx, a, c, contextDocument{Type: "project", ID: *c.ProjectID, ProjectID: c.ProjectID, Creator: owner, Class: out.Scope.Classification}, []string{a.UserID}); err != nil {
			return out, err
		}
	}
	if in.PostID != nil {
		p, err := knowledge.AgentPost(ctx, tx, a, *in.PostID)
		if err != nil {
			return out, err
		}
		if !sameProject(p.ProjectID, c.ProjectID) || classRank(p.Classification) > classRank(c.Classification) {
			return out, core.Missing()
		}
		out.Post = &p
		refs, err := postAttachments(ctx, tx, a, p.ProjectID, p.Classification, p.SourceRefs)
		if err != nil {
			return out, err
		}
		out.Attachments = append(out.Attachments, refs...)
		if in.Tool == "draft_post" && (p.Type != in.Type || p.Classification != in.Classification) {
			return out, core.Invalid("post_write", "正文修订不能改变文档类型、项目或分类")
		}
	}
	value := postValue(in, c.ProjectID, out.Post)
	if classRank(value.Classification) == 0 || classRank(value.Classification) > classRank(c.Classification) || classRank(value.Classification) < classRank(out.Scope.Classification) {
		return out, core.Missing()
	}
	refs, err := postAttachments(ctx, tx, a, value.ProjectID, value.Classification, value.SourceRefs)
	if err != nil {
		return out, err
	}
	out.Attachments = append(out.Attachments, refs...)
	if writable {
		if err = knowledge.AgentPermission(ctx, tx, a, value, out.Post, in.Tool == "publish_post"); err != nil {
			return out, err
		}
	}
	return out, nil
}
func preparePostWrite(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, runID string, in PostWriteInput, message string) (*postWriteRecord, error) {
	if err := validatePostWrite(in); err != nil {
		return nil, err
	}
	if err := identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return nil, err
	}
	ai := a
	ai.RunID = runID
	basis, err := postBasis(ctx, tx, ai, c, in, true)
	if err != nil {
		return nil, err
	}
	if in.PostID != nil && (basis.Post == nil || basis.Post.Version != *in.ExpectedVersion || basis.Post.CurrentRevisionID != *in.BaseRevisionID || basis.Post.ContentHash != *in.ContentHash) {
		return nil, core.Conflict()
	}
	if in.Tool == "publish_post" && (strings.TrimSpace(basis.Post.Body) == "" || !basis.Post.HasUnpublishedChanges) {
		return nil, core.Invalid("post_write", "请选择有完整未发布修订的文档")
	}
	raw, _ := json.Marshal(basis)
	r := &postWriteRecord{PostWriteView: PostWriteView{RunID: runID, Input: in, Before: basis, Status: "authorized"}, OrgID: a.OrgID, ActorID: a.UserID, ConversationID: c.ID, SessionID: a.SessionID, BaseSHA256: core.Digest(raw)}
	intent, _ := json.Marshal(map[string]any{"run_id": runID, "actor_id": a.UserID, "actor_auth_version": a.AuthVersion, "organization_auth_version": a.OrgAuthVersion, "conversation_id": c.ID, "conversation_revision_id": c.CurrentRevisionID, "context_security_version": c.ContextSecurityVersion, "session_id": a.SessionID, "input": in, "before": basis, "base_sha256": r.BaseSHA256, "request": message})
	if len(intent) > 524288 {
		return nil, core.Invalid("post_write", "知识授权资料超过512KiB，请减少附件或缩短原文后重试")
	}
	r.Intent = string(intent)
	r.IntentSHA256 = core.Digest(intent)
	return r, nil
}
func insertPostWrite(ctx context.Context, tx pgx.Tx, r *postWriteRecord) error {
	input, _ := json.Marshal(r.Input)
	before, _ := json.Marshal(r.Before)
	_, err := tx.Exec(ctx, `INSERT INTO agent_post_intents(run_id,org_id,actor_id,conversation_id,session_id,tool,post_id,input,before_data,base_sha256,intent,intent_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, r.RunID, r.OrgID, r.ActorID, r.ConversationID, r.SessionID, r.Input.Tool, r.Input.PostID, input, before, r.BaseSHA256, r.Intent, r.IntentSHA256)
	return err
}
func readPostWrite(ctx context.Context, tx pgx.Tx, org, id string) (*postWriteRecord, error) {
	r := &postWriteRecord{}
	var input, before, result []byte
	err := tx.QueryRow(ctx, `SELECT i.run_id,i.org_id,i.actor_id,i.conversation_id,i.session_id,i.input,i.before_data,i.base_sha256,i.intent,i.intent_sha256,c.operation_id,c.result_data,c.arguments FROM agent_post_intents i LEFT JOIN agent_post_commits c ON c.org_id=i.org_id AND c.run_id=i.run_id WHERE i.org_id=$1 AND i.run_id=$2`, org, id).Scan(&r.RunID, &r.OrgID, &r.ActorID, &r.ConversationID, &r.SessionID, &input, &before, &r.BaseSHA256, &r.Intent, &r.IntentSHA256, &r.OperationID, &result, &r.CommitArguments)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(input, &r.Input); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(before, &r.Before); err != nil {
		return nil, err
	}
	r.Status = "authorized"
	if r.OperationID != nil {
		r.Status = "committed"
		if err = json.Unmarshal(result, &r.Result); err != nil {
			return nil, err
		}
	}
	return r, nil
}
func postWriteSnapshot(r *postWriteRecord) SourceSnapshot {
	return SourceSnapshot{SourceReference: SourceReference{Type: "agent_post_write", ID: r.RunID}, ResourceVersion: 1, SHA256: r.IntentSHA256}
}

// Every input observed by a content-generating run must fit the destination.
// Conversation read permission alone does not authorize declassification or
// copying project material into a company-wide article.
func postWriteReferences(ctx context.Context, tx pgx.Tx, a core.Actor, r *postWriteRecord, snapshots []SourceSnapshot) error {
	class := r.Input.Classification
	if r.Input.Tool == "publish_post" {
		class = r.Before.Post.Classification
	}
	for _, src := range snapshots {
		if src.Type == "agent_post_write" && src.ID == r.RunID {
			continue
		}
		var project *string
		var sourceClass string
		var query string
		switch src.Type {
		case "project":
			query = "SELECT id,classification FROM projects WHERE org_id=$1 AND id=$2"
		case "task":
			query = "SELECT t.project_id,p.classification FROM tasks t JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id WHERE t.org_id=$1 AND t.id=$2"
		case "post":
			query = "SELECT project_id,classification FROM posts WHERE org_id=$1 AND id=$2"
		case "file":
			query = "SELECT d.project_id,f.classification FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id WHERE f.org_id=$1 AND f.id=$2"
		default:
			return core.Missing()
		}
		if err := tx.QueryRow(ctx, query, a.OrgID, src.ID).Scan(&project, &sourceClass); err != nil {
			return platform.DatabaseError(err)
		}
		if !sameProject(project, r.Before.Scope.ProjectID) || classRank(sourceClass) == 0 || classRank(sourceClass) > classRank(class) {
			return core.Fail(403, "POST_SOURCE_SCOPE_DENIED", "知识来源不得越过目标项目或降低资料分类")
		}
	}
	return nil
}
func postWriteSource(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, id string, members []string) (SourceSnapshot, error) {
	r, err := readPostWrite(ctx, tx, a.OrgID, id)
	if err != nil {
		return SourceSnapshot{}, err
	}
	if r == nil || r.ActorID != a.UserID || r.ConversationID != c.ID || c.Type != "private" || c.CreatorID != a.UserID || len(members) != 1 || members[0] != a.UserID || core.Digest([]byte(r.Intent)) != r.IntentSHA256 {
		return SourceSnapshot{}, core.Missing()
	}
	ai := a
	ai.RunID = r.RunID
	if _, err = postBasis(ctx, tx, ai, c, r.Input, false); err != nil {
		return SourceSnapshot{}, err
	}
	if r.Before.Post != nil {
		if _, err = postAttachments(ctx, tx, ai, r.Before.Post.ProjectID, r.Before.Post.Classification, r.Before.Post.SourceRefs); err != nil {
			return SourceSnapshot{}, err
		}
	}
	if r.Result != nil {
		p, e := knowledge.AgentPost(ctx, tx, ai, r.Result.ID)
		if e != nil {
			return SourceSnapshot{}, e
		}
		if !sameProject(p.ProjectID, c.ProjectID) || classRank(p.Classification) > classRank(c.Classification) {
			return SourceSnapshot{}, core.Missing()
		}
		if _, err = postAttachments(ctx, tx, ai, r.Result.ProjectID, r.Result.Classification, r.Result.SourceRefs); err != nil {
			return SourceSnapshot{}, err
		}
	}
	return postWriteSnapshot(r), nil
}
func executionPostWriteState(ctx context.Context, tx pgx.Tx, st executionState) (*postWriteRecord, error) {
	r, err := readPostWrite(ctx, tx, st.Actor.OrgID, st.Run.ID)
	if err != nil || r == nil {
		return r, err
	}
	if st.Profile.SkillID != "project-briefing" || r.ActorID != st.Actor.UserID || r.ConversationID != st.Conversation.ID {
		return nil, runtimeFailure("POST_WRITE_AUTHORITY_MISMATCH")
	}
	human := st.Actor
	human.SessionID = r.SessionID
	if err = identity.RequireInteractiveSession(ctx, tx, human, false); err != nil {
		return nil, err
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	if _, err = postWriteSource(ctx, tx, ai, st.Conversation, r.RunID, st.Run.audience); err != nil {
		return nil, err
	}
	if r.OperationID == nil {
		basis, e := postBasis(ctx, tx, ai, st.Conversation, r.Input, true)
		if e != nil {
			return nil, e
		}
		raw, _ := json.Marshal(basis)
		if core.Digest(raw) != r.BaseSHA256 {
			return nil, runtimeFailure("RUNTIME_SOURCE_CHANGED")
		}
	}
	return r, nil
}
func (s *Service) executionPostWrite(ctx context.Context, tx pgx.Tx, st executionState, tool string, args json.RawMessage) (map[string]any, []SourceSnapshot, error) {
	r, err := executionPostWriteState(ctx, tx, st)
	if err != nil {
		return nil, nil, err
	}
	if r == nil {
		return nil, nil, runtimeFailure("POST_WRITE_INTENT_REQUIRED")
	}
	var in postWriteArguments
	if err = strictArguments(args, &in); err != nil {
		return nil, nil, err
	}
	if tool != r.Input.Tool || in.RunID != r.RunID || in.IntentSHA256 != r.IntentSHA256 {
		return nil, nil, runtimeFailure("POST_WRITE_INTENT_MISMATCH")
	}
	if tool == "draft_post" {
		if in.Body == nil || in.Summary == nil || in.Tags == nil || strings.TrimSpace(*in.Body) == "" || utf8.RuneCountInString(*in.Body) > 20000 || utf8.RuneCountInString(*in.Summary) > 20000 {
			return nil, nil, core.Invalid("body", "请提供完整有限正文、摘要与标签")
		}
		for _, tag := range *in.Tags {
			if strings.TrimSpace(tag) != tag {
				return nil, nil, core.Invalid("tags", "标签首尾不能包含空白")
			}
		}
	} else if in.Body != nil || in.Summary != nil || in.Tags != nil {
		return nil, nil, core.Invalid("arguments", "内部发布不能夹带正文修改")
	}
	canonical, err := core.CanonicalJSON(args)
	if err != nil {
		return nil, nil, err
	}
	if r.OperationID != nil {
		old, _ := core.CanonicalJSON(r.CommitArguments)
		if string(old) != string(canonical) {
			return nil, nil, runtimeFailure("POST_WRITE_ALREADY_COMMITTED")
		}
	} else {
		ai := st.Actor
		ai.RunID = st.Run.ID
		business := knowledge.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB, Tx: tx}}
		command := core.Command{Actor: ai, RequestID: core.NewID(), Route: "/internal/agent-post/" + tool, Key: "agent-post:" + r.RunID, Body: canonical}
		var result core.Result
		if tool == "draft_post" {
			value := postValue(r.Input, st.Conversation.ProjectID, r.Before.Post)
			value.Body = *in.Body
			value.Summary = *in.Summary
			value.Tags = *in.Tags
			if r.Input.PostID == nil {
				result, err = business.Create(ctx, command, value)
			} else {
				result, err = business.Update(ctx, command, *r.Input.PostID, knowledge.Patch{ExpectedVersion: *r.Input.ExpectedVersion, BaseRevisionID: *r.Input.BaseRevisionID, Title: &value.Title, Body: &value.Body, Summary: &value.Summary, Tags: &value.Tags, SourceRefs: &value.SourceRefs})
			}
		} else {
			result, err = business.Transition(ctx, command, *r.Input.PostID, knowledge.Transition{ExpectedVersion: *r.Input.ExpectedVersion, BaseRevisionID: *r.Input.BaseRevisionID, Action: "publish", Reason: r.Input.Reason})
		}
		if err != nil {
			return nil, nil, err
		}
		raw, err := json.Marshal(result.Data)
		if err != nil {
			return nil, nil, err
		}
		var p knowledge.Post
		if json.Unmarshal(raw, &p) != nil || !core.ValidID(result.OperationID) || !core.ValidID(p.ID) || p.CurrentRevisionID == "" || p.ContentHash == "" {
			return nil, nil, runtimeFailure("POST_WRITE_RECEIPT_INVALID")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO agent_post_commits(run_id,org_id,operation_id,post_id,arguments,result_data) VALUES($1,$2,$3,$4,$5,$6)`, r.RunID, r.OrgID, result.OperationID, p.ID, canonical, raw); err != nil {
			return nil, nil, err
		}
		r, err = readPostWrite(ctx, tx, r.OrgID, r.RunID)
		if err != nil {
			return nil, nil, err
		}
	}
	ai := st.Actor
	ai.RunID = st.Run.ID
	src, err := postWriteSource(ctx, tx, ai, st.Conversation, r.RunID, st.Run.audience)
	if err != nil {
		return nil, nil, err
	}
	return map[string]any{"status": "succeeded", "committed": true, "operation_id": r.OperationID, "resource_refs": []ResultReference{{Type: "post", ID: r.Result.ID, VersionID: &r.Result.CurrentRevisionID}}, "source_refs": []ResultReference{reference(src)}, "safe_summary": "已保存知识草稿或发布明确核对的内部修订；不发送站外通知", "retryable": false, "next_action": "", "content": postWriteReceipt(r)}, []SourceSnapshot{src}, nil
}
func postWriteReceipt(r *postWriteRecord) any {
	value := map[string]any{"run_id": r.RunID, "tool": r.Input.Tool, "status": r.Status, "operation_id": r.OperationID}
	if r.Result != nil {
		p := r.Result
		value["post_id"] = p.ID
		value["version"] = p.Version
		value["revision_id"] = p.CurrentRevisionID
		value["content_hash"] = p.ContentHash
		value["state"] = p.State
		value["published_revision_id"] = p.PublishedRevisionID
		value["has_unpublished_changes"] = p.HasUnpublishedChanges
	}
	return value
}

func executionPostWriteText(ctx context.Context, tx pgx.Tx, st executionState, id string, version *string, offset, maxChars int) (any, []SourceSnapshot, error) {
	if id != st.Run.ID || version != nil || offset < 0 || offset > 524288 || maxChars < 1 || maxChars > 10000 {
		return nil, nil, core.Invalid("arguments", "请指定本运行知识授权及有限范围")
	}
	r, err := executionPostWriteState(ctx, tx, st)
	if err != nil {
		return nil, nil, err
	}
	if r == nil {
		return nil, nil, core.Missing()
	}
	raw, _ := json.Marshal(r.PostWriteView)
	body := []rune(string(raw))
	if offset > len(body) {
		return nil, nil, core.Invalid("offset", "字符范围无效")
	}
	end := min(offset+maxChars, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	src := postWriteSnapshot(r)
	return map[string]any{"type": "agent_post_write", "id": id, "resource_version": 1, "source_sha256": src.SHA256, "text": string(body[offset:end]), "offset": offset, "next_offset": next, "total_chars": len(body), "untrusted_data": true}, []SourceSnapshot{src}, nil
}
