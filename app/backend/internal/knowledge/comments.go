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

type Comment struct {
	ID                 string    `json:"id"`
	ResourceType       string    `json:"resource_type"`
	ResourceID         string    `json:"resource_id"`
	AuthorID           string    `json:"author_id"`
	AuthorDisplayName  string    `json:"author_display_name"`
	Body               string    `json:"body"`
	Revision           int64     `json:"revision"`
	AttachmentVersions []string  `json:"attachment_versions"`
	ReplacesID         *string   `json:"replaces_id"`
	ContextRevisionID  *string   `json:"context_revision_id"`
	CreatedAt          time.Time `json:"created_at"`
}
type CommentInput struct {
	Body                      string   `json:"body"`
	AttachmentVersions        []string `json:"attachment_versions,omitempty"`
	ExpectedRevision          int64    `json:"expected_revision,omitempty"`
	ExpectedContextRevisionID *string  `json:"expected_context_revision_id,omitempty"`
}

const commentColumns = `c.id,c.resource_type,c.resource_id,c.author_id,c.body,c.revision,c.attachment_versions,c.replaces_id,c.created_at,c.context_revision_id,
    (SELECT u.display_name FROM users u WHERE u.org_id=c.org_id AND u.id=c.author_id)`

func scanComment(row pgx.Row) (Comment, error) {
	var c Comment
	err := row.Scan(&c.ID, &c.ResourceType, &c.ResourceID, &c.AuthorID, &c.Body, &c.Revision, &c.AttachmentVersions, &c.ReplacesID, &c.CreatedAt, &c.ContextRevisionID, &c.AuthorDisplayName)
	if c.AttachmentVersions == nil {
		c.AttachmentVersions = []string{}
	}
	return c, platform.DatabaseError(err)
}
func commentResource(ctx context.Context, db platform.DBTX, a core.Actor, kind, id string, write, lock bool) (authz.Resource, error) {
	var r authz.Resource
	if !core.ValidID(id) {
		return r, core.Missing()
	}
	switch kind {
	case "post":
		p, err := metadata(ctx, db, a.OrgID, id)
		if err != nil {
			return r, err
		}
		if err = accessible(ctx, db, a, p); err != nil {
			return r, err
		}
		r = resource(p)
		if write && p.State == "archived" {
			return r, core.Fail(409, "POST_READ_ONLY", "归档文档禁止新增或修改评论")
		}
	case "task":
		var creator, projectID, class string
		err := db.QueryRow(ctx, `SELECT t.creator_id,t.project_id,p.classification FROM tasks t JOIN projects p ON p.org_id=t.org_id AND p.id=t.project_id
			WHERE t.org_id=$1 AND t.id=$2`, a.OrgID, id).Scan(&creator, &projectID, &class)
		if err != nil {
			return r, platform.DatabaseError(err)
		}
		r = authz.Resource{Type: "task", ID: id, OwnerID: &creator, ProjectID: &projectID, Classification: class}
		if err = authz.Require(ctx, db, a, "task.read", r); err != nil {
			return r, err
		}
	default:
		return r, core.Missing()
	}
	if write {
		if err := authz.Require(ctx, db, a, "comment.write", r); err != nil {
			return r, err
		}
		if err := projectWritable(ctx, db, a, r.ProjectID, lock); err != nil {
			return r, err
		}
		if lock {
			table := "posts"
			if kind == "task" {
				table = "tasks"
			}
			if _, err := db.Exec(ctx, "SELECT id FROM "+table+" WHERE org_id=$1 AND id=$2 FOR UPDATE", a.OrgID, id); err != nil {
				return r, err
			}
			// State may have changed while waiting for the row lock.
			if kind == "post" {
				p, err := metadata(ctx, db, a.OrgID, id)
				if err != nil {
					return r, err
				}
				if p.State == "archived" {
					return r, core.Fail(409, "POST_READ_ONLY", "归档文档禁止新增或修改评论")
				}
			}
		}
	}
	return r, nil
}
func (s *Service) Comments(ctx context.Context, a core.Actor, kind, id string, f Filter) (core.Page[Comment], error) {
	empty := core.Page[Comment]{Items: []Comment{}}
	if _, err := commentResource(ctx, s.DB, a, kind, id, false, false); err != nil {
		return empty, err
	}
	var contextRevision *string
	contextKey := ""
	if kind == "post" {
		p, err := s.Get(ctx, a, id)
		if err != nil {
			return empty, err
		}
		contextRevision = &p.CurrentRevisionID
		contextKey = p.CurrentRevisionID
	}
	binding := core.CursorBinding(a, "comments", kind, id, contextKey)
	before, beforeID, limit, err := s.commentCursor(f, binding)
	if err != nil {
		return empty, err
	}
	rows, err := s.DB.Query(ctx, "SELECT "+commentColumns+` FROM comments c WHERE c.org_id=$1 AND c.resource_type=$2 AND c.resource_id=$3
        AND c.context_revision_id IS NOT DISTINCT FROM $4::uuid
        AND NOT EXISTS(SELECT 1 FROM comments successor WHERE successor.org_id=c.org_id AND successor.replaces_id=c.id)
        AND ($5::timestamptz IS NULL OR (c.created_at,c.id)<($5,$6::uuid))
        ORDER BY c.created_at DESC,c.id DESC LIMIT $7`, a.OrgID, kind, id, contextRevision, before, beforeID, limit+1)
	if err != nil {
		return empty, err
	}
	return s.commentPage(ctx, a, rows, limit, binding)
}
func (s *Service) commentCursor(f Filter, binding string) (*time.Time, *string, int, error) {
	limit := f.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if f.Cursor == "" {
		return nil, nil, limit, nil
	}
	c, err := s.Pager.Decode(f.Cursor, binding)
	if err != nil {
		return nil, nil, limit, err
	}
	return &c.UpdatedAt, &c.ID, limit, nil
}
func (s *Service) commentPage(ctx context.Context, a core.Actor, rows pgx.Rows, limit int, binding string) (core.Page[Comment], error) {
	out := core.Page[Comment]{Items: []Comment{}}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, c)
	}
	err := rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		c := out.Items[limit-1]
		cursor := s.Pager.Encode(platform.Cursor{ID: c.ID, UpdatedAt: c.CreatedAt, Binding: binding})
		out.NextCursor = &cursor
	}
	for index := range out.Items {
		versions, err := filterCommentAttachments(ctx, s.DB, a, out.Items[index].AttachmentVersions)
		if err != nil {
			return out, err
		}
		out.Items[index].AttachmentVersions = versions
	}
	return out, nil
}
func commentAttachments(ctx context.Context, db platform.DBTX, a core.Actor, ids []string) error {
	if len(ids) > 20 {
		return core.Invalid("attachment_versions", "单条评论最多关联20个附件")
	}
	for _, id := range ids {
		if !core.ValidID(id) {
			return core.Invalid("attachment_versions", "附件版本标识无效")
		}
		var fileID string
		if err := db.QueryRow(ctx, "SELECT file_id FROM file_versions WHERE org_id=$1 AND id=$2", a.OrgID, id).Scan(&fileID); err != nil {
			return platform.DatabaseError(err)
		}
		if err := sourceAccess(ctx, db, a, Source{Type: "file", ID: fileID, VersionID: &id}); err != nil {
			return err
		}
	}
	return nil
}
func filterCommentAttachments(ctx context.Context, db platform.DBTX, a core.Actor, ids []string) ([]string, error) {
	out := []string{}
	for _, id := range ids {
		err := commentAttachments(ctx, db, a, []string{id})
		if err == nil {
			out = append(out, id)
		} else if core.PublicError(platform.DatabaseError(err)).Status >= 500 {
			return nil, err
		}
	}
	return out, nil
}

