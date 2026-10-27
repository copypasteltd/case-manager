package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// ContextSource is an explicit business reference, never a URL or a local path.
type ContextSource struct {
	Type                   string  `json:"type"`
	ID                     string  `json:"id"`
	VersionID              *string `json:"version_id"`
	ContextSecurityVersion int64   `json:"context_security_version"`
	Offset                 int     `json:"offset"`
	MaxChars               int     `json:"max_chars"`
}
type ContextView struct {
	ConversationID         string                     `json:"conversation_id"`
	ConversationRevisionID string                     `json:"conversation_revision_id"`
	ContextSecurityVersion int64                      `json:"context_security_version"`
	Type                   string                     `json:"type"`
	ID                     string                     `json:"id"`
	VersionID              *string                    `json:"version_id"`
	ResourceVersion        int64                      `json:"resource_version"`
	Title                  string                     `json:"title"`
	State                  string                     `json:"state"`
	Text                   string                     `json:"text"`
	Offset                 int                        `json:"offset"`
	TotalChars             int                        `json:"total_chars"`
	NextOffset             *int                       `json:"next_offset"`
	SourceSHA256           string                     `json:"source_sha256"`
	ContextSHA256          string                     `json:"context_sha256"`
	Evidence               []fileref.Evidence         `json:"evidence"`
	UntrustedData          bool                       `json:"untrusted_data"`
	ObservedAt             time.Time                  `json:"observed_at"`
	TextIndex              *files.IndexedTextEvidence `json:"text_index,omitempty"`
}
type contextDocument struct {
	ID, Type, Title, State, Body, Class, Creator string
	ProjectID                                    *string
	VersionID                                    *string
	ResourceVersion                              int64
	Evidence                                     []fileref.Evidence
	TextIndex                                    *files.IndexedTextEvidence
}

func sameProject(left, right *string) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}
func classRank(value string) int {
	switch value {
	case "internal":
		return 1
	case "project_restricted":
		return 2
	case "confidential":
		return 3
	}
	return 0
}
func resourceAudience(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, doc contextDocument, members []string) error {
	if !sameProject(c.ProjectID, doc.ProjectID) || classRank(doc.Class) == 0 || classRank(doc.Class) > classRank(c.Classification) {
		return core.Missing()
	}
	var creator *string
	if doc.Creator != "" {
		creator = &doc.Creator
	}
	resourceType := doc.Type
	action := doc.Type + ".read"
	if doc.Type == "agent_finance_report" {
		resourceType = "report"
		action = "report.read"
	}
	r := authz.Resource{Type: resourceType, ID: doc.ID, OwnerID: creator, ProjectID: doc.ProjectID, Classification: doc.Class}
	for _, user := range members {
		target := a
		target.UserID = user
		if err := authz.Require(ctx, tx, target, action, r); err != nil {
			return err
		}
	}
	return nil
}

