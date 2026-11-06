package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/notification"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
	status "copypaste.hk/platform/internal/servicestatus"
	"encoding/json"
	"errors"
	"testing"
)

func round168Revoked(t *testing.T, err error) {
	t.Helper()
	var e *core.Error
	if !errors.As(err, &e) || e.Code != "AUTHORIZATION_CHANGED" {
		t.Fatalf("captured human original session must be rejected: %v", err)
	}
}
func TestRound168ReadSessionBoundary(t *testing.T) {
	for _, mode := range []string{"logout", "family"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			ctx := context.Background()
			p := f.createProject("168 bounded human sources", []string{f.bob})
			for _, role := range []string{"agent_user", "status_operator"} {
				if _, err := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, f.alice, role); err != nil {
					t.Fatal(err)
				}
			}
			f.aliceToken = memberMFAToken(f, "alice", "android")
			rec, res := f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), map[string]any{"title": "168 original notice", "priority": "normal", "assignee_id": f.alice, "review_required": false}, nil)
			expect(t, rec, 201, res)
			task := data[project.Task](t, res)
			drainNotifications(t, f)
			notice := notificationFor(t, f, f.aliceToken, task.ID)
			if notice == nil {
				t.Fatal("actual notice missing")
			}
			rec, res = f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: &p.ID, Title: "168 private current session", MemberIDs: []string{}}, nil)
			expect(t, rec, 201, res)
			conversation := data[agent.Conversation](t, res)
			rec, res = f.request("POST", "/v1/services", f.aliceToken, core.NewID(), status.Create{ProjectID: &p.ID, Classification: "project_restricted", Content: status.Content{Name: "168 local-only service", OwnerID: f.alice, HostLabel: "synthetic", EndpointLabel: "no network probe", Description: "original-session source"}, Reason: "synthetic source boundary"}, nil)
			expect(t, rec, 201, res)
			service := data[status.Record](t, res)
			actor, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
			if err != nil {
				t.Fatal(err)
			}
			commands := platform.Commands{DB: f.api.DB}
			pager := platform.Pager{Key: f.api.Config.CursorKey}
			notes := &notification.Service{DB: f.api.DB, Commands: commands, Pager: pager}
			secretary := &agent.Service{DB: f.api.DB, Commands: commands, Pager: pager}
			services := &status.Service{DB: f.api.DB, Commands: commands, Pager: pager}
			if _, err = notes.Get(ctx, actor, notice.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = secretary.Get(ctx, actor, conversation.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = services.Get(ctx, actor, service.ID); err != nil {
				t.Fatal(err)
			}
			if mode == "logout" {
				rec, res = f.request("POST", "/v1/auth/logout", f.aliceToken, core.NewID(), map[string]any{}, nil)
			} else {
				var family string
				if err = f.owner.QueryRow(ctx, "SELECT family_id FROM sessions WHERE id=$1", actor.SessionID).Scan(&family); err != nil {
					t.Fatal(err)
				}
				rec, res = f.request("POST", "/v1/me/sessions/"+family+"/revoke", f.aliceToken, core.NewID(), identity.SessionRevokeInput{Reason: "synthetic current family revoke"}, nil)
			}
			expect(t, rec, 200, res)
			checks := []func() error{
				func() error { _, e := notes.List(ctx, actor, notification.Filter{Limit: 20}); return e }, func() error { _, e := notes.Summary(ctx, actor); return e }, func() error { _, e := notes.Get(ctx, actor, notice.ID); return e }, func() error { _, e := notes.Failures(ctx, actor, "", 20); return e },
				func() error { _, e := secretary.Get(ctx, actor, conversation.ID); return e }, func() error { _, e := secretary.List(ctx, actor, agent.Filter{Limit: 20}); return e }, func() error { _, e := secretary.Messages(ctx, actor, conversation.ID, "", 20); return e }, func() error { _, e := secretary.ListPlans(ctx, actor, conversation.ID); return e }, func() error { _, e := secretary.GetRun(ctx, actor, core.NewID()); return e },
				func() error { _, e := services.Get(ctx, actor, service.ID); return e }, func() error { _, e := services.List(ctx, actor, status.Filter{Limit: 20}); return e }, func() error { _, e := services.Observations(ctx, actor, service.ID, "", 20); return e }, func() error { _, e := services.MaintenanceWindows(ctx, actor, service.ID, "", 20); return e }, func() error { _, e := services.Incidents(ctx, actor, "", "", 20); return e }, func() error { _, e := services.GetIncident(ctx, actor, core.NewID()); return e },
			}
			for i, check := range checks {
				t.Run("captured_read_"+string(rune('A'+i)), func(t *testing.T) { round168Revoked(t, check()) })
			}
			input := notification.ReadInput{DeliverySeq: notice.DeliverySeq}
			body, _ := json.Marshal(input)
			cmd := core.Command{Actor: actor, Key: core.NewID(), RequestID: core.NewID(), Route: "POST /v1/notifications/" + notice.ID + "/read", Body: body}
			_, err = notes.MarkRead(ctx, cmd, notice.ID, input)
			round168Revoked(t, err)
			cancel := agent.RunCancel{ExpectedVersion: 1, Action: "cancel", Reason: "captured session cannot cancel"}
			cmd.Body, _ = json.Marshal(cancel)
			cmd.Key = core.NewID()
			cmd.Route = "POST /v1/runs/" + core.NewID() + "/cancel"
			_, err = secretary.CancelRun(ctx, cmd, core.NewID(), cancel)
			round168Revoked(t, err)
		})
	}
}
