package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/workbench"
)

func (s *Server) workbenchRoutes(mux *http.ServeMux) {
	service := &workbench.Service{DB: s.DB, Pager: s.Projects.Pager}
	mux.HandleFunc("GET /v1/dashboard/work-items/{kind}", read(s, "WorkInventoryResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		for k, values := range r.URL.Query() {
			if (k != "limit" && k != "cursor") || len(values) != 1 {
				return nil, core.Invalid("query", "待办列表仅接受单个分页参数")
			}
		}
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Inventory(ctx, a, r.PathValue("kind"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/dashboard/followups", read(s, "FollowupSummaryResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "业务跟进不接受筛选参数，请从对应业务页面查询")
		}
		return service.Followups(ctx, a)
	}))
	mux.HandleFunc("GET /v1/dashboard/business", read(s, "WorkbenchSummaryResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "工作台业务汇总不接受筛选参数，请从对应业务页面查询")
		}
		return service.Read(ctx, a)
	}))
}
