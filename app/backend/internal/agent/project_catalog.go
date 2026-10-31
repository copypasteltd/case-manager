package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"github.com/jackc/pgx/v5"
)

// Only project-planning identities and published template content are exposed.
// Login names, contacts, account security state and role bindings stay private.
type ProjectCatalogItem struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Name           string    `json:"name"`
	Version        int64     `json:"version"`
	Revision       *int64    `json:"revision"`
	Description    string    `json:"description"`
	FolderCount    int       `json:"folder_count"`
	TaskCount      int       `json:"task_count"`
	MilestoneCount int       `json:"milestone_count"`
	SourceSHA256   string    `json:"source_sha256"`
	UpdatedAt      time.Time `json:"-"`
}
type ProjectCatalogPage struct {
	Items         []ProjectCatalogItem `json:"items"`
	NextCursor    *string              `json:"next_cursor"`
	ObservedAt    time.Time            `json:"observed_at"`
	UntrustedData bool                 `json:"untrusted_data"`
}
type ProjectCatalogInput struct {
	Kind   string `json:"kind"`
	Query  string `json:"query"`
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}
type ProjectCatalogDocument struct {
	Item          ProjectCatalogItem `json:"item"`
	Text          string             `json:"text"`
	Offset        int                `json:"offset"`
	TotalChars    int                `json:"total_chars"`
	NextOffset    *int               `json:"next_offset"`
	UntrustedData bool               `json:"untrusted_data"`
}

func catalogKind(kind string) string {
	if kind == "people" {
		return "agent_project_person"
	}
	if kind == "templates" {
		return "agent_project_template"
	}
	return ""
}
func catalogSourceType(kind string) bool {
	return kind == "agent_project_person" || kind == "agent_project_template" || kind == "agent_project_catalog"
}

func catalogAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, kind string) error {
	if a.RunID == "" {
		a.RunID = "catalog-policy"
	}
	if c.Type != "private" || c.ProjectID != nil || c.CreatorID != a.UserID || c.State != "active" || a.Channel == "telegram" {
		return core.Missing()
	}
	if err := require(ctx, tx, a, c, "conversation.read"); err != nil {
		return err
	}
	for _, action := range []string{"agent.plan.read", "project.create"} {
		if err := authz.RequireOrganization(ctx, tx, a, action); err != nil {
			return err
		}
	}
	if kind == "people" || kind == "agent_project_person" {
		return authz.RequireOrganization(ctx, tx, a, "project.manage_members")
	}
	return nil
}

