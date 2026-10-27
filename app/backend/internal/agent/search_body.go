package agent

import (
	"context"
	"strings"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"github.com/jackc/pgx/v5"
)

func searchSnippet(body, query string) string {
	lower := strings.ToLower(body)
	position := strings.Index(lower, strings.ToLower(query))
	if position < 0 {
		return ""
	}
	// Count runes in the lowered prefix: case conversion may change UTF-8 byte
	// lengths, so its byte position must not slice the original string.
	start := max(0, utf8.RuneCountInString(lower[:position])-60)
	characters := []rune(body)
	return string(characters[start:min(len(characters), start+240)])
}

// Called only after the common document projection has checked every current
// audience member, the conversation namespace, AI policy and file dependencies.
func executionSearchBody(ctx context.Context, tx pgx.Tx, a core.Actor, doc contextDocument, query string) (string, *files.IndexedTextEvidence, error) {
	if doc.Type != "file" {
		return searchSnippet(doc.Body, query), nil, nil
	}
	index, err := files.AgentTextIndex(ctx, tx, a, doc.ID, *doc.VersionID)
	if err != nil {
		return "", nil, err
	}
	if index.State != "ready" || index.TextSHA256 == nil {
		return "", &index, nil
	}
	var snippet string
	err = tx.QueryRow(ctx, `SELECT substring(body FROM greatest(1,strpos(lower(body),lower($6))-60)::int FOR 240)
 FROM file_text_derivations WHERE org_id=$1 AND file_id=$2 AND version_id=$3 AND extractor=$4
 AND source_sha256=$5 AND text_sha256=$7 AND state='ready' AND strpos(lower(body),lower($6))>0`,
		a.OrgID, doc.ID, *doc.VersionID, index.Extractor, index.SourceSHA256, query, *index.TextSHA256).Scan(&snippet)
	if err == pgx.ErrNoRows {
		return "", &index, nil
	}
	return snippet, &index, err
}
