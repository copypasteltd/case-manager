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
	mux.HandleFunc("GET /v1/files/{id}/text-comparison", read(s, "FileTextComparisonResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		query := r.URL.Query()
		allowed := map[string]bool{"base_version_id": true, "target_version_id": true, "view": true, "offset": true, "limit": true, "comparison_sha256": true}
		for key, values := range query {
			if !allowed[key] || len(values) != 1 {
				return nil, core.Invalid("query", "比较参数需唯一且属于明确的版本、范围与分页字段")
			}
		}
		in := files.TextComparisonInput{BaseVersionID: query.Get("base_version_id"), TargetVersionID: query.Get("target_version_id"), View: "changes", Limit: 60, ComparisonSHA256: query.Get("comparison_sha256")}
		if value := query.Get("view"); value != "" {
			in.View = value
		}
		for key, target := range map[string]*int{"offset": &in.Offset, "limit": &in.Limit} {
			if values, exists := query[key]; exists {
				n, err := strconv.Atoi(values[0])
				if err != nil || strconv.Itoa(n) != values[0] {
					return nil, core.Invalid(key, "比较分页须使用准确整数")
				}
				*target = n
			}
		}
		return (&files.Service{DB: s.DB}).CompareText(ctx, a, r.PathValue("id"), in)
	}))
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
