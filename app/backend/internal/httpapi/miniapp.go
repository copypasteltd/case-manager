package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"net/http"
)

func (s *Server) miniappRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/auth/miniapps", func(w http.ResponseWriter, r *http.Request) {
		s.respond(w, r, 200, "MiniappProfilesResponse", s.Identity.MiniappProfiles())
	})
	mux.HandleFunc("POST /v1/auth/miniapps/login", func(w http.ResponseWriter, r *http.Request) {
		in, _, err := decode[identity.MiniappCodeInput](s, w, r, "MiniappCodeInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		result, err := s.Identity.MiniappLogin(r.Context(), in, clientIP(r), requestID(r))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.loginResponse(w, r, result)
	})
	mux.HandleFunc("POST /v1/me/miniapp-binding-proofs", func(w http.ResponseWriter, r *http.Request) {
		a, err := s.actor(r, true)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		in, _, err := decode[identity.MiniappProofInput](s, w, r, "MiniappProofInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		result, err := s.Identity.CreateMiniappProof(r.Context(), a, in, clientIP(r), requestID(r))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.respond(w, r, 201, "MiniappProofResponse", result)
	})
	mux.HandleFunc("GET /v1/me/miniapp-bindings", read(s, "MiniappBindingListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Identity.MiniappBindings(ctx, a, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/me/miniapp-bindings", command(s, "MiniappBindInput", "MiniappBindingResponse", func(ctx context.Context, c core.Command, r *http.Request, in identity.MiniappBindInput) (core.Result, error) {
		return s.Identity.BindMiniapp(ctx, c, in)
	}))
	mux.HandleFunc("POST /v1/me/miniapp-bindings/{id}/revoke", command(s, "MiniappRevokeInput", "MiniappBindingResponse", func(ctx context.Context, c core.Command, r *http.Request, in identity.MiniappRevokeInput) (core.Result, error) {
		return s.Identity.RevokeMiniapp(ctx, c, r.PathValue("id"), in)
	}))
}
