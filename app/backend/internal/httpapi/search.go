package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/search"
	"net/http"
	"strconv"
)

func (s *Server) searchRoutes(mux *http.ServeMux) {
	service := &search.Service{DB: s.DB, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/search", read(s, "SearchHitListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		company, err := queryBoolean(r, "company_only")
		if err != nil {
			return nil, err
		}
		return service.Search(ctx, a, search.Filter{Query: f.Query, Type: r.URL.Query().Get("type"), ProjectID: r.URL.Query().Get("project_id"), CompanyOnly: company, Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/files/{id}/versions/{version}/text", read(s, "FileTextResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		offset, limit := 0, 4000
		for key, target := range map[string]*int{"offset": &offset, "limit": &limit} {
			if value := r.URL.Query().Get(key); value != "" {
				n, err := strconv.Atoi(value)
				if err != nil || strconv.Itoa(n) != value {
					return nil, core.Invalid(key, "正文分页数值无效")
				}
				*target = n
			}
		}
		return (&files.Service{DB: s.DB}).Text(ctx, a, r.PathValue("id"), r.PathValue("version"), offset, limit)
	}))
}
