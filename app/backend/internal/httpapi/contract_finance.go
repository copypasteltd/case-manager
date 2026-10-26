package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/platform"
	"net/http"
)

func (s *Server) contractFinanceRoutes(mux *http.ServeMux) {
	v := &finance.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/contracts/{id}/finance", read(s, "ContractFinanceResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.ContractFinance(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/contracts/{id}/finance/events", read(s, "ContractFinanceEventListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.ContractFinanceEvents(ctx, a, r.PathValue("id"), r.URL.Query().Get("milestone_id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/contracts/{id}/finance/entries", command(s, "ContractFinanceLink", "ContractFinanceReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.ContractFinanceLink) (core.Result, error) {
		return v.LinkContractFinance(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/contracts/{id}/finance/entries/{entry}/release", command(s, "ContractFinanceRelease", "ContractFinanceReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.ContractFinanceRelease) (core.Result, error) {
		return v.ReleaseContractFinance(ctx, c, r.PathValue("id"), r.PathValue("entry"), in)
	}))
	mux.HandleFunc("POST /v1/contracts/{id}/finance/milestones/{milestone}/review", command(s, "ContractFinanceReview", "ContractFinanceReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.ContractFinanceReview) (core.Result, error) {
		return v.ReviewContractFinance(ctx, c, r.PathValue("id"), r.PathValue("milestone"), in)
	}))
}