func catalogRecord(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, kind, id string) (ProjectCatalogItem, string, error) {
	var item ProjectCatalogItem
	if !catalogSourceType(kind) || !exactOAID(id) {
		return item, "", core.Missing()
	}
	if err := catalogAuthority(ctx, tx, a, c, kind); err != nil {
		return item, "", err
	}
	item.ID, item.Type = id, kind
	if kind == "agent_project_catalog" {
		var actor, conversation string
		var payload []byte
		err := tx.QueryRow(ctx, `SELECT r.actor_id,r.conversation_id,s.payload::text,s.sha256,s.created_at FROM agent_project_catalogs s JOIN agent_runs r ON r.org_id=s.org_id AND r.id=s.run_id WHERE s.org_id=$1 AND s.id=$2`, a.OrgID, id).Scan(&actor, &conversation, &payload, &item.SourceSHA256, &item.UpdatedAt)
		if err != nil {
			return item, "", platform.DatabaseError(err)
		}
		if actor != a.UserID || conversation != c.ID {
			return item, "", core.Missing()
		}
		var stored struct {
			Kind string             `json:"kind"`
			Page ProjectCatalogPage `json:"page"`
		}
		if err = json.Unmarshal(payload, &stored); err != nil {
			return item, "", err
		}
		if catalogKind(stored.Kind) == "" {
			return item, "", core.Missing()
		}
		if err = catalogAuthority(ctx, tx, a, c, stored.Kind); err != nil {
			return item, "", err
		}
		for _, old := range stored.Page.Items {
			if old.Type != catalogKind(stored.Kind) {
				return item, "", core.Missing()
			}
			current, _, e := catalogRecord(ctx, tx, a, c, old.Type, old.ID)
			if e != nil {
				return item, "", e
			}
			if current.Version != old.Version || current.SourceSHA256 != old.SourceSHA256 {
				return item, "", runtimeFailure("RUNTIME_SOURCE_CHANGED")
			}
		}
		item.Name = "项目模板目录观察"
		if stored.Kind == "people" {
			item.Name = "项目人员目录观察"
		}
		item.Version = 1
		return item, string(payload), nil
	}
	var body any
	if kind == "agent_project_person" {
		err := tx.QueryRow(ctx, `SELECT version,display_name,updated_at FROM users WHERE org_id=$1 AND id=$2 AND status='active'`, a.OrgID, id).Scan(&item.Version, &item.Name, &item.UpdatedAt)
		if err != nil {
			return item, "", platform.DatabaseError(err)
		}
		body = map[string]any{"id": id, "type": kind, "version": item.Version, "display_name": item.Name, "state": "active"}
	} else {
		var raw []byte
		var revision int64
		err := tx.QueryRow(ctx, `SELECT version,revision,name,description,task_and_folder_schema,updated_at FROM project_templates WHERE org_id=$1 AND id=$2 AND state='published'`, a.OrgID, id).Scan(&item.Version, &revision, &item.Name, &item.Description, &raw, &item.UpdatedAt)
		if err != nil {
			return item, "", platform.DatabaseError(err)
		}
		var schema project.TemplateSchema
		if err = json.Unmarshal(raw, &schema); err != nil {
			return item, "", err
		}
		item.Revision = &revision
		item.FolderCount, item.TaskCount, item.MilestoneCount = len(schema.Folders), len(schema.Tasks), len(schema.Milestones)
		body = map[string]any{"id": id, "type": kind, "version": item.Version, "revision": revision, "name": item.Name, "description": item.Description, "schema": schema, "state": "published"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return item, "", err
	}
	canonical, err := core.CanonicalJSON(raw)
	if err != nil {
		return item, "", err
	}
	item.SourceSHA256 = core.Digest(canonical)
	return item, string(canonical), nil
}

func catalogSnapshot(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, kind, id string) (SourceSnapshot, error) {
	item, _, err := catalogRecord(ctx, tx, a, c, kind, id)
	return SourceSnapshot{SourceReference: SourceReference{Type: kind, ID: id}, ResourceVersion: item.Version, SHA256: item.SourceSHA256}, err
}

func (s *Service) catalogPage(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, in ProjectCatalogInput) (ProjectCatalogPage, []SourceSnapshot, error) {
	out := ProjectCatalogPage{Items: []ProjectCatalogItem{}, UntrustedData: true}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if catalogKind(in.Kind) == "" || !utf8.ValidString(in.Query) || strings.ContainsRune(in.Query, 0) || len([]rune(in.Query)) > 200 || in.Query != strings.TrimSpace(in.Query) || in.Limit < 1 || in.Limit > 20 || len(in.Cursor) > 2048 {
		return out, nil, core.Invalid("catalog", "请核对人员／模板目录、查询条件及每页1至20项的范围")
	}
	if err := catalogAuthority(ctx, tx, a, c, in.Kind); err != nil {
		return out, nil, err
	}
	binding := core.CursorBinding(a, "agent-project-catalog", c.ID, strconv.FormatInt(c.ContextSecurityVersion, 10), a.RunID, in.Kind, in.Query, strconv.Itoa(in.Limit))
	var before *time.Time
	var beforeID *string
	if in.Cursor != "" {
		v, err := s.Pager.Decode(in.Cursor, binding)
		if err != nil {
			return out, nil, err
		}
		before, beforeID = &v.UpdatedAt, &v.ID
	}
	query := `SELECT id FROM users WHERE org_id=$1 AND status='active' AND ($2='' OR strpos(lower(display_name),lower($2))>0 OR id::text=$2) AND ($3::timestamptz IS NULL OR (updated_at,id)<($3,$4::uuid)) ORDER BY updated_at DESC,id DESC LIMIT $5`
	if in.Kind == "templates" {
		query = `SELECT id FROM project_templates WHERE org_id=$1 AND state='published' AND ($2='' OR strpos(lower(name||' '||key||' '||description),lower($2))>0 OR id::text=$2) AND ($3::timestamptz IS NULL OR (updated_at,id)<($3,$4::uuid)) ORDER BY updated_at DESC,id DESC LIMIT $5`
	}
	rows, err := tx.Query(ctx, query, a.OrgID, in.Query, before, beforeID, in.Limit+1)
	if err != nil {
		return out, nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, nil, err
	}
	more := len(ids) > in.Limit
	if more {
		ids = ids[:in.Limit]
	}
	sources := []SourceSnapshot{}
	for _, id := range ids {
		item, _, e := catalogRecord(ctx, tx, a, c, catalogKind(in.Kind), id)
		if e != nil {
			return out, nil, e
		}
		out.Items = append(out.Items, item)
		sources = append(sources, SourceSnapshot{SourceReference: SourceReference{Type: item.Type, ID: id}, ResourceVersion: item.Version, SHA256: item.SourceSHA256})
	}
	if more {
		last := out.Items[len(out.Items)-1]
		next := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: binding})
		out.NextCursor = &next
	}
	err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&out.ObservedAt)
	return out, sources, err
}

