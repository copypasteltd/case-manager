package httpapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/telegram"
)

func (s *Server) telegramService() *telegram.Service {
	if s.Telegram != nil {
		return s.Telegram
	}
	return &telegram.Service{DB: s.DB, Config: s.Config}
}
func (s *Server) telegramRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/me/telegram", read(s, "TelegramStatusResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.telegramService().Status(ctx, a)
	}))
	mux.HandleFunc("POST /v1/me/telegram/challenges", command(s, "TelegramChallengeCreate", "TelegramChallengeResponse", func(ctx context.Context, c core.Command, r *http.Request, in telegram.ChallengeCreate) (core.Result, error) {
		result, err := s.telegramService().CreateChallenge(ctx, c, in)
		if err != nil {
			return result, err
		}
		return s.telegramService().DecorateChallenge(ctx, c.Actor, result)
	}))
	mux.HandleFunc("POST /v1/me/telegram/challenges/{id}/confirm", command(s, "TelegramChallengeConfirm", "TelegramBindingResponse", func(ctx context.Context, c core.Command, r *http.Request, in telegram.ChallengeConfirm) (core.Result, error) {
		return s.telegramService().ConfirmChallenge(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/me/telegram/challenges/{id}/cancel", command(s, "TelegramReasonInput", "TelegramChallengeResponse", func(ctx context.Context, c core.Command, r *http.Request, in telegram.ReasonInput) (core.Result, error) {
		return s.telegramService().CancelChallenge(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/me/telegram/bindings/{id}/revoke", command(s, "TelegramReasonInput", "TelegramBindingResponse", func(ctx context.Context, c core.Command, r *http.Request, in telegram.ReasonInput) (core.Result, error) {
		return s.telegramService().RevokeBinding(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/me/telegram/requests", read(s, "TelegramRequestListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.telegramService().Requests(ctx, a, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/me/telegram/requests/{id}", read(s, "TelegramRequestResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.telegramService().Request(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/me/telegram/requests/{id}/deliveries", read(s, "TelegramDeliveryDetailsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.telegramService().DeliveryDetails(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/me/telegram/requests/{id}/deliveries/{delivery_id}/reconcile", command(s, "TelegramDeliveryReconcile", "TelegramDeliveryResponse", func(ctx context.Context, c core.Command, r *http.Request, in telegram.DeliveryReconcile) (core.Result, error) {
		return s.telegramService().ReconcileDelivery(ctx, c, r.PathValue("id"), r.PathValue("delivery_id"), in)
	}))
	mux.HandleFunc("GET /v1/telegram/operations", read(s, "TelegramOperationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		for key, values := range r.URL.Query() {
			if (key != "state" && key != "channel_id" && key != "cursor" && key != "limit") || len(values) != 1 {
				return nil, core.Invalid("filters", "查询条件无效或重复")
			}
		}
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.telegramService().Operations(ctx, a, f.State, r.URL.Query().Get("channel_id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/telegram/operations/{id}", read(s, "TelegramOperationDetailsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.telegramService().Operation(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/telegram/operations/{id}/reconcile", command(s, "TelegramOperationReconcile", "TelegramDeliveryResponse", func(ctx context.Context, c core.Command, r *http.Request, in telegram.OperationReconcile) (core.Result, error) {
		return s.telegramService().ReconcileOperation(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/telegram/inbox-operations", read(s, "TelegramInboxOperationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		for key, values := range r.URL.Query() {
			if (key != "state" && key != "channel_id" && key != "skill_id" && key != "cursor" && key != "limit") || len(values) != 1 {
				return nil, core.Invalid("filters", "查询条件无效或重复")
			}
		}
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.telegramService().InboxOperations(ctx, a, f.State, r.URL.Query().Get("channel_id"), r.URL.Query().Get("skill_id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/telegram/inbox-operations/{id}", read(s, "TelegramInboxOperationDetailsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if len(r.URL.Query()) != 0 {
			return nil, core.Invalid("filters", "详情不接受查询条件")
		}
		return s.telegramService().InboxOperation(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/telegram/inbox-operations/{id}/history", read(s, "TelegramInboxOperationEventListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		for key, values := range r.URL.Query() {
			if (key != "cursor" && key != "limit") || len(values) != 1 {
				return nil, core.Invalid("filters", "历史分页条件无效或重复")
			}
		}
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.telegramService().InboxOperationHistory(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/telegram/inbox-operations/{id}/withdraw", command(s, "TelegramInboxWithdraw", "TelegramInboxOperationResponse", func(ctx context.Context, c core.Command, r *http.Request, in telegram.InboxWithdraw) (core.Result, error) {
		return s.telegramService().WithdrawInboxOperation(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/telegram/channels", read(s, "TelegramChannelListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.telegramService().Channels(ctx, a, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/telegram/channels", command(s, "TelegramChannelCreate", "TelegramChannelResponse", func(ctx context.Context, c core.Command, r *http.Request, in telegram.ChannelCreate) (core.Result, error) {
		return s.telegramService().CreateChannel(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/telegram/channels/{id}", command(s, "TelegramChannelPatch", "TelegramChannelResponse", func(ctx context.Context, c core.Command, r *http.Request, in telegram.ChannelPatch) (core.Result, error) {
		return s.telegramService().UpdateChannel(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/telegram/channels/{id}/history", read(s, "TelegramChannelRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.telegramService().History(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/integrations/telegram/{bot_id}/webhook", func(w http.ResponseWriter, r *http.Request) {
		service := s.telegramService()
		headers := r.Header.Values("X-Telegram-Bot-Api-Secret-Token")
		if len(headers) != 1 {
			s.fail(w, r, core.Missing())
			return
		}
		if _, ok := service.WebhookBot(r.PathValue("bot_id"), headers[0]); !ok {
			s.fail(w, r, core.Missing())
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" || r.Header.Get("Content-Encoding") != "" {
			s.fail(w, r, core.Fail(415, "UNSUPPORTED_MEDIA_TYPE", "Webhook仅接受JSON消息"))
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
		if err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				s.fail(w, r, core.Fail(413, "BODY_TOO_LARGE", "Telegram消息超过接收限制"))
			} else {
				s.fail(w, r, core.Invalid("update", "无法读取Telegram消息"))
			}
			return
		}
		if err = service.Accept(r.Context(), r.PathValue("bot_id"), headers[0], raw); err != nil {
			s.fail(w, r, err)
			return
		}
		s.write(w, 200, []byte(`{"ok":true}`))
	})
}