// Context reads are a separate projection from a person's ordinary Web view.
// Every current conversation participant must independently admit AI processing.
// No native process or model is started here; the digest is not an approval grant.
func (s *Service) Context(ctx context.Context, a core.Actor, id string, in ContextSource) (ContextView, error) {
	var out ContextView
	in.ID = strings.ToLower(in.ID)
	if in.VersionID != nil {
		value := strings.ToLower(*in.VersionID)
		in.VersionID = &value
	}
	maxOffset := 200000
	if in.Type == "file" {
		maxOffset = files.TextOutputMaxBytes
	}
	if !core.ValidID(in.ID) || core.RequireVersion(in.ContextSecurityVersion) != nil || in.Offset < 0 || in.Offset > maxOffset || in.MaxChars < 1 || in.MaxChars > 10000 {
		return out, core.Invalid("context", "请指定准确资料、安全版本及有限字符范围")
	}
	if in.Type == "file" {
		return s.fileContext(ctx, a, id, in)
	}
	if in.Type != "project" && in.Type != "task" && in.Type != "post" && in.Type != "service" && in.Type != "agent_finance_report" {
		return out, core.Invalid("type", "当前仅支持项目、任务和知识文本；文件原件需独立受控读取")
	}
	if in.VersionID != nil && (!core.ValidID(*in.VersionID) || in.Type != "post" && in.Type != "service") {
		return out, core.Invalid("version_id", "仅知识文本可指定准确修订ID")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
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
	if c.State != "active" {
		return out, core.Fail(409, "CONVERSATION_READ_ONLY", "请先恢复会话再准备秘书资料")
	}
	if c.ContextSecurityVersion != in.ContextSecurityVersion {
		return out, core.Fail(409, "CONTEXT_SECURITY_CHANGED", "会话成员范围已变化，请重新核对资料")
	}
	members, err := memberIDs(ctx, tx, a.OrgID, c)
	if err != nil {
		return out, err
	}
	ai := a
	if ai.RunID == "" {
		ai.RunID = core.NewID() // Internal AI-data check, never accepted from a public request.
	}
	for _, user := range members {
		target := ai
		target.UserID = user
		if err = require(ctx, tx, target, c, "conversation.read"); err != nil {
			return out, err
		}
	}
	doc, err := contextDocumentRead(ctx, tx, ai, c, in, members)
	if err != nil {
		return out, err
	}
	if err = resourceAudience(ctx, tx, ai, c, doc, members); err != nil {
		return out, err
	}
	body := []rune(doc.Body)
	if in.Offset > len(body) {
		return out, core.Invalid("offset", "字符起点超出当前固定资料")
	}
	end := min(in.Offset+in.MaxChars, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	if doc.Evidence == nil {
		doc.Evidence = []fileref.Evidence{}
	}
	sourceHash, err := documentDigest(doc)
	if err != nil {
		return out, err
	}
	contextBytes, _ := json.Marshal([]any{core.CursorBinding(a, "context-view"), c.ID, c.CurrentRevisionID, c.ContextSecurityVersion, members, sourceHash, in.Offset, end})
	out = ContextView{ConversationID: c.ID, ConversationRevisionID: c.CurrentRevisionID, ContextSecurityVersion: c.ContextSecurityVersion,
		Type: doc.Type, ID: doc.ID, VersionID: doc.VersionID, ResourceVersion: doc.ResourceVersion, Title: doc.Title, State: doc.State,
		Text: string(body[in.Offset:end]), Offset: in.Offset, TotalChars: len(body), NextOffset: next, SourceSHA256: sourceHash,
		ContextSHA256: core.Digest(contextBytes), Evidence: doc.Evidence, UntrustedData: true, ObservedAt: time.Now().UTC()}
	return out, platform.DatabaseError(tx.Commit(ctx))
}

func documentDigest(doc contextDocument) (string, error) {
	if doc.Evidence == nil {
		doc.Evidence = []fileref.Evidence{}
	}
	value, err := json.Marshal(struct {
		Type, ID, Title, State, Body, Classification string
		Project                                      *string
		Version                                      int64
		Revision                                     *string
		Evidence                                     []fileref.Evidence
	}{doc.Type, doc.ID, doc.Title, doc.State, doc.Body, doc.Class, doc.ProjectID, doc.ResourceVersion, doc.VersionID, doc.Evidence})
	return core.Digest(value), err
}

func contextDocumentRead(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in ContextSource, members []string) (contextDocument, error) {
	d := contextDocument{ID: in.ID, Type: in.Type}
	var err error
	switch in.Type {
	case "agent_finance_report":
		return contextFinanceReport(ctx, tx, a, c, in, members)
	case "service":
		return contextService(ctx, tx, a, c, in, members)
	case "project":
		d.ProjectID = &d.ID
		err = tx.QueryRow(ctx, "SELECT name,scope,state,classification,creator_id,version FROM projects WHERE org_id=$1 AND id=$2", a.OrgID, in.ID).
			Scan(&d.Title, &d.Body, &d.State, &d.Class, &d.Creator, &d.ResourceVersion)
	case "task":
		err = tx.QueryRow(ctx, `SELECT t.title,t.body,t.state,p.classification,t.creator_id,t.project_id,t.version FROM tasks t
            JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id WHERE t.org_id=$1 AND t.id=$2`, a.OrgID, in.ID).
			Scan(&d.Title, &d.Body, &d.State, &d.Class, &d.Creator, &d.ProjectID, &d.ResourceVersion)
	case "deliverable":
		err = tx.QueryRow(ctx, `SELECT d.name,jsonb_build_object('name',d.name,'acceptance_criteria',d.acceptance_criteria,'owner_id',d.owner_id,'milestone_id',d.milestone_id,'state',d.state,'protocol_version',d.protocol_version,'current_revision_id',d.current_revision_id)::text,d.state,p.classification,coalesce(d.creator_id,p.creator_id),d.project_id,d.version
 FROM deliverables d JOIN projects p ON p.org_id=d.org_id AND p.id=d.project_id WHERE d.org_id=$1 AND d.id=$2`, a.OrgID, in.ID).
			Scan(&d.Title, &d.Body, &d.State, &d.Class, &d.Creator, &d.ProjectID, &d.ResourceVersion)
	case "milestone":
		err = tx.QueryRow(ctx, `SELECT m.title,jsonb_build_object('title',m.title,'owner_id',m.owner_id,'due_at',m.due_at,'state',m.state)::text,m.state,p.classification,m.creator_id,m.project_id,m.version
 FROM milestones m JOIN projects p ON p.org_id=m.org_id AND p.id=m.project_id WHERE m.org_id=$1 AND m.id=$2`, a.OrgID, in.ID).
			Scan(&d.Title, &d.Body, &d.State, &d.Class, &d.Creator, &d.ProjectID, &d.ResourceVersion)
	case "risk", "issue":
		err = tx.QueryRow(ctx, "SELECT r.title,(to_jsonb(r)-ARRAY['id','org_id','created_at','updated_at','creator_id','project_id','version'])::text,r.state,p.classification,r.creator_id,r.project_id,r.version FROM "+in.Type+"s r JOIN projects p ON p.org_id=r.org_id AND p.id=r.project_id WHERE r.org_id=$1 AND r.id=$2", a.OrgID, in.ID).Scan(&d.Title, &d.Body, &d.State, &d.Class, &d.Creator, &d.ProjectID, &d.ResourceVersion)
	case "post":
		return contextPost(ctx, tx, a, c, in, members)
	case "file":
		return contextFile(ctx, tx, a, c, in, members)
	}
	return d, platform.DatabaseError(err)
}

func contextPost(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in ContextSource, members []string) (contextDocument, error) {
	d := contextDocument{ID: in.ID, Type: "post"}
	var current string
	var published *string
	err := tx.QueryRow(ctx, "SELECT project_id,author_id,classification,state,current_revision_id,published_revision_id,version FROM posts WHERE org_id=$1 AND id=$2", a.OrgID, in.ID).
		Scan(&d.ProjectID, &d.Creator, &d.Class, &d.State, &current, &published, &d.ResourceVersion)
	if err != nil {
		return d, platform.DatabaseError(err)
	}
	if err = resourceAudience(ctx, tx, a, c, d, members); err != nil {
		return d, err
	}
	allEditors := true
	for _, user := range members {
		target := a
		target.UserID = user
		if user != d.Creator {
			if e := authz.Require(ctx, tx, target, "post.write", authz.Resource{Type: "post", ID: d.ID, OwnerID: &d.Creator, ProjectID: d.ProjectID, Classification: d.Class}); e != nil {
				if core.PublicError(e).Status >= 500 {
					return d, e
				}
				allEditors = false
			}
		}
	}
	selected := current
	if !allEditors {
		if published == nil || d.State == "draft" {
			return d, core.Missing()
		}
		selected = *published
	}
	if in.VersionID != nil {
		selected = *in.VersionID
		if !allEditors && (published == nil || selected != *published) {
			return d, core.Missing()
		}
	}
	var sources []byte
	var revision int64
	err = tx.QueryRow(ctx, "SELECT title,body,seq,source_refs FROM post_revisions WHERE org_id=$1 AND post_id=$2 AND id=$3", a.OrgID, in.ID, selected).
		Scan(&d.Title, &d.Body, &revision, &sources)
	if err != nil {
		return d, platform.DatabaseError(err)
	}
	d.VersionID = &selected
	// The immutable revision number describes the selected body. A private newer
	// draft's mutable head version is not part of this common audience view.
	d.ResourceVersion = revision
	var refs []struct {
		Type      string  `json:"type"`
		ID        string  `json:"id"`
		VersionID *string `json:"version_id"`
	}
	if err = json.Unmarshal(sources, &refs); err != nil {
		return d, err
	}
	if len(refs) > 20 {
		return d, core.Fail(422, "CONTEXT_REFERENCE_LIMIT", "资料引用超出本次秘书读取范围")
	}
	for _, ref := range refs {
		if ref.Type != "file" || !core.ValidID(ref.ID) {
			return d, core.Missing()
		}
		version := ref.VersionID
		if version == nil {
			if err = tx.QueryRow(ctx, "SELECT current_version_id FROM files WHERE org_id=$1 AND id=$2", a.OrgID, ref.ID).Scan(&version); err != nil {
				return d, platform.DatabaseError(err)
			}
		}
		if version == nil {
			return d, core.Missing()
		}
		var evidence fileref.Evidence
		for _, user := range members {
			target := a
			target.UserID = user
			evidence, err = fileref.Resolve(ctx, tx, target, *version, d.ProjectID, d.Class)
			if err != nil {
				return d, err
			}
			if evidence.FileID != ref.ID {
				return d, core.Missing()
			}
		}
		d.Evidence = append(d.Evidence, evidence)
	}
	return d, nil
}
