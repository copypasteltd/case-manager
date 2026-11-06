package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"net/http"
)

func (s *Server) objectRetirementRoutes(mux *http.ServeMux, metadata *files.Service) {
	service := &files.ObjectRetirementService{Files: metadata, Backends: s.OriginalCleanup}
	mux.HandleFunc("GET /v1/folders/{id}/terminal-upload-candidates", read(s, "TerminalCandidateListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.TerminalCandidates(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/uploads/{id}/terminal-observations", read(s, "TerminalOverviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "准确观察历史不接受筛选参数")
		}
		return service.TerminalOverview(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/uploads/{id}/terminal-observations", command(s, "TerminalObserveCreate", "TerminalObservationResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.TerminalObserveCreate) (core.Result, error) {
		return service.ObserveTerminal(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/folders/{id}/completed-draft-retirement-candidates", read(s, "RetirementCandidateListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.CompletedDraftCandidates(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/folders/{id}/completed-draft-retirement-plans", read(s, "RetirementPlanListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.CompletedDraftPlans(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("POST /v1/folders/{id}/completed-draft-retirement-plans", command(s, "CompletedDraftRetirementCreate", "RetirementPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.CompletedDraftRetirementCreate) (core.Result, error) {
		return service.CreateCompletedDraft(ctx, c, r.PathValue("id"), in)
	}))
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
