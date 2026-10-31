package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func (s *Server) agentProjectCatalogRoutes(mux *http.ServeMux) {
	service := &agent.Service{DB: s.DB, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/conversations/{id}/project-catalog", read(s, "AgentProjectCatalogPageResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		q := r.URL.Query()
		for k, v := range q {
			if (k != "kind" && k != "query" && k != "cursor" && k != "limit") || len(v) != 1 {
				return nil, core.Invalid("filters", "目录查询条件无效或重复")
			}
		}
		limit := 20
		if q.Has("limit") {
			n, e := strconv.Atoi(q.Get("limit"))
			if e != nil || n < 1 || n > 20 {
				return nil, core.Invalid("limit", "每页1至20项")
			}
			limit = n
		}
		return service.ProjectCatalog(ctx, a, r.PathValue("id"), agent.ProjectCatalogInput{Kind: q.Get("kind"), Query: q.Get("query"), Cursor: q.Get("cursor"), Limit: limit})
	}))
	mux.HandleFunc("GET /v1/conversations/{id}/project-catalog/{kind}/{sourceID}", read(s, "AgentProjectCatalogDocumentResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		q := r.URL.Query()
		for k, v := range q {
			if (k != "offset" && k != "max_chars") || len(v) != 1 {
				return nil, core.Invalid("filters", "目录正文查询条件无效或重复")
			}
		}
		offset, limit := 0, 4000
		if q.Has("offset") {
			n, e := strconv.Atoi(q.Get("offset"))
			if e != nil {
				return nil, core.Invalid("offset", "字符位置无效")
			}
			offset = n
		}
		if q.Has("max_chars") {
			n, e := strconv.Atoi(q.Get("max_chars"))
			if e != nil {
				return nil, core.Invalid("max_chars", "字符范围无效")
			}
			limit = n
		}
		return service.ProjectCatalogSource(ctx, a, r.PathValue("id"), r.PathValue("kind"), r.PathValue("sourceID"), offset, limit)
	}))
}
