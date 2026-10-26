package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/spend"
	"net/http"
)

func (s *Server) purchaseRoutes(mux *http.ServeMux, service *spend.Service) {
	s.purchaseVarianceRoutes(mux, service)
	s.purchaseCompletionRoutes(mux, service)
	base := "/v1/purchases/{id}"
	mux.HandleFunc("GET "+base+"/fulfillment", read(s, "PurchaseFulfillmentResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Fulfillment(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET "+base+"/orders", read(s, "PurchaseOrderListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.Orders(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET "+base+"/orders/{order}", read(s, "PurchaseOrderResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Order(ctx, a, r.PathValue("id"), r.PathValue("order"))
	}))
	mux.HandleFunc("GET "+base+"/receipts", read(s, "PurchaseReceiptListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.Receipts(ctx, a, r.PathValue("id"), r.URL.Query().Get("order_id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET "+base+"/receipts/{receipt}", read(s, "PurchaseReceiptResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Receipt(ctx, a, r.PathValue("id"), r.PathValue("receipt"))
	}))
	mux.HandleFunc("GET "+base+"/execution-events", read(s, "PurchaseExecutionEventListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.ExecutionHistory(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST "+base+"/orders", command(s, "PurchaseOrderCreate", "SpendResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.OrderInput) (core.Result, error) {
		return service.RegisterOrder(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST "+base+"/receipts", command(s, "PurchaseReceiptCreate", "SpendResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.ReceiptInput) (core.Result, error) {
		return service.RegisterReceipt(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST "+base+"/orders/{order}/cancel", command(s, "PurchaseExecutionInput", "SpendResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.ExecutionBase) (core.Result, error) {
		return service.CancelOrder(ctx, c, r.PathValue("id"), r.PathValue("order"), in)
	}))
	mux.HandleFunc("POST "+base+"/cancel", command(s, "PurchaseExecutionInput", "SpendResponse", func(ctx context.Context, c core.Command, r *http.Request, in spend.ExecutionBase) (core.Result, error) {
		return service.CancelPurchase(ctx, c, r.PathValue("id"), in)
	}))
}
