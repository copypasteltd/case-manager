package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/spend"
	"net/http"
)

func (s *Server) purchaseCompletionRoutes(mux *http.ServeMux, service *spend.Service) {
	mux.HandleFunc("GET /v1/purchases/{id}/completion-preview", read(s, "PurchaseCompletionPreviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.CompletionPreview(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/purchases/{id}/completion", read(s, "PurchaseCompletionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Completion(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/purchases/{id}/completion", command(s, "PurchaseCompletionInput", "PurchaseCompletionCommandReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.CompletionInput) (core.Result, error) {
		return service.CompletePurchase(ctx, c, r.PathValue("id"), in)
	}))
}
