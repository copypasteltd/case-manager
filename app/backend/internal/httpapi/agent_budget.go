package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"net/http"
)

func budgetFilter(r *http.Request) (agent.BudgetFilter, error) {
	for _, key := range []string{"month", "scope", "scope_id"} {
		if len(r.URL.Query()[key]) > 1 {
			return agent.BudgetFilter{}, core.Invalid(key, "筛选参数不能重复")
		}
	}
	return agent.BudgetFilter{Month: r.URL.Query().Get("month"), Scope: r.URL.Query().Get("scope"), ID: r.URL.Query().Get("scope_id")}, nil
}
func (s *Server) agentBudgetRoutes(mux *http.ServeMux, service *agent.Service) {
	mux.HandleFunc("GET /v1/agent-budget", read(s, "BudgetSummaryResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := budgetFilter(r)
		if err != nil {
			return nil, err
		}
		return service.BudgetSummary(ctx, a, f)
	}))
	mux.HandleFunc("GET /v1/agent-budget/entries", read(s, "BudgetEntryListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := budgetFilter(r)
		if err != nil {
			return nil, err
		}
		page, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.BudgetEntries(ctx, a, f, page.Cursor, page.Limit)
	}))
	mux.HandleFunc("POST /v1/agent-budget/policies", command(s, "BudgetPolicyInput", "BudgetPolicyResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.BudgetPolicyInput) (core.Result, error) {
		return service.PublishBudgetPolicy(ctx, c, in)
	}))
	mux.HandleFunc("POST /v1/agent-budget/entries/{id}/reconcile", command(s, "BudgetReconcileInput", "BudgetEntryResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.BudgetReconcileInput) (core.Result, error) {
		return service.ReconcileBudget(ctx, c, r.PathValue("id"), in)
	}))
}
