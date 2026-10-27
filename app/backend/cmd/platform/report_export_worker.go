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

func runReportExportWorker(ctx context.Context, logger *slog.Logger) error {
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
		return &startupError{"report export worker requires matching database migrations"}
	}
	service := &finance.Service{DB: db}
	queue := jobqueue.Queue{DB: db}
	owner := "report-export." + core.NewID()
	logger.Info("report export worker started", "owner", owner)
	var nextPurge time.Time
	for ctx.Err() == nil {
		if time.Now().After(nextPurge) {
			if _, err = service.PurgeReportSnapshots(ctx); err != nil && ctx.Err() == nil {
				logger.Warn("report expiry cleanup failed", "code", core.PublicError(err).Code)
			}
			nextPurge = time.Now().Add(time.Minute)
		}
		if _, err = queue.SweepExhausted(ctx, []string{finance.ReportExportKind}, "reports"); err != nil && ctx.Err() == nil {
			logger.Warn("report export sweep failed", "code", core.PublicError(err).Code)
		}
		job, e := queue.Claim(ctx, []string{finance.ReportExportKind}, "reports", owner, 2*time.Minute)
		if e != nil && ctx.Err() == nil {
			logger.Warn("report export claim failed", "code", core.PublicError(e).Code)
		}
		if job != nil {
			// This database-only task is bounded below its lease; the final fence
			// and clock check still reject a late worker. No external I/O occurs.
			work, cancel := context.WithTimeout(ctx, 90*time.Second)
			e = service.ProcessReportExport(work, *job)
			cancel()
			if e != nil && ctx.Err() == nil {
				public := core.PublicError(e)
				permanent := public.Status >= 400 && public.Status < 500
				delay := time.Duration(1<<min(job.Attempt, 6)) * time.Second
				if failure := queue.Fail(ctx, *job, public.Code, permanent, delay); failure != nil {
					logger.Warn("report export failure receipt rejected", "code", core.PublicError(failure).Code)
				}
				logger.Warn("report export failed", "job_id", job.ID, "code", public.Code)
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
