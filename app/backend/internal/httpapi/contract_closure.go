package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/platform"
)

func (s *Server) contractClosureRoutes(mux *http.ServeMux) {
	v := &finance.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}}
	mux.HandleFunc("GET /v1/contracts/{id}/closure-preview", read(s, "ContractClosurePreviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.ContractClosurePreview(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/contracts/{id}/closure", read(s, "ContractClosureResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.ContractClosure(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/contracts/{id}/closure", command(s, "ContractClosureInput", "ContractClosureReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.ContractClosureInput) (core.Result, error) {
		return v.CloseContract(ctx, c, r.PathValue("id"), in)
	}))
}
