package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/party"
	"copypaste.hk/platform/internal/platform"
)

func (s *Server) partyRoutes(mux *http.ServeMux) {
	service := &party.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/parties", read(s, "PartyListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		var project *string
		if value := r.URL.Query().Get("project_id"); value != "" {
			project = &value
		}
		companyOnly, err := queryBoolean(r, "company_only")
		if err != nil {
			return nil, err
		}
		return service.List(ctx, a, party.Filter{CompanyOnly: companyOnly, Query: r.URL.Query().Get("q"), State: r.URL.Query().Get("state"), Relationship: r.URL.Query().Get("relationship"), ProjectID: project, Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/parties/{id}", read(s, "PartyResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Get(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/parties", command(s, "PartyCreate", "PartyResponse", func(ctx context.Context, c core.Command, r *http.Request, in party.Create) (core.Result, error) {
		return service.Create(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/parties/{id}", command(s, "PartyPatch", "PartyResponse", func(ctx context.Context, c core.Command, r *http.Request, in party.Patch) (core.Result, error) {
		return service.Update(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/parties/{id}/state", command(s, "PartyStateInput", "PartyResponse", func(ctx context.Context, c core.Command, r *http.Request, in party.StateInput) (core.Result, error) {
		return service.ChangeState(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/parties/{id}/revisions", read(s, "PartyRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.History(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/parties/{id}/revisions/{revision}", read(s, "PartyRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Revision(ctx, a, r.PathValue("id"), r.PathValue("revision"))
	}))
}
