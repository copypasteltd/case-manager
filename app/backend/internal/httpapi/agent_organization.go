package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"net/http"
)

func (s *Server) agentOrganizationRoutes(mux *http.ServeMux) {
	service := &agent.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}}
	mux.HandleFunc("GET /v1/conversations/{id}/directories/{folderID}", read(s, "AgentDirectoryResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.OrganizationDirectory(ctx, a, r.PathValue("id"), r.PathValue("folderID"))
	}))
	mux.HandleFunc("GET /v1/conversations/{id}/organization-plans", read(s, "AgentOrganizationPlanListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.ListOrganizationPlans(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/conversations/{id}/organization-plans", command(s, "AgentOrganizationPlanCreate", "AgentOrganizationPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.OrganizationPlanCreate) (core.Result, error) {
		return service.ProposeOrganization(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/organization-plans/{id}", read(s, "AgentOrganizationPlanResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.GetOrganizationPlan(ctx, a, r.PathValue("id"))
	}))
	for _, action := range []string{"approve", "revoke", "commit"} {
		mux.HandleFunc("POST /v1/organization-plans/{id}/"+action, command(s, "AgentPlanAction", "AgentOrganizationPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.PlanAction) (core.Result, error) {
			return service.ActOnOrganizationPlan(ctx, c, r.PathValue("id"), action, in)
		}))
	}
}
