package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"encoding/json"
	"net/http"
)

func (s *Server) administrationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/classification-policies", read(s, "ClassificationPolicyListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Identity.ClassificationPolicies(ctx, a)
	}))
	mux.HandleFunc("POST /v1/classification-policies", command(s, "ClassificationPolicyInput", "ClassificationPolicyResponse", func(ctx context.Context, c core.Command, r *http.Request, in identity.ClassificationPolicyInput) (core.Result, error) {
		return s.Identity.PublishClassificationPolicy(ctx, c, in)
	}))
	mux.HandleFunc("GET /v1/classification-policies/{level}/history", read(s, "ClassificationPolicyListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Identity.ClassificationPolicyHistory(ctx, a, r.PathValue("level"), f.Cursor, f.Limit)
	}))
	mux.HandleFunc("GET /v1/users/{id}/invitation", read(s, "InvitationReceiptResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		return s.Identity.Invitation(ctx, a, r.PathValue("id"))
	}))
	mux.HandleFunc("POST /v1/auth/activate", func(w http.ResponseWriter, r *http.Request) {
		input, _, err := decode[identity.ActivateInput](s, w, r, "ActivateInput")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if err = s.Identity.Activate(r.Context(), input, clientIP(r), requestID(r)); err != nil {
			s.fail(w, r, err)
			return
		}
		s.respond(w, r, 200, "ActivationResultResponse", map[string]bool{"activated": true})
	})
	mux.HandleFunc("GET /v1/users", read(s, "UserListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Identity.Users(ctx, a, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/users", command(s, "UserInvite", "InvitationReceiptResponse", func(ctx context.Context, c core.Command, r *http.Request, input identity.InviteInput) (core.Result, error) {
		result, err := s.Identity.Invite(ctx, c, input)
		if err != nil {
			return result, err
		}
		body, err := json.Marshal(result.Data)
		if err != nil {
			return result, err
		}
		var receipt identity.InvitationReceipt
		if err = json.Unmarshal(body, &receipt); err != nil {
			return result, err
		}
		result.Data, err = s.Identity.DecorateInvitation(ctx, c.Actor, receipt)
		return result, err
	}))
	mux.HandleFunc("POST /v1/users/{id}/disable", command(s, "Transition", "UserResponse", func(ctx context.Context, c core.Command, r *http.Request, input identity.DisableInput) (core.Result, error) {
		return s.Identity.Disable(ctx, c, r.PathValue("id"), input)
	}))
	mux.HandleFunc("GET /v1/roles", read(s, "RoleListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) { return s.Identity.Roles(ctx, a) }))
	mux.HandleFunc("GET /v1/role-bindings", read(s, "RoleBindingListResponse", func(ctx context.Context, a core.Actor, r *http.Request) (any, error) {
		f, err := filter(r)
		if err != nil {
			return nil, err
		}
		return s.Identity.Bindings(ctx, a, f.Cursor, f.Limit)
	}))
	mux.HandleFunc("POST /v1/role-bindings", command(s, "RoleBindingCreate", "RoleBindingResponse", func(ctx context.Context, c core.Command, r *http.Request, input identity.BindingCreate) (core.Result, error) {
		return s.Identity.CreateBinding(ctx, c, input)
	}))
	mux.HandleFunc("POST /v1/role-bindings/{id}/revoke", command(s, "RoleBindingRevoke", "RevocationResultResponse", func(ctx context.Context, c core.Command, r *http.Request, input identity.RevokeInput) (core.Result, error) {
		return s.Identity.RevokeBinding(ctx, c, r.PathValue("id"), input)
	}))
}
