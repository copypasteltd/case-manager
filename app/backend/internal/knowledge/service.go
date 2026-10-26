package knowledge

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

func resource(p Post) authz.Resource {
	return authz.Resource{Type: "post", ID: p.ID, ProjectID: p.ProjectID, OwnerID: &p.AuthorID, Classification: p.Classification}
}
func canEdit(ctx context.Context, db platform.DBTX, a core.Actor, p Post) bool {
	return authz.Require(ctx, db, a, "post.write", resource(p)) == nil
}

// Metadata is internal to the domain. Public readers receive the revision
// selected by their authority, including that revision's title and tags.
func metadata(ctx context.Context, db platform.DBTX, org, id string) (Post, error) {
	if !core.ValidID(id) {
		return Post{}, core.Missing()
	}
	var p Post
	err := db.QueryRow(ctx, `SELECT id,version,created_at,updated_at,project_id,author_id,type,title,classification,state,current_revision_id,published_revision_id
		FROM posts WHERE org_id=$1 AND id=$2`, org, id).Scan(&p.ID, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.ProjectID, &p.AuthorID, &p.Type, &p.Title, &p.Classification, &p.State, &p.CurrentRevisionID, &p.PublishedRevisionID)
	return p, platform.DatabaseError(err)
}
func revision(ctx context.Context, db platform.DBTX, org, postID, id string) (Revision, error) {
	var r Revision
	var refs []byte
	err := db.QueryRow(ctx, `SELECT id,post_id,seq,base_revision_id,title,body,summary,tags,author_id,created_at,source_refs
		FROM post_revisions WHERE org_id=$1 AND post_id=$2 AND id=$3`, org, postID, id).Scan(&r.ID, &r.PostID, &r.Seq, &r.BaseRevisionID, &r.Title, &r.Body, &r.Summary, &r.Tags, &r.AuthorID, &r.CreatedAt, &refs)
	if err != nil {
		return r, platform.DatabaseError(err)
	}
	err = json.Unmarshal(refs, &r.SourceRefs)
	if r.SourceRefs == nil {
		r.SourceRefs = []Source{}
	}
	if r.Tags == nil {
		r.Tags = []string{}
	}
	return r, err
}
func accessible(ctx context.Context, db platform.DBTX, a core.Actor, p Post) error {
	if err := authz.Require(ctx, db, a, "post.read", resource(p)); err != nil {
		return err
	}
	if p.State == "draft" && p.AuthorID != a.UserID && !canEdit(ctx, db, a, p) {
		return core.Missing()
	}
	return nil
}
func present(ctx context.Context, db platform.DBTX, a core.Actor, p Post) (Post, error) {
	if err := accessible(ctx, db, a, p); err != nil {
		return Post{}, err
	}
	selected := p.CurrentRevisionID
	editor := canEdit(ctx, db, a, p)
	if !editor && p.AuthorID != a.UserID && p.PublishedRevisionID != nil {
		selected = *p.PublishedRevisionID
	}
	r, err := revision(ctx, db, a.OrgID, p.ID, selected)
	if err != nil {
		return Post{}, err
	}
	p.Title, p.Body, p.Summary, p.Tags = r.Title, r.Body, r.Summary, r.Tags
	p.CurrentRevisionID = selected
	p.HasUnpublishedChanges = (editor || p.AuthorID == a.UserID) && (p.PublishedRevisionID == nil || *p.PublishedRevisionID != selected)
	// Hash the complete fixed content before attachment visibility filtering.
	p.SourceRefs = r.SourceRefs
	p.ContentHash = ContentHash(p)
	p.SourceRefs, err = filterSources(ctx, db, a, r.SourceRefs)
	writable := projectWritable(ctx, db, a, p.ProjectID, false) == nil
	p.CanEdit = editor && writable && p.State != "archived"
	p.CanReadHistory = editor
	p.CanPublish = writable && authz.Require(ctx, db, a, "post.publish", resource(p)) == nil
	p.CanComment = writable && p.State != "archived" && authz.Require(ctx, db, a, "comment.write", resource(p)) == nil
	return p, err
}
func (s *Service) Get(ctx context.Context, a core.Actor, id string) (Post, error) {
	p, err := metadata(ctx, s.DB, a.OrgID, id)
	if err != nil {
		return p, err
	}
	return present(ctx, s.DB, a, p)
}
func (s *Service) List(ctx context.Context, a core.Actor, f Filter) (core.Page[Post], error) {
	out := core.Page[Post]{Items: []Post{}}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.ProjectID != "" && !core.ValidID(f.ProjectID) {
		return out, core.Invalid("project_id", "项目标识无效")
	}
	binding := core.CursorBinding(a, "posts", f.Query, f.State, f.ProjectID)
	var before *time.Time
	var beforeID *string
	if f.Cursor != "" {
		c, err := s.Pager.Decode(f.Cursor, binding)
		if err != nil {
			return out, err
		}
		before = &c.UpdatedAt
		beforeID = &c.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT p.id FROM posts p
		JOIN post_revisions r ON r.org_id=p.org_id AND r.post_id=p.id AND r.id=CASE
			WHEN p.author_id=$2 OR can_access(p.org_id,$2,'post.write','post',p.id,p.project_id,p.author_id,p.classification,$3,$10)
			THEN p.current_revision_id ELSE coalesce(p.published_revision_id,p.current_revision_id) END
		WHERE p.org_id=$1 AND can_access(p.org_id,$2,'post.read','post',p.id,p.project_id,p.author_id,p.classification,$3,$10)
		AND (p.state<>'draft' OR p.author_id=$2 OR can_access(p.org_id,$2,'post.write','post',p.id,p.project_id,p.author_id,p.classification,$3,$10))
		AND (($4='' AND p.state<>'archived') OR p.state=$4)
		AND ($5='' OR position(lower($5) IN lower(r.title||' '||r.body||' '||array_to_string(r.tags,' ')))>0)
		AND ($6='' OR p.project_id=NULLIF($6,'')::uuid)
		AND ($7::timestamptz IS NULL OR (p.updated_at,p.id)<($7,$8::uuid))
		ORDER BY p.updated_at DESC,p.id DESC LIMIT $9`, a.OrgID, a.UserID, a.Channel, f.State, f.Query, f.ProjectID, before, beforeID, f.Limit+1, a.RunID != "")
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		p, err := s.Get(ctx, a, id)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, p)
	}
	if len(out.Items) > f.Limit {
		out.Items = out.Items[:f.Limit]
		p := out.Items[f.Limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: p.ID, UpdatedAt: p.UpdatedAt, Binding: binding})
		out.NextCursor = &c
	}
	return out, nil
}
func validate(input Create) error {
	if err := core.Name(input.Title); err != nil {
		return err
	}
	if !core.Classification(input.Classification) {
		return core.Invalid("classification", "资料分类无效")
	}
	if input.Type != "project_log" && input.Type != "meeting_note" && input.Type != "sop" && input.Type != "article" {
		return core.Invalid("type", "文档类型无效")
	}
	if input.ProjectID != nil && !core.ValidID(*input.ProjectID) {
		return core.Invalid("project_id", "项目标识无效")
	}
	if len([]rune(input.Body)) > 20000 || len([]rune(input.Summary)) > 20000 {
		return core.Invalid("body", "正文或摘要超过20000字符")
	}
	if len(input.Tags) > 20 || len(input.SourceRefs) > 100 {
		return core.Invalid("tags", "标签最多20项，来源最多100项")
	}
	seen := map[string]bool{}
	for _, tag := range input.Tags {
		key := strings.ToLower(strings.TrimSpace(tag))
		if len([]rune(key)) < 1 || len([]rune(key)) > 50 || seen[key] {
			return core.Invalid("tags", "标签需为1至50字符且不能重复")
		}
		seen[key] = true
	}
	return nil
}
func projectWritable(ctx context.Context, db platform.DBTX, a core.Actor, id *string, lock bool) error {
	if id == nil {
		return nil
	}
	query := "SELECT state,classification FROM projects WHERE org_id=$1 AND id=$2"
	if lock {
		query += " FOR UPDATE"
	}
	var state, class string
	if err := db.QueryRow(ctx, query, a.OrgID, *id).Scan(&state, &class); err != nil {
		return platform.DatabaseError(err)
	}
	if state == "archived" || state == "cancelled" || state == "closing" {
		return core.Fail(409, "PROJECT_READ_ONLY", "项目当前阶段不允许修改知识文档")
	}
	return nil
}
func (s *Service) Create(ctx context.Context, c core.Command, input Create) (core.Result, error) {
	if err := validate(input); err != nil {
		return core.Result{}, err
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if input.Classification == "project_restricted" && input.ProjectID == nil {
			return core.Invalid("project_id", "项目受限文档必须关联项目")
		}
		if input.Classification == "confidential" {
			return core.Fail(422, "EXPLICIT_GRANT_REQUIRED", "严格受限文档须使用独立分类与对象授权流程")
		}
		r := authz.Resource{Type: "post", ProjectID: input.ProjectID, OwnerID: &c.Actor.UserID, Classification: input.Classification}
		if err := authz.Require(ctx, tx, c.Actor, "post.write", r); err != nil {
			return err
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		if err := projectWritable(ctx, tx, c.Actor, input.ProjectID, true); err != nil {
			return core.Result{}, err
		}
		if input.ProjectID != nil {
			var class string
			if err := tx.QueryRow(ctx, "SELECT classification FROM projects WHERE org_id=$1 AND id=$2", c.Actor.OrgID, *input.ProjectID).Scan(&class); err != nil {
				return core.Result{}, err
			}
			if (class == "confidential" && input.Classification != "confidential") || (class == "project_restricted" && input.Classification == "internal") {
				return core.Result{}, core.Invalid("classification", "文档分类不能低于所属项目")
			}
		}
		if err := validateSources(ctx, tx, c.Actor, input.SourceRefs); err != nil {
			return core.Result{}, err
		}
		id, rev := core.NewID(), core.NewID()
		_, err := tx.Exec(ctx, `INSERT INTO posts(id,org_id,project_id,author_id,type,title,classification,state,current_revision_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,'draft',$8)`, id, c.Actor.OrgID, input.ProjectID, c.Actor.UserID, input.Type, strings.TrimSpace(input.Title), input.Classification, rev)
		if err != nil {
			return core.Result{}, err
		}
		if err = insertRevision(ctx, tx, c.Actor, id, rev, nil, input); err != nil {
			return core.Result{}, err
		}
		p, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		p, err = present(ctx, tx, c.Actor, p)
		return postResult(p, "post.created", 201, ""), err
	})
}
func insertRevision(ctx context.Context, tx pgx.Tx, a core.Actor, id, rev string, base *string, input Create) error {
	if input.Tags == nil {
		input.Tags = []string{}
	}
	if input.SourceRefs == nil {
		input.SourceRefs = []Source{}
	}
	for i := range input.Tags {
		input.Tags[i] = strings.TrimSpace(input.Tags[i])
	}
	refs, err := json.Marshal(input.SourceRefs)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO post_revisions(id,org_id,post_id,seq,base_revision_id,title,body,summary,tags,author_id,source_refs)
		VALUES($1,$2,$3,(SELECT coalesce(max(seq),0)+1 FROM post_revisions WHERE org_id=$2 AND post_id=$3),$4,$5,$6,$7,$8,$9,$10)`,
		rev, a.OrgID, id, base, strings.TrimSpace(input.Title), input.Body, input.Summary, input.Tags, a.UserID, refs)
	return err
}
func postResult(p Post, event string, status int, reason string) core.Result {
	return core.Result{Status: status, Data: p, Event: core.Event{Type: event, ResourceType: "post", ResourceID: p.ID, Version: p.Version,
		Payload: map[string]any{"project_id": p.ProjectID, "revision_id": p.CurrentRevisionID, "reason": reason}}}
}
