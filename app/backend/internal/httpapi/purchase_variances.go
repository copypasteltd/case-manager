package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/spend"
)

func (s *Server) purchaseVarianceRoutes(mux *http.ServeMux, service *spend.Service) {
	mux.HandleFunc("GET /v1/purchases/{id}/execution-plan", read(s, "PurchaseExecutionPlanResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.ExecutionPlan(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/purchases/{id}/variances", read(s, "PurchaseVarianceListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.Variances(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/purchases/{id}/variances", command(s, "PurchaseVarianceCreate", "PurchaseVarianceResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.VarianceCreate) (core.Result, error) {
		return service.CreateVariance(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/purchase-variances/{id}", read(s, "PurchaseVarianceResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Variance(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("PATCH /v1/purchase-variances/{id}", command(s, "PurchaseVariancePatch", "PurchaseVarianceResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.VariancePatch) (core.Result, error) {
		return service.UpdateVariance(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/purchase-variances/{id}/revisions", read(s, "PurchaseVarianceRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.VarianceHistory(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/purchase-variances/{id}/revisions/{revision}", read(s, "PurchaseVarianceRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.VarianceRevision(ctx, a, r.PathValue("id"), r.PathValue("revision"))
	}))
	mux.HandleFunc("POST /v1/purchase-variances/{id}/activate", command(s, "PurchaseVarianceActivation", "PurchaseVarianceResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.VarianceActivation) (core.Result, error) {
		return service.ActivateVariance(ctx, c, r.PathValue("id"), in)
	}))
}
