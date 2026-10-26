package main

import (
	"context"
	"log/slog"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/migrations"
)

func runSubscriptionWorker(ctx context.Context, logger *slog.Logger) error {
	config, err := platform.LoadWorkerConfig()
	if err != nil {
		return &startupError{err.Error()}
	}
	db, err := platform.OpenDatabase(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = migrations.CheckCompiled(ctx, db); err != nil {
		return &startupError{"subscription worker requires matching database migrations"}
	}
	service := &finance.Service{DB: db}
	queue := jobqueue.Queue{DB: db}
	owner := "subscription." + core.NewID()
	var nextSchedule time.Time
	logger.Info("subscription worker started", "owner", owner)
	for ctx.Err() == nil {
		if time.Now().After(nextSchedule) {
			result, e := service.ScheduleSubscriptions(ctx)
			if e != nil {
				logger.Warn("subscription scheduling failed", "code", core.PublicError(e).Code)
			} else if result.LimitReached {
				logger.Warn("subscription scheduling scope exceeds configured batch limit")
			}
			nextSchedule = time.Now().Add(time.Minute)
		}
		if _, err = queue.SweepExhausted(ctx, []string{"subscription.generate.v1"}, "finance"); err != nil && ctx.Err() == nil {
			logger.Warn("subscription job sweep failed", "code", core.PublicError(err).Code)
		}
		job, e := queue.Claim(ctx, []string{"subscription.generate.v1"}, "finance", owner, 2*time.Minute)
		if e != nil {
			if ctx.Err() != nil {
				return nil
			}
			logger.Warn("subscription claim failed", "code", core.PublicError(e).Code)
		}
		if job != nil {
			_, e = service.ProcessSubscriptionJob(ctx, *job)
			if e != nil && ctx.Err() == nil {
				code := core.PublicError(e).Code
				permanent := core.PublicError(e).Status == 422 || core.PublicError(e).Status == 404
				if failErr := queue.Fail(ctx, *job, code, permanent, time.Minute); failErr != nil {
					logger.Warn("subscription failure receipt rejected", "code", core.PublicError(failErr).Code)
				}
				logger.Warn("subscription job failed", "job_id", job.ID, "code", code)
			}
			continue
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}
