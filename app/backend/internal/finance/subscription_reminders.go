package finance

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func (s *Service) SubscriptionReminders(ctx context.Context, a core.Actor, id, cursor string, limit int) (core.Page[SubscriptionReminder], error) {
	out := core.Page[SubscriptionReminder]{Items: []SubscriptionReminder{}}
	if _, err := s.Subscription(ctx, a, id); err != nil {
		return out, err
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	bound := core.CursorBinding(a, "subscription-reminders", id)
	var at *time.Time
	var before *string
	if cursor != "" {
		v, err := s.Pager.Decode(cursor, bound)
		if err != nil {
			return out, err
		}
		at, before = &v.UpdatedAt, &v.ID
	}
	rows, err := s.DB.Query(ctx, `SELECT n.id,n.period_id,n.days_before,n.remind_on::text,n.recipient_id,u.display_name,p.start_date::text,r.timezone,n.created_at,
        CASE WHEN EXISTS(SELECT 1 FROM notification_deliveries d WHERE d.org_id=n.org_id AND d.business_event_id=n.outbox_id AND d.recipient_id=n.recipient_id) THEN 'delivered'
        WHEN EXISTS(SELECT 1 FROM notification_job_results r WHERE r.org_id=n.org_id AND r.business_event_id=n.outbox_id) THEN 'suppressed'
        WHEN EXISTS(SELECT 1 FROM jobs j WHERE j.org_id=n.org_id AND j.kind='notification.deliver.v1' AND j.dedup_key='notification/event/'||n.outbox_id::text AND j.state='dead') THEN 'failed'
        WHEN EXISTS(SELECT 1 FROM jobs j WHERE j.org_id=n.org_id AND j.kind='notification.deliver.v1' AND j.dedup_key='notification/event/'||n.outbox_id::text AND j.state='retry_wait') THEN 'retry_wait'
        ELSE 'queued' END
		FROM subscription_reminders n JOIN subscription_periods p ON p.org_id=n.org_id AND p.id=n.period_id JOIN subscription_period_revisions r ON r.org_id=p.org_id AND r.id=p.current_revision_id
		JOIN users u ON u.org_id=n.org_id AND u.id=n.recipient_id
		WHERE n.org_id=$1 AND p.subscription_id=$2 AND can_read_subscription(p.org_id,$3,p.subscription_id,$4,$5)
		AND ($6::timestamptz IS NULL OR (n.created_at,n.id)<($6,$7::uuid)) ORDER BY n.created_at DESC,n.id DESC LIMIT $8`, a.OrgID, id, a.UserID, a.Channel, a.RunID != "", at, before, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v SubscriptionReminder
		if err = rows.Scan(&v.ID, &v.PeriodID, &v.DaysBefore, &v.RemindOn, &v.RecipientID, &v.RecipientDisplayName, &v.PeriodStartDate, &v.Timezone, &v.CreatedAt, &v.DeliveryState); err != nil {
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		v := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.CreatedAt, Binding: bound})
		out.NextCursor = &v
	}
	return out, nil
}
