package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"net/http"
)

func (s *Server) projectTemplateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/project-templates", read(s, "ProjectTemplateListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Projects.Templates(ctx, a, f, r.URL.Query().Get("key"))
	}))
	mux.HandleFunc("GET /v1/project-templates/{id}", read(s, "ProjectTemplateResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.GetTemplate(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/project-templates/{id}/history", read(s, "ProjectTemplateEventListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Projects.TemplateHistory(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("POST /v1/project-templates", command(s, "ProjectTemplateCreate", "ProjectTemplateResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.TemplateCreate) (core.Result, error) {
		return s.Projects.CreateTemplate(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/project-templates/{id}", command(s, "ProjectTemplatePatch", "ProjectTemplateResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.TemplatePatch) (core.Result, error) {
		return s.Projects.UpdateTemplate(ctx, c, r.PathValue("id"), in)
	}))
	for _, action := range []string{"publish", "retire", "revise"} {
		mux.HandleFunc("POST /v1/project-templates/{id}/"+action, command(s, "ProjectTemplateAction", "ProjectTemplateResponse", func(ctx context.Context, c core.Command, r *http.Request, in project.TemplateAction) (core.Result, error) {
			return s.Projects.ActTemplate(ctx, c, r.PathValue("id"), action, in)
		}))
	}
}