// Bind a write to the revision actually displayed to the caller. This check
// also runs before idempotency lookup, so a replay cannot reveal an inaccessible
// historical discussion after the visible publication or authority changes.
func commentWriteContext(ctx context.Context, db platform.DBTX, a core.Actor, kind, id, previousID string, input CommentInput) (*string, error) {
	var previousContext *string
	if previousID != "" {
		var author, storedKind, storedResource string
		if err := db.QueryRow(ctx, `SELECT author_id,resource_type,resource_id,context_revision_id
			FROM comments WHERE org_id=$1 AND id=$2`, a.OrgID, previousID).Scan(&author, &storedKind, &storedResource, &previousContext); err != nil {
			return nil, platform.DatabaseError(err)
		}
		if author != a.UserID || storedKind != kind || storedResource != id {
			return nil, core.Missing()
		}
	}
	if kind != "post" {
		if input.ExpectedContextRevisionID != nil {
			return nil, core.Invalid("expected_context_revision_id", "任务评论不关联知识正文修订")
		}
		return nil, nil
	}
	p, err := metadata(ctx, db, a.OrgID, id)
	if err != nil {
		return nil, err
	}
	p, err = present(ctx, db, a, p)
	if err != nil {
		return nil, err
	}
	selected := p.CurrentRevisionID
	if previousID != "" {
		if previousContext == nil {
			return nil, core.Fail(409, "COMMENT_CONTEXT_UNAVAILABLE", "此历史评论缺少正文修订，请在当前正文下新建讨论")
		}
		if *previousContext != selected && !canEdit(ctx, db, a, p) {
			return nil, core.Missing()
		}
		selected = *previousContext
	}
	if input.ExpectedContextRevisionID == nil || !core.ValidID(*input.ExpectedContextRevisionID) {
		return nil, core.Invalid("expected_context_revision_id", "请指定正在阅读的知识正文修订")
	}
	if *input.ExpectedContextRevisionID != selected {
		return nil, core.Fail(409, "COMMENT_CONTEXT_CHANGED", "正文修订已经变化，请保留评论内容并核对最新正文后重试")
	}
	return &selected, nil
}

