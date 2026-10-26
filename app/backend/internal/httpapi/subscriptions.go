package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
	"copypaste.hk/platform/internal/platform"
)

func subscriptionFilter(r *http.Request) (finance.SubscriptionFilter, error) {
	f, err := costFilter(r)
	if err != nil {
		return finance.SubscriptionFilter{}, err
	}
	forecastOnly, err := queryBoolean(r, "forecast_only")
	if err != nil {
		return finance.SubscriptionFilter{}, err
	}
	v := finance.SubscriptionFilter{Query: f.Query, State: f.State, Currency: f.Currency, Category: f.Category, From: f.From, To: f.To, Cursor: f.Cursor, ProjectID: f.ProjectID, CompanyOnly: f.CompanyOnly, ForecastOnly: forecastOnly, Limit: f.Limit}
	if id := r.URL.Query().Get("supplier_id"); id != "" {
		v.SupplierID = &id
	}
	if id := r.URL.Query().Get("owner_id"); id != "" {
		v.OwnerID = &id
	}
	return v, nil
}
func (s *Server) subscriptionRoutes(mux *http.ServeMux) {
	v := &finance.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/subscriptions", read(s, "SubscriptionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := subscriptionFilter(r)
		if e != nil {
			return nil, e
		}
		return v.Subscriptions(ctx, a, f)
	}))
	mux.HandleFunc("GET /v1/subscriptions/{id}", read(s, "SubscriptionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.Subscription(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/subscriptions", command(s, "SubscriptionCreate", "SubscriptionResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.SubscriptionCreate) (core.Result, error) {
		return v.CreateSubscription(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/subscriptions/{id}", command(s, "SubscriptionPatch", "SubscriptionResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.SubscriptionPatch) (core.Result, error) {
		return v.UpdateSubscription(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/subscriptions/{id}/transition", command(s, "SubscriptionTransition", "SubscriptionResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.SubscriptionTransition) (core.Result, error) {
		return v.TransitionSubscription(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/subscriptions/{id}/prices", command(s, "SubscriptionPriceChange", "SubscriptionResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.SubscriptionPriceChange) (core.Result, error) {
		return v.ChangeSubscriptionPrice(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/subscriptions/{id}/revisions", read(s, "SubscriptionRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.SubscriptionHistory(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/subscriptions/{id}/revisions/{revision}", read(s, "SubscriptionRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.SubscriptionRevision(ctx, a, r.PathValue("id"), r.PathValue("revision"))
	}))
	mux.HandleFunc("GET /v1/subscriptions/{id}/events", read(s, "SubscriptionEventListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.SubscriptionEvents(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/subscriptions/{id}/reminders", read(s, "SubscriptionReminderListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.SubscriptionReminders(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/subscriptions/owners", read(s, "SubscriptionOwnerListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := subscriptionFilter(r)
		if e != nil {
			return nil, e
		}
		return v.SubscriptionOwners(ctx, a, f.ProjectID, r.URL.Query().Get("classification"))
	}))
	mux.HandleFunc("GET /v1/subscriptions/{id}/periods", read(s, "SubscriptionPeriodListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := subscriptionFilter(r)
		if e != nil {
			return nil, e
		}
		return v.SubscriptionPeriods(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("POST /v1/subscriptions/{id}/periods/generate", command(s, "SubscriptionGenerate", "SubscriptionGenerationResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.SubscriptionGenerate) (core.Result, error) {
		return v.GenerateSubscription(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/subscriptions/{id}/periods/unexpected", command(s, "SubscriptionUnexpectedPeriod", "SubscriptionPeriodResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.SubscriptionUnexpectedPeriod) (core.Result, error) {
		return v.UnexpectedSubscriptionPeriod(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/subscription-periods", read(s, "SubscriptionPeriodListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := subscriptionFilter(r)
		if e != nil {
			return nil, e
		}
		return v.SubscriptionPeriods(ctx, a, "", f)
	}))
	mux.HandleFunc("GET /v1/subscription-periods/{id}", read(s, "SubscriptionPeriodResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return v.SubscriptionPeriod(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/subscription-periods/{id}/reforecast", command(s, "SubscriptionPeriodReforecast", "SubscriptionPeriodResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.SubscriptionPeriodReforecast) (core.Result, error) {
		return v.ReforecastSubscriptionPeriod(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/subscription-periods/{id}/costs", command(s, "SubscriptionPeriodLink", "SubscriptionPeriodResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.SubscriptionPeriodLink) (core.Result, error) {
		return v.LinkSubscriptionCost(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/subscription-periods/{id}/review", command(s, "SubscriptionPeriodDecision", "SubscriptionPeriodResponse", func(ctx context.Context, c core.Command, r *http.Request, in finance.SubscriptionPeriodDecision) (core.Result, error) {
		return v.DecideSubscriptionPeriod(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/subscription-periods/{id}/revisions", read(s, "SubscriptionPeriodRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.SubscriptionPeriodHistory(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/subscription-periods/{id}/events", read(s, "SubscriptionPeriodEventListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return v.SubscriptionPeriodEvents(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/reports/forecast", read(s, "ForecastReportResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := subscriptionFilter(r)
		if e != nil {
			return nil, e
		}
		return v.ForecastReport(ctx, a, f)
	}))
}
