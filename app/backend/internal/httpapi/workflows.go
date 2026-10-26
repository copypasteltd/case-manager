package httpapi

import (
	"context"
	"net/http"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/workflow"
)

func (s *Server) workflowRoutes(mux *http.ServeMux) {
	service := &workflow.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}}
	list := func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, e := filter(r)
		if e != nil {
			return nil, e
		}
		return service.Definitions(ctx, a, workflow.Filter{Kind: r.URL.Query().Get("kind"), Cursor: f.Cursor, Limit: f.Limit}, r.PathValue("id"))
	}
	mux.HandleFunc("GET /v1/workflow-definitions", read(s, "WorkflowDefinitionListResponse", list))
	mux.HandleFunc("GET /v1/workflow-definitions/{id}/history", read(s, "WorkflowDefinitionListResponse", list))
	mux.HandleFunc("GET /v1/workflow-definitions/{id}", read(s, "WorkflowDefinitionResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Definition(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/workflow-definitions", command(s, "WorkflowDefinitionInput", "WorkflowDefinitionResponse", func(ctx context.Context, c core.Command, r *http.Request, in workflow.DefinitionInput) (core.Result, error) {
		return service.PublishDefinition(ctx, c, in)
	}))
}
