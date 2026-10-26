package httpapi

import (
	"context"
	"net/http"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

func (s *Server) authRoutes(mux *http.ServeMux) {
	s.accountSecurityRoutes(mux)
	mux.HandleFunc("POST /v1/auth/mfa/step-up", func(w http.ResponseWriter, r *http.Request) {
		a, err := s.actor(r, true)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		input, _, err := decode[identity.StepUpInput](s, w, r, "MFAStepUpInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		result, err := s.Identity.StepUp(r.Context(), a, input, requestID(r))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.respond(w, r, 200, "MFAStepUpResultResponse", result)
	})
	mux.HandleFunc("POST /v1/auth/mfa/recover", func(w http.ResponseWriter, r *http.Request) {
		input, _, err := decode[identity.MFAInput](s, w, r, "MFARecoveryInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		result, err := s.Identity.RecoverMFA(r.Context(), input, requestID(r))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.loginResponse(w, r, result)
	})
	mux.HandleFunc("POST /v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		input, _, err := decode[identity.LoginInput](s, w, r, "LoginInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if input.Device == "web" && !s.Config.AllowedOrigins[r.Header.Get("Origin")] {
			s.fail(w, r, core.Forbidden())
			return
		}
		result, err := s.Identity.Login(r.Context(), input, clientIP(r), requestID(r))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.loginResponse(w, r, result)
	})
	mux.HandleFunc("POST /v1/auth/mfa/enroll", func(w http.ResponseWriter, r *http.Request) {
		input, _, err := decode[struct {
			Challenge string `json:"challenge"`
		}](s, w, r, "MFAEnrollInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		result, err := s.Identity.Enroll(r.Context(), input.Challenge)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.respond(w, r, 200, "LoginResultResponse", result)
	})
	mux.HandleFunc("POST /v1/auth/mfa/verify", func(w http.ResponseWriter, r *http.Request) {
		input, _, err := decode[identity.MFAInput](s, w, r, "MFAInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		result, err := s.Identity.VerifyMFA(r.Context(), input, requestID(r))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.loginResponse(w, r, result)
	})
	mux.HandleFunc("POST /v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		input, _, err := decode[struct {
			Token string `json:"refresh_token"`
		}](s, w, r, "RefreshInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		token := input.Token
		cookieAuth := false
		if cookie, cookieErr := r.Cookie("cp_refresh"); cookieErr == nil {
			if token != "" {
				s.fail(w, r, core.Fail(400, "MIXED_AUTHENTICATION", "一次请求只能使用一种会话凭证"))
				return
			}
			token = cookie.Value
			cookieAuth = true
			if !s.Config.AllowedOrigins[r.Header.Get("Origin")] {
				s.fail(w, r, core.Forbidden())
				return
			}
		}
		result, err := s.Identity.Refresh(r.Context(), token, r.Header.Get("X-CSRF-Token"), cookieAuth, requestID(r))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if (cookieAuth && result.Device != "web") || (!cookieAuth && result.Device == "web") {
			s.fail(w, r, core.Forbidden())
			return
		}
		s.loginResponse(w, r, result)
	})
	mux.HandleFunc("POST /v1/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		a, err := s.actor(r, true)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if _, _, err = decode[struct{}](s, w, r, "EmptyInput"); err != nil {
			s.fail(w, r, err)
			return
		}
		if err = s.Identity.Logout(r.Context(), a, requestID(r)); err != nil {
			s.fail(w, r, err)
			return
		}
		s.clearSessionCookies(w)
		s.respond(w, r, 200, "LogoutResultResponse", map[string]bool{"logged_out": true})
	})
	mux.HandleFunc("GET /v1/me", read(s, "MeResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) { return s.Identity.Me(ctx, a) }))
	mux.HandleFunc("GET /v1/capabilities", read(s, "CapabilityResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		size := "0"
		if a.Channel == "web" || a.Channel == "android" {
			size = "120000000000"
		}
		channel := a.Channel
		if channel == "wechat" {
			channel = "weixin"
		}
		return map[string]any{"platform": channel, "max_upload_bytes": size, "agent_enabled": false}, nil
	}))
}
func (s *Server) loginResponse(w http.ResponseWriter, r *http.Request, result identity.LoginResult) {
	if result.Step == "authenticated" && result.Device == "web" {
		http.SetCookie(w, &http.Cookie{Name: "cp_session", Value: result.AccessToken, Path: "/v1", HttpOnly: true, Secure: s.Config.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: int(s.Config.AccessTTL.Seconds()), Expires: result.ExpiresAt})
		http.SetCookie(w, &http.Cookie{Name: "cp_refresh", Value: result.RefreshToken, Path: "/v1/auth", HttpOnly: true, Secure: s.Config.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: int(s.Config.RefreshTTL.Seconds()), Expires: time.Now().Add(s.Config.RefreshTTL)})
		http.SetCookie(w, &http.Cookie{Name: "cp_csrf", Value: result.CSRFToken, Path: "/", Secure: s.Config.SecureCookies, SameSite: http.SameSiteLaxMode, MaxAge: int(s.Config.RefreshTTL.Seconds())})
		result.AccessToken = ""
		result.RefreshToken = ""
	}
	s.respond(w, r, 200, "LoginResultResponse", result)
}
