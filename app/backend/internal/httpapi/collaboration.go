package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"net/http"
)

func (s *Server) collaborationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/projects/{id}/member-candidates", read(s, "ProjectMembersResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.MemberCandidates(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/projects/{id}/members", command(s, "ProjectMemberChange", "ProjectResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.MemberChange) (core.Result, error) {
		return s.Projects.ChangeMember(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /v1/projects/{id}/milestones", read(s, "MilestoneListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Projects.ListMilestones(ctx, a, r.PathValue("id"), f)
	}))
	mux.HandleFunc("POST /v1/projects/{id}/milestones", command(s, "MilestoneCreate", "MilestoneResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.MilestoneCreate) (core.Result, error) {
		return s.Projects.CreateMilestone(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /v1/milestones/{id}", read(s, "MilestoneResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Projects.GetMilestone(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("PATCH /v1/milestones/{id}", command(s, "MilestonePatch", "MilestoneResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.MilestonePatch) (core.Result, error) {
		return s.Projects.UpdateMilestone(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("POST /v1/milestones/{id}/transition", command(s, "Transition", "MilestoneResponse", func(ctx context.Context, c core.Command, r *http.Request, input project.Transition) (core.Result, error) {
		return s.Projects.TransitionMilestone(ctx, c, r.PathValue("id"), input)
	}))
	for _, route := range []struct{ table, kind, schema string }{{"risks", "risk", "Risk"}, {"issues", "issue", "Issue"}} {
		mux.HandleFunc("GET /v1/projects/{id}/"+route.table, read(s, route.schema+"ListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			f, err := filter(r)
			if err != nil {
				return nil, err
			}
			return s.Projects.ListConcerns(ctx, a, r.PathValue("id"), route.kind, f)
		}))
		mux.HandleFunc("POST /v1/projects/{id}/"+route.table, command(s, route.schema+"Create", route.schema+"Response", func(ctx context.Context, c core.Command, r *http.Request, input project.ConcernCreate) (core.Result, error) {
			return s.Projects.CreateConcern(ctx, c, r.PathValue("id"), route.kind, input)
		}))
		mux.HandleFunc("GET /v1/"+route.table+"/{id}", read(s, route.schema+"Response", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
			return s.Projects.GetConcern(ctx, a, r.PathValue("id"), route.kind)
		}))
		mux.HandleFunc("PATCH /v1/"+route.table+"/{id}", command(s, route.schema+"Patch", route.schema+"Response", func(ctx context.Context, c core.Command, r *http.Request, input project.ConcernPatch) (core.Result, error) {
			return s.Projects.UpdateConcern(ctx, c, r.PathValue("id"), route.kind, input)
		}))
	}
}
