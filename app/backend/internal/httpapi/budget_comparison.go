package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
)

func comparisonFilter(r *http.Request, details bool) (project.BudgetComparisonFilter, error) {
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return project.BudgetComparisonFilter{}, core.Invalid("query", "查询参数编码或分隔格式无效")
	}
	f := project.BudgetComparisonFilter{Currency: query.Get("currency"), Limit: 20}
	for key, values := range query {
		allowed := key == "currency" || details && (key == "basis_hash" || key == "line" || key == "state" || key == "cursor" || key == "limit")
		if !allowed || len(values) != 1 {
			return f, core.Invalid("query", "查询参数无效或重复")
		}
	}
	if details {
		page, err := filter(r)
		if err != nil {
			return f, err
		}
		f.Cursor, f.Limit = page.Cursor, page.Limit
		f.BasisHash, f.Line, f.State = r.URL.Query().Get("basis_hash"), r.URL.Query().Get("line"), r.URL.Query().Get("state")
		if len(f.BasisHash) != 64 || strings.Trim(f.BasisHash, "0123456789abcdef") != "" {
			return f, core.Invalid("basis_hash", "请先读取当前预算对比依据")
		}
	}
	return f, nil
}

func (s *Server) budgetComparisonRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/projects/{id}/budget-comparison", read(s, "BudgetComparisonResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := comparisonFilter(r, false)
		if err != nil {
			return nil, err
		}
		return s.Projects.CompareBudget(ctx, a, r.PathValue("id"), f.Currency)
	}))
	mux.HandleFunc("GET /v1/projects/{id}/budget-comparison/entries", read(s, "BudgetComparisonEntriesResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := comparisonFilter(r, true)
		if err != nil {
			return nil, err
		}
		return s.Projects.BudgetComparisonEntries(ctx, a, r.PathValue("id"), f)
	}))
}
