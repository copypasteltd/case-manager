package httpapi

import (
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
	"copypaste.hk/platform/internal/workbench"
	"net/url"
	"testing"
	"time"
)

func TestRound161TaskCollaboration(t *testing.T) {
	f := setup(t)
	p := f.createProject("Complete collaboration inventory", []string{f.bob})
	create := func(ids []string, code int) project.Task {
		r, v := f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), project.TaskCreate{Title: "Exact shared task", Priority: "normal", ParticipantIDs: ids}, nil)
		expect(t, r, code, v)
		if code != 201 {
			return project.Task{}
		}
		return data[project.Task](t, v)
	}
	first := create([]string{f.bob}, 201)
	create([]string{f.bob, f.bob}, 422)
	create([]string{f.member}, 422)
	for range 5 {
		create([]string{f.bob}, 201)
	}
	read := func(kind, cursor, token string, code int) workbench.Inventory {
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
	t.Run("complete collaboration and unassigned pages do not grant access", func(t *testing.T) {
		seen := map[string]bool{}
		v := read("participating_tasks", "", f.bobToken, 200)
		basis := v.BasisSHA256
		for {
			if v.Count != 6 || v.BasisSHA256 != basis {
				t.Fatal(v)
			}
			for _, row := range v.Items {
				if seen[row.ID] || row.ProjectID == nil || *row.ProjectID != p.ID {
					t.Fatal(row)
				}
				seen[row.ID] = true
			}
			if v.NextCursor == nil {
				break
			}
			v = read("participating_tasks", *v.NextCursor, f.bobToken, 200)
		}
		if len(seen) != 6 || read("unassigned_tasks", "", f.aliceToken, 200).Count != 6 || read("participating_tasks", "", f.memberToken, 200).Count != 0 {
			t.Fatal("incomplete or leaked task queue")
		}
	})
	t.Run("explicit replacement keeps immutable version and rejects a stale cursor", func(t *testing.T) {
		old := read("participating_tasks", "", f.bobToken, 200)
		empty := []string{}
		r, v := f.request("PATCH", "/v1/tasks/"+first.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": first.Version, "participant_ids": empty}, nil)
		expect(t, r, 200, v)
		updated := data[project.Task](t, v)
		if updated.Version != first.Version+1 || len(updated.ParticipantIDs) != 0 {
			t.Fatal(updated)
		}
		if old.NextCursor == nil {
			t.Fatal(old)
		}
		read("participating_tasks", *old.NextCursor, f.bobToken, 409)
		r, v = f.request("PATCH", "/v1/tasks/"+first.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": first.Version, "participant_ids": empty}, nil)
		expect(t, r, 409, v)
	})
	t.Run("completion and blocked fields are ordinary current evidence", func(t *testing.T) {
		task := f.createTask(p, "Unassigned completion evidence", false)
		for _, action := range []string{"start", "block", "resume", "complete"} {
			r, v := f.transition(task, action, f.aliceToken)
			expect(t, r, 200, v)
			task = data[project.Task](t, v)
			if action == "block" && (task.BlockedReason == nil || task.ResumeState == nil) {
				t.Fatal(task)
			}
		}
		if task.CompletedAt == nil || task.CompletedAt.After(time.Now().Add(time.Minute)) || task.State != "done" {
			t.Fatal(task)
		}
	})
}
