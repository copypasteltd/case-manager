package httpapi

import (
	"context"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/knowledge"
	"copypaste.hk/platform/internal/platform"
	"copypaste.hk/platform/internal/project"
)

func TestAgentBusinessReadBoundaries(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	projects := f.api.Projects
	posts := &knowledge.Service{DB: f.api.DB, Commands: platform.Commands{DB: f.api.DB}, Pager: platform.Pager{Key: f.api.Config.CursorKey}}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	policy := func(allowed bool) {
		t.Helper()
		exec(`INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version)
            SELECT $1,$2,'internal',$3,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='internal'`, core.NewID(), f.org, allowed)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	}
	actor := func(ai bool) core.Actor {
		t.Helper()
		a, err := f.api.Identity.Authenticate(ctx, f.aliceToken)
		if err != nil {
			t.Fatal(err)
		}
		if ai {
			a.RunID = core.NewID() // Trusted domain identity fixture; no model or task credential is involved.
		}
		return a
	}
	createProject := func(name, classification string) project.Project {
		t.Helper()
		rec, res := f.request("POST", "/v1/projects", f.aliceToken, core.NewID(), map[string]any{"name": name, "scope": "Synthetic AI read boundary", "owner_id": f.alice, "member_ids": []string{f.bob}, "classification": classification}, nil)
		expect(t, rec, 201, res)
		p := data[project.Project](t, res)
		rec, res = f.request("POST", "/v1/projects/"+p.ID+"/transition", f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "action": "activate", "reason": "Synthetic dashboard read boundary"}, nil)
		expect(t, rec, 200, res)
		return data[project.Project](t, res)
	}
	first := createProject("Internal first", "internal")
	createProject("Internal second", "internal")
	restricted := createProject("AI blocked restricted", "project_restricted")
	var internalPost knowledge.Post
	for i, p := range []project.Project{first, first, restricted} {
		rec, res := f.request("POST", "/v1/posts", f.aliceToken, core.NewID(), knowledge.Create{ProjectID: &p.ID, Type: "sop", Title: "Synthetic bounded knowledge", Body: "Only the permitted classification may be searched", Classification: p.Classification, Tags: []string{}, SourceRefs: []knowledge.Source{}}, nil)
		expect(t, rec, 201, res)
		if i == 0 {
			internalPost = data[knowledge.Post](t, res)
		}
	}
	rec, res := f.request("PATCH", "/v1/posts/"+internalPost.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": internalPost.Version, "base_revision_id": internalPost.CurrentRevisionID, "body": "Updated permitted knowledge with immutable history"}, nil)
	expect(t, rec, 200, res)
	internalPost = data[knowledge.Post](t, res)
	var comment knowledge.Comment
	for range 2 {
		rec, res = f.request("POST", "/v1/posts/"+internalPost.ID+"/comments", f.aliceToken, core.NewID(), map[string]any{"body": "Synthetic bounded comment", "expected_context_revision_id": internalPost.CurrentRevisionID}, nil)
		expect(t, rec, 201, res)
		comment = data[knowledge.Comment](t, res)
	}
	rec, res = f.request("PATCH", "/v1/comments/"+comment.ID, f.aliceToken, core.NewID(), map[string]any{"body": "Updated bounded comment", "expected_revision": comment.Revision, "expected_context_revision_id": internalPost.CurrentRevisionID}, nil)
	expect(t, rec, 201, res)
	comment = data[knowledge.Comment](t, res)
	var tasks []project.Task
	for _, p := range []project.Project{first, first, restricted} {
		rec, res := f.request("POST", "/v1/projects/"+p.ID+"/tasks", f.aliceToken, core.NewID(), map[string]any{"title": "Synthetic assigned task", "priority": "normal", "review_required": false, "assignee_id": f.alice, "due_at": "2025-01-01T00:00:00Z"}, nil)
		expect(t, rec, 201, res)
		tasks = append(tasks, data[project.Task](t, res))
	}
	for range 2 {
		rec, res := f.request("POST", "/v1/projects/"+first.ID+"/milestones", f.aliceToken, core.NewID(), map[string]any{"title": "Synthetic milestone", "owner_id": f.alice, "due_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}, nil)
		expect(t, rec, 201, res)
		for _, kind := range []string{"risks", "issues"} {
			rec, res = f.request("POST", "/v1/projects/"+first.ID+"/"+kind, f.aliceToken, core.NewID(), map[string]any{"title": "Synthetic concern", "description": "Read boundary test", "owner_id": f.alice, "severity": "low"}, nil)
			expect(t, rec, 201, res)
		}
	}
	rec, res = f.request("POST", "/v1/tasks/"+tasks[1].ID+"/dependencies", f.aliceToken, core.NewID(), map[string]any{"expected_version": tasks[1].Version, "predecessor_id": tasks[0].ID, "action": "add"}, nil)
	expect(t, rec, 200, res)
	missing := func(err error) {
		t.Helper()
		if err == nil || core.PublicError(err).Status != 404 {
			t.Fatal("expected denied data without a resource disclosure", err)
		}
	}
	checkDenied := func(a core.Actor) {
		t.Helper()
		p, err := projects.List(ctx, a, project.Filter{Limit: 1})
		if err != nil || len(p.Items) != 0 || p.NextCursor != nil {
			t.Fatal("AI-disabled project list or pagination leaked", p, err)
		}
		k, err := posts.List(ctx, a, knowledge.Filter{Limit: 1, Query: "permitted"})
		if err != nil || len(k.Items) != 0 || k.NextCursor != nil {
			t.Fatal("AI-disabled knowledge search did not filter before pagination", k, err)
		}
		d, err := projects.Dashboard(ctx, a)
		if err != nil || d.ActiveProjects != 0 || d.MyOpenTasks != 0 || d.MyOverdueTasks != 0 || len(d.Tasks) != 0 {
			t.Fatal("AI-disabled aggregate leaked", d, err)
		}
		_, err = projects.Get(ctx, a, first.ID)
		missing(err)
		_, err = posts.Get(ctx, a, internalPost.ID)
		missing(err)
		_, err = posts.History(ctx, a, internalPost.ID, "", 1)
		missing(err)
		_, err = posts.Comments(ctx, a, "post", internalPost.ID, knowledge.Filter{})
		missing(err)
		_, err = posts.CommentHistory(ctx, a, comment.ID, knowledge.Filter{})
		missing(err)
		_, err = projects.ListTasks(ctx, a, first.ID, project.Filter{})
		missing(err)
		_, err = projects.Dependencies(ctx, a, tasks[1].ID)
		missing(err)
	}
	t.Run("AI disabled classifications filter lists searches counts and direct reads independently of manual roles", func(t *testing.T) {
		checkDenied(actor(true))
	})
	t.Run("explicitly enabled classification admits only its current authorized project and knowledge data", func(t *testing.T) {
		policy(true)
		a := actor(true)
		p, err := projects.List(ctx, a, project.Filter{})
		if err != nil || len(p.Items) != 2 {
			t.Fatal(p, err)
		}
		for _, item := range p.Items {
			if item.Classification != "internal" {
				t.Fatal("disabled classification leaked", item)
			}
		}
		k, err := posts.List(ctx, a, knowledge.Filter{Query: "permitted"})
		if err != nil || len(k.Items) != 2 {
			t.Fatal(k, err)
		}
		d, err := projects.Dashboard(ctx, a)
		if err != nil || d.ActiveProjects != 2 || d.MyOpenTasks != 2 || d.MyOverdueTasks != 2 || len(d.Tasks) != 2 {
			t.Fatal(d, err)
		}
		tasks, err := projects.ListTasks(ctx, a, first.ID, project.Filter{})
		if err != nil || len(tasks.Items) != 2 {
			t.Fatal(tasks, err)
		}
		milestones, err := projects.ListMilestones(ctx, a, first.ID, project.Filter{})
		if err != nil || len(milestones.Items) != 2 {
			t.Fatal(milestones, err)
		}
		for _, kind := range []string{"risk", "issue"} {
			concerns, err := projects.ListConcerns(ctx, a, first.ID, kind, project.Filter{})
			if err != nil || len(concerns.Items) != 2 {
				t.Fatal(concerns, err)
			}
		}
		if _, err = projects.Dependencies(ctx, a, tasks.Items[0].ID); err != nil {
			t.Fatal(err)
		}
		_, err = projects.Get(ctx, a, restricted.ID)
		missing(err)
	})
	t.Run("project task collaboration and knowledge cursors cannot cross channel user versions or company runs", func(t *testing.T) {
		a := actor(false)
		type query func(core.Actor, string) (*string, error)
		queries := map[string]query{
			"projects": func(a core.Actor, cursor string) (*string, error) {
				p, e := projects.List(ctx, a, project.Filter{Limit: 1, Cursor: cursor})
				return p.NextCursor, e
			},
			"tasks": func(a core.Actor, cursor string) (*string, error) {
				p, e := projects.ListTasks(ctx, a, first.ID, project.Filter{Limit: 1, Cursor: cursor})
				return p.NextCursor, e
			},
			"milestones": func(a core.Actor, cursor string) (*string, error) {
				p, e := projects.ListMilestones(ctx, a, first.ID, project.Filter{Limit: 1, Cursor: cursor})
				return p.NextCursor, e
			},
			"risks": func(a core.Actor, cursor string) (*string, error) {
				p, e := projects.ListConcerns(ctx, a, first.ID, "risk", project.Filter{Limit: 1, Cursor: cursor})
				return p.NextCursor, e
			},
			"issues": func(a core.Actor, cursor string) (*string, error) {
				p, e := projects.ListConcerns(ctx, a, first.ID, "issue", project.Filter{Limit: 1, Cursor: cursor})
				return p.NextCursor, e
			},
			"knowledge": func(a core.Actor, cursor string) (*string, error) {
				p, e := posts.List(ctx, a, knowledge.Filter{Limit: 1, Cursor: cursor})
				return p.NextCursor, e
			},
			"knowledge history": func(a core.Actor, cursor string) (*string, error) {
				p, e := posts.History(ctx, a, internalPost.ID, cursor, 1)
				return p.NextCursor, e
			},
			"comments": func(a core.Actor, cursor string) (*string, error) {
				p, e := posts.Comments(ctx, a, "post", internalPost.ID, knowledge.Filter{Limit: 1, Cursor: cursor})
				return p.NextCursor, e
			},
			"comment history": func(a core.Actor, cursor string) (*string, error) {
				p, e := posts.CommentHistory(ctx, a, comment.ID, knowledge.Filter{Limit: 1, Cursor: cursor})
				return p.NextCursor, e
			},
		}
		for name, read := range queries {
			cursor, err := read(a, "")
			if err != nil || cursor == nil {
				t.Fatal(name, "expected a real page", err)
			}
			if _, err = read(a, *cursor); err != nil {
				t.Fatal(name, "same authority failed", err)
			}
			for _, change := range []func(*core.Actor){func(v *core.Actor) { v.Channel = "web" }, func(v *core.Actor) { v.RunID = core.NewID() }, func(v *core.Actor) { v.AuthVersion++ }, func(v *core.Actor) { v.OrgAuthVersion++ }, func(v *core.Actor) { v.UserID = f.bob }} {
				changed := a
				change(&changed)
				if _, err = read(changed, *cursor); err == nil || core.PublicError(err).Status != 400 {
					t.Fatal(name, "cursor crossed authority boundary", err)
				}
			}
			ai := a
			ai.RunID = core.NewID()
			aiCursor, err := read(ai, "")
			if err != nil || aiCursor == nil {
				t.Fatal(name, "expected an admitted AI page", err)
			}
			ai.RunID = core.NewID()
			if _, err = read(ai, *aiCursor); err == nil || core.PublicError(err).Status != 400 {
				t.Fatal(name, "cursor crossed company runs", err)
			}
		}
	})
	t.Run("withdrawing AI data permission prevents new reads without changing manual content grants", func(t *testing.T) {
		policy(false)
		checkDenied(actor(true))
	})
	t.Run("manual project knowledge and dashboard workflows remain available when AI is disabled", func(t *testing.T) {
		a := actor(false)
		p, err := projects.List(ctx, a, project.Filter{})
		if err != nil || len(p.Items) != 3 {
			t.Fatal(p, err)
		}
		k, err := posts.List(ctx, a, knowledge.Filter{})
		if err != nil || len(k.Items) != 3 {
			t.Fatal(k, err)
		}
		d, err := projects.Dashboard(ctx, a)
		if err != nil || d.ActiveProjects != 3 || d.MyOpenTasks != 3 || len(d.Tasks) != 3 {
			t.Fatal(d, err)
		}
	})
}
