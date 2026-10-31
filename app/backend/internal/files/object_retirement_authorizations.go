package files

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type RetirementRenewalItem struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
}
type RetirementRenewal struct {
	ExpectedVersion int64                   `json:"expected_version"`
	SnapshotSHA256  string                  `json:"snapshot_sha256"`
	Reason          string                  `json:"reason"`
	Items           []RetirementRenewalItem `json:"items"`
}
type RetirementAuthorizationItem struct {
	ItemID          string `json:"item_id"`
	PreviousVersion int64  `json:"previous_version"`
	Generation      int    `json:"generation"`
}
type RetirementAuthorization struct {
	ID                      string                        `json:"id"`
	Sequence                int64                         `json:"sequence"`
	PlanVersion             int64                         `json:"plan_version"`
	SnapshotSHA256          string                        `json:"snapshot_sha256"`
	Items                   []RetirementAuthorizationItem `json:"items"`
	Reason                  string                        `json:"reason"`
	AuthorizedAt            time.Time                     `json:"authorized_at"`
	AuthorizedBy            string                        `json:"authorized_by"`
	Channel                 string                        `json:"channel"`
	SessionID               string                        `json:"-"`
	AuthVersion, OrgVersion int64                         `json:"-"`
}

const retirementAuthorizationColumns = `id,sequence,plan_version,snapshot_sha256,items,reason,authorized_at,authorized_by,channel,session_id,auth_version,org_version`

func retirementAuthorization(row pgx.Row) (RetirementAuthorization, error) {
	var out RetirementAuthorization
	var raw []byte
	e := row.Scan(&out.ID, &out.Sequence, &out.PlanVersion, &out.SnapshotSHA256, &raw, &out.Reason, &out.AuthorizedAt, &out.AuthorizedBy, &out.Channel, &out.SessionID, &out.AuthVersion, &out.OrgVersion)
	if e != nil {
		return out, platform.DatabaseError(e)
	}
	e = json.Unmarshal(raw, &out.Items)
	return out, e
}
func (s *ObjectRetirementService) Authorizations(ctx context.Context, a core.Actor, planID string, filter Filter) (core.Page[RetirementAuthorization], error) {
	out := core.Page[RetirementAuthorization]{Items: []RetirementAuthorization{}}
	if _, e := s.readPlan(ctx, s.Files.DB, a, planID, false, false); e != nil {
		return out, e
	}
	at, id, limit, binding, e := cleanupPage(s.Files, a, "object-retirement-authorizations/"+planID, filter)
	if e != nil {
		return out, e
	}
	rows, e := s.Files.DB.Query(ctx, "SELECT "+retirementAuthorizationColumns+` FROM object_retirement_authorizations
 WHERE org_id=$1 AND plan_id=$2 AND ($3::timestamptz IS NULL OR (authorized_at,id)<($3,$4::uuid))
 ORDER BY authorized_at DESC,id DESC LIMIT $5`, a.OrgID, planID, at, id, limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := retirementAuthorization(rows)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		cleanupNext(s.Files, &out, limit, binding, last.ID, last.AuthorizedAt)
	}
	return out, nil
}

// A fresh human explicitly adopts the exact remaining items. Initial approval,
// source identities, permanent barriers and confirmed results remain untouched.
func (s *ObjectRetirementService) Reauthorize(ctx context.Context, c core.Command, id string, in RetirementRenewal) (core.Result, error) {
	if in.ExpectedVersion < 1 || core.Name(in.Reason) != nil || len(in.Items) < 1 || len(in.Items) > 2 {
		return core.Result{}, core.Invalid("items", "请核对剩余原件及修订，并说明重新授权依据")
	}
	seen := map[string]bool{}
	for _, item := range in.Items {
		if !core.ValidID(item.ID) || item.ExpectedVersion < 1 || seen[item.ID] {
			return core.Result{}, core.Invalid("items", "剩余原件不得重复，且需提供当前修订")
		}
		seen[item.ID] = true
	}
	c.ExclusiveOrganizationLock = true
	return s.Files.Commands.Do(ctx, c, func(tx pgx.Tx) error { _, e := s.readPlan(ctx, tx, c.Actor, id, true, false); return e }, func(tx pgx.Tx) (core.Result, error) {
		p, e := s.readPlan(ctx, tx, c.Actor, id, true, true)
		if e != nil {
			return core.Result{}, e
		}
		if p.State != "approved" || p.Version != in.ExpectedVersion || p.SnapshotSHA256 != in.SnapshotSHA256 {
			return core.Result{}, core.Conflict()
		}
		selected := []RetirementAuthorizationItem{}
		for _, item := range p.Items {
			if item.State == "succeeded" {
				continue
			}
			if !slices.Contains([]string{"blocked", "failed"}, item.State) {
				return core.Result{}, core.Fail(409, "CLEANUP_WORK_NOT_STOPPED", "请等待剩余清理任务停止后刷新核对")
			}
			i := slices.IndexFunc(in.Items, func(v RetirementRenewalItem) bool { return v.ID == item.ID && v.ExpectedVersion == item.Version })
			if i < 0 {
				return core.Result{}, core.Conflict()
			}
			selected = append(selected, RetirementAuthorizationItem{ItemID: item.ID, PreviousVersion: item.Version, Generation: item.Generation + 1})
		}
		if len(selected) == 0 || len(selected) != len(in.Items) {
			return core.Result{}, core.Conflict()
		}
		// Recheck the fixed source/policy/barriers using current authority, never the
		// expired grant. This deliberately cannot adopt a changed source or policy.
		if e = s.retirementSelectionAllowed(ctx, tx, p, c.Actor.OrgID); e != nil {
			return core.Result{}, e
		}
		seq := int64(1)
		if p.Authorization != nil {
			seq = p.Authorization.Sequence + 1
		}
		grant := core.NewID()
		raw, e := json.Marshal(selected)
		if e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO object_retirement_authorizations(id,org_id,plan_id,sequence,plan_version,previous_id,snapshot_sha256,items,reason,authorized_by,session_id,channel,auth_version,org_version)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, grant, c.Actor.OrgID, id, seq, p.Version+1, p.ExecutionAuthorizationID, p.SnapshotSHA256, raw, in.Reason, c.Actor.UserID, c.Actor.SessionID, c.Actor.Channel, c.Actor.AuthVersion, c.Actor.OrgAuthVersion); e != nil {
			return core.Result{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE object_retirement_plans SET version=version+1,execution_authorization_id=$3 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, grant); e != nil {
			return core.Result{}, e
		}
		for _, item := range selected {
			if _, e = tx.Exec(ctx, "UPDATE object_retirement_items SET state='queued',version=version+1,generation=generation+1,last_error=NULL,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, item.ItemID); e != nil {
				return core.Result{}, e
			}
			if e = s.enqueue(ctx, tx, c.Actor.OrgID, id, item.ItemID, item.Generation); e != nil {
				return core.Result{}, e
			}
		}
		p, e = retirementPlan(ctx, tx, c.Actor.OrgID, id, false)
		return core.Result{Status: 202, Data: p, Event: retirementEvent("file.original.retirement.reauthorized", p, in.Reason)}, e
	})
}
