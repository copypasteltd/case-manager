package notification

import (
	"context"
	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"slices"
	"time"
)

const candidateLimit = 1000

func scan(row pgx.Row) (thread, error) {
	var n thread
	err := row.Scan(&n.ID, &n.OrgID, &n.RecipientID, &n.Category, &n.ResourceType, &n.ResourceID, &n.ContextID, &n.DedupKey,
		&n.DeliveredSeq, &n.ReadSeq, &n.Version, &n.CreatedAt, &n.UpdatedAt, &n.ReadAt, &n.EventID, &n.EventOccurredAt)
	return n, platform.DatabaseError(err)
}
func record(ctx context.Context, tx pgx.Tx, a core.Actor, id string, lock bool) (thread, error) {
	if !core.ValidID(id) {
		return thread{}, core.Missing()
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF n"
	}
	return scan(tx.QueryRow(ctx, "SELECT "+threadColumns+threadJoin+"WHERE n.org_id=$1 AND n.recipient_id=$2 AND n.id=$3"+suffix, a.OrgID, a.UserID, id))
}
func (s *Service) readTx(ctx context.Context, a core.Actor) (pgx.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	if err = snapshot(ctx, tx, a); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func candidates(ctx context.Context, tx pgx.Tx, a core.Actor, f Filter, before *platform.Cursor) ([]thread, error) {
	var at *time.Time
	var id *string
	if before != nil {
		at = &before.UpdatedAt
		id = &before.ID
	}
	rows, err := tx.Query(ctx, "SELECT "+threadColumns+threadJoin+`WHERE n.org_id=$1 AND n.recipient_id=$2
  AND ($3<>'unread' OR n.read_seq<n.delivered_seq) AND ($4='' OR n.category=$4)
  AND ($5::timestamptz IS NULL OR (n.updated_at,n.id)<($5,$6::uuid))
  ORDER BY n.updated_at DESC,n.id DESC LIMIT $7`, a.OrgID, a.UserID, f.State, f.Category, at, id, candidateLimit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []thread{}
	for rows.Next() {
		n, e := scan(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, n)
	}
	return result, rows.Err()
}
func (s *Service) List(ctx context.Context, a core.Actor, f Filter) (core.Page[Notification], error) {
	out := core.Page[Notification]{Items: []Notification{}}
	if f.State == "" {
		f.State = "all"
	}
	if !slices.Contains([]string{"all", "unread", "attention"}, f.State) {
		return out, core.Invalid("state", "请选择全部、未读或待处理通知")
	}
	if f.Category != "" && categories[f.Category] == "" {
		return out, core.Invalid("category", "通知类型无效")
	}
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 50 {
		return out, core.Invalid("limit", "每次读取1至50条通知")
	}
	tx, err := s.readTx(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	binding := core.CursorBinding(a, "notifications", f.State, f.Category)
	var before *platform.Cursor
	if f.Cursor != "" {
		c, e := s.Pager.Decode(f.Cursor, binding)
		if e != nil {
			return out, e
		}
		before = &c
	}
	pending, err := candidates(ctx, tx, a, f, before)
	if err != nil {
		return out, err
	}
	var lastVisible *thread
	var lastScanned *thread
	more := len(pending) > candidateLimit
	for i, n := range pending {
		if i == candidateLimit {
			break
		}
		current := n
		lastScanned = &current
		v, e := view(ctx, tx, a, n)
		if e != nil {
			return out, e
		}
		if v == nil || (f.State == "attention" && !v.AttentionRequired) {
			continue
		}
		if len(out.Items) == f.Limit {
			more = true
			break
		}
		out.Items = append(out.Items, *v)
		lastVisible = &current
	}
	if more {
		last := lastVisible
		if last == nil {
			last = lastScanned
		}
		if last != nil {
			cursor := s.Pager.Encode(platform.Cursor{ID: last.ID, UpdatedAt: last.UpdatedAt, Binding: binding})
			out.NextCursor = &cursor
		}
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) Summary(ctx context.Context, a core.Actor) (Summary, error) {
	out := Summary{CountComplete: true}
	tx, err := s.readTx(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	pending, err := candidates(ctx, tx, a, Filter{State: "all"}, nil)
	if err != nil {
		return out, err
	}
	out.CountComplete = len(pending) <= candidateLimit
	for i, n := range pending {
		if i == candidateLimit {
			break
		}
		v, e := view(ctx, tx, a, n)
		if e != nil {
			return out, e
		}
		if v == nil {
			continue
		}
		if !v.Read {
			out.UnreadCount++
		}
		if v.AttentionRequired {
			out.AttentionCount++
		}
	}
	if out.UnreadCount > 100 {
		out.UnreadCount = 100
		out.CountComplete = false
	}
	if out.AttentionCount > 100 {
		out.AttentionCount = 100
		out.CountComplete = false
	}
	if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&out.AsOf); err != nil {
		return out, err
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) Get(ctx context.Context, a core.Actor, id string) (Notification, error) {
	tx, err := s.readTx(ctx, a)
	if err != nil {
		return Notification{}, err
	}
	defer tx.Rollback(ctx)
	n, err := record(ctx, tx, a, id, false)
	if err != nil {
		return Notification{}, err
	}
	v, err := view(ctx, tx, a, n)
	if err != nil {
		return Notification{}, err
	}
	if v == nil {
		return Notification{}, core.Missing()
	}
	return *v, platform.DatabaseError(tx.Commit(ctx))
}
func (s *Service) MarkRead(ctx context.Context, c core.Command, id string, in ReadInput) (core.Result, error) {
	if in.DeliverySeq < 1 || in.DeliverySeq > 9007199254740991 {
		return core.Result{}, core.Invalid("delivery_seq", "请指定实际看到的通知投递序号")
	}
	authorize := func(tx pgx.Tx) error {
		if err := snapshot(ctx, tx, c.Actor); err != nil {
			return err
		}
		n, err := record(ctx, tx, c.Actor, id, false)
		if err != nil {
			return err
		}
		v, err := view(ctx, tx, c.Actor, n)
		if err != nil {
			return err
		}
		if v == nil {
			return core.Missing()
		}
		return nil
	}
	return s.Commands.Do(ctx, c, authorize, func(tx pgx.Tx) (core.Result, error) {
		n, err := record(ctx, tx, c.Actor, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if in.DeliverySeq > n.DeliveredSeq {
			return core.Result{}, core.Fail(409, "NOTIFICATION_DELIVERY_CHANGED", "通知序号不一致，请刷新后重试")
		}
		if in.DeliverySeq > n.ReadSeq {
			if _, err = tx.Exec(ctx, "UPDATE notification_threads SET read_seq=$3,read_at=clock_timestamp(),version=version+1 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, in.DeliverySeq); err != nil {
				return core.Result{}, err
			}
		}
		n, err = record(ctx, tx, c.Actor, id, false)
		if err != nil {
			return core.Result{}, err
		}
		v, err := view(ctx, tx, c.Actor, n)
		if err != nil {
			return core.Result{}, err
		}
		if v == nil {
			return core.Result{}, core.Missing()
		}
		return core.Result{Data: v, Event: core.Event{Type: "notification.read", ResourceType: "notification", ResourceID: id, Version: n.Version, Payload: map[string]any{"read_seq": n.ReadSeq, "delivered_seq": n.DeliveredSeq}}}, nil
	})
}
func (s *Service) Failures(ctx context.Context, a core.Actor, cursor string, limit int) (core.Page[Failure], error) {
	out := core.Page[Failure]{Items: []Failure{}}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 {
		return out, core.Invalid("limit", "每次读取1至50条投递失败")
	}
	tx, err := s.readTx(ctx, a)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = authz.Require(ctx, tx, a, "notification.delivery.read", authz.Resource{Type: "notification_delivery", Classification: "internal"}); err != nil {
		return out, err
	}
	bound := core.CursorBinding(a, "notification-failures")
	var at *time.Time
	var id *string
	if cursor != "" {
		c, e := s.Pager.Decode(cursor, bound)
		if e != nil {
			return out, e
		}
		at = &c.UpdatedAt
		id = &c.ID
	}
	rows, err := tx.Query(ctx, `SELECT id,state,last_error,attempt,max_attempts,first_failed_at,updated_at FROM jobs
  WHERE org_id=$1 AND ((kind='notification.deliver.v1' AND lane='notification') OR (kind='event.rejected.v1' AND dedup_key LIKE 'notification-consumer.v1/%'))
  AND state IN ('dead','retry_wait') AND ($2::timestamptz IS NULL OR (updated_at,id)<($2,$3::uuid)) ORDER BY updated_at DESC,id DESC LIMIT $4`, a.OrgID, at, id, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v Failure
		if err = rows.Scan(&v.JobID, &v.State, &v.SafeCode, &v.Attempt, &v.MaxAttempts, &v.FirstFailedAt, &v.UpdatedAt); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		n := out.Items[limit-1]
		c := s.Pager.Encode(platform.Cursor{ID: n.JobID, UpdatedAt: n.UpdatedAt, Binding: bound})
		out.NextCursor = &c
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}
