package agent

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"github.com/jackc/pgx/v5"
)

func indexedFileContext(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, members []string, in ContextSource, prepared fileReadPrepared) (ContextView, error) {
	var out ContextView
	part, next, total, err := files.ReadAgentIndexedText(ctx, tx, a, in.ID, *in.VersionID, *prepared.Document.TextIndex, in.Offset, in.MaxChars)
	if err != nil {
		return out, err
	}
	bound, _ := json.Marshal([]any{core.CursorBinding(a, "context-view"), c.ID, c.CurrentRevisionID, c.ContextSecurityVersion, members, prepared.Hash, in.Offset, in.Offset + len([]rune(part))})
	out = ContextView{ConversationID: c.ID, ConversationRevisionID: c.CurrentRevisionID, ContextSecurityVersion: c.ContextSecurityVersion,
		Type: "file", ID: in.ID, VersionID: in.VersionID, ResourceVersion: prepared.Document.ResourceVersion, Title: prepared.Document.Title,
		State: prepared.Document.State, Text: part, Offset: in.Offset, TotalChars: total, NextOffset: next,
		SourceSHA256: prepared.Hash, ContextSHA256: core.Digest(bound), Evidence: prepared.Document.Evidence, UntrustedData: true,
		ObservedAt: time.Now().UTC(), TextIndex: prepared.Document.TextIndex}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result)
 VALUES($1,$2,$3,$3,$4,$5,'agent.file.preview','file',$6,$7,$8,'succeeded')`, core.NewID(), a.OrgID, a.UserID, a.Channel,
		core.NewID(), in.ID, prepared.Document.ResourceVersion, core.Digest(bound))
	if err != nil {
		return ContextView{}, err
	}
	return out, tx.Commit(ctx)
}
