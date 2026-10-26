package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/platform"
	"net/http"
)

func costFilter(r *http.Request) (finance.Filter, error) {
	f, err := filter(r)
	if err != nil {
		return finance.Filter{}, err
	}
	company, e := queryBoolean(r, "company_only")
	if e != nil {
		return finance.Filter{}, e
	}
	var project *string
	if id := r.URL.Query().Get("project_id"); id != "" {
		project = &id
	}
	q := r.URL.Query()
	return finance.Filter{Query: q.Get("q"), State: q.Get("state"), Currency: q.Get("currency"), Category: q.Get("category"), From: q.Get("from"), To: q.Get("to"), Cursor: f.Cursor, Limit: f.Limit, ProjectID: project, CompanyOnly: company}, nil
}
func (s *Server) costRoutes(mux *http.ServeMux) {
	v := &finance.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/costs", read(s, "CostListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := costFilter(r)
		if e != nil {
			return nil, e
		}
		return v.List(ctx, a, f)
	}))
	mux.HandleFunc("GET /v1/costs/{id}", read(s, "CostResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.Get(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/costs", command(s, "CostCreate", "CostResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.Create) (core.Result, error) {
		return v.Create(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/costs/{id}", command(s, "CostPatch", "CostResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.Patch) (core.Result, error) {
		return v.Update(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/costs/{id}/confirm", command(s, "CostConfirm", "CostResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.Base) (core.Result, error) {
		return v.Confirm(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/costs/{id}/reverse", command(s, "CostReverse", "CostResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.Reverse) (core.Result, error) {
		return v.Reverse(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/costs/{id}/links", command(s, "CostLink", "CostResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.LinkInput) (core.Result, error) {
		return v.Link(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/costs/{id}/revisions", read(s, "CostRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.History(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/costs/{id}/revisions/{revision}", read(s, "CostRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.Revision(ctx, a, r.PathValue("id"), r.PathValue("revision"))
	}))
	mux.HandleFunc("GET /v1/costs/{id}/events", read(s, "CostEventListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.Events(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/reports/expenses", read(s, "CostReportResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := costFilter(r)
		if e != nil {
			return nil, e
		}
		return v.Report(ctx, a, r.URL.Query().Get("kind"), f)
	}))
}
