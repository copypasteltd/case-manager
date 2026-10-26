package uploadworker

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/filescan"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
)

const lease = 2 * time.Minute

type Runner struct {
	Service     *files.UploadService
	Store       objectstore.Store
	TargetStore objectstore.Store
	Owner       string
	Logger      *slog.Logger
	Mode        string
	Scanner     filescan.Scanner
}
type profile struct {
	lane     string
	kinds    []string
	consumer jobqueue.Consumer
	timeout  time.Duration
	verify   bool
	promote  bool
	replica  bool
	restore  bool
}

func (r *Runner) profile() (profile, error) {
	switch r.Mode {
	case "", "control":
		return profile{lane: files.UploadLane, kinds: files.UploadJobKinds, consumer: files.UploadConsumer(), timeout: 10 * time.Minute}, nil
	case "verify-small":
		return profile{lane: files.VerifySmallLane, kinds: files.VerifyJobKinds, consumer: files.UploadIntegrityConsumer(), timeout: 30 * time.Minute, verify: true}, nil
	case "verify-large":
		return profile{lane: files.VerifyLargeLane, kinds: files.VerifyJobKinds, consumer: files.UploadIntegrityConsumer(), timeout: 12 * time.Hour, verify: true}, nil
	case "promote-small":
		return profile{lane: files.PromoteSmallLane, kinds: files.PromotionJobKinds, consumer: files.UploadPromotionConsumer(), timeout: 30 * time.Minute, promote: true}, nil
	case "promote-large":
		return profile{lane: files.PromoteLargeLane, kinds: files.PromotionJobKinds, consumer: files.UploadPromotionConsumer(), timeout: 12 * time.Hour, promote: true}, nil
	case "replica-small":
		return profile{lane: files.ReplicaSmallLane, kinds: files.ReplicaJobKinds, consumer: files.ReplicaConsumer(), timeout: 30 * time.Minute, replica: true}, nil
	case "replica-large":
		return profile{lane: files.ReplicaLargeLane, kinds: files.ReplicaJobKinds, consumer: files.ReplicaConsumer(), timeout: 12 * time.Hour, replica: true}, nil
	case "restore-small":
		return profile{lane: files.RestoreSmallLane, kinds: files.RestoreJobKinds, consumer: files.RestoreConsumer(), timeout: 30 * time.Minute, restore: true}, nil
	case "restore-large":
		return profile{lane: files.RestoreLargeLane, kinds: files.RestoreJobKinds, consumer: files.RestoreConsumer(), timeout: 12 * time.Hour, restore: true}, nil
	default:
		return profile{}, errors.New("unknown upload worker mode")
	}
}

