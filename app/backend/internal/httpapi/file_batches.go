package httpapi

import (
	"context"
	"net/http"
	"net/url"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
)

func (s *Server) fileBatchRoutes(mux *http.ServeMux, service *files.Service) {
	mux.HandleFunc("POST /v1/file-batch-plans", command(s, "FileBatchCreate", "FileBatchPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.FileBatchCreate) (core.Result, error) {
		if r.URL.RawQuery != "" {
			return core.Result{}, core.Invalid("query", "批量清单不接受额外参数")
		}
		return service.CreateFileBatch(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/file-batch-plans/{id}", read(s, "FileBatchPlanResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "准确清单读取不接受额外筛选")
		}
		return service.FileBatchPlan(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/files/{id}/batch-plans", read(s, "FileBatchPlanListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil {
			return nil, core.Invalid("query", "清单分页参数无效")
		}
		for key, values := range q {
			if (key != "limit" && key != "cursor") || len(values) != 1 {
				return nil, core.Invalid("query", "仅接受单个limit与cursor")
			}
		}
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.FileBatchPlans(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	for _, action := range []string{"execute", "cancel"} {
		mux.HandleFunc("POST /v1/file-batch-plans/{id}/"+action, command(s, "FileBatchDecision", "FileBatchPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.FileBatchDecision) (core.Result, error) {
			if r.URL.RawQuery != "" {
				return core.Result{}, core.Invalid("query", "清单决定不接受额外参数")
			}
			return service.DecideFileBatch(ctx, c, r.PathValue("id"), in, action == "execute")
		}))
	}
}
