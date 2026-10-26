package knowledge

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
)

func ContentHash(p Post) string {
	raw, _ := json.Marshal(struct {
		ProjectID                                              *string `json:"project_id"`
		Type, Classification, RevisionID, Title, Body, Summary string
		Tags                                                   []string
		Sources                                                []Source
	}{p.ProjectID, p.Type, p.Classification, p.CurrentRevisionID, p.Title, p.Body, p.Summary, p.Tags, p.SourceRefs})
	return core.Digest(raw)
}

// AgentPost reads the actual current revision, including every raw attachment.
// A filtered public representation must never hide a lost dependency from AI.
func AgentPost(ctx context.Context, tx pgx.Tx, a core.Actor, id string) (Post, error) {
	p, err := metadata(ctx, tx, a.OrgID, id)
	if err != nil {
		return p, err
	}
	current := p.CurrentRevisionID
	p, err = present(ctx, tx, a, p)
	if err != nil {
		return p, err
	}
	if p.CurrentRevisionID != current {
		return Post{}, core.Missing()
	}
	r, err := revision(ctx, tx, a.OrgID, id, current)
	if err != nil {
		return Post{}, err
	}
	if err = validateSources(ctx, tx, a, r.SourceRefs); err != nil {
		return Post{}, err
	}
	p.SourceRefs = r.SourceRefs
	p.CanEdit, p.CanPublish, p.CanComment, p.CanReadHistory = false, false, false, false
	return p, nil
}

// AgentPermission deliberately retains the original actor's RunID and policy.
func AgentPermission(ctx context.Context, tx pgx.Tx, a core.Actor, value Create, existing *Post, publish bool) error {
	if err := validate(value); err != nil {
		return err
	}
	r := authz.Resource{Type: "post", ProjectID: value.ProjectID, OwnerID: &a.UserID, Classification: value.Classification}
	if existing != nil {
		r = resource(*existing)
		if existing.State == "archived" {
			return core.Fail(409, "POST_READ_ONLY", "请先恢复归档文档")
		}
	}
	if value.Classification == "confidential" || value.Classification == "project_restricted" && value.ProjectID == nil {
		return core.Invalid("classification", "此知识范围不支持秘书写入")
	}
	if err := authz.Require(ctx, tx, a, "post.write", r); err != nil {
		return err
	}
	if publish {
		if err := authz.Require(ctx, tx, a, "post.publish", r); err != nil {
			return err
		}
	}
	return projectWritable(ctx, tx, a, value.ProjectID, true)
}

func AgentSources(ctx context.Context, tx pgx.Tx, a core.Actor, refs []Source) error {
	return validateSources(ctx, tx, a, refs)
}
