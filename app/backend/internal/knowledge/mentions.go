package knowledge

import (
	"context"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

type MentionCandidate struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

func commentMentions(ctx context.Context, db platform.DBTX, a core.Actor, kind, taskID, authorID string, ids []string) error {
	if len(ids) > 20 || (kind != "task" && len(ids) > 0) {
		return core.Invalid("mentioned_user_ids", "仅任务讨论可提及成员，每条最多20人")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		key := strings.ToLower(id)
		if !core.ValidID(id) || seen[key] || key == strings.ToLower(authorID) {
			return core.Invalid("mentioned_user_ids", "请选择不同的、当前有权读取任务的其他项目成员")
		}
		seen[key] = true
		var eligible bool
		if err := db.QueryRow(ctx, "SELECT task_comment_mention_eligible($1,$2,$3,$4)", a.OrgID, taskID, authorID, id).Scan(&eligible); err != nil {
			return err
		}
		if !eligible {
			return core.Forbidden()
		}
	}
	return nil
}

func filterCommentMentions(ctx context.Context, db platform.DBTX, a core.Actor, c *Comment) error {
	out := []string{}
	for _, id := range c.MentionedUserIDs {
		var eligible bool
		if err := db.QueryRow(ctx, "SELECT task_comment_mention_eligible($1,$2,$3,$4)", a.OrgID, c.ResourceID, c.AuthorID, id).Scan(&eligible); err != nil {
			return err
		}
		if eligible {
			out = append(out, id)
		}
	}
	c.MentionsFiltered = len(out) != len(c.MentionedUserIDs)
	c.MentionedUserIDs = out
	return nil
}

func (s *Service) MentionCandidates(ctx context.Context, a core.Actor, taskID string, f Filter) (core.Page[MentionCandidate], error) {
	out := core.Page[MentionCandidate]{Items: []MentionCandidate{}}
	if _, err := commentResource(ctx, s.DB, a, "task", taskID, true, false); err != nil {
		return out, err
	}
	query := strings.TrimSpace(f.Query)
	if len([]rune(query)) > 200 {
		return out, core.Invalid("q", "搜索内容最多200字符")
	}
	binding := core.CursorBinding(a, "task-mention-candidates", taskID, query)
	before, beforeID, limit, err := s.commentCursor(f, binding)
	if err != nil {
		return out, err
	}
	rows, err := s.DB.Query(ctx, `SELECT u.id,u.display_name,u.created_at FROM users u
 WHERE u.org_id=$1 AND task_comment_mention_eligible($1,$2,$3,u.id)
 AND ($4='' OR strpos(lower(u.display_name),lower($4))>0)
 AND ($5::timestamptz IS NULL OR (u.created_at,u.id)<($5,$6::uuid))
 ORDER BY u.created_at DESC,u.id DESC LIMIT $7`, a.OrgID, taskID, a.UserID, query, before, beforeID, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	var tail platform.Cursor
	for rows.Next() {
		var c MentionCandidate
		var cursor platform.Cursor
		if err = rows.Scan(&c.ID, &c.DisplayName, &cursor.UpdatedAt); err != nil {
			return out, err
		}
		cursor.ID = c.ID
		cursor.Binding = binding
		if len(out.Items) < limit {
			out.Items = append(out.Items, c)
			tail = cursor
		} else {
			encoded := s.Pager.Encode(tail)
			out.NextCursor = &encoded
		}
	}
	return out, rows.Err()
}

// Resolve the current leaf only if this recipient remained mentioned in EVERY
// revision. Removing then re-adding must not resurrect an earlier notification.
func NotificationTaskMention(ctx context.Context, db platform.DBTX, a core.Actor, commentID, taskID string) (string, error) {
	if _, err := commentResource(ctx, db, a, "task", taskID, false, false); err != nil {
		return "", err
	}
	var leaf string
	err := db.QueryRow(ctx, `WITH RECURSIVE chain AS (
  SELECT c.*,($4::uuid=ANY(c.mentioned_user_ids) AND c.author_id<>$4::uuid) retained
  FROM comments c WHERE c.org_id=$1 AND c.id=$2 AND c.resource_type='task' AND c.resource_id=$3
  UNION ALL SELECT n.*,h.retained AND $4::uuid=ANY(n.mentioned_user_ids) FROM comments n
  JOIN chain h ON n.org_id=h.org_id AND n.replaces_id=h.id AND n.resource_type=h.resource_type AND n.resource_id=h.resource_id
 ) SELECT c.id FROM chain c WHERE retained
 AND NOT EXISTS(SELECT 1 FROM comments n WHERE n.org_id=c.org_id AND n.replaces_id=c.id)
 AND task_comment_mention_eligible($1,$3,c.author_id,$4)`, a.OrgID, commentID, taskID, a.UserID).Scan(&leaf)
	return leaf, platform.DatabaseError(err)
}

// An exact discussion link must work independently of the first list page.
func (s *Service) GetComment(ctx context.Context, a core.Actor, id string) (Comment, error) {
	if !core.ValidID(id) {
		return Comment{}, core.Missing()
	}
	c, err := scanComment(s.DB.QueryRow(ctx, "SELECT "+commentColumns+" FROM comments c WHERE c.org_id=$1 AND c.id=$2", a.OrgID, id))
	if err != nil {
		return c, err
	}
	if _, err = commentResource(ctx, s.DB, a, c.ResourceType, c.ResourceID, false, false); err != nil {
		return Comment{}, err
	}
	if c.ResourceType == "post" {
		p, e := s.Get(ctx, a, c.ResourceID)
		if e != nil {
			return Comment{}, e
		}
		if (c.ContextRevisionID == nil || *c.ContextRevisionID != p.CurrentRevisionID) && !canEdit(ctx, s.DB, a, p) {
			return Comment{}, core.Missing()
		}
	}
	c.AttachmentVersions, err = filterCommentAttachments(ctx, s.DB, a, c.AttachmentVersions)
	if err == nil {
		err = filterCommentMentions(ctx, s.DB, a, &c)
	}
	return c, err
}
