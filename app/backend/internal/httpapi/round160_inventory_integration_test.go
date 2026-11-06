package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	status "copypaste.hk/platform/internal/servicestatus"
	"copypaste.hk/platform/internal/workbench"
	"net/url"
	"testing"
)

func TestRound160CompleteWorkInventory(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	operationsRole(t, f, f.alice, "status_operator")
	operationsRole(t, f, f.bob, "status_reader")
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = f.login("bob", "android").AccessToken
	project := f.createProject("Round160 complete work", []string{f.bob})
	service := operationsService(t, f, &project.ID)
	var assigned []status.Incident
	for range 7 {
		assigned = append(assigned, operationsIncident(t, f, f.alice, service.ID))
	}
	operationsIncident(t, f, f.bob, service.ID)
	read := func(kind, token, cursor string, code int) workbench.Inventory {
		t.Helper()
		path := "/v1/dashboard/work-items/" + kind + "?limit=2"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		r, v := f.request("GET", path, token, "", nil, nil)
		expect(t, r, code, v)
		if code != 200 {
			return workbench.Inventory{}
		}
		return data[workbench.Inventory](t, v)
	}
	t.Run("all nine existing groups have an authorized read route", func(t *testing.T) {
		for _, kind := range []string{"approvals", "contracts", "subscription_periods", "incidents", "file_attention", "resource_attention", "requests_to_revise", "deliverables_to_submit", "deliverables_to_review"} {
			v := read(kind, f.aliceToken, "", 200)
			if v.Kind != kind || v.Scope != "my_readable_inventory" {
				t.Fatal(v)
			}
		}
	})
	first := read("incidents", f.aliceToken, "", 200)
	t.Run("four pages preserve all seven current assigned rows without acknowledging work", func(t *testing.T) {
		seen := map[string]bool{}
		v := first
		for {
			if v.Count != 7 || v.BasisSHA256 != first.BasisSHA256 {
				t.Fatal(v)
			}
			for _, item := range v.Items {
				if seen[item.ID] {
					t.Fatal("duplicate", item.ID)
				}
				seen[item.ID] = true
				if item.RevisionID == nil {
					t.Fatal(item)
				}
			}
			if v.NextCursor == nil {
				break
			}
			v = read("incidents", f.aliceToken, *v.NextCursor, 200)
		}
		if len(seen) != 7 || operationsGroup(t, f, f.aliceToken, "incidents").Count != 7 {
			t.Fatal(seen)
		}
	})
	t.Run("cursor is bound to exact kind session user and full current set", func(t *testing.T) {
		if first.NextCursor == nil {
			t.Fatal(first)
		}
		read("approvals", f.aliceToken, *first.NextCursor, 409)
		read("incidents", f.bobToken, *first.NextCursor, 409)
		if read("incidents", f.bobToken, "", 200).Count != 1 {
			t.Fatal("other owner's rows leaked")
		}
		changed := assigned[6]
		r, v := f.request("PATCH", "/v1/incidents/"+changed.ID, f.aliceToken, core.NewID(), status.IncidentPatch{ExpectedVersion: changed.Version, State: "investigating", TimelineNote: "Round160 whole inventory changed", Reason: "explicit human progress"}, nil)
		expect(t, r, 200, v)
		read("incidents", f.aliceToken, *first.NextCursor, 409)
	})
	t.Run("unsupported and actor override filters fail closed", func(t *testing.T) {
		read("not-a-group", f.aliceToken, "", 422)
		for _, suffix := range []string{"?limit=0", "?limit=51", "?limit=2&limit=2", "?owner_id=" + f.bob} {
			r, v := f.request("GET", "/v1/dashboard/work-items/incidents"+suffix, f.aliceToken, "", nil, nil)
			expect(t, r, 422, v)
		}
	})
	t.Run("ordinary authority revocation suppresses the inventory", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, "DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='status_operator')", f.org, f.alice); err != nil {
			t.Fatal(err)
		}
		v := read("incidents", f.aliceToken, "", 200)
		if v.Count != 0 || len(v.Items) != 0 {
			t.Fatal(v)
		}
	})
}
