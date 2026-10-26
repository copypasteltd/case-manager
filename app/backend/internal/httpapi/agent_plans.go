package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
)

func (s *Server) agentPlanRoutes(mux *http.ServeMux) {
	s.agentOrganizationRoutes(mux)
	service := &agent.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}}
	mux.HandleFunc("GET /v1/conversations/{id}/plans", read(s, "AgentPlanListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.ListPlans(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/conversations/{id}/plans", command(s, "AgentProjectPlanCreate", "AgentPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.ProjectPlanCreate) (core.Result, error) {
		return service.ProposeProjectPlan(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/plans/{id}", read(s, "AgentPlanResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.GetPlan(ctx, a, r.PathValue("id"))
	}))
	for _, action := range []string{"approve", "revoke", "commit"} {
		mux.HandleFunc("POST /v1/plans/{id}/"+action, command(s, "AgentPlanAction", "AgentPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.PlanAction) (core.Result, error) {
			return service.ActOnPlan(ctx, c, r.PathValue("id"), action, in)
		}))
	}
}
