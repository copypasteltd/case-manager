package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"net/http"
)

func (s *Server) projectRoutes(mux *http.ServeMux) {
	s.projectTemplateRoutes(mux)
	s.deliverableRoutes(mux)
	s.projectChangeRoutes(mux)
	s.projectCloseoutRoutes(mux)
	s.collaborationRoutes(mux)
	mux.HandleFunc("GET /v1/projects/{id}/overview", read(s, "ProjectOverviewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.Overview(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/dashboard", read(s, "DashboardResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.Dashboard(ctx, a)
	}))
	mux.HandleFunc("GET /v1/projects/{id}/members", read(s, "ProjectMembersResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.Members(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/tasks/{id}/dependencies", read(s, "TaskDependenciesResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.Dependencies(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/projects", read(s, "ProjectListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Projects.List(ctx, a, f)
	}))
	mux.HandleFunc("GET /v1/projects/{id}", read(s, "ProjectResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.Get(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/projects", command(s, "ProjectCreate", "ProjectResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.Create) (core.Result, error) {
		return s.Projects.Create(ctx, c, input)
	}))
	mux.HandleFunc("PATCH /v1/projects/{id}", command(s, "ProjectPatch", "ProjectResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.Patch) (core.Result, error) {
		return s.Projects.Update(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /v1/projects/{id}/transition", command(s, "Transition", "ProjectResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.Transition) (core.Result, error) {
		return s.Projects.Transition(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /v1/projects/{id}/tasks", read(s, "TaskListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Projects.ListTasks(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("GET /v1/tasks/{id}", read(s, "TaskResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.GetTask(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/projects/{id}/tasks", command(s, "TaskCreate", "TaskResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.TaskCreate) (core.Result, error) {
		return s.Projects.CreateTask(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("PATCH /v1/tasks/{id}", command(s, "TaskPatch", "TaskResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.TaskPatch) (core.Result, error) {
		return s.Projects.UpdateTask(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /v1/tasks/{id}/transition", command(s, "Transition", "TaskResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.Transition) (core.Result, error) {
		return s.Projects.TransitionTask(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /v1/tasks/{id}/dependencies", command(s, "TaskDependency", "TaskResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.Dependency) (core.Result, error) {
		return s.Projects.Dependency(ctx, c, r.PathValue("id"), input)
	}))
}
