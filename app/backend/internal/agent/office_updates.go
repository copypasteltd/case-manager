package agent

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"github.com/jackc/pgx/v5"
)

func contextFileUpdate(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in ContextSource, members []string) (contextDocument, error) {
	d, err := contextFileReference(ctx, tx, a, c, in, members)
	if err != nil || files.AgentOfficeKind(d.Evidence[0].MediaType) != "" {
		return d, err
	}
	return contextFile(ctx, tx, a, c, in, members)
}

func (s *Service) prepareFileUpdateRead(ctx context.Context, tx pgx.Tx, st executionState, i *fileIntentRecord) (fileReadPrepared, error) {
	a := st.Actor
	a.RunID = st.Run.ID
	in := ContextSource{Type: "file", ID: i.FileID, VersionID: &i.BaseVersionID}
	if files.AgentOfficeKind(i.Target.MediaType) == "" {
		return s.prepareFileRead(ctx, tx, a, st.Conversation, in, st.Run.audience)
	}
	var p fileReadPrepared
	var err error
	p.Document, err = contextFileReference(ctx, tx, a, st.Conversation, in, st.Run.audience)
	if err == nil {
		p.Hash, err = documentDigest(p.Document)
	}
	if err == nil {
		p.Source, err = s.FileText.PrepareOffice(ctx, tx, a, i.FileID, i.BaseVersionID)
	}
	return p, err
}

type officeReadPrepared struct {
	Base      fileReadPrepared
	Workspace files.OfficeWorkspace
}

func (s *Service) executionOfficeWorkspace(ctx context.Context, tx pgx.Tx, st executionState, lease ExecutionLease, key string, args json.RawMessage, work *executionIO) (any, []SourceSnapshot, json.RawMessage, bool, error) {
	var in struct {
		FileID        string `json:"file_id"`
		BaseVersionID string `json:"base_version_id"`
		Offset        int    `json:"offset"`
		Limit         int    `json:"limit"`
	}
	if err := strictArguments(args, &in); err != nil {
		return nil, nil, nil, false, err
	}
	if in.Limit == 0 {
		in.Limit = 10
	}
	i, err := s.executionFileIntent(ctx, tx, st)
	if err != nil {
		return nil, nil, nil, false, err
	}
	if i == nil || i.Mode == "create" || i.OperationID != nil || in.FileID != i.FileID || in.BaseVersionID != i.BaseVersionID || files.AgentOfficeKind(i.Target.MediaType) == "" || in.Offset < 0 || in.Limit < 1 || in.Limit > 20 {
		return nil, nil, nil, false, core.Invalid("office", "只能分页读取本次已授权Office更新的准确原件工作区")
	}
	p, err := s.prepareFileUpdateRead(ctx, tx, st, i)
	if err != nil {
		return nil, nil, nil, false, err
	}
	if p.Hash != i.BaseSHA256 {
		return nil, nil, nil, false, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	if work == nil || work.Office == nil {
		if err = tx.Rollback(ctx); err != nil {
			return nil, nil, nil, false, err
		}
		body, e := s.FileText.ReadOffice(ctx, p.Source)
		if e != nil {
			return nil, nil, nil, false, e
		}
		workspace, e := files.OfficeDocumentWorkspace(ctx, files.AgentOfficeKind(i.Target.MediaType), body)
		if e != nil {
			return nil, nil, nil, false, e
		}
		raw, e := s.executionTool(ctx, lease, key, "read_office_workspace", args, &executionIO{Office: &officeReadPrepared{p, workspace}})
		return nil, nil, raw, true, e
	}
	prior := work.Office
	if prior.Base.Hash != p.Hash || prior.Base.Source.Fingerprint != p.Source.Fingerprint {
		return nil, nil, nil, false, runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	w := prior.Workspace
	if in.Offset > len(w.Units) {
		return nil, nil, nil, false, core.Invalid("offset", "工作区分页位置超出完整单位列表")
	}
	units := []files.OfficeUnit{}
	next := in.Offset
	for next < len(w.Units) && len(units) < in.Limit {
		candidate := append(append([]files.OfficeUnit{}, units...), w.Units[next])
		encoded, _ := json.Marshal(candidate)
		if len(encoded) > 24*1024 {
			if len(units) == 0 {
				return nil, nil, nil, false, core.Invalid("office", "单个Office单位超出模型安全读取边界，请使用人工编辑入口")
			}
			break
		}
		units = candidate
		next++
	}
	content := map[string]any{"file_id": i.FileID, "base_version_id": i.BaseVersionID, "content_sha256": p.Source.SHA256, "workspace_sha256": w.SHA256, "adapter": w.Adapter, "kind": w.Kind, "units": units, "total_units": len(w.Units), "offset": in.Offset, "next_offset": next, "complete": next == len(w.Units), "warnings": w.Warnings, "untrusted_data": true}
	return content, []SourceSnapshot{fileUpdateSource(i)}, nil, false, nil
}
