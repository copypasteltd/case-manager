package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"net/http"
	"net/url"
)

func (s *Server) fxJournalRoutes(mux *http.ServeMux, v *finance.Service) {
	mux.HandleFunc("GET /v1/payments/{id}/fx-differences", read(s, "FXJournalLedgerResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if r.URL.RawQuery != "" {
			return nil, core.Invalid("query", "完整汇兑台账不接受附加查询参数")
		}
		return v.FXJournals(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/payments/{id}/fx-differences/basis", read(s, "FXJournalPreviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		q, e := url.ParseQuery(r.URL.RawQuery)
		if e != nil {
			return nil, core.Invalid("query", "汇兑依据查询无效")
		}
		for k, values := range q {
			if len(values) != 1 || (k != "allocation_id" && k != "fx_book_id") {
				return nil, core.Invalid("query", "汇兑查询参数未知或重复")
			}
		}
		return v.FXJournalBasis(ctx, a, r.PathValue("id"), q.Get("allocation_id"), q.Get("fx_book_id"))
	}))
	mux.HandleFunc("POST /v1/payments/{id}/fx-differences", command(s, "FXJournalCreate", "FXJournalResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.FXJournalCreate) (core.Result, error) {
		return v.CreateFXJournal(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/payments/{id}/fx-differences/{journal}/revoke", command(s, "FXJournalRevoke", "FXJournalResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.FXJournalRevoke) (core.Result, error) {
		return v.RevokeFXJournal(ctx, c, r.PathValue("id"), r.PathValue("journal"), in)
	}))
}
