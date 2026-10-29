package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/notification"
	"copypaste.hk/platform/internal/platform"
	"net/http"
	"net/url"
	"strconv"
)

func notificationLimit(r *http.Request, allowed ...string) (int, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return 0, core.Invalid("query", "通知查询参数编码无效")
	}
	known := map[string]bool{"limit": true, "cursor": true}
	for _, key := range allowed {
		known[key] = true
	}
	for key, values := range query {
		if !known[key] || len(values) != 1 {
			return 0, core.Invalid("query", "通知查询字段无效或重复")
		}
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 50 {
			return 0, core.Invalid("limit", "每次读取1至50条通知")
		}
		return limit, nil
	}
	return 20, nil
}
func (s *Server) notificationRoutes(mux *http.ServeMux) {
	service := &notification.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/notification-contract-policy", read(s, "NotificationContractPolicyStateResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "当前策略不接受查询字段")
		}
		return service.ContractPolicy(ctx, a)
	}))
	mux.HandleFunc("POST /v1/notification-contract-policy", command(s, "NotificationContractPolicyInput", "NotificationContractPolicyResponse", func(ctx context.Context, c core.Command, r *http.Request, in notification.ContractPolicyInput) (core.Result, error) {
		if r.URL.RawQuery != "" {
			return core.Result{}, core.Invalid("query", "发布策略不接受查询字段")
		}
		return service.PublishContractPolicy(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/notification-contract-policy-history", read(s, "NotificationContractPolicyListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		limit, err := notificationLimit(r)
		if err != nil {
			return nil, err
		}
		return service.ContractPolicyHistory(ctx, a, r.URL.Query().Get("cursor"), limit)
	}))
	mux.HandleFunc("GET /v1/notification-delivery-jobs/{id}", read(s, "NotificationDeliveryJobResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "任务详情不接受查询参数")
		}
		return service.DeliveryJob(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/notification-delivery-jobs/{id}/retry", command(s, "NotificationRetryInput", "NotificationRetryReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in notification.RetryInput) (core.Result, error) {
		if r.URL.RawQuery != "" {
			return core.Result{}, core.Invalid("query", "重试命令不接受查询参数")
		}
		return service.RetryDelivery(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/notification-delivery-retries", read(s, "NotificationRetryReceiptListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		limit, err := notificationLimit(r)
		if err != nil {
			return nil, err
		}
		return service.RetryHistory(ctx, a, r.URL.Query().Get("cursor"), limit)
	}))
	mux.HandleFunc("GET /v1/notifications", read(s, "NotificationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		limit, err := notificationLimit(r, "state", "category")
		if err != nil {
			return nil, err
		}
		return service.List(ctx, a, notification.Filter{State: r.URL.Query().Get("state"), Category: r.URL.Query().Get("category"), Cursor: r.URL.Query().Get("cursor"), Limit: limit})
	}))
	mux.HandleFunc("GET /v1/notifications/summary", read(s, "NotificationSummaryResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "通知汇总不接受查询字段")
		}
		return service.Summary(ctx, a)
	}))
	mux.HandleFunc("GET /v1/notifications/{id}", read(s, "NotificationResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Get(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/notifications/{id}/read", command(s, "NotificationRead", "NotificationResponse", func(ctx context.Context, c core.Command, r *http.Request, in notification.ReadInput) (core.Result, error) {
		return service.MarkRead(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/notification-delivery-failures", read(s, "NotificationFailureListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		limit, err := notificationLimit(r)
		if err != nil {
			return nil, err
		}
		return service.Failures(ctx, a, r.URL.Query().Get("cursor"), limit)
	}))
}
