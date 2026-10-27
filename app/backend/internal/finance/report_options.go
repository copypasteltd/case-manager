package finance

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type ReportOption struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	Label string `json:"label"`
}

// Choices derive only from matching, readable canonical facts. The owner label
// discloses business attribution, not the unrelated company user directory.
func (s *Service) ReportOptions(ctx context.Context, a core.Actor, f ReportQuery, field, search, cursor string, limit int) (core.Page[ReportOption], error) {
	out := core.Page[ReportOption]{Items: []ReportOption{}}
	if err := f.Validate(); err != nil {
		return out, err
	}
	if len([]rune(search)) > 200 || strings.ContainsRune(search, 0) || limit < 1 || limit > 100 {
		return out, core.Invalid("filter", "候选搜索或分页无效")
	}
	var projection string
	switch field {
	case "category":
		projection = `SELECT DISTINCT category value,category label FROM facts WHERE category<>''`
	case "owner":
		projection = `SELECT DISTINCT f.owner_id::text value,u.display_name label FROM facts f JOIN users u ON u.org_id=$1 AND u.id=f.owner_id`
	case "supplier":
		projection = `SELECT f.supplier_id::text value,COALESCE(max(CASE WHEN can_access(p.org_id,$2,'party.read','party',p.id,p.project_id,p.creator_id,p.classification,$3,false) THEN r.name END),'相关方（'||left(min(f.title),480)||'）') label FROM facts f LEFT JOIN parties p ON p.org_id=$1 AND p.id=f.supplier_id LEFT JOIN party_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id WHERE f.supplier_id IS NOT NULL GROUP BY f.supplier_id`
	default:
		return out, core.Invalid("field", "请选择类别、供应方或经办／负责人")
	}
	raw, _ := json.Marshal(f)
	bound := core.CursorBinding(a, "financial-report-options", core.Digest(raw), field, search)
	var before *string
	if cursor != "" {
		position, err := s.Pager.Decode(cursor, bound)
		if err != nil {
			return out, err
		}
		before = &position.ID
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = reportAuthority(ctx, tx, a, f, false); err != nil {
		return out, err
	}
	base, _, err := reportSourceStatement(f)
	if err != nil {
		return out, err
	}
	query := `WITH facts(source_type,id,resource_id,revision_id,version,title,period_start,period_end,state,entry_kind,currency,original_amount,contribution_amount,category,supplier_id,owner_id,project_id,created_at,timezone) AS (` + base + `), choices AS (` + projection + `), keyed AS (SELECT substr(encode(sha256(convert_to(value,'UTF8')),'hex'),1,32)::uuid id,value,label FROM choices)
        SELECT id,value,label FROM keyed WHERE ($14='' OR position(lower($14) in lower(label))>0) AND ($15::uuid IS NULL OR id<$15) ORDER BY id DESC LIMIT $16`
	rows, err := tx.Query(ctx, query, append(reportArguments(a, f), search, before, limit+1)...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item ReportOption
		if err = rows.Scan(&item.ID, &item.Value, &item.Label); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[len(out.Items)-1]
		value := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: time.Unix(1, 0).UTC(), Binding: bound})
		out.NextCursor = &value
	}
	return out, tx.Commit(ctx)
}
