package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"net/http"
)

func (s *Server) multipartCleanupRoutes(mux *http.ServeMux, metadata *files.Service) {
	service := &files.MultipartCleanupService{Files: metadata, Backends: s.MultipartCleanup}
	mux.HandleFunc("GET /v1/folders/{id}/upload-cleanup-candidates", read(s, "CleanupCandidateListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Candidates(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/folders/{id}/upload-cleanup-plans", read(s, "CleanupPlanListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Plans(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("POST /v1/folders/{id}/upload-cleanup-plans", command(s, "CleanupCreate", "CleanupPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.CleanupCreate) (core.Result, error) {
		return service.Create(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/upload-cleanup-plans/{id}", read(s, "CleanupPlanResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Plan(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/upload-cleanup-plans/{id}/approve", command(s, "CleanupDecision", "CleanupPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.CleanupDecision) (core.Result, error) {
		return service.Approve(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/upload-cleanup-plans/{id}/cancel", command(s, "CleanupDecision", "CleanupPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.CleanupDecision) (core.Result, error) {
		return service.Cancel(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/upload-cleanup-items/{id}/retry", command(s, "UploadFinalize", "CleanupPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.UploadExpected) (core.Result, error) {
		return service.Retry(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/upload-cleanup-items/{id}/receipts", read(s, "CleanupReceiptListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Receipts(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
}
