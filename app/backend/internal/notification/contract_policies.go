package notification

import (
	"context"
	"slices"
	"strings"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type ContractPolicy struct {
	ID          string    `json:"id"`
	Version     int64     `json:"version"`
	Enabled     bool      `json:"enabled"`
	Timezone    string    `json:"timezone"`
	DaysBefore  []int     `json:"days_before"`
	EffectiveOn string    `json:"effective_on"`
	CreatedBy   string    `json:"created_by"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
}
type ContractPolicyState struct {
	Current    *ContractPolicy `json:"current"`
	CanPublish bool            `json:"can_publish"`
}
type ContractPolicyInput struct {
	ExpectedVersion int64  `json:"expected_version"`
	Enabled         bool   `json:"enabled"`
	Timezone        string `json:"timezone"`
	DaysBefore      []int  `json:"days_before"`
	Reason          string `json:"reason"`
}

const contractPolicyColumns = `id,version,enabled,timezone,days_before,effective_on::text,created_by,reason,created_at`

func scanContractPolicy(row pgx.Row) (ContractPolicy, error) {
	var p ContractPolicy
	err := row.Scan(&p.ID, &p.Version, &p.Enabled, &p.Timezone, &p.DaysBefore, &p.EffectiveOn, &p.CreatedBy, &p.Reason, &p.CreatedAt)
	return p, err
}
func currentContractPolicy(ctx context.Context, tx platform.DBTX, org string) (*ContractPolicy, error) {
	p, err := scanContractPolicy(tx.QueryRow(ctx, `SELECT `+contractPolicyColumns+` FROM notification_contract_policies WHERE org_id=$1 ORDER BY version DESC LIMIT 1`, org))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
func policyAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, write bool) error {
	if err := authz.RequireOrganization(ctx, tx, a, "notification.policy.read"); err != nil {
		return err
	}
	if write {
		if err := authz.RequireOrganization(ctx, tx, a, "notification.policy.manage"); err != nil {
			return err
		}
	}
	return identity.RequireInteractiveSession(ctx, tx, a, write)
}
func (s *Service) ContractPolicy(ctx context.Context, a core.Actor) (ContractPolicyState, error) {
	var out ContractPolicyState
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = policyAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	out.Current, err = currentContractPolicy(ctx, tx, a.OrgID)
	if err != nil {
		return out, err
	}
	out.CanPublish = authz.RequireOrganization(ctx, tx, a, "notification.policy.manage") == nil
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) PublishContractPolicy(ctx context.Context, c core.Command, in ContractPolicyInput) (core.Result, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if in.ExpectedVersion < 0 || in.ExpectedVersion >= 9007199254740991 || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("policy", "请核对当前策略版本并填写1至200字的变更原因")
	}
	// Disabled policies may retain chosen thresholds or use an explicit empty list.
	rule := ContractRule{OrgID: c.Actor.OrgID, Timezone: in.Timezone, DaysBefore: in.DaysBefore}
	if !in.Enabled && len(in.DaysBefore) == 0 {
		rule.DaysBefore = []int{0}
	}
	if err := rule.Validate(); err != nil {
		return core.Result{}, err
	}
	if in.DaysBefore == nil {
		return core.Result{}, core.Invalid("days_before", "请明确提供提醒提前量列表")
	}
	in.DaysBefore = slices.Clone(in.DaysBefore)
	slices.Sort(in.DaysBefore)
	c.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return policyAuthority(ctx, tx, c.Actor, true) }, func(tx pgx.Tx) (core.Result, error) {
		previous, err := currentContractPolicy(ctx, tx, c.Actor.OrgID)
		if err != nil {
			return core.Result{}, err
		}
		version := int64(0)
		if previous != nil {
			version = previous.Version
		}
		if version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		p, err := scanContractPolicy(tx.QueryRow(ctx, `INSERT INTO notification_contract_policies(id,org_id,version,enabled,timezone,days_before,effective_on,created_by,reason,created_at)
   SELECT $1,$2,$3,$4,$5,$6,(stamp AT TIME ZONE $5)::date,$7,$8,stamp FROM (SELECT clock_timestamp() stamp) at
   RETURNING `+contractPolicyColumns, core.NewID(), c.Actor.OrgID, version+1, in.Enabled, in.Timezone, in.DaysBefore, c.Actor.UserID, in.Reason))
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "notification.contract_policy.publish", ResourceType: "notification_contract_policy", ResourceID: p.ID, Version: p.Version,
			Payload: map[string]any{"policy_id": p.ID, "enabled": p.Enabled, "timezone": p.Timezone, "days_before": p.DaysBefore, "effective_on": p.EffectiveOn, "reason": p.Reason}}}, nil
	})
}
func (s *Service) ContractPolicyHistory(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[ContractPolicy], error) {
	out := core.Page[ContractPolicy]{Items: []ContractPolicy{}}
	if limit < 1 || limit > 50 {
		return out, core.Invalid("limit", "每次读取1至50条策略")
	}
	tx, err := identity.BeginInteractiveRead(ctx, s.DB, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = policyAuthority(ctx, tx, a, false); err != nil {
		return out, err
	}
	binding := core.CursorBinding(a, "notification-contract-policies")
	var before *time.Time
	var id *string
	if cursor != "" {
		v, e := s.Pager.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		before = &v.UpdatedAt
		id = &v.ID
	}
	rows, err := tx.Query(ctx, `SELECT `+contractPolicyColumns+` FROM notification_contract_policies WHERE org_id=$1
  AND ($2::timestamptz IS NULL OR (created_at,id)<($2,$3::uuid)) ORDER BY created_at DESC,id DESC LIMIT $4`, a.OrgID, before, id, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		p, e := scanContractPolicy(rows)
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
		p := out.Items[limit-1]
		next := s.Pager.Encode(platform.Cursor{ID: p.ID, UpdatedAt: p.CreatedAt, Binding: binding})
		out.NextCursor = &next
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