func (r *Runner) Run(ctx context.Context) error {
	if r.Service == nil || r.Store == nil || r.Logger == nil || r.Owner == "" {
		return errors.New("incomplete upload worker configuration")
	}
	if _, err := r.profile(); err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		worked, err := r.RunOne(ctx)
		if err != nil && ctx.Err() == nil {
			r.Logger.Error("upload worker iteration failed", "code", "WORKER_ITERATION_FAILED")
		}
		if err == nil && worked {
			continue
		}
		delay := 500 * time.Millisecond
		if err != nil {
			delay = 5 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
func (r *Runner) RunOne(ctx context.Context) (bool, error) {
	if r.Service == nil || r.Store == nil || r.Logger == nil || r.Owner == "" {
		return false, errors.New("incomplete upload worker configuration")
	}
	p, err := r.profile()
	if err != nil {
		return false, err
	}
	if (p.promote || p.replica || p.restore) && r.TargetStore == nil {
		return false, errors.New("transfer target identity is required")
	}
	// Bounded relay batches preserve task execution under a busy event stream.
	for n := 0; n < 20; n++ {
		delivery, err := r.Service.Queue.DispatchOne(ctx, p.consumer)
		if err != nil {
			return false, err
		}
		if delivery == nil {
			break
		}
		if delivery.Rejected {
			r.Logger.Warn("upload event rejected", "event_id", delivery.EventID, "job_id", delivery.JobID)
		}
	}
	if _, err := r.Service.Queue.SweepExhausted(ctx, p.kinds, p.lane); err != nil {
		return false, err
	}
	if p.replica {
		if err := r.Service.ReconcileReplicaFailures(ctx, p.lane); err != nil {
			return false, err
		}
	}
	if p.restore {
		if err := r.Service.ReconcileRestoreFailures(ctx, p.lane); err != nil {
			return false, err
		}
	}
	job, err := r.Service.Queue.Claim(ctx, p.kinds, p.lane, r.Owner, lease)
	if err != nil || job == nil {
		return false, err
	}
	r.execute(ctx, *job, p)
	return true, nil
}
func (r *Runner) execute(parent context.Context, j jobqueue.Job, p profile) {
	jobCtx, cancel := context.WithTimeout(parent, p.timeout)
	defer cancel()
	stop := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		timer := time.NewTicker(30 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-stop:
				return
			case <-jobCtx.Done():
				return
			case <-timer.C:
				beatCtx, beatCancel := context.WithTimeout(jobCtx, 10*time.Second)
				_, err := r.Service.Queue.Renew(beatCtx, j, lease)
				beatCancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	err := safeRun(jobCtx, r.Service, j, r.Store, r.TargetStore, r.Scanner, p)
	close(stop)
	<-heartbeatDone
	if err == nil {
		r.Logger.Info("upload task completed", "job_id", j.ID, "kind", j.Kind)
		return
	}
	if parent.Err() != nil {
		return
	} // Restart recovery owns the unacknowledged lease.
	code, permanent := classify(err)
	recoveryCtx, recoveryCancel := context.WithTimeout(context.WithoutCancel(parent), 15*time.Second)
	defer recoveryCancel()
	var failureErr error
	if p.restore {
		failureErr = r.Service.RecordRestoreFailure(recoveryCtx, j, code, permanent, backoff(j.Attempt))
	} else if p.replica {
		failureErr = r.Service.RecordReplicaFailure(recoveryCtx, j, code, permanent, backoff(j.Attempt))
	} else {
		failureErr = r.Service.RecordUploadFailure(recoveryCtx, j, code, permanent, backoff(j.Attempt))
	}
	if failureErr != nil {
		if core.PublicError(failureErr).Code == "JOB_LEASE_LOST" {
			r.Logger.Warn("upload task lease lost", "job_id", j.ID)
			return
		}
		// Malformed references cannot mutate a domain row. Preserve the
		// controlled diagnostic on the still-current job when possible.
		failureCode := core.PublicError(failureErr).Code
		if failureCode == "RESOURCE_NOT_FOUND" || failureCode == "INVALID_JOB_INPUT" {
			_ = r.Service.Queue.Fail(recoveryCtx, j, code, permanent, backoff(j.Attempt))
		}
		r.Logger.Error("upload task failure checkpoint unavailable", "job_id", j.ID, "code", "FAILURE_CHECKPOINT_FAILED")
		return
	}
	r.Logger.Warn("upload task failed", "job_id", j.ID, "code", code, "terminal", permanent || j.Attempt >= j.MaxAttempts)
}
func safeRun(ctx context.Context, service *files.UploadService, j jobqueue.Job, store, target objectstore.Store, scanner filescan.Scanner, p profile) (err error) {
	defer func() {
		if recover() != nil {
			err = core.Fail(500, "JOB_HANDLER_PANIC", "任务执行中断")
		}
	}()
	if p.restore {
		return service.RunRestoreJob(ctx, j, store, target)
	}
	if p.promote {
		return service.RunPromotionJob(ctx, j, store, target)
	}
	if p.replica {
		return service.RunReplicaJob(ctx, j, store, target)
	}
	if p.verify {
		return service.RunIntegrityJob(ctx, j, store, scanner)
	}
	return service.RunUploadJob(ctx, j, store)
}
func classify(err error) (string, bool) {
	var scan *filescan.Error
	if errors.As(err, &scan) {
		return scan.Code, !scan.Retryable
	}
	var provider *objectstore.Error
	if errors.As(err, &provider) {
		return provider.Code, !provider.Retryable && !provider.Uncertain
	}
	var public *core.Error
	if errors.As(err, &public) {
		return public.Code, public.Status >= 400 && public.Status < 500 && public.Status != 429
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "WORKER_TIMEOUT", false
	}
	return "DATABASE_OPERATION_FAILED", false
}
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 7 {
		attempt = 7
	}
	delay := 5 * time.Second * time.Duration(1<<uint(attempt-1))
	if delay > 5*time.Minute {
		delay = 5 * time.Minute
	}
	return delay - time.Duration(rand.Int64N(int64(delay/5)+1))
}