func (s *Service) WriteComment(ctx context.Context, c core.Command, kind, id, previousID string, input CommentInput) (core.Result, error) {
	if len([]rune(strings.TrimSpace(input.Body))) < 1 || len([]rune(input.Body)) > 20000 {
		return core.Result{}, core.Invalid("body", "评论需为1至20000字符")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if _, err := commentResource(ctx, tx, c.Actor, kind, id, true, false); err != nil {
			return err
		}
		_, err := commentWriteContext(ctx, tx, c.Actor, kind, id, previousID, input)
		return err
	}, func(tx pgx.Tx) (core.Result, error) {
		if _, err := commentResource(ctx, tx, c.Actor, kind, id, true, true); err != nil {
			return core.Result{}, err
		}
		seq := int64(1)
		beforeDigest := ""
		var previous *string
		contextRevision, err := commentWriteContext(ctx, tx, c.Actor, kind, id, previousID, input)
		if err != nil {
			return core.Result{}, err
		}
		if previousID != "" {
			prior, err := scanComment(tx.QueryRow(ctx, "SELECT "+commentColumns+" FROM comments c WHERE c.org_id=$1 AND c.id=$2", c.Actor.OrgID, previousID))
			if err != nil {
				return core.Result{}, err
			}
			priorJSON, err := json.Marshal(prior)
			if err != nil {
				return core.Result{}, err
			}
			beforeDigest = core.Digest(priorJSON)
			var author, storedKind, storedResource string
			var successor bool
			var previousContext *string
			err = tx.QueryRow(ctx, `SELECT c.author_id,c.resource_type,c.resource_id,c.revision,c.context_revision_id,
				EXISTS(SELECT 1 FROM comments next WHERE next.org_id=c.org_id AND next.replaces_id=c.id)
				FROM comments c WHERE c.org_id=$1 AND c.id=$2`, c.Actor.OrgID, previousID).Scan(&author, &storedKind, &storedResource, &seq, &previousContext, &successor)
			if err != nil {
				return core.Result{}, platform.DatabaseError(err)
			}
			if author != c.Actor.UserID || storedKind != kind || storedResource != id {
				return core.Result{}, core.Missing()
			}
			if kind == "post" && previousContext != nil && contextRevision != nil && *previousContext != *contextRevision {
				p, err := metadata(ctx, tx, c.Actor.OrgID, id)
				if err != nil {
					return core.Result{}, err
				}
				if !canEdit(ctx, tx, c.Actor, p) {
					return core.Result{}, core.Missing()
				}
			}
			contextRevision = previousContext
			if successor || seq != input.ExpectedRevision {
				return core.Result{}, core.Conflict()
			}
			seq++
			previous = &previousID
		}
		if err := commentAttachments(ctx, tx, c.Actor, input.AttachmentVersions); err != nil {
			return core.Result{}, err
		}
		if input.AttachmentVersions == nil {
			input.AttachmentVersions = []string{}
		}
		commentID := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO comments(id,org_id,resource_type,resource_id,author_id,body,revision,attachment_versions,replaces_id,context_revision_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, commentID, c.Actor.OrgID, kind, id, c.Actor.UserID, input.Body, seq, input.AttachmentVersions, previous, contextRevision)
		if err != nil {
			return core.Result{}, err
		}
		value, err := scanComment(tx.QueryRow(ctx, "SELECT "+commentColumns+" FROM comments c WHERE c.org_id=$1 AND c.id=$2", c.Actor.OrgID, commentID))
		event := "comment.created"
		if previous != nil {
			event = "comment.revised"
		}
		return core.Result{Status: 201, Data: value, Event: core.Event{Type: event, ResourceType: "comment", ResourceID: commentID, Version: seq, BeforeDigest: beforeDigest, Payload: map[string]any{"resource_type": kind, "resource_id": id, "replaces_id": previous}}}, err
	})
}
func (s *Service) EditComment(ctx context.Context, c core.Command, id string, input CommentInput) (core.Result, error) {
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	var kind, resourceID string
	if err := s.DB.QueryRow(ctx, "SELECT resource_type,resource_id FROM comments WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id).Scan(&kind, &resourceID); err != nil {
		return core.Result{}, platform.DatabaseError(err)
	}
	return s.WriteComment(ctx, c, kind, resourceID, id, input)
}
func (s *Service) CommentHistory(ctx context.Context, a core.Actor, id string, f Filter) (core.Page[Comment], error) {
	empty := core.Page[Comment]{Items: []Comment{}}
	if !core.ValidID(id) {
		return empty, core.Missing()
	}
	var kind, resourceID string
	if err := s.DB.QueryRow(ctx, "SELECT resource_type,resource_id FROM comments WHERE org_id=$1 AND id=$2", a.OrgID, id).Scan(&kind, &resourceID); err != nil {
		return empty, platform.DatabaseError(err)
	}
	if _, err := commentResource(ctx, s.DB, a, kind, resourceID, false, false); err != nil {
		return empty, err
	}
	if kind == "post" {
		p, err := s.Get(ctx, a, resourceID)
		if err != nil {
			return empty, err
		}
		var commentContext *string
		if err = s.DB.QueryRow(ctx, "SELECT context_revision_id FROM comments WHERE org_id=$1 AND id=$2", a.OrgID, id).Scan(&commentContext); err != nil {
			return empty, err
		}
		if commentContext == nil || *commentContext != p.CurrentRevisionID {
			if !canEdit(ctx, s.DB, a, p) {
				return empty, core.Missing()
			}
		}
	}
	binding := core.CursorBinding(a, "comment-history", id)
	before, beforeID, limit, err := s.commentCursor(f, binding)
	if err != nil {
		return empty, err
	}
	rows, err := s.DB.Query(ctx, "WITH RECURSIVE history AS (SELECT * FROM comments WHERE org_id=$1 AND id=$2 UNION ALL SELECT parent.* FROM comments parent JOIN history h ON parent.org_id=h.org_id AND parent.id=h.replaces_id) SELECT "+commentColumns+" FROM history c WHERE ($3::timestamptz IS NULL OR (c.created_at,c.id)<($3,$4::uuid)) ORDER BY c.created_at DESC,c.id DESC LIMIT $5", a.OrgID, id, before, beforeID, limit+1)
	if err != nil {
		return empty, err
	}
	return s.commentPage(ctx, a, rows, limit, binding)
}
