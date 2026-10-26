package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

type SubscriptionGenerationJob struct {
	SubscriptionID          string `json:"subscription_id"`
	ConfigurationRevisionID string `json:"configuration_revision_id"`
	ConfigurationHash       string `json:"configuration_hash"`
	ControlVersion          int64  `json:"control_version"`
	BusinessDate            string `json:"business_date"`
	Window                  string `json:"window"`
	From                    string `json:"from"`
	To                      string `json:"to"`
}
type SubscriptionScheduleResult struct {
	Enqueued     int
	LimitReached bool
}
type SubscriptionJobResult struct {
	Generated            *SubscriptionGeneration
	RemindersQueued      int
	ReminderLimitReached bool
	Skipped              bool
}

// Each business date has fixed forward and catch-up windows. Long gaps remain
// visible as coverage gaps and can be backfilled by bounded manual generation.
func (s *Service) ScheduleSubscriptions(ctx context.Context) (SubscriptionScheduleResult, error) {
	result := SubscriptionScheduleResult{}
	rows, err := s.DB.Query(ctx, "SELECT "+subscriptionColumns+subscriptionJoin+`
        CROSS JOIN LATERAL(SELECT (clock_timestamp() AT TIME ZONE COALESCE(NULLIF(r.payload->>'timezone',''),'Asia/Hong_Kong'))::date today) d
        CROSS JOIN LATERAL(SELECT max(subscription_version) control_version FROM subscription_events e WHERE e.org_id=s.org_id AND e.subscription_id=s.id AND e.kind IN ('activated','resumed','restored')) control
        WHERE s.state IN ('active','cancelled') AND control.control_version IS NOT NULL AND (r.payload->>'first_charge_date')::date<d.today+366 AND (s.cancel_effective_on IS NULL OR s.cancel_effective_on>d.today-366)
        AND (NOT EXISTS(SELECT 1 FROM jobs j WHERE j.org_id=s.org_id AND j.dedup_key=format('subscription/%s/%s/%s/%s/forward',s.id,s.current_revision_id,control.control_version,d.today))
            OR ((r.payload->>'first_charge_date')::date<d.today AND NOT EXISTS(SELECT 1 FROM jobs j WHERE j.org_id=s.org_id AND j.dedup_key=format('subscription/%s/%s/%s/%s/catchup',s.id,s.current_revision_id,control.control_version,d.today)))) ORDER BY s.org_id,s.id LIMIT 1001`)
	if err != nil {
		return result, err
	}
	subs := []Subscription{}
	orgs := []string{}
	controls := []int64{}
	// subscriptionColumns intentionally contains no organization; obtain it in
	// a separate projection rather than assume one company in the process.
	for rows.Next() {
		v, e := scanSubscription(rows)
		if e != nil {
			rows.Close()
			return result, e
		}
		subs = append(subs, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(subs) > 1000 {
		result.LimitReached = true
		subs = subs[:1000]
	}
	for _, sub := range subs {
		var org string
		var control int64
		if err = s.DB.QueryRow(ctx, "SELECT org_id,(SELECT max(subscription_version) FROM subscription_events e WHERE e.org_id=s.org_id AND e.subscription_id=s.id AND e.kind IN ('activated','resumed','restored')) FROM subscriptions s WHERE id=$1", sub.ID).Scan(&org, &control); err != nil {
			return result, err
		}
		orgs = append(orgs, org)
		controls = append(controls, control)
	}
	queue := jobqueue.Queue{DB: s.DB}
	for i, sub := range subs {
		today, e := subscriptionDate(ctx, s.DB, sub.Timezone)
		if e != nil {
			return result, e
		}
		date, _ := civilDate(today)
		for _, window := range []string{"forward", "catchup"} {
			from, to := date, date.AddDate(0, 0, 366)
			if window == "catchup" {
				from, to = date.AddDate(0, 0, -366), date
			}
			if sub.FirstChargeDate != nil && from.Format(time.DateOnly) < *sub.FirstChargeDate {
				from, e = civilDate(*sub.FirstChargeDate)
				if e != nil {
					return result, e
				}
			}
			if !to.After(from) {
				continue
			}
			input := SubscriptionGenerationJob{SubscriptionID: sub.ID, ConfigurationRevisionID: sub.CurrentRevisionID, ConfigurationHash: sub.ContentHash, ControlVersion: controls[i], BusinessDate: today, Window: window, From: from.Format(time.DateOnly), To: to.Format(time.DateOnly)}
			payload, _ := json.Marshal(input)
			tx, e := s.DB.Begin(ctx)
			if e != nil {
				return result, e
			}
			_, inserted, e := queue.Enqueue(ctx, tx, jobqueue.Input{OrgID: orgs[i], Kind: "subscription.generate.v1", Lane: "finance", DedupKey: fmt.Sprintf("subscription/%s/%s/%d/%s/%s", sub.ID, sub.CurrentRevisionID, controls[i], today, window), PayloadVersion: 1, Payload: payload, MaxAttempts: 8})
			if e == nil {
				e = tx.Commit(ctx)
			} else {
				tx.Rollback(ctx)
			}
			if e != nil {
				return result, e
			}
			if inserted {
				result.Enqueued++
			}
		}
	}
	return result, nil
}
func validateSubscriptionJob(j jobqueue.Job) (SubscriptionGenerationJob, error) {
	var in SubscriptionGenerationJob
	if j.Kind != "subscription.generate.v1" || j.Lane != "finance" || j.PayloadVersion != 1 || json.Unmarshal(j.Payload, &in) != nil || !core.ValidID(in.SubscriptionID) || !core.ValidID(in.ConfigurationRevisionID) || len(in.ConfigurationHash) != 64 || in.ControlVersion < 1 || !slices.Contains([]string{"forward", "catchup"}, in.Window) {
		return in, core.Invalid("job", "订阅任务固定输入无效")
	}
	from, e := civilDate(in.From)
	if e != nil {
		return in, e
	}
	to, e := civilDate(in.To)
	if e != nil || !to.After(from) || (to.Unix()-from.Unix())/86400 > 366 {
		return in, core.Invalid("job", "订阅任务区间超出366个自然日")
	}
	if _, e = civilDate(in.BusinessDate); e != nil {
		return in, e
	}
	return in, nil
}
func subscriptionJobEvent(ctx context.Context, tx pgx.Tx, j jobqueue.Job, s Subscription, kind string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request := "job/" + j.ID
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,job_id,action,resource_type,resource_id,resource_version,after_digest,result,reason) VALUES($1,$2,NULL,NULL,'system',$3,$4,$5,'subscription',$6,$7,$8,'succeeded','固定账期生成，不代表真实扣款')`, core.NewID(), j.OrgID, request, j.ID, kind, s.ID, s.Version, core.Digest(raw)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id) VALUES($1,$2,$3,'subscription',$4,$5,$6,$7)`, core.NewID(), j.OrgID, kind, s.ID, s.Version, raw, request)
	return err
}
func (s *Service) ProcessSubscriptionJob(ctx context.Context, j jobqueue.Job) (SubscriptionJobResult, error) {
	result := SubscriptionJobResult{}
	in, err := validateSubscriptionJob(j)
	if err != nil {
		return result, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", j.OrgID); err != nil {
		return result, err
	}
	sub, err := subscriptionMetadata(ctx, tx, j.OrgID, in.SubscriptionID)
	if err != nil {
		return result, err
	}
	if sub.ProjectID != nil {
		if _, err = tx.Exec(ctx, "SELECT id FROM projects WHERE org_id=$1 AND id=$2 FOR UPDATE", j.OrgID, *sub.ProjectID); err != nil {
			return result, err
		}
	}
	if _, err = tx.Exec(ctx, "SELECT id FROM subscriptions WHERE org_id=$1 AND id=$2 FOR UPDATE", j.OrgID, sub.ID); err != nil {
		return result, err
	}
	queue := jobqueue.Queue{DB: s.DB}
	if err = queue.CheckInTx(ctx, tx, j); err != nil {
		return result, err
	}
	sub, err = subscriptionMetadata(ctx, tx, j.OrgID, sub.ID)
	if err != nil {
		return result, err
	}
	var control int64
	if err = tx.QueryRow(ctx, "SELECT COALESCE(max(subscription_version),0) FROM subscription_events WHERE org_id=$1 AND subscription_id=$2 AND kind IN ('activated','resumed','restored')", j.OrgID, sub.ID).Scan(&control); err != nil {
		return result, err
	}
	if sub.CurrentRevisionID != in.ConfigurationRevisionID || sub.ContentHash != in.ConfigurationHash || control != in.ControlVersion || !slices.Contains([]string{"active", "cancelled"}, sub.State) {
		result.Skipped = true
		if err = subscriptionJobEvent(ctx, tx, j, sub, "subscription.generation.skipped", map[string]any{"job_id": j.ID, "configuration_revision_id": in.ConfigurationRevisionID, "reason": "固定配置已过期或订阅已暂停/归档"}); err != nil {
			return result, err
		}
	} else {
		sub.Version++
		if _, err = tx.Exec(ctx, "UPDATE subscriptions SET version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", j.OrgID, sub.ID); err != nil {
			return result, err
		}
		parent := core.NewID()
		if _, err = tx.Exec(ctx, `INSERT INTO subscription_events(id,org_id,subscription_id,subscription_version,revision_id,kind,state,job_id,job_fence,job_owner,reason) VALUES($1,$2,$3,$4,$5,'periods_generated',$6,$7,$8,$9,'持久任务按固定日历生成预测')`, parent, j.OrgID, sub.ID, sub.Version, sub.CurrentRevisionID, sub.State, j.ID, j.FencingToken, j.LeaseOwner); err != nil {
			return result, err
		}
		generated, e := generateSubscriptionPeriods(ctx, tx, core.Actor{OrgID: j.OrgID}, sub, in.From, in.To, parent, "持久任务按固定日历生成预测", &j)
		if e != nil {
			return result, e
		}
		result.Generated = &generated
		queued, limited, e := queueSubscriptionReminders(ctx, tx, j, sub)
		if e != nil {
			return result, e
		}
		result.RemindersQueued = queued
		result.ReminderLimitReached = limited
		if err = subscriptionJobEvent(ctx, tx, j, sub, "subscription.periods.generated", map[string]any{"job_id": j.ID, "configuration_revision_id": sub.CurrentRevisionID, "generation": generated, "reminders_queued": queued, "reminder_limit_reached": limited}); err != nil {
			return result, err
		}
	}
	if err = queue.CompleteInTx(ctx, tx, j); err != nil {
		return result, err
	}
	return result, platform.DatabaseError(tx.Commit(ctx))
}
func queueSubscriptionReminders(ctx context.Context, tx pgx.Tx, j jobqueue.Job, sub Subscription) (int, bool, error) {
	today, err := subscriptionDate(ctx, tx, sub.Timezone)
	if err != nil {
		return 0, false, err
	}
	rows, err := tx.Query(ctx, `SELECT p.id,p.version,p.start_date::text,r.owner_id,d.days,(p.start_date-d.days)::text FROM subscription_periods p JOIN subscription_period_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id CROSS JOIN unnest($3::integer[]) d(days)
        WHERE p.org_id=$1 AND p.subscription_id=$2 AND p.state='forecast' AND subscription_charge_eligible(p.org_id,p.subscription_id,p.start_date) AND p.start_date-d.days<=$4::date AND p.start_date-d.days>=$4::date-366
        AND NOT EXISTS(SELECT 1 FROM subscription_reminders n WHERE n.org_id=p.org_id AND n.period_id=p.id AND n.days_before=d.days) ORDER BY p.start_date,d.days,p.id LIMIT 501`, j.OrgID, sub.ID, sub.ReminderDays, today)
	if err != nil {
		return 0, false, err
	}
	type pending struct {
		ID, Date, Owner, Remind string
		Version                 int64
		Days                    int
	}
	items := []pending{}
	for rows.Next() {
		var n pending
		if err = rows.Scan(&n.ID, &n.Version, &n.Date, &n.Owner, &n.Days, &n.Remind); err != nil {
			rows.Close()
			return 0, false, err
		}
		items = append(items, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, false, err
	}
	limited := len(items) > 500
	if limited {
		items = items[:500]
	}
	for _, n := range items {
		outbox := core.NewID()
		payload, _ := json.Marshal(map[string]any{"subscription_id": sub.ID, "period_id": n.ID, "period_version": n.Version, "recipient_id": n.Owner, "days_before": n.Days, "remind_on": n.Remind, "delivery_state": "queued"})
		if _, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id) VALUES($1,$2,'subscription.reminder.due','subscription_period',$3,$4,$5,$6)`, outbox, j.OrgID, n.ID, n.Version, payload, "job/"+j.ID); err != nil {
			return 0, false, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO subscription_reminders(id,org_id,period_id,days_before,remind_on,recipient_id,outbox_id,job_id,job_fence,job_owner) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, core.NewID(), j.OrgID, n.ID, n.Days, n.Remind, n.Owner, outbox, j.ID, j.FencingToken, j.LeaseOwner); err != nil {
			return 0, false, err
		}
	}
	return len(items), limited, nil
}
