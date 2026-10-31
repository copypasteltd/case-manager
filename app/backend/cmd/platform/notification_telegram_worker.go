package main

import (
	"context"
	"log/slog"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/notification"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/telegram"
	"copypaste.hk/platform/migrations"
)

func runNotificationTelegramWorker(ctx context.Context, logger *slog.Logger) error {
	config, err := platform.LoadWorkerConfig()
	if err != nil {
		return &startupError{err.Error()}
	}
	bots, err := telegram.LoadDeliveryBots()
	if err != nil {
		return &startupError{err.Error()}
	}
	if len(bots) == 0 {
		return &startupError{"worker-notification-telegram requires explicit CP_TELEGRAM_DELIVERY_BOTS"}
	}
	db, err := platform.OpenDatabase(ctx, config.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = migrations.CheckCompiled(ctx, db); err != nil {
		return err
	}
	service := notification.Service{DB: db}
	transport := telegram.BotAPITransport{}
	logger.Info("Personal Telegram business-notice worker started", "configured_bots", len(bots))
	for ctx.Err() == nil {
		for _, bot := range bots {
			bounded, cancel := context.WithTimeout(ctx, 12*time.Second)
			_, failure := service.DeliverTelegramOnce(bounded, bot, transport)
			cancel()
			if failure != nil {
				logger.Error("Business-notice delivery transaction failed", "code", core.PublicError(failure).Code)
			}
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
