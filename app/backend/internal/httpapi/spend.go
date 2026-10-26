package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/spend"
	"net/http"
)

func (s *Server) spendRoutes(mux *http.ServeMux) {
	service := &spend.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	for _, route := range []struct{ path, kind string }{{"purchases", "purchase"}, {"claims", "claim"}} {
		kind, path := route.kind, "/v1/"+route.path
		mux.HandleFunc("GET "+path, read(s, "SpendListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			f, err := filter(r)
			if err != nil {
				return nil, err
			}
			var project *string
			if v := r.URL.Query().Get("project_id"); v != "" {
				project = &v
			}
			return service.List(ctx, a, kind, spend.Filter{Query: r.URL.Query().Get("q"), Lifecycle: r.URL.Query().Get("lifecycle"), ApprovalState: r.URL.Query().Get("approval_state"), ProjectID: project, Cursor: f.Cursor, Limit: f.Limit})
		}))
		mux.HandleFunc("GET "+path+"/{id}", read(s, "SpendResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			return service.Get(ctx, a, kind, r.PathValue("id"))
		}))
		mux.HandleFunc("POST "+path, command(s, "SpendCreate", "SpendResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.Create) (core.Result, error) {
			return service.Create(ctx, c, kind, in)
		}))
		mux.HandleFunc("PATCH "+path+"/{id}", command(s, "SpendPatch", "SpendResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.Patch) (core.Result, error) {
			return service.Update(ctx, c, kind, r.PathValue("id"), in)
		}))
		mux.HandleFunc("POST "+path+"/{id}/archive", command(s, "SpendArchiveInput", "SpendResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.ArchiveInput) (core.Result, error) {
			return service.Archive(ctx, c, kind, r.PathValue("id"), in)
		}))
		mux.HandleFunc("GET "+path+"/{id}/revisions", read(s, "SpendRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			f, err := filter(r)
			if err != nil {
				return nil, err
			}
			return service.History(ctx, a, kind, r.PathValue("id"), f.Cursor, f.Limit)
		}))
		mux.HandleFunc("GET "+path+"/{id}/revisions/{revision}", read(s, "SpendRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			return service.Revision(ctx, a, kind, r.PathValue("id"), r.PathValue("revision"))
		}))
	}
	s.purchaseRoutes(mux, service)
}
