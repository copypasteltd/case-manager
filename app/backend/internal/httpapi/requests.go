package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/workflow"
	"net/http"
)

func (s *Server) requestRoutes(mux *http.ServeMux) {
	service := &workflow.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/requests", read(s, "RequestListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Requests(ctx, a, workflow.RequestFilter{Kind: r.URL.Query().Get("kind"), State: r.URL.Query().Get("state"), Query: f.Query, Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("POST /v1/requests", command(s, "RequestCreate", "RequestResponse", func(ctx context.Context, c core.Command, r *http.Request, in workflow.RequestCreate) (core.Result, error) {
		return service.CreateRequest(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/requests/{id}", read(s, "RequestResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Request(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/requests/{id}/revisions", command(s, "RequestRevise", "RequestResponse", func(ctx context.Context, c core.Command, r *http.Request, in workflow.RequestRevise) (core.Result, error) {
		return service.ReviseRequest(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/requests/{id}/revisions", read(s, "RequestRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.RequestHistory(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/requests/{id}/revisions/{revision}", read(s, "RequestRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.RequestRevision(ctx, a, r.PathValue("id"), r.PathValue("revision"))
	}))
	mux.HandleFunc("GET /v1/requests/{id}/submission-preview", read(s, "SubmissionPreviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.SubmissionPreview(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/requests/{id}/submit", command(s, "RequestSubmit", "RequestResponse", func(ctx context.Context, c core.Command, r *http.Request, in workflow.SubmitInput) (core.Result, error) {
		return service.SubmitRequest(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/requests/{id}/withdraw", command(s, "RequestWithdraw", "RequestResponse", func(ctx context.Context, c core.Command, r *http.Request, in workflow.WithdrawInput) (core.Result, error) {
		return service.WithdrawRequest(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/approval-tasks", read(s, "ApprovalTaskListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.ApprovalTasks(ctx, a, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/approval-steps/{id}/decisions", command(s, "ApprovalDecision", "RequestResponse", func(ctx context.Context, c core.Command, r *http.Request, in workflow.DecisionInput) (core.Result, error) {
		return service.Decide(ctx, c, r.PathValue("id"), in)
	}))
}
