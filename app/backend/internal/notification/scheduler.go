package notification

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
	_ "time/tzdata"
)

// A deployment must explicitly select the organization, business timezone and
// advance days. No company reminder policy is invented by an enabled default.
type ContractRule struct {
	OrgID, Timezone       string
	DaysBefore            []int
	policyID, effectiveOn string
}

func (r ContractRule) Validate() error {
	if r.OrgID == "" && r.Timezone == "" && len(r.DaysBefore) == 0 {
		return nil
	}
	if !core.ValidID(r.OrgID) || r.Timezone == "" || r.Timezone == "Local" || len(r.DaysBefore) < 1 || len(r.DaysBefore) > 12 {
		return core.Invalid("contract_reminders", "合同提醒需指定组织、业务时区及1至12个提前量")
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return core.Invalid("timezone", "合同提醒时区无效")
	}
	seen := map[int]bool{}
	for _, days := range r.DaysBefore {
		if days < 0 || days > 365 || seen[days] {
			return core.Invalid("days_before", "合同提醒提前量须为互不重复的0至365天")
		}
		seen[days] = true
	}
	return nil
}

type ScheduleResult struct {
	Queued       int  `json:"queued"`
	LimitReached bool `json:"limit_reached"`
	Enabled      bool `json:"enabled"`
}

func (s *Service) ScheduleContracts(ctx context.Context, rule ContractRule) (ScheduleResult, error) {
	out := ScheduleResult{Enabled: len(rule.DaysBefore) > 0}
	if err := rule.Validate(); err != nil {
		return out, err
	}
	if rule.OrgID != "" {
		p, err := currentContractPolicy(ctx, s.DB, rule.OrgID)
		if err != nil {
			return out, err
		}
		if p != nil {
			out.Enabled = p.Enabled
			if !p.Enabled {
				return out, nil
			}
			rule.Timezone = p.Timezone
			rule.DaysBefore = p.DaysBefore
			rule.policyID = p.ID
			rule.effectiveOn = p.EffectiveOn
		}
	}
	if !out.Enabled {
		return out, nil
	}
	days := slices.Clone(rule.DaysBefore)
	slices.Sort(days)
	rows, err := s.DB.Query(ctx, `SELECT c.id FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.active_revision_id
  JOIN LATERAL (SELECT min(d) AS days FROM unnest($2::int[]) d WHERE r.end_date-d <= (clock_timestamp() AT TIME ZONE $3)::date
    AND (NULLIF($4,'')::date IS NULL OR r.end_date-d >= NULLIF($4,'')::date)) next ON next.days IS NOT NULL
  WHERE c.org_id=$1 AND c.lifecycle='effective' AND r.end_date IS NOT NULL
  AND NOT EXISTS(SELECT 1 FROM notification_due_events n WHERE n.org_id=c.org_id AND n.contract_id=c.id AND n.revision_id=r.id AND n.due_on=r.end_date AND n.days_before=next.days)
  ORDER BY r.end_date,c.id LIMIT 101`, rule.OrgID, days, rule.Timezone, rule.effectiveOn)
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
	out.LimitReached = len(ids) > 100
	if len(ids) > 100 {
		ids = ids[:100]
	}
	for _, id := range ids {
		queued, e := s.scheduleContract(ctx, rule, days, id)
		if e != nil {
			return out, e
		}
		if queued {
			out.Queued++
		}
	}
	return out, nil
}
func (s *Service) scheduleContract(ctx context.Context, rule ContractRule, days []int, id string) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", rule.OrgID); err != nil {
		return false, err
	}
	current, err := currentContractPolicy(ctx, tx, rule.OrgID)
	if err != nil {
		return false, err
	}
	// Publication takes the exclusive organization lock. A candidate discovered
	// under an old policy (or fallback) cannot commit after that publication.
	if (current == nil && rule.policyID != "") || (current != nil && (!current.Enabled || current.ID != rule.policyID)) {
		return false, nil
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 59313))", rule.OrgID+"/"+id); err != nil {
		return false, err
	}
	var revision, due string
	var version int64
	var advance int
	err = tx.QueryRow(ctx, `SELECT r.id,r.end_date::text,c.version,next.days FROM contracts c JOIN contract_revisions r ON r.org_id=c.org_id AND r.id=c.active_revision_id
  JOIN LATERAL (SELECT min(d) AS days FROM unnest($3::int[]) d WHERE r.end_date-d <= (clock_timestamp() AT TIME ZONE $4)::date
    AND (NULLIF($5,'')::date IS NULL OR r.end_date-d >= NULLIF($5,'')::date)) next ON next.days IS NOT NULL
  WHERE c.org_id=$1 AND c.id=$2 AND c.lifecycle='effective' AND r.end_date IS NOT NULL FOR SHARE OF c`, rule.OrgID, id, days, rule.Timezone, rule.effectiveOn).Scan(&revision, &due, &version, &advance)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_due_events WHERE org_id=$1 AND contract_id=$2 AND revision_id=$3 AND due_on=$4::date AND days_before=$5)`, rule.OrgID, id, revision, due, advance).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	eventID := core.NewID()
	payload := map[string]any{"revision_id": revision, "due_on": due, "days_before": advance, "timezone": rule.Timezone}
	if rule.policyID != "" {
		payload["policy_id"] = rule.policyID
		payload["policy_version"] = current.Version
	}
	raw, _ := json.Marshal(payload)
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
  VALUES($1,$2,'contract.reminder.due','contract',$3,$4,$5,$6)`, eventID, rule.OrgID, id, version, raw, "notification-schedule/"+eventID); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO notification_due_events(id,org_id,contract_id,revision_id,due_on,days_before,timezone,event_id,policy_id)
  VALUES($1,$2,$3,$4,$5::date,$6,$7,$8,NULLIF($9,'')::uuid)`, core.NewID(), rule.OrgID, id, revision, due, advance, rule.Timezone, eventID, rule.policyID); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// Bound each scheduler pass without starving organizations beyond the first page.
// The worker keeps the last organization key and wraps after the final page.
func (s *Service) ScheduleContractPolicies(ctx context.Context, after string) (string, int, error) {
	rows, err := s.DB.Query(ctx, `SELECT o.id,p.timezone FROM organizations o
  JOIN LATERAL (SELECT timezone FROM notification_contract_policies WHERE org_id=o.id ORDER BY version DESC LIMIT 1) p ON true
  WHERE ($1='' OR o.id>NULLIF($1,'')::uuid) ORDER BY o.id LIMIT 101`, after)
	if err != nil {
		return "", 0, err
	}
	rules := []ContractRule{}
	for rows.Next() {
		var r ContractRule
		if err = rows.Scan(&r.OrgID, &r.Timezone); err != nil {
			rows.Close()
			return "", 0, err
		}
		r.DaysBefore = []int{0}
		rules = append(rules, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return "", 0, err
	}
	next := ""
	if len(rules) > 100 {
		rules = rules[:100]
		next = rules[99].OrgID
	}
	queued := 0
	for _, r := range rules {
		v, e := s.ScheduleContracts(ctx, r)
		if e != nil {
			return after, queued, e
		}
		queued += v.Queued
	}
	return next, queued, nil
}
