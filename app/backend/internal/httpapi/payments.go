package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/platform"
	"net/http"
)

func paymentFilter(r *http.Request) (finance.PaymentFilter, error) {
	f, err := costFilter(r)
	return finance.PaymentFilter{Query: f.Query, State: f.State, Direction: r.URL.Query().Get("direction"), Currency: f.Currency, Category: f.Category, From: f.From, To: f.To, Cursor: f.Cursor, ProjectID: f.ProjectID, CompanyOnly: f.CompanyOnly, ProjectRecords: f.ProjectRecords, Limit: f.Limit}, err
}
func (s *Server) paymentRoutes(mux *http.ServeMux) {
	v := &finance.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/payments/{id}/fees", read(s, "PaymentFeeLedgerResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.PaymentFees(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/payments/{id}/fees/basis", read(s, "PaymentFeePreviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.PaymentFeeBasis(ctx, a, r.PathValue("id"), r.URL.Query().Get("fee_payment_id"), r.URL.Query().Get("allocation_id"))
	}))
	mux.HandleFunc("POST /v1/payments/{id}/fees", command(s, "PaymentFeeCreate", "PaymentFeeLinkResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.PaymentFeeCreate) (core.Result, error) {
		return v.CreatePaymentFee(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/payments/{id}/fees/{fee}/revoke", command(s, "PaymentFeeRevoke", "PaymentFeeLinkResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.PaymentFeeRevoke) (core.Result, error) {
		return v.RevokePaymentFee(ctx, c, r.PathValue("id"), r.PathValue("fee"), in)
	}))
	mux.HandleFunc("GET /v1/payments", read(s, "PaymentListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := paymentFilter(r)
		if e != nil {
			return nil, e
		}
		return v.Payments(ctx, a, f)
	}))
	mux.HandleFunc("GET /v1/payments/{id}", read(s, "PaymentResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.Payment(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/payments", command(s, "PaymentCreate", "PaymentResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.PaymentCreate) (core.Result, error) {
		return v.CreatePayment(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/payments/{id}", command(s, "PaymentPatch", "PaymentResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.PaymentPatch) (core.Result, error) {
		return v.UpdatePayment(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/payments/{id}/record", command(s, "PaymentRecord", "PaymentResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.PaymentRecordInput) (core.Result, error) {
		return v.RecordPayment(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/payments/{id}/reverse", command(s, "PaymentReverse", "PaymentResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.PaymentReverseInput) (core.Result, error) {
		return v.ReversePayment(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/payments/{id}/allocations", command(s, "PaymentAllocate", "PaymentResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.PaymentAllocate) (core.Result, error) {
		return v.AllocatePayment(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/payments/{id}/allocations/{allocation}/release", command(s, "PaymentRelease", "PaymentResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.PaymentRelease) (core.Result, error) {
		return v.ReleasePaymentAllocation(ctx, c, r.PathValue("id"), r.PathValue("allocation"), in)
	}))
	mux.HandleFunc("POST /v1/payments/{id}/refund-allocations", command(s, "PaymentRefundAllocate", "PaymentResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.PaymentRefundAllocate) (core.Result, error) {
		return v.AllocateRefund(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/payments/{id}/allocations", read(s, "PaymentAllocationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.PaymentAllocations(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/payments/{id}/revisions", read(s, "PaymentRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.PaymentHistory(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/payments/{id}/revisions/{revision}", read(s, "PaymentRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.PaymentRevision(ctx, a, r.PathValue("id"), r.PathValue("revision"))
	}))
	mux.HandleFunc("GET /v1/payments/{id}/events", read(s, "PaymentEventListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.PaymentEvents(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/claims/{id}/settlement", read(s, "ClaimSettlementResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.ClaimSettlement(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/reports/cash", read(s, "CashReportResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := paymentFilter(r)
		if e != nil {
			return nil, e
		}
		return v.CashReport(ctx, a, f)
	}))
}
