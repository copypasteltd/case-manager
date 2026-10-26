package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/project"
)

// Actual native PostgreSQL, authorization, HTTP contract, transactions and
// command receipts. No cloud/model/platform service is substituted for these.
func TestProjectTemplateManagement(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	admin := f.administratorToken()
	var draft, published, newRevision project.Template
	var initialized project.Project
	content := project.TemplateContent{Name: "Synthetic delivery template", Description: "Reusable neutral structure; no customer material", Schema: project.TemplateSchema{
		Folders: []string{"合同", "Evidence"}, Tasks: []project.TemplateTask{{Title: "Verify scope", Body: "Record the agreed scope.", Priority: "high", ReviewRequired: true}, {Title: "Prepare delivery", Body: "", Priority: "normal"}}, Milestones: []project.TemplateMilestone{{Title: "Delivery checkpoint"}},
	}}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	create := func(t *testing.T, key string) project.Template {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/project-templates", admin, core.NewID(), project.TemplateCreate{TemplateContent: content, Key: key, Reason: "Synthetic reviewed structure"}, nil)
		expect(t, r, 201, v)
		return data[project.Template](t, v)
	}
	get := func(t *testing.T, id, token string) project.Template {
		t.Helper()
		r, v := f.with(t).request("GET", "/v1/project-templates/"+id, token, "", nil, nil)
		expect(t, r, 200, v)
		return data[project.Template](t, v)
	}
	act := func(t *testing.T, x project.Template, action string, status int) project.Template {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/project-templates/"+x.ID+"/"+action, admin, core.NewID(), project.TemplateAction{ExpectedVersion: x.Version, ContentHash: x.ContentHash, Reason: "Explicit synthetic " + action}, nil)
		expect(t, r, status, v)
		if status != 200 && status != 201 {
			return project.Template{}
		}
		return data[project.Template](t, v)
	}
	createProject := func(t *testing.T, x project.Template, status int) project.Project {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/projects", f.aliceToken, core.NewID(), project.Create{Name: "From " + x.Name + " " + core.NewID(), Scope: "Exact template initialization", Type: "general", OwnerID: f.alice, Classification: "project_restricted", TemplateID: &x.ID}, nil)
		expect(t, r, status, v)
		if status != 201 {
			return project.Project{}
		}
		return data[project.Project](t, v)
	}

	t.Run("configuration and creation authority remain separate", func(t *testing.T) {
		r, v := f.with(t).request("POST", "/v1/project-templates", f.aliceToken, core.NewID(), project.TemplateCreate{TemplateContent: content, Key: "unauthorized", Reason: "Must deny"}, nil)
		expect(t, r, 404, v)
		r, v = f.with(t).request("GET", "/v1/project-templates", f.memberToken, "", nil, nil)
		expect(t, r, 404, v)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.admin)
		r, v = f.with(t).request("POST", "/v1/project-templates", admin, core.NewID(), project.TemplateCreate{TemplateContent: content, Key: "stale-mfa", Reason: "Must deny stale proof"}, nil)
		expect(t, r, 403, v)
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.admin)
	})
	t.Run("create durable draft and replay exact command only", func(t *testing.T) {
		input := project.TemplateCreate{TemplateContent: content, Key: "delivery", Reason: "Synthetic initial revision"}
		key := core.NewID()
		r, v := f.with(t).request("POST", "/v1/project-templates", admin, key, input, nil)
		expect(t, r, 201, v)
		draft = data[project.Template](t, v)
		if draft.State != "draft" || draft.Revision != 1 || draft.Version != 1 || draft.CreatorID == nil || draft.PublishedAt != nil || len(draft.ContentHash) != 64 {
			t.Fatal("incorrect draft", draft)
		}
		r, v = f.with(t).request("POST", "/v1/project-templates", admin, key, input, nil)
		expect(t, r, 201, v)
		if data[project.Template](t, v).ID != draft.ID {
			t.Fatal("duplicate idempotent draft")
		}
		r, v = f.with(t).request("POST", "/v1/project-templates", admin, core.NewID(), input, nil)
		expect(t, r, 409, v)
		input.Name = "Different intent"
		r, v = f.with(t).request("POST", "/v1/project-templates", admin, key, input, nil)
		expect(t, r, 409, v)
	})
	t.Run("ordinary creators cannot enumerate drafts or their history", func(t *testing.T) {
		for _, path := range []string{"/v1/project-templates/" + draft.ID, "/v1/project-templates/" + draft.ID + "/history", "/v1/project-templates?state=draft"} {
			r, v := f.with(t).request("GET", path, f.aliceToken, "", nil, nil)
			expect(t, r, 404, v)
		}
		r, v := f.with(t).request("GET", "/v1/project-templates", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[project.Template]](t, v).Items) != 0 {
			t.Fatal("draft leaked")
		}
		createProject(t, draft, 409)
	})
	t.Run("schema validation rejects paths Unicode collisions unknown fields and oversized manifests", func(t *testing.T) {
		for _, folders := range [][]string{{"Evidence", "Ｅｖｉｄｅｎｃｅ"}, {"../escape"}, {"CON"}, {" trailing "}} {
			c := content
			c.Schema = project.TemplateSchema{Folders: folders, Tasks: []project.TemplateTask{}, Milestones: []project.TemplateMilestone{}}
			r, v := f.with(t).request("POST", "/v1/project-templates", admin, core.NewID(), project.TemplateCreate{TemplateContent: c, Key: "invalid", Reason: "Invalid folder test"}, nil)
			expect(t, r, 422, v)
		}
		c := content
		c.Schema = project.TemplateSchema{Folders: []string{}, Milestones: []project.TemplateMilestone{}, Tasks: make([]project.TemplateTask, 201)}
		r, v := f.with(t).request("POST", "/v1/project-templates", admin, core.NewID(), project.TemplateCreate{TemplateContent: c, Key: "invalid", Reason: "Invalid bound"}, nil)
		expect(t, r, 422, v)
		c.Schema.Tasks = make([]project.TemplateTask, 20)
		for i := range c.Schema.Tasks {
			c.Schema.Tasks[i] = project.TemplateTask{Title: "Bounded item", Priority: "normal", Body: strings.Repeat("x", 15000)}
		}
		r, v = f.with(t).request("POST", "/v1/project-templates", admin, core.NewID(), project.TemplateCreate{TemplateContent: c, Key: "invalid", Reason: "Invalid byte bound"}, nil)
		expect(t, r, 422, v)
		raw := map[string]any{"key": "invalid", "name": "Unknown field", "description": "", "schema": map[string]any{"folders": []string{}, "tasks": []any{}, "milestones": []any{}, "shell": "forbidden"}, "reason": "Reject executable metadata"}
		r, v = f.with(t).request("POST", "/v1/project-templates", admin, core.NewID(), raw, nil)
		expect(t, r, 422, v)
	})
	t.Run("draft edits require exact version and retain prior snapshot", func(t *testing.T) {
		patch := project.TemplatePatch{TemplateContent: content, ExpectedVersion: 1, Reason: "Clarify description"}
		patch.Description = "Updated reusable structure"
		r, v := f.with(t).request("PATCH", "/v1/project-templates/"+draft.ID, admin, core.NewID(), patch, nil)
		expect(t, r, 200, v)
		draft = data[project.Template](t, v)
		if draft.Version != 2 {
			t.Fatal("version not advanced")
		}
		r, v = f.with(t).request("PATCH", "/v1/project-templates/"+draft.ID, admin, core.NewID(), patch, nil)
		expect(t, r, 409, v)
		r, v = f.with(t).request("GET", "/v1/project-templates/"+draft.ID+"/history", admin, "", nil, nil)
		expect(t, r, 200, v)
		history := data[core.Page[project.TemplateEvent]](t, v)
		if len(history.Items) != 2 || history.Items[1].Snapshot.Description != content.Description || history.Items[0].Snapshot.ContentHash != draft.ContentHash {
			t.Fatal("history overwritten")
		}
	})
	t.Run("publication requires exact hash and freezes content in PostgreSQL", func(t *testing.T) {
		r, v := f.with(t).request("POST", "/v1/project-templates/"+draft.ID+"/publish", admin, core.NewID(), project.TemplateAction{ExpectedVersion: draft.Version, ContentHash: strings.Repeat("0", 64), Reason: "Reject changed content"}, nil)
		expect(t, r, 409, v)
		published = act(t, draft, "publish", 200)
		if published.PublishedAt == nil || published.PublishedBy == nil || published.ContentHash != draft.ContentHash {
			t.Fatal("publication snapshot differs")
		}
		r, v = f.with(t).request("PATCH", "/v1/project-templates/"+published.ID, admin, core.NewID(), project.TemplatePatch{TemplateContent: content, ExpectedVersion: published.Version, Reason: "Attempt replacement"}, nil)
		expect(t, r, 409, v)
		if _, err := f.owner.Exec(ctx, "UPDATE project_templates SET name='Replaced',version=version+1 WHERE id=$1", published.ID); err == nil {
			t.Fatal("database allowed published replacement")
		}
		if _, err := f.api.DB.Exec(ctx, "DELETE FROM project_templates WHERE id=$1", published.ID); err == nil {
			t.Fatal("app could delete template")
		}
		if _, err := f.api.DB.Exec(ctx, "UPDATE project_template_events SET reason='Replaced' WHERE template_id=$1", published.ID); err == nil {
			t.Fatal("app could overwrite history")
		}
	})
	t.Run("published revision is selectable and initializes all logical content atomically", func(t *testing.T) {
		read := get(t, published.ID, f.aliceToken)
		if read.ContentHash != published.ContentHash {
			t.Fatal("wrong published content")
		}
		initialized = createProject(t, published, 201)
		if initialized.TemplateVersion == nil || *initialized.TemplateVersion != 1 || initialized.InitializationState != "complete" {
			t.Fatal("missing exact revision")
		}
		var tasks, milestones, folders int
		if err := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM tasks WHERE project_id=$1),(SELECT count(*) FROM milestones WHERE project_id=$1 AND owner_id=$2),(SELECT count(*) FROM folders WHERE project_id=$1)`, initialized.ID, f.alice).Scan(&tasks, &milestones, &folders); err != nil {
			t.Fatal(err)
		}
		if tasks != 2 || milestones != 1 || folders != 5 {
			t.Fatal("incomplete initialization", tasks, milestones, folders)
		}
	})
	t.Run("new revision has independent identity and cannot alter existing projects", func(t *testing.T) {
		newRevision = act(t, published, "revise", 201)
		if newRevision.ID == published.ID || newRevision.Revision != 2 || newRevision.Version != 1 || newRevision.BasedOnID == nil || *newRevision.BasedOnID != published.ID {
			t.Fatal("moving template identity")
		}
		act(t, published, "revise", 409)
		c := content
		c.Name = "Synthetic delivery second revision"
		c.Schema = project.TemplateSchema{Folders: []string{"New evidence"}, Tasks: []project.TemplateTask{{Title: "New revision task", Priority: "urgent"}}, Milestones: []project.TemplateMilestone{}}
		r, v := f.with(t).request("PATCH", "/v1/project-templates/"+newRevision.ID, admin, core.NewID(), project.TemplatePatch{TemplateContent: c, ExpectedVersion: 1, Reason: "New independent structure"}, nil)
		expect(t, r, 200, v)
		newRevision = data[project.Template](t, v)
		newRevision = act(t, newRevision, "publish", 200)
		createProject(t, newRevision, 201)
		old := get(t, published.ID, admin)
		if old.ContentHash != published.ContentHash {
			t.Fatal("old template changed")
		}
		r, v = f.with(t).request("GET", "/v1/projects/"+initialized.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		p := data[project.Project](t, v)
		if p.TemplateID == nil || *p.TemplateID != published.ID || *p.TemplateVersion != 1 {
			t.Fatal("old project silently upgraded")
		}
	})
	t.Run("retirement prevents new initialization and preserves existing content", func(t *testing.T) {
		published = act(t, published, "retire", 200)
		createProject(t, published, 409)
		r, v := f.with(t).request("GET", "/v1/project-templates/"+published.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 404, v)
		r, v = f.with(t).request("GET", "/v1/projects/"+initialized.ID+"/tasks", f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		if len(data[core.Page[project.Task]](t, v).Items) != 2 {
			t.Fatal("retirement removed project content")
		}
		get(t, published.ID, admin)
		act(t, published, "publish", 409)
	})
	t.Run("family and history pagination bind cursor to scope", func(t *testing.T) {
		r, v := f.with(t).request("GET", "/v1/project-templates?key=delivery&limit=1", admin, "", nil, nil)
		expect(t, r, 200, v)
		page := data[core.Page[project.Template]](t, v)
		if page.NextCursor == nil {
			t.Fatal("missing family cursor")
		}
		r, v = f.with(t).request("GET", "/v1/project-templates?key=delivery&limit=1&cursor="+*page.NextCursor, admin, "", nil, nil)
		expect(t, r, 200, v)
		if data[core.Page[project.Template]](t, v).Items[0].ID == page.Items[0].ID {
			t.Fatal("duplicate template page")
		}
		r, v = f.with(t).request("GET", "/v1/project-templates?key=other&limit=1&cursor="+*page.NextCursor, admin, "", nil, nil)
		expect(t, r, 400, v)
		r, v = f.with(t).request("GET", "/v1/project-templates/"+published.ID+"/history?limit=1", admin, "", nil, nil)
		expect(t, r, 200, v)
		events := data[core.Page[project.TemplateEvent]](t, v)
		if events.NextCursor == nil {
			t.Fatal("missing history cursor")
		}
		r, v = f.with(t).request("GET", "/v1/project-templates/"+newRevision.ID+"/history?limit=1&cursor="+*events.NextCursor, admin, "", nil, nil)
		expect(t, r, 400, v)
	})
	t.Run("concurrent revision creation permits only one current draft", func(t *testing.T) {
		var wait sync.WaitGroup
		codes := make(chan int, 2)
		for range 2 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				r, _ := f.with(t).request("POST", "/v1/project-templates/"+newRevision.ID+"/revise", admin, core.NewID(), project.TemplateAction{ExpectedVersion: newRevision.Version, ContentHash: newRevision.ContentHash, Reason: "Concurrent draft creation"}, nil)
				codes <- r.Code
			}()
		}
		wait.Wait()
		close(codes)
		successes, conflicts := 0, 0
		for c := range codes {
			if c == 201 {
				successes++
			} else if c == 409 {
				conflicts++
			} else {
				t.Fatal("unexpected concurrent result", c)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatal("duplicate draft", successes, conflicts)
		}
	})
	t.Run("history failure rolls back draft and idempotency receipt together", func(t *testing.T) {
		exec("CREATE FUNCTION fail_template_fixture_event() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'controlled history failure'; END;$$")
		exec("CREATE TRIGGER fail_template_fixture BEFORE INSERT ON project_template_events FOR EACH ROW EXECUTE FUNCTION fail_template_fixture_event()")
		key := core.NewID()
		in := project.TemplateCreate{TemplateContent: content, Key: "atomic-history", Reason: "Atomic rollback"}
		r, v := f.with(t).request("POST", "/v1/project-templates", admin, key, in, nil)
		expect(t, r, 500, v)
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM project_templates WHERE org_id=$1 AND key='atomic-history'", f.org).Scan(&n); err != nil || n != 0 {
			t.Fatal("partial draft survived", n, err)
		}
		exec("DROP TRIGGER fail_template_fixture ON project_template_events")
		exec("DROP FUNCTION fail_template_fixture_event()")
		r, v = f.with(t).request("POST", "/v1/project-templates", admin, key, in, nil)
		expect(t, r, 201, v)
	})
	t.Run("managed drafts cannot bypass exact history using SQL", func(t *testing.T) {
		x := create(t, "sql-guard")
		if _, err := f.api.DB.Exec(ctx, "UPDATE project_templates SET name='Silent replacement',version=version+1 WHERE id=$1", x.ID); err == nil {
			t.Fatal("managed update without history survived")
		}
		after := get(t, x.ID, admin)
		if after.Version != x.Version || after.Name != x.Name {
			t.Fatal("failed guard changed source")
		}
		if _, err := f.api.DB.Exec(ctx, `INSERT INTO project_templates(id,org_id,key,name,revision,state,task_and_folder_schema) VALUES(gen_random_uuid(),$1,'unattributed','Bypass',1,'published','{"folders":[],"tasks":[],"milestones":[]}')`, f.org); err == nil {
			t.Fatal("application bypassed draft and attribution")
		}
	})
	t.Run("failed task initialization leaves no project or logical directories", func(t *testing.T) {
		exec("CREATE FUNCTION fail_template_fixture_task() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'controlled task failure'; END;$$")
		exec("CREATE TRIGGER fail_template_fixture_task BEFORE INSERT ON tasks FOR EACH ROW EXECUTE FUNCTION fail_template_fixture_task()")
		r, v := f.with(t).request("POST", "/v1/projects", f.aliceToken, core.NewID(), project.Create{Name: "Atomic initialization probe", Scope: "Must roll back", Type: "general", OwnerID: f.alice, Classification: "internal", TemplateID: &newRevision.ID}, nil)
		expect(t, r, 500, v)
		var n int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM projects WHERE org_id=$1 AND name='Atomic initialization probe'", f.org).Scan(&n); err != nil || n != 0 {
			t.Fatal("partial project survived", n, err)
		}
		exec("DROP TRIGGER fail_template_fixture_task ON tasks")
		exec("DROP FUNCTION fail_template_fixture_task()")
	})
	// A real HTTP/browser fixture can opt in; its helper owns and cleans the DB.
	if os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") == "1" {
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES(gen_random_uuid(),$1,'template_browser_proof','Browser MFA proof only',true)", f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT gen_random_uuid(),$1,$2,id,'org' FROM roles WHERE org_id=$1 AND key='template_browser_proof'", f.org, f.alice)
		f.aliceToken = memberMFAToken(f, "alice", "android")
	}
	raw, _ := json.Marshal(map[string]any{"published_template_id": newRevision.ID, "project_id": initialized.ID})
	var values map[string]any
	_ = json.Unmarshal(raw, &values)
	holdBrowserBusinessFixture(t, f, values)
}
