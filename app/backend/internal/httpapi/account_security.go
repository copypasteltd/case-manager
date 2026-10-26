package httpapi

import (
	"context"
	"net/http"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

func (s *Server) accountSecurityRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/me/sessions", read(s, "DeviceSessionListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Identity.Sessions(ctx, a, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/me/sessions/{id}/revoke", command(s, "SessionRevokeInput", "SessionRevocationResponse", func(ctx context.Context, c core.Command, r *http.Request, in identity.SessionRevokeInput) (core.Result, error) {
		return s.Identity.RevokeSession(ctx, c, r.PathValue("id"), in)
	}, func(w http.ResponseWriter, r *http.Request, a core.Actor) {
		if a.Channel == "web" {
			family, err := s.Identity.CurrentFamily(r.Context(), a)
			if err == nil && family == r.PathValue("id") {
				s.clearSessionCookies(w)
			}
		}
	}))
	// These two commands can invalidate the requesting browser itself. The
	// client clears its local state after a confirmed result; every stale
	// access/refresh credential is already rejected in the database.
	mux.HandleFunc("POST /v1/me/password", command(s, "PasswordChangeInput", "PasswordChangedResponse", func(ctx context.Context, c core.Command, r *http.Request, in identity.PasswordChangeInput) (core.Result, error) {
		return s.Identity.ChangePassword(ctx, c, in)
	}, func(w http.ResponseWriter, r *http.Request, a core.Actor) {
		if a.Channel == "web" {
			s.clearSessionCookies(w)
		}
	}))
	mux.HandleFunc("POST /v1/users/{id}/password-resets", command(s, "PasswordResetInput", "PasswordResetReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, in identity.PasswordResetInput) (core.Result, error) {
		result, err := s.Identity.IssuePasswordReset(ctx, c, r.PathValue("id"), in)
		if err != nil {
			return result, err
		}
		return s.Identity.DecoratePasswordReset(ctx, c.Actor, result)
	}))
	mux.HandleFunc("POST /v1/auth/password/reset", func(w http.ResponseWriter, r *http.Request) {
		in, _, err := decode[identity.CompletePasswordResetInput](s, w, r, "CompletePasswordResetInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err = s.Identity.CompletePasswordReset(r.Context(), in, clientIP(r), requestID(r)); err != nil {
			s.fail(w, r, err)
			return
		}
		s.respond(w, r, 200, "PasswordChangedResponse", identity.PasswordChanged{Changed: true})
	})
}

// Clear public CSRF and HttpOnly credentials without trying a second mutation
// through the now-revoked session. Reused by the existing logout endpoint.
func (s *Server) clearSessionCookies(w http.ResponseWriter) {
	for _, item := range []struct{ name, path string }{{"cp_session", "/v1"}, {"cp_refresh", "/v1/auth"}, {"cp_csrf", "/"}} {
		http.SetCookie(w, &http.Cookie{Name: item.name, Value: "", Path: item.path, HttpOnly: item.name != "cp_csrf", Secure: s.Config.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	}
}
