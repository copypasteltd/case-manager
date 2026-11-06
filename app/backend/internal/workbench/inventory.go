package workbench

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

// Inventory pages pin the complete current authorized set, not an old cached
// list. A change anywhere in that set invalidates the continuation cursor.
type InventoryItem struct {
	ID         string    `json:"id"`
	Version    int64     `json:"version"`
	RevisionID *string   `json:"revision_id"`
	ProjectID  *string   `json:"project_id"`
	RelatedID  *string   `json:"related_id"`
	DueOn      *string   `json:"due_on"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	UpdatedAt  time.Time `json:"updated_at"`
}
type Inventory struct {
	AsOf        time.Time       `json:"as_of"`
	Scope       string          `json:"scope"`
	Kind        string          `json:"kind"`
	BasisSHA256 string          `json:"basis_sha256"`
	Count       int             `json:"count"`
	Items       []InventoryItem `json:"items"`
	NextCursor  *string         `json:"next_cursor"`
}

func (s *Service) Inventory(ctx context.Context, a core.Actor, kind, cursor string, limit int) (Inventory, error) {
	out := Inventory{Scope: "my_readable_inventory", Kind: kind, Items: []InventoryItem{}}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if a.RunID != "" || (a.Channel != "web" && a.Channel != "wechat" && a.Channel != "douyin" && a.Channel != "android") {
		return out, core.Forbidden()
	}
	if limit < 1 || limit > 50 {
		return out, core.Invalid("limit", "每页数量需为1至50")
	}
	var selected *query
	followup := false
	taskKind := false
	for _, q := range queries() {
		if q.kind == kind {
			v := q
			selected = &v
		}
	}
	for _, q := range followupQueries() {
		if q.kind == kind {
			v := q
			selected = &v
			followup = true
		}
	}
	for _, q := range taskQueries() {
		if q.kind == kind {
			v := q
			selected = &v
			taskKind = true
		}
	}
	if selected == nil {
		return out, core.Invalid("kind", "请选择现有的个人待办分组")
	}
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return out, e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var orgVersion, userVersion int64
	var status string
	e = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&orgVersion)
	if e == nil {
		e = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&userVersion, &status)
	}
	if e != nil {
		return out, platform.DatabaseError(e)
	}
	if orgVersion != a.OrgAuthVersion || userVersion != a.AuthVersion || status != "active" {
		return out, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变化，请重新读取待办")
	}
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return out, e
	}
	order := "due_on ASC NULLS LAST,updated_at DESC,id DESC"
	fields := "id,version,revision_id,NULL::uuid project_id,related_id,due_on,title,state,updated_at"
	if followup {
		order = "updated_at DESC,id DESC"
		fields = "id,version,revision_id,project_id,NULL::uuid related_id,NULL::text due_on,title,state,updated_at"
	}
	if taskKind {
		fields = "id,version,revision_id,project_id,related_id,due_on,title,state,updated_at"
	}
	var raw []byte
	e = tx.QueryRow(ctx, `WITH visible AS MATERIALIZED (`+selected.sql+`), bounded AS (SELECT `+fields+` FROM visible ORDER BY `+order+` LIMIT 5001)
 SELECT transaction_timestamp(),COALESCE(jsonb_agg(to_jsonb(v) ORDER BY `+order+`),'[]'::jsonb) FROM bounded v`, a.OrgID, a.UserID, a.Channel, false).Scan(&out.AsOf, &raw)
	if e != nil {
		return out, e
	}
	var all []InventoryItem
	if e = json.Unmarshal(raw, &all); e != nil {
		return out, e
	}
	if len(all) > 5000 {
		return out, core.Fail(422, "WORK_INVENTORY_LIMIT", "此分组超过5000项，请先从对应业务列表分批处理")
	}
	out.Count = len(all)
	out.BasisSHA256 = core.Digest(raw)
	binding := core.CursorBinding(a, "work-inventory", a.SessionID, kind, out.BasisSHA256)
	start := 0
	if cursor != "" {
		c, err := s.Pager.Decode(cursor, binding)
		if err != nil {
			return out, core.Fail(409, "WORK_INVENTORY_CHANGED", "待办或权限已变化，请从首页重新读取")
		}
		found := false
		for i, v := range all {
			if v.ID == c.ID && v.UpdatedAt.Equal(c.UpdatedAt) {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return out, core.Fail(409, "WORK_INVENTORY_CHANGED", "待办已变化，请从首页重新读取")
		}
	}
	end := min(start+limit, len(all))
	out.Items = all[start:end]
	if end < len(all) {
		last := all[end-1]
		v := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: binding})
		out.NextCursor = &v
	}
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return Inventory{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Inventory{}, platform.DatabaseError(e)
	}
	return out, nil
}
