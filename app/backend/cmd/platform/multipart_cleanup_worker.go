package main

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/storageconfig"
	"copypaste.hk/platform/migrations"
	"log/slog"
	"time"
)

func runMultipartCleanupWorker(ctx context.Context, logger *slog.Logger) error {
	config, e := platform.LoadWorkerConfig()
	if e != nil {
		return &startupError{e.Error()}
	}
	backends, e := storageconfig.LoadMultipartCleaners()
	if e != nil {
		return &startupError{e.Error()}
	}
	if len(backends) == 0 {
		return &startupError{"multipart cleanup requires independently configured cleaner identities"}
	}
	db, e := platform.OpenDatabase(ctx, config.DatabaseURL)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = migrations.CheckCompiled(ctx, db); e != nil {
		return &startupError{"multipart cleanup requires matching database migrations"}
	}
	service := &files.MultipartCleanupService{Files: &files.Service{DB: db, Commands: platform.Commands{DB: db}}, Backends: backends}
	owner := "multipart-cleanup." + core.NewID()
	logger.Info("multipart cleanup worker started", "owner", owner)
	for ctx.Err() == nil {
		worked, e := service.RunOne(ctx, owner)
		if e != nil && ctx.Err() == nil {
			logger.Warn("multipart cleanup iteration failed", "code", core.PublicError(e).Code)
		}
		if worked && e == nil {
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
