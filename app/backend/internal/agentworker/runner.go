package agentworker

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
)

type Runner struct {
	Service   *agent.Service
	Executor  Executor
	Owner     string
	Heartbeat time.Duration
}

func (r *Runner) Once(ctx context.Context) (bool, error) {
	if r.Executor == nil {
		return false, core.Fail(409, "RUNTIME_DISABLED", "执行器未配置")
	}
	if _, err := r.Service.ReconcileExecutions(ctx); err != nil {
		return false, err
	}
	j, err := r.Service.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", r.Owner, agent.ExecutionLeaseDuration)
	if err != nil || j == nil {
		return false, err
	}
	run, err := r.Service.IssueExecution(ctx, *j)
	if err != nil {
		return true, r.finish(ctx, *j, core.PublicError(err).Code)
	}
	lease, err := r.Service.LeaseForCredential(ctx, run.Credential)
	if err != nil {
		return true, r.finish(ctx, *j, core.PublicError(err).Code)
	}
	executionCtx, cancel := context.WithDeadline(ctx, run.ExpiresAt)
	defer cancel()
	done := make(chan error, 1)
	go func() { var err error; defer func() { done <- err }(); err = r.Executor.Execute(executionCtx, run) }()
	interval := r.Heartbeat
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	code := ""
	for {
		select {
		case <-done:
			if code == "" {
				code = r.Service.ExecutionStopCode(ctx, *j)
			}
			return true, r.finish(ctx, *j, code)
		case <-executionCtx.Done():
			if code == "" {
				code = "RUNTIME_CANCEL_REQUESTED"
				if time.Now().After(run.ExpiresAt) {
					code = "RUNTIME_DEADLINE_EXCEEDED"
				}
			}
			cancel()
			<-done
			return true, r.finish(ctx, *j, code)
		case <-ticker.C:
			heartbeatCtx, stop := context.WithTimeout(ctx, 5*time.Second)
			err = r.Service.RenewExecution(heartbeatCtx, lease)
			stop()
			if err != nil {
				code = core.PublicError(err).Code
				cancel()
			}
		}
	}
}
func (r *Runner) finish(ctx context.Context, j jobqueue.Job, code string) error {
	// A bounded controller context outlives shutdown only long enough to settle a stopped
	// process. If the worker is killed, the expired-lease reconciler records interruption.
	settlement, stop := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer stop()
	settled, err := r.Service.ExecutionSettled(settlement, j)
	if err != nil {
		return err
	}
	if settled {
		return nil
	}
	return r.Service.FinishStoppedExecution(settlement, j, code)
}
