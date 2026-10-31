package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"net/http"
)

func (s *Server) objectRetirementRoutes(mux *http.ServeMux, metadata *files.Service) {
	service := &files.ObjectRetirementService{Files: metadata, Backends: s.OriginalCleanup}
	mux.HandleFunc("GET /v1/files/{id}/object-retirement-candidates", read(s, "RetirementCandidateListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Candidates(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/files/{id}/object-retirement-plans", read(s, "RetirementPlanListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Plans(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("POST /v1/files/{id}/object-retirement-plans", command(s, "RetirementCreate", "RetirementPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.RetirementCreate) (core.Result, error) {
		return service.Create(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/object-retirement-plans/{id}", read(s, "RetirementPlanResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Plan(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/object-retirement-plans/{id}/approve", command(s, "CleanupDecision", "RetirementPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.CleanupDecision) (core.Result, error) {
		return service.Approve(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/object-retirement-plans/{id}/reauthorize", command(s, "RetirementRenewal", "RetirementPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.RetirementRenewal) (core.Result, error) {
		return service.Reauthorize(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/object-retirement-plans/{id}/authorizations", read(s, "RetirementAuthorizationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Authorizations(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("POST /v1/object-retirement-plans/{id}/cancel", command(s, "CleanupDecision", "RetirementPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.CleanupDecision) (core.Result, error) {
		return service.Cancel(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/object-retirement-items/{id}/retry", command(s, "UploadFinalize", "RetirementPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.UploadExpected) (core.Result, error) {
		return service.Retry(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/object-retirement-items/{id}/receipts", read(s, "CleanupReceiptListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Receipts(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
}
