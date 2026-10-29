package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/workbench"
)

func (s *Server) workbenchRoutes(mux *http.ServeMux) {
	service := &workbench.Service{DB: s.DB}
	mux.HandleFunc("GET /v1/dashboard/business", read(s, "WorkbenchSummaryResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "工作台业务汇总不接受筛选参数，请从对应业务页面查询")
		}
		return service.Read(ctx, a)
	}))
}
