package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"net/http"
)

func (s *Server) projectCloseoutRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/projects/{id}/closeout", read(s, "ProjectCloseoutResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.Closeout(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/projects/{id}/archives", read(s, "ProjectArchiveListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return s.Projects.Archives(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("POST /v1/projects/{id}/archive", command(s, "ProjectArchiveInput", "ProjectArchiveReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.ArchiveInput) (core.Result, error) {
		return s.Projects.ArchiveProject(ctx, c, r.PathValue("id"), in)
	}))
}
