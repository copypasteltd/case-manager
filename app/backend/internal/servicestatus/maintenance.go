package servicestatus

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Service) CreateMaintenance(ctx context.Context, c core.Command, service string, in MaintenanceInput) (core.Result, error) {
	if core.Name(in.Reason) != nil || in.Start.IsZero() || !in.End.After(in.Start) || in.End.Sub(in.Start) > 31*24*time.Hour {
		return core.Result{}, core.Invalid("maintenance", "请说明维护原因并提供不超过31天的有效窗口")
	}
	return s.mutate(ctx, c, service, in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		if d.Lifecycle != "active" {
			return core.Result{}, core.UnexpectedState(d.Lifecycle, "schedule_maintenance")
		}
		var now time.Time
		if e := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
			return core.Result{}, e
		}
		if in.Start.Before(now.Add(-5*time.Minute)) || !in.End.After(now) {
			return core.Result{}, core.Invalid("start_at", "不能补记已结束的维护；开始时间最多回溯5分钟")
		}
		var overlap bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM maintenance_windows w WHERE w.org_id=$1 AND w.service_id=$2 AND w.start_at<$4 AND w.end_at>$3
 AND NOT EXISTS(SELECT 1 FROM maintenance_cancellations c WHERE c.org_id=w.org_id AND c.window_id=w.id))`, c.Actor.OrgID, service, in.Start, in.End).Scan(&overlap); e != nil {
			return core.Result{}, e
		}
		if overlap {
			return core.Result{}, core.Fail(409, "MAINTENANCE_OVERLAP", "当前服务已有重叠的有效维护窗口")
		}
		id := core.NewID()
		if _, e := tx.Exec(ctx, "INSERT INTO maintenance_windows(id,org_id,service_id,start_at,end_at,reason,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", id, c.Actor.OrgID, service, in.Start, in.End, in.Reason, c.Actor.UserID, now); e != nil {
			return core.Result{}, e
		}
		if _, e := tx.Exec(ctx, "UPDATE services SET version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, service); e != nil {
			return core.Result{}, e
		}
		out, e := s.changed(ctx, tx, c.Actor, service, "service.maintenance_scheduled", in.Reason)
		out.Event.Payload["window_id"] = id
		return out, e
	})
}
func (s *Service) CancelMaintenance(ctx context.Context, c core.Command, service, id string, in MaintenanceCancel) (core.Result, error) {
	if core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请说明取消原因")
	}
	if !core.ValidID(id) {
		return core.Result{}, core.Missing()
	}
	return s.mutate(ctx, c, service, in.ExpectedVersion, func(tx pgx.Tx, d Record) (core.Result, error) {
		var ended bool
		if e := tx.QueryRow(ctx, "SELECT end_at<=clock_timestamp() FROM maintenance_windows WHERE org_id=$1 AND service_id=$2 AND id=$3", c.Actor.OrgID, service, id).Scan(&ended); e != nil {
			return core.Result{}, platform.DatabaseError(e)
		}
		if ended {
			return core.Result{}, core.Fail(409, "MAINTENANCE_ENDED", "已结束的维护只能查阅记录")
		}
		if _, e := tx.Exec(ctx, "INSERT INTO maintenance_cancellations(id,org_id,window_id,reason,cancelled_by) VALUES($1,$2,$3,$4,$5)", core.NewID(), c.Actor.OrgID, id, in.Reason, c.Actor.UserID); e != nil {
			return core.Result{}, e
		}
		if _, e := tx.Exec(ctx, "UPDATE services SET version=version+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", c.Actor.OrgID, service); e != nil {
			return core.Result{}, e
		}
		out, e := s.changed(ctx, tx, c.Actor, service, "service.maintenance_cancelled", in.Reason)
		out.Event.Payload["window_id"] = id
		return out, e
	})
}
