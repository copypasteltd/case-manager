package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/workflow"
	"net/http"
)

func (s *Server) projectChangeRoutes(mux *http.ServeMux) {
	service := &workflow.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/projects/{id}/changes", read(s, "ChangeListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return s.Projects.ListChanges(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("POST /v1/projects/{id}/changes", command(s, "ChangeCreate", "ChangeReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.ChangeCreate) (core.Result, error) {
		return s.Projects.CreateChange(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/projects/{id}/change-basis", read(s, "ChangeBasisResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.GetChangeBasis(ctx, a, r.PathValue("id"), r.URL.Query().Get("currency"))
	}))
	mux.HandleFunc("GET /v1/projects/{id}/budgets", read(s, "ProjectBudgetListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		f.Query = r.URL.Query().Get("currency")
		return s.Projects.ListBudgets(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("GET /v1/changes/{id}", read(s, "ChangeResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.GetChange(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("PATCH /v1/changes/{id}", command(s, "ChangePatch", "ChangeReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.ChangePatch) (core.Result, error) {
		return s.Projects.UpdateChange(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/changes/{id}/revisions", read(s, "ChangeRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return s.Projects.ListChangeRevisions(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("GET /v1/changes/{id}/revisions/{revision}", read(s, "ChangeRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.GetChangeRevision(ctx, a, r.PathValue("id"), r.PathValue("revision"))
	}))
	mux.HandleFunc("POST /v1/changes/{id}/submit", command(s, "ChangeSubmit", "RequestResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.ChangeSubmit) (core.Result, error) {
		return service.CreateRequest(ctx, c, workflow.RequestCreate{Kind: "change", BusinessID: r.PathValue("id"), ExpectedBusinessVersion: in.ExpectedVersion, BusinessRevisionID: in.RevisionID, BusinessContentHash: in.ContentHash, Reason: in.Reason})
	}))
	mux.HandleFunc("POST /v1/changes/{id}/implement", command(s, "ChangeImplement", "ChangeReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.ChangeImplement) (core.Result, error) {
		return s.Projects.ImplementChange(ctx, c, r.PathValue("id"), in)
	}))
}
