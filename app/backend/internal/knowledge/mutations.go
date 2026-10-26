package knowledge

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (s *Service) mutate(ctx context.Context, c core.Command, id string, version int64, base, action string, fn func(pgx.Tx, Post) (core.Result, error)) (core.Result, error) {
	if err := core.RequireVersion(version); err != nil {
		return core.Result{}, err
	}
	if !core.ValidID(base) {
		return core.Result{}, core.Invalid("base_revision_id", "请提供当前正文修订")
	}
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		p, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return err
		}
		return authz.Require(ctx, tx, c.Actor, action, resource(p))
	}, func(tx pgx.Tx) (core.Result, error) {
		p, err := metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if err = projectWritable(ctx, tx, c.Actor, p.ProjectID, true); err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "SELECT id FROM posts WHERE org_id=$1 AND id=$2 FOR UPDATE", c.Actor.OrgID, id); err != nil {
			return core.Result{}, err
		}
		p, err = metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		if p.Version != version || p.CurrentRevisionID != base {
			return core.Result{}, core.Conflict()
		}
		previous, err := revision(ctx, tx, c.Actor.OrgID, id, p.CurrentRevisionID)
		if err != nil {
			return core.Result{}, err
		}
		before, err := json.Marshal(struct {
			Post     Post
			Revision Revision
		}{p, previous})
		if err != nil {
			return core.Result{}, err
		}
		result, err := fn(tx, p)
		result.Event.BeforeDigest = core.Digest(before)
		return result, err
	})
}
func (s *Service) Update(ctx context.Context, c core.Command, id string, input Patch) (core.Result, error) {
	return s.mutate(ctx, c, id, input.ExpectedVersion, input.BaseRevisionID, "post.write", func(tx pgx.Tx, p Post) (core.Result, error) {
		if p.State == "archived" {
			return core.Result{}, core.Fail(409, "POST_READ_ONLY", "请先恢复归档文档")
		}
		if input.ProjectID.Set && ((p.ProjectID == nil) != (input.ProjectID.Value == nil) || (p.ProjectID != nil && input.ProjectID.Value != nil && *p.ProjectID != *input.ProjectID.Value)) {
			return core.Result{}, core.Fail(422, "POST_SCOPE_CHANGE_REQUIRES_PLAN", "文档项目范围变更须通过独立影响计划")
		}
		if input.Type != nil && *input.Type != p.Type {
			return core.Result{}, core.Invalid("type", "文档类型创建后不通过正文编辑变更")
		}
		if input.Classification != nil && *input.Classification != p.Classification {
			return core.Result{}, core.Fail(422, "POLICY_CHANGE_REQUIRES_PLAN", "文档分类变更须通过独立影响计划")
		}
		r, err := revision(ctx, tx, c.Actor.OrgID, id, p.CurrentRevisionID)
		if err != nil {
			return core.Result{}, err
		}
		value := Create{ProjectID: p.ProjectID, Type: p.Type, Classification: p.Classification, Title: r.Title, Body: r.Body, Summary: r.Summary, Tags: r.Tags, SourceRefs: r.SourceRefs}
		if input.Title != nil {
			value.Title = *input.Title
		}
		if input.Body != nil {
			value.Body = *input.Body
		}
		if input.Summary != nil {
			value.Summary = *input.Summary
		}
		if input.Tags != nil {
			value.Tags = *input.Tags
		}
		if input.SourceRefs != nil {
			value.SourceRefs = *input.SourceRefs
		}
		return s.newRevision(ctx, tx, c.Actor, p, value, "post.revised", "")
	})
}
func (s *Service) newRevision(ctx context.Context, tx pgx.Tx, a core.Actor, p Post, value Create, event, reason string) (core.Result, error) {
	if err := validate(value); err != nil {
		return core.Result{}, err
	}
	if err := validateSources(ctx, tx, a, value.SourceRefs); err != nil {
		return core.Result{}, err
	}
	id := core.NewID()
	if err := insertRevision(ctx, tx, a, p.ID, id, &p.CurrentRevisionID, value); err != nil {
		return core.Result{}, err
	}
	_, err := tx.Exec(ctx, "UPDATE posts SET title=$3,current_revision_id=$4,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", a.OrgID, p.ID, strings.TrimSpace(value.Title), id)
	if err != nil {
		return core.Result{}, err
	}
	p, err = metadata(ctx, tx, a.OrgID, p.ID)
	if err != nil {
		return core.Result{}, err
	}
	p, err = present(ctx, tx, a, p)
	return postResult(p, event, 200, reason), err
}
func (s *Service) Transition(ctx context.Context, c core.Command, id string, input Transition) (core.Result, error) {
	if strings.TrimSpace(input.Reason) == "" {
		return core.Result{}, core.Invalid("reason", "请说明文档状态变更原因")
	}
	return s.mutate(ctx, c, id, input.ExpectedVersion, input.BaseRevisionID, "post.publish", func(tx pgx.Tx, p Post) (core.Result, error) {
		state := p.State
		published := p.PublishedRevisionID
		switch input.Action {
		case "publish":
			if p.State == "archived" {
				return core.Result{}, core.UnexpectedState(p.State, input.Action)
			}
			r, err := revision(ctx, tx, c.Actor.OrgID, id, p.CurrentRevisionID)
			if err != nil {
				return core.Result{}, err
			}
			if strings.TrimSpace(r.Body) == "" {
				return core.Result{}, core.Invalid("body", "正文为空，不能发布")
			}
			if err = validateSources(ctx, tx, c.Actor, r.SourceRefs); err != nil {
				return core.Result{}, err
			}
			state = "published"
			published = &p.CurrentRevisionID
		case "archive":
			if p.State != "published" {
				return core.Result{}, core.UnexpectedState(p.State, input.Action)
			}
			state = "archived"
		case "draft":
			if p.State != "published" && p.State != "archived" {
				return core.Result{}, core.UnexpectedState(p.State, input.Action)
			}
			state = "draft"
		default:
			return core.Result{}, core.Invalid("action", "文档操作无效")
		}
		_, err := tx.Exec(ctx, "UPDATE posts SET state=$3,published_revision_id=$4,version=version+1,updated_at=now() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, state, published)
		if err != nil {
			return core.Result{}, err
		}
		p, err = metadata(ctx, tx, c.Actor.OrgID, id)
		if err != nil {
			return core.Result{}, err
		}
		p, err = present(ctx, tx, c.Actor, p)
		return postResult(p, "post."+input.Action, 200, input.Reason), err
	})
}
func (s *Service) Restore(ctx context.Context, c core.Command, id string, input Restore) (core.Result, error) {
	if !core.ValidID(input.RevisionID) || strings.TrimSpace(input.Reason) == "" {
		return core.Result{}, core.Invalid("revision_id", "请选择历史修订并填写恢复原因")
	}
	return s.mutate(ctx, c, id, input.ExpectedVersion, input.BaseRevisionID, "post.write", func(tx pgx.Tx, p Post) (core.Result, error) {
		if p.State == "archived" {
			return core.Result{}, core.Fail(409, "POST_READ_ONLY", "请先恢复归档文档")
		}
		r, err := revision(ctx, tx, c.Actor.OrgID, id, input.RevisionID)
		if err != nil {
			return core.Result{}, err
		}
		return s.newRevision(ctx, tx, c.Actor, p, Create{ProjectID: p.ProjectID, Type: p.Type, Classification: p.Classification, Title: r.Title, Body: r.Body, Summary: r.Summary, Tags: r.Tags, SourceRefs: r.SourceRefs}, "post.restored", input.Reason)
	})
}
