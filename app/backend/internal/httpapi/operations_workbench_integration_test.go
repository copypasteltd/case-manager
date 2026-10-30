package httpapi

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	status "copypaste.hk/platform/internal/servicestatus"
	"copypaste.hk/platform/internal/workbench"
)

func operationsGroup(t *testing.T, f *fixture, token, kind string) workbench.Group {
	t.Helper()
	r, v := f.with(t).request("GET", "/v1/dashboard/business", token, "", nil, nil)
	expect(t, r, 200, v)
	s := data[workbench.Summary](t, v)
	if len(s.Groups) != 6 {
		t.Fatal("incomplete workbench", s)
	}
	// A summary must not leak provider coordinates, error strings or bodies.
	for _, forbidden := range []string{"object_key", "bucket", "last_error", "description", "postmortem"} {
		if strings.Contains(r.Body.String(), `"`+forbidden+`"`) {
			t.Fatal("non-summary field leaked", forbidden)
		}
	}
	for _, g := range s.Groups {
		if g.Kind == kind {
			return g
		}
	}
	t.Fatal("missing group", kind)
	return workbench.Group{}
}

func operationsRole(t *testing.T, f *fixture, user, key string) string {
	t.Helper()
	id := core.NewID()
	result, err := f.owner.Exec(context.Background(), `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
	 SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key=$4`, id, f.org, user, key)
	if err != nil || result.RowsAffected() != 1 {
		t.Fatal("bind synthetic role", err)
	}
	return id
}

func operationsService(t *testing.T, f *fixture, projectID *string) status.Record {
	t.Helper()
	r, v := f.with(t).request("POST", "/v1/services", f.aliceToken, core.NewID(), status.Create{
		Content:   status.Content{Name: "合成工作台服务", OwnerID: f.alice, HostLabel: "synthetic", EndpointLabel: "no network target"},
		ProjectID: projectID, Classification: "project_restricted", Reason: "isolated operations workbench"}, nil)
	expect(t, r, 201, v)
	return data[status.Record](t, v)
}

func operationsIncident(t *testing.T, f *fixture, owner string, serviceIDs ...string) status.Incident {
	t.Helper()
	r, v := f.with(t).request("POST", "/v1/incidents", f.aliceToken, core.NewID(), status.IncidentCreate{
		Title: "合成服务事件 <b>原样标题</b>", Severity: "high", Description: "Private synthetic investigation body",
		OwnerID: owner, ServiceIDs: serviceIDs, DedupKey: core.NewID(), TimelineNote: "Synthetic initial timeline", Reason: "explicit assignment"}, nil)
	expect(t, r, 201, v)
	return data[status.Incident](t, v)
}

