package agent

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"slices"
)

type RunCancel struct {
	ExpectedVersion int64  `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}

func (s *Service) CancelRun(ctx context.Context, command core.Command, id string, in RunCancel) (core.Result, error) {
	if core.RequireVersion(in.ExpectedVersion) != nil || in.Action != "cancel" || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("reason", "请明确取消、准确版本与操作依据")
	}
	authorize := func(tx pgx.Tx) error {
		r, c, err := s.readableRun(ctx, tx, command.Actor, id)
		if err != nil {
			return err
		}
		if r.ActorID != command.Actor.UserID || command.Actor.RunID != "" || command.Actor.Channel == "telegram" {
			return core.Forbidden()
		}
		return require(ctx, tx, command.Actor, c, "agent.run")
	}
	result, err := s.Commands.Do(ctx, command, authorize, func(tx pgx.Tx) (core.Result, error) {
		r, err := runRecord(ctx, tx, command.Actor.OrgID, id, true)
		if err != nil {
			return core.Result{}, err
		}
		if r.Version != in.ExpectedVersion {
			return core.Result{}, core.Conflict()
		}
		if r.State == "committing" {
			return core.Result{}, core.Fail(409, "COMMIT_RECONCILIATION_REQUIRED", "正式提交正在核对，不能将其标记为已撤销")
		}
		if !slices.Contains([]string{"queued", "running", "awaiting_approval"}, r.State) {
			return core.Result{}, core.Fail(409, "RUN_TERMINAL", "任务已结束，请读取实际结果")
		}
		if r.CancelRequestedAt != nil {
			return core.Result{}, core.Fail(409, "CANCEL_ALREADY_REQUESTED", "取消请求已经记录，请等待执行服务核对")
		}
		state, kind := r.State, "run.cancel_requested"
		if r.State != "running" {
			state = "cancelled"
			kind = "run.cancelled"
		}
		_, err = tx.Exec(ctx, "UPDATE agent_runs SET state=$3,cancel_requested_at=clock_timestamp(),version=version+1,last_event_seq=last_event_seq+1,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2", command.Actor.OrgID, id, state)
		if err != nil {
			return core.Result{}, err
		}
		if err = appendRunEvent(ctx, tx, command.Actor.OrgID, id, r.LastEventSeq+1, kind, state, nil); err != nil {
			return core.Result{}, err
		}
		if state == "cancelled" {
			_, err = tx.Exec(ctx, `UPDATE jobs SET state='cancelled',lease_owner=NULL,lease_until=NULL,updated_at=clock_timestamp()
 WHERE id=(SELECT job_id FROM agent_run_jobs WHERE org_id=$1 AND run_id=$2) AND state IN ('queued','running','retry_wait')`, command.Actor.OrgID, id)
			if err != nil {
				return core.Result{}, err
			}
		}
		r, err = runRecord(ctx, tx, command.Actor.OrgID, id, false)
		if err != nil {
			return core.Result{}, err
		}
		return core.Result{Data: r, Event: core.Event{Type: kind, ResourceType: "agent_run", ResourceID: id, Version: r.Version, Payload: map[string]any{"reason": in.Reason, "conversation_id": r.ConversationID, "state": state}}}, nil
	})
	if err == nil && result.Replayed {
		var r Run
		if e := json.Unmarshal(result.Data.(json.RawMessage), &r); e != nil {
			return core.Result{}, e
		}
		r, e := s.GetRun(ctx, command.Actor, r.ID)
		if e != nil {
			return core.Result{}, e
		}
		result.Data = r
	}
	return result, err
}
