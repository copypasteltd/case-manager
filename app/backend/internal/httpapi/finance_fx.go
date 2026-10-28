package httpapi

import (
	"context"
	"net/http"
	"net/url"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
)

func financeFXQuery(r *http.Request, list bool) (*string, string, int, error) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, "", 0, core.Invalid("query", "汇率查询编码无效")
	}
	for key, items := range values {
		if len(items) != 1 || key != "project_id" && (!list || key != "cursor" && key != "limit") {
			return nil, "", 0, core.Invalid("query", "汇率查询参数未知或重复")
		}
	}
	var project *string
	if v := values.Get("project_id"); v != "" {
		if !core.ValidID(v) {
			return nil, "", 0, core.Invalid("project_id", "项目标识无效")
		}
		project = &v
	}
	page, err := filter(r)
	return project, page.Cursor, page.Limit, err
}
func (s *Server) financeFXRoutes(mux *http.ServeMux, service *finance.Service) {
	mux.HandleFunc("GET /v1/finance/fx-policy", read(s, "FinanceFXPolicyResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		p, _, _, err := financeFXQuery(r, false)
		if err != nil {
			return nil, err
		}
		return service.FXPolicy(ctx, a, p)
	}))
	mux.HandleFunc("GET /v1/finance/fx-books", read(s, "FinanceFXBookListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		p, c, l, err := financeFXQuery(r, true)
		if err != nil {
			return nil, err
		}
		return service.FXBooks(ctx, a, p, c, l)
	}))
	mux.HandleFunc("GET /v1/finance/fx-books/{id}", read(s, "FinanceFXBookResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		p, _, _, err := financeFXQuery(r, false)
		if err != nil {
			return nil, err
		}
		return service.FXBook(ctx, a, r.PathValue("id"), p)
	}))
	mux.HandleFunc("POST /v1/finance/fx-books", command(s, "FinanceFXBookCreate", "FinanceFXBookResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.FXBookCreate) (core.Result, error) {
		return service.PublishFXBook(ctx, c, in)
	}))
}
