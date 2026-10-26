package main

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/telegram"
	"copypaste.hk/platform/migrations"
	"log/slog"
	"time"
)

func runTelegramWorker(ctx context.Context, logger *slog.Logger) error {
	config, err := platform.LoadWorkerConfig()
	if err != nil {
		return &startupError{err.Error()}
	}
	bots, err := telegram.LoadDispatchBots()
	if err != nil {
		return &startupError{err.Error()}
	}
	if len(bots) == 0 {
		return &startupError{"worker-telegram requires explicit CP_TELEGRAM_DISPATCH_BOTS"}
	}
	db, err := platform.OpenDatabase(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = migrations.CheckCompiled(ctx, db); err != nil {
		return err
	}
	service := agent.Service{DB: db}
	logger.Info("Telegram dispatch worker started", "configured_bots", len(bots))
	for ctx.Err() == nil {
		worked := false
		for _, bot := range bots {
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			handled, failure := service.DispatchTelegramOnce(bounded, bot.OrgID, bot.BotID)
			cancel()
			if failure != nil {
				logger.Error("Telegram dispatch transaction failed", "code", core.PublicError(failure).Code)
			} else {
				worked = worked || handled
			}
		}
		if worked {
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
