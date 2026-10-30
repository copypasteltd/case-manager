package main

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/documenttext"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/storageconfig"
	"copypaste.hk/platform/migrations"
	"log/slog"
	"os"
	"strings"
	"time"
)

func runTextWorker(ctx context.Context, logger *slog.Logger) error {
	config, err := platform.LoadWorkerConfig()
	if err != nil {
		return &startupError{err.Error()}
	}
	backends, err := storageconfig.LoadTextIndexReaders()
	if err != nil {
		return &startupError{err.Error()}
	}
	if len(backends) == 0 {
		return &startupError{"file text worker requires independent text-index reader credentials"}
	}
	db, err := platform.OpenDatabase(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = migrations.CheckCompiled(ctx, db); err != nil {
		return &startupError{"file text worker requires matching database migrations"}
	}
	var native *documenttext.Engine
	if name := strings.TrimSpace(os.Getenv("CP_DOCUMENT_TEXT_PROFILE")); name != "" {
		native, err = documenttext.Load(ctx, name)
		if err != nil {
			return &startupError{"native document text profile or pinned artifacts are invalid"}
		}
	}
	service := &files.TextIndexService{DB: db, Backends: backends, Native: native}
	queue := jobqueue.Queue{DB: db}
	owner := "file-text." + core.NewID()
	logger.Info("file text worker started", "owner", owner)
	var nextSchedule time.Time
	for ctx.Err() == nil {
		if time.Now().After(nextSchedule) {
			if _, err = service.Schedule(ctx); err != nil && ctx.Err() == nil {
				logger.Warn("file text schedule failed", "code", core.PublicError(err).Code)
			}
			if _, err = queue.SweepExhausted(ctx, []string{files.TextJobKind}, files.TextJobLane); err == nil {
				err = service.SealDead(ctx)
			}
			if err != nil && ctx.Err() == nil {
				logger.Warn("file text sweep failed", "code", core.PublicError(err).Code)
			}
			nextSchedule = time.Now().Add(10 * time.Second)
		}
		job, e := queue.Claim(ctx, []string{files.TextJobKind}, files.TextJobLane, owner, 2*time.Minute)
		if e != nil && ctx.Err() == nil {
			logger.Warn("file text claim failed", "code", core.PublicError(e).Code)
		}
		if job != nil {
			work, cancel := context.WithTimeout(ctx, 90*time.Second)
			e = service.Process(work, *job)
			cancel()
			if e != nil && ctx.Err() == nil {
				failure, cancel := context.WithTimeout(ctx, 5*time.Second)
				failed := service.Fail(failure, *job, e)
				cancel()
				if failed != nil {
					logger.Warn("file text failure receipt rejected", "code", core.PublicError(failed).Code)
				}
				logger.Warn("file text extraction failed", "job_id", job.ID, "code", core.PublicError(e).Code)
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
