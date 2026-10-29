package httpapi

import (
	"context"
	"net/http"
	"net/url"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
)

func (s *Server) fileScopeTransferRoutes(mux *http.ServeMux, service *files.Service) {
	mux.HandleFunc("POST /v1/file-scope-transfers", command(s, "ScopeTransferCreate", "ScopeTransferResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.ScopeTransferCreate) (core.Result, error) {
		return service.CreateScopeTransfer(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/file-scope-transfers/{id}", read(s, "ScopeTransferResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "准确计划读取不接受额外筛选参数")
		}
		return service.ScopeTransfer(ctx, a, r.PathValue("id"))
	}))
	for _, kind := range []string{"file", "folder"} {
		mux.HandleFunc("GET /v1/"+kind+"s/{id}/scope-transfers", read(s, "ScopeTransferListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			q, e := url.ParseQuery(r.URL.RawQuery)
			if e != nil {
				return nil, core.Invalid("query", "计划分页参数无效")
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
			return service.ScopeTransfers(ctx, a, kind, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
		}))
	}
	for _, action := range []string{"execute", "cancel"} {
		mux.HandleFunc("POST /v1/file-scope-transfers/{id}/"+action, command(s, "ScopeTransferDecision", "ScopeTransferResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.ScopeTransferDecision) (core.Result, error) {
			return service.DecideScopeTransfer(ctx, c, r.PathValue("id"), in, action == "execute")
		}))
	}
}
