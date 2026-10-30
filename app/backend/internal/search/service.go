package search

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	DB    *pgxpool.Pool
	Pager platform.Pager
}
type Filter struct {
	Query, Type, ProjectID, Cursor string
	CompanyOnly                    bool
	Limit                          int
}
type Resource struct {
	Type      string  `json:"type"`
	ID        string  `json:"id"`
	VersionID *string `json:"version_id,omitempty"`
}
type Hit struct {
	Resource      Resource   `json:"resource"`
	Title         string     `json:"title"`
	Snippet       string     `json:"snippet"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ProjectID     *string    `json:"project_id"`
	MatchTypes    []string   `json:"match_types"`
	TextState     *string    `json:"text_state"`
	IndexedAt     *time.Time `json:"indexed_at"`
	TextExtractor *string    `json:"text_extractor,omitempty"`
}
type cursor struct {
	Kind, ID, Binding string
	UpdatedAt         time.Time
}

var kinds = []string{"project", "task", "post", "file", "folder"}

func (s *Service) encode(c cursor) string {
	payload, _ := json.Marshal(c)
	mac := hmac.New(sha256.New, s.Pager.Key)
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Service) decode(value, binding string) (cursor, error) {
	var c cursor
	invalid := core.Fail(400, "INVALID_CURSOR", "分页标识无效或搜索条件已变更")
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(value) > 2048 {
		return c, invalid
	}
	payload, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return c, invalid
	}
	signature, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return c, invalid
	}
	mac := hmac.New(sha256.New, s.Pager.Key)
	mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(payload, &c) != nil || c.Binding != binding || !core.ValidID(c.ID) || !slices.Contains(kinds, c.Kind) || c.UpdatedAt.IsZero() {
		return c, invalid
	}
	return c, nil
}

// Every branch applies current authorization before ordering/limiting. Derived
// text is selected only for the current clean, available formal version. Post
// search uses exactly the revision that the reader can open in the knowledge UI.
const query = `WITH RECURSIVE active_folders AS (
 SELECT org_id,id,project_id,ARRAY[id] AS path FROM folders WHERE org_id=$1 AND parent_id IS NULL AND state='active'
 UNION ALL SELECT f.org_id,f.id,f.project_id,a.path||f.id FROM folders f JOIN active_folders a ON a.org_id=f.org_id AND f.parent_id=a.id
 WHERE f.state='active' AND NOT f.id=ANY(a.path)
), hits AS (
 SELECT 'project'::text AS kind,p.id,NULL::uuid AS version_id,p.name AS title,p.scope AS body,''::text AS tags,
 p.id AS project_id,p.updated_at,NULL::text AS text_state,NULL::timestamptz AS indexed_at
 FROM projects p WHERE p.org_id=$1 AND can_access(p.org_id,$2,'project.read','project',p.id,p.id,p.creator_id,p.classification,$3,false)
 UNION ALL SELECT 'task',t.id,NULL::uuid,t.title,t.body,'',t.project_id,t.updated_at,NULL,NULL
 FROM tasks t JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id
 WHERE t.org_id=$1 AND can_access(t.org_id,$2,'task.read','task',t.id,t.project_id,t.creator_id,p.classification,$3,false)
 UNION ALL SELECT 'post',p.id,r.id,r.title,r.body,array_to_string(r.tags,' '),p.project_id,p.updated_at,NULL,NULL
 FROM posts p JOIN post_revisions r ON r.org_id=p.org_id AND r.post_id=p.id AND r.id=CASE
 WHEN p.author_id=$2 OR can_access(p.org_id,$2,'post.write','post',p.id,p.project_id,p.author_id,p.classification,$3,false)
 THEN p.current_revision_id ELSE coalesce(p.published_revision_id,p.current_revision_id) END
 WHERE p.org_id=$1 AND p.state<>'archived' AND can_access(p.org_id,$2,'post.read','post',p.id,p.project_id,p.author_id,p.classification,$3,false)
 AND (p.state<>'draft' OR p.author_id=$2 OR can_access(p.org_id,$2,'post.write','post',p.id,p.project_id,p.author_id,p.classification,$3,false))
 UNION ALL SELECT 'folder',f.id,NULL::uuid,f.name,'','',f.project_id,f.updated_at,NULL,NULL
 FROM folders f JOIN active_folders a ON a.org_id=f.org_id AND a.id=f.id
 WHERE f.org_id=$1 AND can_access(f.org_id,$2,'folder.read','folder',f.id,f.project_id,f.creator_id,f.classification,$3,false)
 UNION ALL SELECT 'file',f.id,f.current_version_id,f.name,
 CASE WHEN v.availability='available' AND v.scan_state='clean' AND d.state='ready' THEN d.body ELSE '' END,'',a.project_id,f.updated_at,
 CASE WHEN v.id IS NULL THEN 'pending' WHEN v.availability<>'available' OR v.scan_state<>'clean' THEN 'blocked'
 WHEN d.state IS NULL OR d.state='queued' THEN 'pending' ELSE d.state END,d.indexed_at
 FROM files f JOIN active_folders a ON a.org_id=f.org_id AND a.id=f.folder_id
 LEFT JOIN file_versions v ON v.org_id=f.org_id AND v.file_id=f.id AND v.id=f.current_version_id
 LEFT JOIN file_text_derivations d ON d.org_id=v.org_id AND d.file_id=v.file_id AND d.version_id=v.id AND d.source_sha256=v.sha256 AND d.extractor=file_text_extractor(v.media_type)
 WHERE f.org_id=$1 AND f.state='active' AND can_access(f.org_id,$2,'file.read','file',f.id,a.project_id,f.creator_id,f.classification,$3,false)
)
SELECT kind,id,version_id,title,
 CASE WHEN body ILIKE $4 ESCAPE '\' THEN substring(body FROM greatest(1,strpos(lower(body),lower($5))-60)::int FOR 240) ELSE '' END,
 project_id,updated_at,text_state,indexed_at,
 title ILIKE $4 ESCAPE '\',body ILIKE $4 ESCAPE '\',tags ILIKE $4 ESCAPE '\',
 CASE WHEN kind='file' THEN (SELECT file_text_extractor(fv.media_type) FROM file_versions fv WHERE fv.org_id=$1 AND fv.id=hits.version_id) ELSE NULL END
 FROM hits WHERE ($6='' OR kind=$6) AND ($7='' OR project_id=NULLIF($7,'')::uuid) AND (NOT $8 OR project_id IS NULL)
 AND (title ILIKE $4 ESCAPE '\' OR body ILIKE $4 ESCAPE '\' OR tags ILIKE $4 ESCAPE '\')
 AND ($9::timestamptz IS NULL OR (updated_at,id,kind)<($9,$10::uuid,$11))
 ORDER BY updated_at DESC,id DESC,kind DESC LIMIT $12`

func (s *Service) Search(ctx context.Context, a core.Actor, f Filter) (core.Page[Hit], error) {
	out := core.Page[Hit]{Items: []Hit{}}
	f.Query = strings.TrimSpace(f.Query)
	if !utf8.ValidString(f.Query) || len([]rune(f.Query)) < 1 || len([]rune(f.Query)) > 200 || strings.ContainsRune(f.Query, 0) {
		return out, core.Invalid("q", "请输入1至200个字符的搜索文字")
	}
	if f.Type != "" && !slices.Contains(kinds, f.Type) {
		return out, core.Invalid("type", "资源类型无效")
	}
	if f.ProjectID != "" && !core.ValidID(f.ProjectID) || f.CompanyOnly && f.ProjectID != "" {
		return out, core.Invalid("project_id", "项目范围无效")
	}
	if f.Limit < 1 || f.Limit > 50 {
		return out, core.Invalid("limit", "每页需为1至50条")
	}
	binding := core.CursorBinding(a, "unified-search-v1", f.Query, f.Type, f.ProjectID, strconv.FormatBool(f.CompanyOnly), strconv.Itoa(f.Limit))
	var before *time.Time
	var beforeID *string
	beforeKind := ""
	if f.Cursor != "" {
		c, e := s.decode(f.Cursor, binding)
		if e != nil {
			return out, e
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
		beforeKind = c.Kind
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.Query) + "%"
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, query, a.OrgID, a.UserID, a.Channel, pattern, f.Query, f.Type, f.ProjectID, f.CompanyOnly, before, beforeID, beforeKind, f.Limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var h Hit
		var name, body, tags bool
		if err = rows.Scan(&h.Resource.Type, &h.Resource.ID, &h.Resource.VersionID, &h.Title, &h.Snippet, &h.ProjectID, &h.UpdatedAt, &h.TextState, &h.IndexedAt, &name, &body, &tags, &h.TextExtractor); err != nil {
			rows.Close()
			return out, err
		}
		h.MatchTypes = []string{}
		if name {
			h.MatchTypes = append(h.MatchTypes, "name")
		}
		if body {
			h.MatchTypes = append(h.MatchTypes, "body")
		}
		if tags {
			h.MatchTypes = append(h.MatchTypes, "tags")
		}
		out.Items = append(out.Items, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		last := out.Items[len(out.Items)-1]
		next := s.encode(cursor{last.Resource.Type, last.Resource.ID, binding, last.UpdatedAt})
		out.NextCursor = &next
	}
	return out, tx.Commit(ctx)
}
