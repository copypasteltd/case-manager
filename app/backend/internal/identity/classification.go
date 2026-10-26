package identity

import (
	"context"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type ClassificationPolicy struct {
	ID              string    `json:"id"`
	Level           string    `json:"level"`
	Version         int64     `json:"version"`
	AIAllowed       bool      `json:"ai_allowed"`
	AllowedChannels []string  `json:"allowed_channels"`
	PublishedBy     *string   `json:"published_by"`
	BasePolicyID    *string   `json:"base_policy_id"`
	Reason          *string   `json:"reason"`
	CreatedAt       time.Time `json:"created_at"`
}
type ClassificationPolicyInput struct {
	ExpectedVersion int64    `json:"expected_version"`
	BasePolicyID    string   `json:"base_policy_id"`
	Level           string   `json:"level"`
	AIAllowed       bool     `json:"ai_allowed"`
	AllowedChannels []string `json:"allowed_channels"`
	Reason          string   `json:"reason"`
}

const classificationColumns = `id,level,policy_version,ai_allowed,allowed_channels,published_by,base_policy_id,reason,created_at`

func scanClassification(row pgx.Row) (ClassificationPolicy, error) {
	var p ClassificationPolicy
	err := row.Scan(&p.ID, &p.Level, &p.Version, &p.AIAllowed, &p.AllowedChannels, &p.PublishedBy, &p.BasePolicyID, &p.Reason, &p.CreatedAt)
	return p, platform.DatabaseError(err)
}
func validClassification(level string) bool {
	return slices.Contains([]string{"internal", "project_restricted", "confidential"}, level)
}

// Configuration remains an interactive human surface, not an agent tool.
func classificationAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, mfa bool) error {
	if a.RunID != "" {
		return core.Forbidden()
	}
	if err := authz.RequireOrganization(ctx, tx, a, "policy.manage"); err != nil {
		return err
	}
	return RequireInteractiveSession(ctx, tx, a, mfa)
}
func (s *Service) classificationSnapshot(ctx context.Context, a core.Actor) (pgx.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", a.OrgID); err == nil {
		err = classificationAuthority(ctx, tx, a, false)
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func (s *Service) ClassificationPolicies(ctx context.Context, a core.Actor) (core.Page[ClassificationPolicy], error) {
	out := core.Page[ClassificationPolicy]{Items: []ClassificationPolicy{}}
	tx, err := s.classificationSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, "SELECT DISTINCT ON(level) "+classificationColumns+" FROM classification_policies WHERE org_id=$1 ORDER BY level,policy_version DESC", a.OrgID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		p, e := scanClassification(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (s *Service) ClassificationPolicyHistory(ctx context.Context, a core.Actor, level, cursor string, limit int) (core.Page[ClassificationPolicy], error) {
	out := core.Page[ClassificationPolicy]{Items: []ClassificationPolicy{}}
	if !validClassification(level) {
		return out, core.Missing()
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	binding := core.CursorBinding(a, "classification-history", level)
	pager := platform.Pager{Key: s.Config.CursorKey}
	at := time.Unix(0, 0)
	id := "00000000-0000-0000-0000-000000000000"
	if cursor != "" {
		c, err := pager.Decode(cursor, binding)
		if err != nil {
			return out, err
		}
		at, id = c.UpdatedAt, c.ID
	}
	tx, err := s.classificationSnapshot(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, "SELECT "+classificationColumns+" FROM classification_policies WHERE org_id=$1 AND level=$2 AND ($3 OR (created_at,id)<($4,$5::uuid)) ORDER BY created_at DESC,id DESC LIMIT $6", a.OrgID, level, cursor == "", at, id, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		p, e := scanClassification(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[len(out.Items)-1]
		next := pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, tx.Commit(ctx)
}
func (s *Service) PublishClassificationPolicy(ctx context.Context, c core.Command, in ClassificationPolicyInput) (core.Result, error) {
	if !validClassification(in.Level) || core.RequireVersion(in.ExpectedVersion) != nil || !core.ValidID(in.BasePolicyID) || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("policy", "请指定准确分类、基础修订、版本与发布原因")
	}
	in.BasePolicyID = strings.ToLower(in.BasePolicyID)
	in.AllowedChannels = slices.Clone(in.AllowedChannels)
	slices.Sort(in.AllowedChannels)
	if len(in.AllowedChannels) > 4 || len(slices.Compact(slices.Clone(in.AllowedChannels))) != len(in.AllowedChannels) {
		return core.Result{}, core.Invalid("allowed_channels", "渠道不能重复或超出已支持范围")
	}
	for _, channel := range in.AllowedChannels {
		if !slices.Contains([]string{"web", "wechat", "douyin", "android"}, channel) {
			return core.Result{}, core.Invalid("allowed_channels", "当前仅支持已接入的员工访问渠道，TG需另行验证披露规则")
		}
	}
	if in.Level == "internal" && !slices.Contains(in.AllowedChannels, "web") {
		return core.Result{}, core.Invalid("allowed_channels", "公司内部策略须保留Web管理入口，防止配置权限被整体锁定")
	}
	if in.Level == "internal" && !slices.Contains(in.AllowedChannels, c.Actor.Channel) {
		return core.Result{}, core.Invalid("allowed_channels", "请保留当前管理渠道；如需关闭该渠道，请先通过Web重新登录并核对")
	}
	if in.Level == "confidential" && in.AIAllowed {
		return core.Result{}, core.Invalid("ai_allowed", "严格受限资料尚未开放AI处理，需要独立范围与准入验证")
	}
	c.AuthorizationMutation = true
	return (platform.Commands{DB: s.DB}).Do(ctx, c, func(tx pgx.Tx) error { return classificationAuthority(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		current, err := scanClassification(tx.QueryRow(ctx, "SELECT "+classificationColumns+" FROM classification_policies WHERE org_id=$1 AND level=$2 ORDER BY policy_version DESC LIMIT 1", c.Actor.OrgID, in.Level))
		if err != nil {
			return core.Result{}, err
		}
		if current.Version != in.ExpectedVersion || current.ID != in.BasePolicyID {
			return core.Result{}, core.Conflict()
		}
		channels := in.AllowedChannels
		if channels == nil {
			channels = []string{}
		}
		existing := slices.Clone(current.AllowedChannels)
		slices.Sort(existing)
		if current.AIAllowed == in.AIAllowed && slices.Equal(existing, channels) {
			return core.Result{}, core.Fail(409, "POLICY_UNCHANGED", "策略内容未变化，无需重复发布")
		}
		id := core.NewID()
		_, err = tx.Exec(ctx, `INSERT INTO classification_policies(id,org_id,level,policy_version,ai_allowed,allowed_channels,published_by,base_policy_id,reason)
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, c.Actor.OrgID, in.Level, current.Version+1, in.AIAllowed, channels, c.Actor.UserID, current.ID, in.Reason)
		if err != nil {
			return core.Result{}, err
		}
		if _, err = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", c.Actor.OrgID); err != nil {
			return core.Result{}, err
		}
		p, err := scanClassification(tx.QueryRow(ctx, "SELECT "+classificationColumns+" FROM classification_policies WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id))
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "identity.classification_policy.published", ResourceType: "classification_policy", ResourceID: id, Version: p.Version, Payload: map[string]any{"level": in.Level, "base_policy_id": current.ID, "ai_allowed": in.AIAllowed, "allowed_channels": channels, "reason": in.Reason}}}, nil
	})
}
