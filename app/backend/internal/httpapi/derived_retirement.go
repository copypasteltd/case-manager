package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"net/http"
)

func (s *Server) derivedRetirementRoutes(mux *http.ServeMux, v *files.Service) {
	mux.HandleFunc("GET /v1/files/{id}/versions/{version}/derived-retirement", read(s, "DerivedRetirementOverviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if len(r.URL.Query()) != 0 {
			return nil, core.Invalid("query", "派生盘点不接受额外查询条件")
		}
		return v.DerivedRetirement(ctx, a, r.PathValue("id"), r.PathValue("version"))
	}))
	mux.HandleFunc("POST /v1/files/{id}/versions/{version}/derived-retirement", command(s, "DerivedRetirementCreate", "DerivedRetirementPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.DerivedRetirementCreate) (core.Result, error) {
		return v.CreateDerivedRetirement(ctx, c, r.PathValue("id"), r.PathValue("version"), in)
	}))
	for _, action := range []string{"execute", "cancel"} {
		action := action
		mux.HandleFunc("POST /v1/derived-retirement-plans/{id}/"+action, command(s, "DerivedRetirementDecision", "DerivedRetirementPlanResponse", func(ctx context.Context, c core.Command, r *http.Request, in files.DerivedRetirementDecision) (core.Result, error) {
			return v.DecideDerivedRetirement(ctx, c, r.PathValue("id"), action, in)
		}))
	}
}
