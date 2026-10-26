package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/platform"
)

func (s *Server) conversationRoutes(mux *http.ServeMux) {
	service := &agent.Service{DB: s.DB, Commands: platform.Commands{DB: s.DB}, Pager: platform.Pager{Key: s.Config.CursorKey}, FileText: &files.AgentTextService{Backends: s.AgentFileReaders}, FileDrafts: s.AgentDraftWriter}
	mux.HandleFunc("GET /v1/conversations", read(s, "ConversationListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		var project *string
		if value := r.URL.Query().Get("project_id"); value != "" {
			project = &value
		}
		return service.List(ctx, a, agent.Filter{Type: r.URL.Query().Get("type"), State: f.State, Query: f.Query, ProjectID: project, Cursor: f.Cursor, Limit: f.Limit})
	}))
	mux.HandleFunc("GET /v1/conversations/{id}", read(s, "ConversationResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.Get(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/conversations", command(s, "ConversationCreate", "ConversationResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.ConversationCreate) (core.Result, error) {
		return service.Create(ctx, c, in)
	}))
	mux.HandleFunc("PATCH /v1/conversations/{id}", command(s, "ConversationPatch", "ConversationResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.ConversationPatch) (core.Result, error) {
		return service.Update(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/conversations/{id}/members", command(s, "ConversationMemberChange", "ConversationAccessResultResponse", func(ctx context.Context, c core.Command, r *http.Request, in agent.MemberChange) (core.Result, error) {
		return service.ChangeMember(ctx, c, r.PathValue("id"), in)
	}))
	mux.HandleFunc("GET /v1/conversations/{id}/member-candidates", read(s, "ConversationCandidatesResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return service.MemberCandidates(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("GET /v1/conversations/{id}/context-sources/{resourceType}/{resourceID}", read(s, "ConversationContextViewResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		in := agent.ContextSource{Type: r.PathValue("resourceType"), ID: r.PathValue("resourceID"), MaxChars: 4000}
		var err error
		in.ContextSecurityVersion, err = strconv.ParseInt(r.URL.Query().Get("context_security_version"), 10, 64)
		if err != nil {
			return nil, core.Invalid("context_security_version", "请指定当前会话安全版本")
		}
		for name, field := range map[string]*int{"offset": &in.Offset, "max_chars": &in.MaxChars} {
			if value := r.URL.Query().Get(name); value != "" {
				*field, err = strconv.Atoi(value)
				if err != nil {
					return nil, core.Invalid(name, "字符范围需为整数")
				}
			}
		}
		if value := r.URL.Query().Get("version_id"); value != "" {
			in.VersionID = &value
		}
		return service.Context(ctx, a, r.PathValue("id"), in)
	}))
}