func (s *Service) ProjectCatalog(ctx context.Context, a core.Actor, id string, in ProjectCatalogInput) (ProjectCatalogPage, error) {
	var out ProjectCatalogPage
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	c, err := metadata(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	// The same AI-processing policy is checked before a human selects material
	// to send to the secretary; an ordinary directory grant alone is insufficient.
	ai := a
	ai.RunID = "catalog-preview"
	out, _, err = s.catalogPage(ctx, tx, ai, c, in)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

func catalogText(ctx context.Context, tx pgx.Tx, a core.Actor, c Conversation, kind, id string, offset, limit int) (ProjectCatalogDocument, SourceSnapshot, error) {
	var out ProjectCatalogDocument
	var source SourceSnapshot
	if offset < 0 || offset > 300000 || limit < 1 || limit > 10000 {
		return out, source, core.Invalid("offset", "请核对目录正文读取范围")
	}
	item, text, err := catalogRecord(ctx, tx, a, c, kind, id)
	if err != nil {
		return out, source, err
	}
	chars := []rune(text)
	if len(chars) > 300000 || offset > len(chars) {
		return out, source, core.Invalid("offset", "目录内容或读取起点超出范围")
	}
	end := min(offset+limit, len(chars))
	var next *int
	if end < len(chars) {
		next = &end
	}
	out = ProjectCatalogDocument{Item: item, Text: string(chars[offset:end]), Offset: offset, TotalChars: len(chars), NextOffset: next, UntrustedData: true}
	source = SourceSnapshot{SourceReference: SourceReference{Type: item.Type, ID: item.ID}, ResourceVersion: item.Version, SHA256: item.SourceSHA256}
	return out, source, nil
}

func (s *Service) ProjectCatalogSource(ctx context.Context, a core.Actor, id, kind, sourceID string, offset, limit int) (ProjectCatalogDocument, error) {
	var out ProjectCatalogDocument
	tx, err := s.readSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = identity.RequireInteractiveSession(ctx, tx, a, false); err != nil {
		return out, err
	}
	c, err := metadata(ctx, tx, a.OrgID, id, false)
	if err != nil {
		return out, err
	}
	ai := a
	ai.RunID = "catalog-preview"
	out, _, err = catalogText(ctx, tx, ai, c, kind, sourceID, offset, limit)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

func (s *Service) executionProjectCatalog(ctx context.Context, tx pgx.Tx, st executionState, lease ExecutionLease, args json.RawMessage) (any, []SourceSnapshot, error) {
	var in ProjectCatalogInput
	if err := strictArguments(args, &in); err != nil {
		return nil, nil, err
	}
	a := st.Actor
	a.RunID = st.Run.ID
	if st.Profile.SkillID != "project-create" || !slices.Equal(st.Run.audience, []string{a.UserID}) {
		return nil, nil, runtimeFailure("SKILL_TOOL_DENIED")
	}
	if lease.credentialSHA256 == "" {
		return nil, nil, runtimeFailure("RUNTIME_CREDENTIAL_INVALID")
	}
	reader := *s
	reader.Pager = platform.Pager{Key: []byte(lease.credentialSHA256)}
	page, _, err := reader.catalogPage(ctx, tx, a, st.Conversation, in)
	if err != nil {
		return nil, nil, err
	}
	id := core.NewID()
	payload, err := json.Marshal(map[string]any{"kind": in.Kind, "query": in.Query, "page": page})
	if err != nil {
		return nil, nil, err
	}
	var sha string
	err = tx.QueryRow(ctx, `INSERT INTO agent_project_catalogs(id,org_id,run_id,payload,sha256) VALUES($1,$2,$3,$4,encode(sha256(convert_to($4::jsonb::text,'UTF8')),'hex')) RETURNING sha256`, id, a.OrgID, st.Run.ID, payload).Scan(&sha)
	if err != nil {
		return nil, nil, err
	}
	source := SourceSnapshot{SourceReference: SourceReference{Type: "agent_project_catalog", ID: id}, ResourceVersion: 1, SHA256: sha}
	return map[string]any{"kind": in.Kind, "page": page, "catalog_id": id, "source_sha256": sha, "untrusted_data": true}, []SourceSnapshot{source}, nil
}
