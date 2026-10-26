package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"net/http"
)

func (s *Server) deliverableRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/projects/{id}/deliverables", read(s, "DeliverableListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return s.Projects.ListDeliverables(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("POST /v1/projects/{id}/deliverables", command(s, "DeliverableCreate", "DeliverableReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.DeliverableCreate) (core.Result, error) {
		return s.Projects.CreateDeliverable(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/deliverables/{id}", read(s, "DeliverableResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.GetDeliverable(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("PATCH /v1/deliverables/{id}", command(s, "DeliverablePatch", "DeliverableReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.DeliverablePatch) (core.Result, error) {
		return s.Projects.UpdateDeliverable(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/deliverables/{id}/submit", command(s, "DeliverableSubmit", "DeliverableReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.DeliverableSubmit) (core.Result, error) {
		return s.Projects.SubmitDeliverable(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/deliverables/{id}/decisions", command(s, "DeliverableDecisionInput", "DeliverableReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.DeliverableDecisionInput) (core.Result, error) {
		return s.Projects.DecideDeliverable(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/deliverables/{id}/revisions", read(s, "DeliverableRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return s.Projects.ListDeliverableRevisions(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("GET /v1/deliverables/{id}/revisions/{revision}", read(s, "DeliverableRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.GetDeliverableRevision(ctx, a, r.PathValue("id"), r.PathValue("revision"))
	}))
}
