package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/finance"
)

func (s *Server) agentSubscriptionPortfolioRoutes(mux *http.ServeMux) {
	service := &agent.Service{DB: s.DB}
	mux.HandleFunc("GET /v1/conversations/{id}/subscription-review", read(s, "AgentSubscriptionPortfolioPreviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		q := r.URL.Query()
		for k, v := range q {
			if (k != "from" && k != "to") || len(v) != 1 {
				return nil, core.Invalid("filters", "请选择准确开始日与不含结束日，不接受额外或重复条件")
			}
		}
		return service.SubscriptionPortfolio(ctx, a, r.PathValue("id"), finance.SubscriptionPortfolioInput{From: q.Get("from"), To: q.Get("to")})
	}))
}
