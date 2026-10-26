package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileref"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// A file citation fixes the formal version and whole-content hash, not a mutable
// head or an extracted plaintext cache. No body is read by sourceSnapshots.
func contextFile(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in ContextSource, members []string) (contextDocument, error) {
	d := contextDocument{ID: in.ID, Type: "file", VersionID: in.VersionID}
	if !core.ValidID(in.ID) || in.VersionID == nil || !core.ValidID(*in.VersionID) {
		return d, core.Invalid("version_id", "秘书读取文件时必须选择准确文件版本")
	}
	var originalName *string
	err := tx.QueryRow(ctx, `SELECT f.classification,coalesce(f.creator_id::text,''),d.project_id,f.state,v.original_name FROM files f JOIN folders d ON d.org_id=f.org_id AND d.id=f.folder_id JOIN file_versions v ON v.org_id=f.org_id AND v.file_id=f.id AND v.id=$3 WHERE f.org_id=$1 AND f.id=$2`, a.OrgID, in.ID, *in.VersionID).Scan(&d.Class, &d.Creator, &d.ProjectID, &d.State, &originalName)
	if err != nil {
		return d, platform.DatabaseError(err)
	}
	if err = resourceAudience(ctx, tx, a, c, d, members); err != nil {
		return d, err
	}
	if originalName == nil {
		return d, core.Fail(422, "AGENT_TEXT_METADATA_UNAVAILABLE", "此历史版本没有原文件名记录，请先核实导入元信息；不会用当前文件名冒充历史记录")
	}
	var evidence fileref.Evidence
	for _, user := range members {
		target := a
		target.UserID = user
		evidence, err = fileref.Resolve(ctx, tx, target, *in.VersionID, c.ProjectID, c.Classification)
		if err != nil {
			return d, err
		}
		if evidence.FileID != in.ID {
			return d, core.Missing()
		}
	}
	if evidence.ScanState != "clean" {
		return d, core.Fail(422, "AGENT_TEXT_SCAN_REQUIRED", "秘书正文处理要求此准确版本的扫描结果为通过")
	}
	var size int64
	if err = tx.QueryRow(ctx, "SELECT size_bytes FROM file_versions WHERE org_id=$1 AND file_id=$2 AND id=$3", a.OrgID, in.ID, *in.VersionID).Scan(&size); err != nil {
		return d, platform.DatabaseError(err)
	}
	if err = files.AgentTextFormat(evidence.MediaType, size); err != nil {
		return d, err
	}
	d.Title = evidence.Name
	d.ResourceVersion = evidence.Seq
	d.Evidence = []fileref.Evidence{evidence}
	meta, _ := json.Marshal(map[string]any{"file_version": evidence, "content_not_included": true, "read_mode": "verified-utf8-text"})
	d.Body = string(meta)
	return d, nil
}

type fileReadPrepared struct {
	Document contextDocument
	Source   files.AgentTextSource
	Hash     string
	Text     string
}

func fileToolArguments(args json.RawMessage) (ContextSource, error) {
	var in struct {
		Type      string  `json:"type"`
		ID        string  `json:"id"`
		VersionID *string `json:"version_id"`
		Offset    int     `json:"offset"`
		MaxChars  int     `json:"max_chars"`
	}
	if err := strictArguments(args, &in); err != nil {
		return ContextSource{}, err
	}
	if in.MaxChars == 0 {
		in.MaxChars = 4000
	}
	if in.Type != "file" || !core.ValidID(in.ID) || in.VersionID == nil || !core.ValidID(*in.VersionID) || in.Offset < 0 || in.Offset > int(files.AgentTextMaxBytes) || in.MaxChars < 1 || in.MaxChars > 10000 {
		return ContextSource{}, core.Invalid("arguments", "请指定准确文件版本及有限字符范围")
	}
	v := strings.ToLower(*in.VersionID)
	return ContextSource{Type: "file", ID: strings.ToLower(in.ID), VersionID: &v, Offset: in.Offset, MaxChars: in.MaxChars}, nil
}
func (s *Service) prepareFileRead(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in ContextSource, members []string) (fileReadPrepared, error) {
	var out fileReadPrepared
	doc, err := contextFile(ctx, tx, a, c, in, members)
	if err != nil {
		return out, err
	}
	source, err := s.FileText.Prepare(ctx, tx, a, in.ID, *in.VersionID)
	if err != nil {
		return out, err
	}
	hash, err := documentDigest(doc)
	if err != nil {
		return out, err
	}
	return fileReadPrepared{Document: doc, Source: source, Hash: hash}, nil
}

