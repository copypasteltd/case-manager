package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/contracts"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func (s *Server) contractRoutes(mux *http.ServeMux) {
	service := &contracts.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/contracts", read(s, "ContractListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		var project *string
		if value := r.URL.Query().Get("project_id"); value != "" {
			project = &value
		}
		trashed, err := queryBoolean(r, "trashed")
		if err != nil {
			return nil, err
		}
		company, err := queryBoolean(r, "company_only")
		if err != nil {
			return nil, err
		}
		return service.List(ctx, a, contracts.Filter{CompanyOnly: company, Query: r.URL.Query().Get("q"), Lifecycle: r.URL.Query().Get("lifecycle"), Trashed: trashed, ProjectID: project, Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/contracts/{id}", read(s, "ContractResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Get(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/contracts", command(s, "ContractCreate", "ContractResponse", func(ctx context.Context, c core.Command, r *http.Request, in contracts.Create) (core.Result, error) {
		return service.Create(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/contracts/{id}", command(s, "ContractPatch", "ContractResponse", func(ctx context.Context, c core.Command, r *http.Request, in contracts.Patch) (core.Result, error) {
		return service.Update(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/contracts/{id}/trash", command(s, "ContractTrashInput", "ContractResponse", func(ctx context.Context, c core.Command, r *http.Request, in contracts.TrashInput) (core.Result, error) {
		return service.Trash(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/contracts/{id}/signature", command(s, "ContractSignature", "ContractResponse", func(ctx context.Context, c core.Command, r *http.Request, in contracts.SignatureInput) (core.Result, error) {
		return service.RegisterSignature(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/contracts/{id}/milestones", command(s, "ContractScheduleInput", "ContractResponse", func(ctx context.Context, c core.Command, r *http.Request, in contracts.ScheduleInput) (core.Result, error) {
		return service.Schedule(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/contracts/{id}/milestones", read(s, "ContractMilestoneListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Milestones(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/contracts/{id}/transition", command(s, "ContractActivation", "ContractResponse", func(ctx context.Context, c core.Command, r *http.Request, in contracts.ActivationInput) (core.Result, error) {
		return service.Activate(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/contracts/{id}/revisions", read(s, "ContractRevisionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.History(ctx, a, r.PathValue("id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/contracts/{id}/revisions/{revision}", read(s, "ContractRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Revision(ctx, a, r.PathValue("id"), r.PathValue("revision"))
	}))
}