func TestOperationsWorkbenchIncidents(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	operator := operationsRole(t, f, f.alice, "status_operator")
	operationsRole(t, f, f.bob, "status_reader")
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.bobToken = f.login("bob", "android").AccessToken
	shared := f.createProject("Synthetic readable services", []string{f.bob})
	hidden := f.createProject("Synthetic hidden service", []string{f.bob})
	service := operationsService(t, f, &shared.ID)
	secret := operationsService(t, f, &hidden.ID)
	var assigned []status.Incident
	for range 7 {
		assigned = append(assigned, operationsIncident(t, f, f.alice, service.ID))
	}
	operationsIncident(t, f, f.bob, service.ID)
	operationsIncident(t, f, f.bob, service.ID, secret.ID)
	if _, err := f.owner.Exec(ctx, "DELETE FROM project_members WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, hidden.ID, f.bob); err != nil {
		t.Fatal(err)
	}
	t.Run("complete assigned count precedes five samples and preserves actual current timeline", func(t *testing.T) {
		g := operationsGroup(t, f, f.aliceToken, "incidents")
		if g.Count != 7 || len(g.Items) != 5 || !g.HasMore {
			t.Fatal(g)
		}
		for _, item := range g.Items {
			r, v := f.with(t).request("GET", "/v1/incidents/"+item.ID+"/timeline", f.aliceToken, "", nil, nil)
			expect(t, r, 200, v)
			entries := data[core.Page[status.IncidentEntry]](t, v).Items
			if len(entries) != 1 || entries[0].ID != item.RevisionID || entries[0].Seq != item.Version || item.DueOn != nil || item.RelatedID != nil {
				t.Fatal("not an exact timeline reference", item, entries)
			}
		}
	})
	t.Run("all affected services must be readable even for the assigned owner", func(t *testing.T) {
		g := operationsGroup(t, f, f.bobToken, "incidents")
		if g.Count != 1 || len(g.Items) != 1 || g.HasMore {
			t.Fatal("hidden service or someone else's assignment leaked", g)
		}
		if operationsGroup(t, f, f.memberToken, "incidents").Count != 0 {
			t.Fatal("unrelated human inherited incidents")
		}
		for _, channel := range []string{"wechat", "douyin", "android"} {
			token := f.login("bob", channel).AccessToken
			if operationsGroup(t, f, token, "incidents").Count != 1 {
				t.Fatal("interactive channel lost the same readable assigned work", channel)
			}
		}
	})
	t.Run("ordinary investigation monitoring and resolution update then remove work", func(t *testing.T) {
		incident := assigned[6]
		for _, state := range []string{"investigating", "monitoring", "resolved"} {
			r, v := f.with(t).request("PATCH", "/v1/incidents/"+incident.ID, f.aliceToken, core.NewID(), status.IncidentPatch{
				ExpectedVersion: incident.Version, State: state, TimelineNote: "Current synthetic progress", Resolution: "Synthetic recovery verified", Reason: "explicit human progress"}, nil)
			expect(t, r, 200, v)
			incident = data[status.Incident](t, v)
			g := operationsGroup(t, f, f.aliceToken, "incidents")
			if state == "resolved" {
				if g.Count != 6 {
					t.Fatal("resolved work remained", g)
				}
				for _, item := range g.Items {
					if item.ID == incident.ID {
						t.Fatal("resolved sample remained")
					}
				}
			} else {
				var timelineID string
				if err := f.owner.QueryRow(ctx, "SELECT id FROM incident_timeline WHERE org_id=$1 AND incident_id=$2 AND seq=$3", f.org, incident.ID, incident.Version).Scan(&timelineID); err != nil {
					t.Fatal(err)
				}
				if g.Count != 7 || g.Items[0].ID != incident.ID || g.Items[0].Version != incident.Version || g.Items[0].State != state || g.Items[0].RevisionID != timelineID {
					t.Fatal("current timeline did not advance", g)
				}
			}
		}
	})
	t.Run("live role and affected project revocations remove counts", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, "DELETE FROM project_members WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, shared.ID, f.bob); err != nil {
			t.Fatal(err)
		}
		if operationsGroup(t, f, f.bobToken, "incidents").Count != 0 {
			t.Fatal("removed project membership leaked a count")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND id=$2", f.org, operator); err != nil {
			t.Fatal(err)
		}
		if operationsGroup(t, f, f.aliceToken, "incidents").Count != 0 {
			t.Fatal("revoked service authority leaked a count")
		}
	})
}

