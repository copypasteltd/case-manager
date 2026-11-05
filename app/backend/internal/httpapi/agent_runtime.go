package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"net/http"
	"strconv"
)

func (s *Server) agentRuntimeRoutes(mux *http.ServeMux) {
	service := &agent.Service{DB: s.DB, Pager: platform.Pager{Key: s.Config.CursorKey}}
	mux.HandleFunc("GET /v1/agent/runtime-profiles/{id}/skill-releases", read(s, "AgentSkillReleaseDetailsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		before := int64(0)
		for k, v := range r.URL.Query() {
			if k != "before_version" || len(v) != 1 {
				return nil, core.Invalid("filters", "仅接受准确历史分页位置")
			}
		}
		if raw := r.URL.Query().Get("before_version"); raw != "" {
			var err error
			before, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || before < 1 {
				return nil, core.Invalid("before_version", "历史分页位置无效")
			}
		}
		return service.SkillReleases(ctx, a, r.PathValue("id"), before)
	}))
	mux.HandleFunc("POST /v1/agent/runtime-profiles/{id}/skill-releases", command(s, "AgentSkillReleaseInput", "AgentSkillReleaseDetailsResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.SkillReleaseInput) (core.Result, error) {
		return service.RecordSkillRelease(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/agent/runtime-profiles", read(s, "AgentProfileOperationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		for k, v := range r.URL.Query() {
			if (k != "state" && k != "skill_id" && k != "cursor" && k != "limit") || len(v) != 1 {
				return nil, core.Invalid("filters", "查询条件无效或重复")
			}
		}
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return service.ProfileOperations(ctx, a, f.State, r.URL.Query().Get("skill_id"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/agent/runtime-profiles/{id}", read(s, "AgentProfileDetailsResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if len(r.URL.Query()) != 0 {
			return nil, core.Invalid("filters", "详情不接受查询条件")
		}
		return service.ProfileOperation(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/agent/runtime-profiles/{id}/retire", command(s, "AgentProfileRetire", "AgentProfileDetailsResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.ProfileRetire) (core.Result, error) {
		return service.RetireProfile(ctx, c, r.PathValue("id"), in)
	}))
}
