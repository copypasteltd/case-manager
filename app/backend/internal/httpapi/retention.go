package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
)

func (s *Server) retentionRoutes(mux *http.ServeMux, service *files.Service) {
	mux.HandleFunc("GET /v1/storage/retention-policies", read(s, "RetentionPolicyListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.RetentionPolicies(ctx, a)
	}))
	mux.HandleFunc("POST /v1/storage/retention-policies", command(s, "RetentionPolicyInput", "RetentionPolicyResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.RetentionPolicyInput) (core.Result, error) {
		return service.PublishRetentionPolicy(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/files/{id}/retention", read(s, "RetentionOverviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Retention(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/files/{id}/retention-holds", read(s, "RetentionHoldListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.RetentionHolds(ctx, a, r.PathValue("id"), files.Filter{Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("POST /v1/files/{id}/retention-holds", command(s, "RetentionHoldCreate", "RetentionHoldResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.RetentionHoldCreate) (core.Result, error) {
		return service.CreateRetentionHold(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/retention-holds/{id}/release", command(s, "RetentionHoldRelease", "RetentionHoldResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.RetentionHoldRelease) (core.Result, error) {
		return service.ReleaseRetentionHold(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/file-versions/{id}/retention-assessments", command(s, "RetentionAssessmentInput", "RetentionAssessmentResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.RetentionAssessmentInput) (core.Result, error) {
		return service.AssessRetention(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/retention-assessments/{id}", read(s, "RetentionAssessmentResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.RetentionAssessment(ctx, a, r.PathValue("id"))
	}))
}
