package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"net/http"
)

// Preparation validates and normalizes typed human input. It neither saves an
// ordinary draft nor grants a model permission; RunCreate is the later consent.
func prepareAgentInput[T any](s *Server, inputSchema, response string, fn func(context.Context, core.Actor, *http.Request, T) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, e := s.actor(r, true)
		if e != nil {
			s.denyAudit(r, a, e)
			s.fail(w, r, e)
			return
		}
		if len(r.URL.Query()) != 0 {
			s.fail(w, r, core.Invalid("filters", "不接受额外条件"))
			return
		}
		in, _, e := decode[T](s, w, r, inputSchema)
		if e != nil {
			s.denyAudit(r, a, e)
			s.fail(w, r, e)
			return
		}
		out, e := fn(r.Context(), a, r, in)
		if e != nil {
			s.denyAudit(r, a, e)
			s.fail(w, r, e)
			return
		}
		s.respond(w, r, http.StatusOK, response, out)
	}
}
func (s *Server) agentBusinessRevisionRoutes(mux *http.ServeMux) {
	service := &agent.Service{DB: s.DB}
	mux.HandleFunc("GET /v1/conversations/{id}/business-revisions/{kind}/{resource}", read(s, "AgentBusinessRevisionBasisResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		if len(r.URL.Query()) != 0 {
			return nil, core.Invalid("filters", "不接受额外条件")
		}
		return service.BusinessRevisionBasis(ctx, a, r.PathValue("id"), r.PathValue("kind"), r.PathValue("resource"))
	}))
	mux.HandleFunc("POST /v1/conversations/{id}/business-revisions/prepare", prepareAgentInput(s, "AgentBusinessRevisionInput", "PreparedBusinessRevisionResponse", func(ctx context.Context, a core.Actor, r *http.Request, in agent.BusinessRevisionInput) (any, error) {
		return service.PrepareBusinessRevision(ctx, a, r.PathValue("id"), in)
	}))
	mux.HandleFunc("POST /v1/conversations/{id}/business-drafts/prepare", prepareAgentInput(s, "AgentBusinessDraftInput", "AgentBusinessDraftInputResponse", func(ctx context.Context, a core.Actor, r *http.Request, in agent.BusinessDraftInput) (any, error) {
		return service.PrepareBusinessDraft(ctx, a, r.PathValue("id"), in)
	}))
}
