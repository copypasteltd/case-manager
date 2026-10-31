package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/notification"
	"net/http"
)

func (s *Server) notificationTelegramRoutes(mux *http.ServeMux, service *notification.Service) {
	mux.HandleFunc("GET /v1/notification-telegram-settings", read(s, "NotificationTelegramSettingsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "当前设置不接受查询字段")
		}
		value, err := service.TelegramSettings(ctx, a)
		return struct {
			Items []notification.TelegramSetting `json:"items"`
		}{value}, err
	}))
	mux.HandleFunc("POST /v1/notification-telegram-policies/{bot}", command(s, "NotificationTelegramPolicyInput", "NotificationTelegramPolicyResponse", func(ctx context.Context, c core.Command, r *http.Request, in notification.TelegramPolicyInput) (core.Result, error) {
		if r.URL.RawQuery != "" {
			return core.Result{}, core.Invalid("query", "提醒设置不接受查询字段")
		}
		return service.PublishTelegramPolicy(ctx, c, r.PathValue("bot"), in)
	}))
	mux.HandleFunc("GET /v1/notification-telegram-policies/{bot}", read(s, "NotificationTelegramPolicyListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		limit, err := notificationLimit(r)
		if err != nil {
			return nil, err
		}
		return service.TelegramPolicyHistory(ctx, a, r.PathValue("bot"), r.URL.Query().Get("cursor"), limit)
	}))
	mux.HandleFunc("GET /v1/notification-telegram-deliveries", read(s, "NotificationTelegramReceiptListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		limit, err := notificationLimit(r)
		if err != nil {
			return nil, err
		}
		return service.TelegramReceipts(ctx, a, r.URL.Query().Get("cursor"), limit)
	}))
	mux.HandleFunc("GET /v1/notification-telegram-deliveries/{id}", read(s, "NotificationTelegramReceiptDetailsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "投递详情不接受查询字段")
		}
		return service.TelegramReceipt(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/notification-telegram-deliveries/{id}/reconcile", command(s, "NotificationTelegramReconcileInput", "NotificationTelegramReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in notification.TelegramReconcile) (core.Result, error) {
		if r.URL.RawQuery != "" {
			return core.Result{}, core.Invalid("query", "回执处置不接受查询字段")
		}
		return service.ReconcileTelegram(ctx, c, r.PathValue("id"), in)
	}))
}
