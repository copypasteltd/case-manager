package httpapi

import (
	"context"
	"net/http"
	"net/url"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
)

func amortizationQuery(r *http.Request, paged bool) (string, int, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return "", 0, core.Invalid("query", "摊销查询编码无效")
	}
	for key, items := range values {
		if !paged || len(items) != 1 || key != "cursor" && key != "limit" {
			return "", 0, core.Invalid("query", "摊销查询参数未知或重复")
		}
	}
	f, err := filter(r)
	return f.Cursor, f.Limit, err
}
func (s *Server) amortizationRoutes(mux *http.ServeMux, v *finance.Service) {
	mux.HandleFunc("GET /v1/costs/{id}/amortization", read(s, "AmortizationOverviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if _, _, err := amortizationQuery(r, false); err != nil {
			return nil, err
		}
		return v.Amortization(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/costs/{id}/amortization/versions", read(s, "AmortizationVersionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		cursor, limit, err := amortizationQuery(r, true)
		if err != nil {
			return nil, err
		}
		return v.AmortizationVersions(ctx, a, r.PathValue("id"), cursor, limit)
	}))
	mux.HandleFunc("GET /v1/costs/{id}/amortization/versions/{version}", read(s, "AmortizationVersionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if _, _, err := amortizationQuery(r, false); err != nil {
			return nil, err
		}
		return v.AmortizationVersion(ctx, a, r.PathValue("id"), r.PathValue("version"))
	}))
	mux.HandleFunc("POST /v1/costs/{id}/amortization/preview", command(s, "AmortizationInput", "AmortizationPreviewResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.AmortizationInput) (core.Result, error) {
		if _, _, err := amortizationQuery(r, false); err != nil {
			return core.Result{}, err
		}
		return v.PreviewAmortization(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/costs/{id}/amortization", command(s, "AmortizationPublish", "AmortizationVersionResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.AmortizationPublish) (core.Result, error) {
		if _, _, err := amortizationQuery(r, false); err != nil {
			return core.Result{}, err
		}
		return v.PublishAmortization(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/costs/{id}/amortization/stop", command(s, "AmortizationStop", "AmortizationVersionResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.AmortizationStop) (core.Result, error) {
		if _, _, err := amortizationQuery(r, false); err != nil {
			return core.Result{}, err
		}
		return v.StopAmortization(ctx, c, r.PathValue("id"), in)
	}))
}