func TestOperationsWorkbenchFiles(t *testing.T) {
	u, committed, primary, secondary, id := committedReplicaFixture(t, 15)
	f := u.f
	ctx := context.Background()
	read := func(t *testing.T) workbench.Group { return operationsGroup(t, f, f.aliceToken, "file_attention") }
	t.Run("a known queued copy stays pending and another reader is not its creator", func(t *testing.T) {
		g := read(t)
		if g.Count != 1 || len(g.Items) != 1 || g.Items[0].RevisionID != committed.Version.ID || g.Items[0].State != "replica_pending" {
			t.Fatal(g)
		}
		if operationsGroup(t, f, f.bobToken, "file_attention").Count != 0 {
			t.Fatal("another reader inherited creator work")
		}
	})
	t.Run("actual corrupt fixture bytes require human retry and full verification before removal", func(t *testing.T) {
		secondary.corrupt = true
		host := replicaHost(u, primary, secondary, 15)
		runReplica(t, host)
		g := read(t)
		if g.Count != 1 || g.Items[0].State != "replica_attention" {
			t.Fatal("actual mismatch not aggregated", g)
		}
		_, version, _ := replicaState(t, u, id)
		r, v := f.with(t).request("POST", "/v1/file-replicas/"+id+"/retry", f.aliceToken, core.NewID(), storage.ReplicaRetry{ExpectedVersion: version, Reason: "Synthetic provider corrected"}, nil)
		expect(t, r, 200, v)
		g = read(t)
		if g.Count != 1 || g.Items[0].State != "replica_pending" {
			t.Fatal("retry click falsely cleared work", g)
		}
		secondary.corrupt = false
		runReplica(t, host)
		if read(t).Count != 0 {
			t.Fatal("verified copy did not clear work")
		}
	})
	t.Run("unavailable current content takes priority and exact active ancestors remain required", func(t *testing.T) {
		if _, err := f.owner.Exec(ctx, "UPDATE file_versions SET availability='missing' WHERE org_id=$1 AND id=$2", f.org, committed.Version.ID); err != nil {
			t.Fatal(err)
		}
		if g := read(t); g.Count != 1 || g.Items[0].State != "file_unavailable" {
			t.Fatal(g)
		}
		if _, err := f.owner.Exec(ctx, "UPDATE folders SET state='trashed' WHERE org_id=$1 AND id=$2", f.org, u.root); err != nil {
			t.Fatal(err)
		}
		if read(t).Count != 0 {
			t.Fatal("inactive ancestor leaked file metadata")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE folders SET state='active' WHERE org_id=$1 AND id=$2", f.org, u.root); err != nil {
			t.Fatal(err)
		}
		if read(t).Count != 1 {
			t.Fatal("restored readable ancestor did not restore work")
		}
		if _, err := f.owner.Exec(ctx, "UPDATE file_versions SET availability='available' WHERE org_id=$1 AND id=$2", f.org, committed.Version.ID); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("a new head excludes old failed replicas and loss of file authority removes counts", func(t *testing.T) {
		current := replacementVersion(t, u, committed)
		if _, err := f.owner.Exec(ctx, "UPDATE file_replicas SET state='failed',updated_at=clock_timestamp() WHERE org_id=$1 AND version_id=$2", f.org, committed.Version.ID); err != nil {
			t.Fatal(err)
		}
		if g := read(t); g.Count != 1 || g.Items[0].RevisionID != current.Version.ID || g.Items[0].State != "replica_pending" {
			t.Fatal("old version replaced current head", g)
		}
		if _, err := f.owner.Exec(ctx, `UPDATE role_bindings b SET revoked_at=clock_timestamp() FROM roles r
		 WHERE b.org_id=$1 AND b.user_id=$2 AND r.id=b.role_id AND r.org_id=b.org_id AND r.key='project_manager'`, f.org, f.alice); err != nil {
			t.Fatal(err)
		}
		if read(t).Count != 0 {
			t.Fatal("revoked file read authority leaked a count")
		}
	})
}

func TestOperationsWorkbenchBrowserFixture(t *testing.T) {
	if os.Getenv("CP_BROWSER_OPERATIONS_WORKBENCH") != "1" {
		t.Skip("explicit owned browser opt-in only")
	}
	u, committed, primary, secondary, replicaID := committedReplicaFixture(t, 15)
	f := u.f
	operationsRole(t, f, f.alice, "status_operator")
	f.aliceToken = memberMFAToken(f, "alice", "android")
	service := operationsService(t, f, committed.File.ProjectID)
	incident := operationsIncident(t, f, f.alice, service.ID)
	secondary.corrupt = true
	host := replicaHost(u, primary, secondary, 15)
	runReplica(t, host)
	if operationsGroup(t, f, f.aliceToken, "file_attention").Count != 1 || operationsGroup(t, f, f.aliceToken, "incidents").Count != 1 {
		t.Fatal("invalid browser business fixture")
	}
	values := map[string]any{"fixture_kind": "human-operations-workbench", "operations_workbench_fixture": true,
		"file_id": committed.File.ID, "file_version_id": committed.Version.ID, "replica_id": replicaID, "incident_id": incident.ID}
	holdBrowserBusinessFixture(t, f, values, func(mux *http.ServeMux, nonce string) {
		mux.HandleFunc("POST /__fixture/operations/verify-copy", func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+nonce {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			// The UI must already have requested the ordinary authorized retry.
			state, _, _ := replicaState(t, u, replicaID)
			if state != "queued" {
				w.WriteHeader(http.StatusConflict)
				return
			}
			secondary.corrupt = false
			if worked, err := host.RunOne(r.Context()); err != nil || !worked {
				t.Error("actual owned copy worker", worked, err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}