func (s *Service) fileContext(ctx context.Context, a core.Actor, id string, in ContextSource) (ContextView, error) {
	var out ContextView
	if in.VersionID == nil || !core.ValidID(*in.VersionID) || in.Offset < 0 || in.Offset > int(files.AgentTextMaxBytes) || in.MaxChars < 1 || in.MaxChars > 10000 {
		return out, core.Invalid("context", "请指定准确文件版本及有限字符范围")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	prepare := func() (pgx.Tx, Conversation, []string, core.Actor, fileReadPrepared, error) {
		tx, err := s.readSnapshot(ctx, a)
		if err != nil {
			return nil, Conversation{}, nil, a, fileReadPrepared{}, err
		}
		fail := func(e error) (pgx.Tx, Conversation, []string, core.Actor, fileReadPrepared, error) {
			_ = tx.Rollback(ctx)
			return nil, Conversation{}, nil, a, fileReadPrepared{}, e
		}
		c, err := metadata(ctx, tx, a.OrgID, id, false)
		if err != nil {
			return fail(err)
		}
		if err = require(ctx, tx, a, c, "conversation.read"); err != nil {
			return fail(err)
		}
		if c.ContextSecurityVersion != in.ContextSecurityVersion {
			return fail(core.Fail(409, "CONTEXT_SECURITY_CHANGED", "会话范围已变化，请重新核对资料"))
		}
		members, err := memberIDs(ctx, tx, a.OrgID, c)
		if err != nil {
			return fail(err)
		}
		ai := a
		ai.RunID = core.NewID()
		for _, user := range members {
			target := ai
			target.UserID = user
			if err = require(ctx, tx, target, c, "conversation.read"); err != nil {
				return fail(err)
			}
		}
		p, err := s.prepareFileRead(ctx, tx, ai, c, in, members)
		if err != nil {
			return fail(err)
		}
		return tx, c, members, ai, p, nil
	}
	tx, c, _, _, first, err := prepare()
	if err != nil {
		return out, err
	}
	_ = tx.Rollback(ctx)
	text, err := s.FileText.Read(ctx, first.Source)
	if err != nil {
		return out, err
	}
	tx, current, members, _, final, err := prepare()
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if current.CurrentRevisionID != c.CurrentRevisionID || final.Hash != first.Hash || final.Source.Fingerprint != first.Source.Fingerprint {
		return out, core.Fail(409, "CONTEXT_SOURCE_CHANGED", "文件或读取来源已变化，请从第一段重新核对")
	}
	part, next, total, err := fileTextSlice(text, in.Offset, in.MaxChars)
	if err != nil {
		return out, err
	}
	bound, _ := json.Marshal([]any{core.CursorBinding(a, "context-view"), current.ID, current.CurrentRevisionID, current.ContextSecurityVersion, members, final.Hash, in.Offset, in.Offset + len([]rune(part))})
	out = ContextView{ConversationID: current.ID, ConversationRevisionID: current.CurrentRevisionID, ContextSecurityVersion: current.ContextSecurityVersion, Type: "file", ID: in.ID, VersionID: in.VersionID, ResourceVersion: final.Document.ResourceVersion, Title: final.Document.Title, State: final.Document.State, Text: part, Offset: in.Offset, TotalChars: total, NextOffset: next, SourceSHA256: final.Hash, ContextSHA256: core.Digest(bound), Evidence: final.Document.Evidence, UntrustedData: true, ObservedAt: time.Now().UTC()}
	// Preview is an AI-processing read even though no model is started. The audit
	// records only a bounded projection digest, never plaintext or cloud locators.
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result)
 VALUES($1,$2,$3,$3,$4,$5,'agent.file.preview','file',$6,$7,$8,'succeeded')`, core.NewID(), a.OrgID, a.UserID, a.Channel, core.NewID(), in.ID, final.Document.ResourceVersion, core.Digest(bound))
	if err != nil {
		return ContextView{}, err
	}
	return out, tx.Commit(ctx)
}
func fileTextSlice(text string, offset, maxChars int) (string, *int, int, error) {
	body := []rune(text)
	if offset > len(body) {
		return "", nil, 0, core.Invalid("offset", "字符起点超出此准确文件版本")
	}
	end := min(offset+maxChars, len(body))
	var next *int
	if end < len(body) {
		next = &end
	}
	return string(body[offset:end]), next, len(body), nil
}
