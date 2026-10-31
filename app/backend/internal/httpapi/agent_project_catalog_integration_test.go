package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/project"
)

// Real native PG, ordinary template/project commands, four actual session
// channels and the execution ledger. No model, cloud or virtual machine.
func TestAgentProjectCatalog(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	current := t
	exec := func(q string, args ...any) {
		current.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			current.Fatal(e)
		}
	}
	run := func(name string, fn func()) {
		if !t.Run(name, func(st *testing.T) { current = st; f.t = st; defer func() { current = t; f.t = t }(); fn() }) {
			t.FailNow()
		}
	}
	for _, user := range []string{f.alice, f.member} {
		for _, role := range []string{"agent_user", "agent_plan_operator"} {
			exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key=$4`, core.NewID(), f.org, user, role)
		}
	}
	exec(`INSERT INTO classification_policies(id,org_id,level,ai_allowed,allowed_channels,policy_version) SELECT $1,$2,'internal',true,ARRAY['web','wechat','douyin','android'],max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='internal'`, core.NewID(), f.org)
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	f.aliceToken = memberMFAToken(f, "alice", "android")
	f.memberToken = memberMFAToken(f, "member", "android")
	admin := f.administratorToken()
	template := func(key string, publish bool) project.Template {
		in := project.TemplateCreate{Key: key, Reason: "Synthetic reviewed catalog structure", TemplateContent: project.TemplateContent{Name: "项目交付模板 " + key, Description: "合成目录；不含真实业务资料", Schema: project.TemplateSchema{Folders: []string{"评审材料"}, Tasks: []project.TemplateTask{{Title: "核对项目范围", Body: "由负责人明确范围", Priority: "high", ReviewRequired: true}}, Milestones: []project.TemplateMilestone{{Title: "阶段交付"}}}}}
		r, v := f.request("POST", "/v1/project-templates", admin, core.NewID(), in, nil)
		expect(current, r, 201, v)
		out := data[project.Template](current, v)
		if publish {
			r, v = f.request("POST", "/v1/project-templates/"+out.ID+"/publish", admin, core.NewID(), project.TemplateAction{ExpectedVersion: out.Version, ContentHash: out.ContentHash, Reason: "Explicit synthetic publication"}, nil)
			expect(current, r, 200, v)
			out = data[project.Template](current, v)
		}
		return out
	}
	published := template("catalog-delivery", true)
	draft := template("private-draft", false)
	conversation := func(token string, h map[string]string) agent.Conversation {
		r, v := f.request("POST", "/v1/conversations", token, core.NewID(), agent.ConversationCreate{Type: "private", Title: "项目规划目录", MemberIDs: []string{}}, h)
		expect(current, r, 201, v)
		return data[agent.Conversation](current, v)
	}
	c := conversation(f.aliceToken, nil)
	login := func(device string) (string, map[string]string) {
		var cipher []byte
		if err := f.owner.QueryRow(ctx, "SELECT secret_cipher FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.alice).Scan(&cipher); err != nil {
			current.Fatal(err)
		}
		secret, e := identity.Open(f.api.Config.MFAKey, cipher, f.org+":"+f.alice)
		if e != nil {
			current.Fatal(e)
		}
		exec("UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", f.org, f.alice, time.Now().Unix()/30-1)
		challenge := f.login("alice", device)
		h := map[string]string{}
		if device == "web" {
			h["Origin"] = "http://localhost:5178"
		}
		r, v := f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(secret, time.Now())}, h)
		expect(current, r, 200, v)
		out := data[identity.LoginResult](current, v)
		if device == "web" {
			h["X-CSRF-Token"] = out.CSRFToken
			for _, cookie := range r.Result().Cookies() {
				if cookie.Name == "cp_session" {
					h["Cookie"] = "cp_session=" + cookie.Value
				}
			}
			return "", h
		}
		return out.AccessToken, h
	}
	for _, channel := range []string{"web", "wechat", "douyin", "android"} {
		run(channel+" catalog selection human plan approval and exact initialization", func() {
			token, h := login(channel)
			cc := conversation(token, h)
			for _, kind := range []string{"people", "templates"} {
				r, v := f.request("GET", "/v1/conversations/"+cc.ID+"/project-catalog?kind="+kind, token, "", nil, h)
				expect(current, r, 200, v)
				page := data[agent.ProjectCatalogPage](current, v)
				if len(page.Items) == 0 || !page.UntrustedData {
					current.Fatal("missing authorized catalog")
				}
			}
			in := agent.ProjectPlanCreate{ExpectedVersion: cc.Version, ContextSecurityVersion: cc.ContextSecurityVersion, Reason: "本人已核对负责人、成员、模板及日期", Project: project.Create{Name: "合成项目 " + channel, Scope: "明确项目范围", Type: "delivery", OwnerID: f.bob, MemberIDs: []string{f.member}, Classification: "internal", TemplateID: &published.ID, StartDate: "2026-11-01", DueDate: "2026-11-30"}}
			r, v := f.request("POST", "/v1/conversations/"+cc.ID+"/plans", token, core.NewID(), in, h)
			expect(current, r, 201, v)
			p := data[agent.Plan](current, v)
			if p.Project.OwnerID != f.bob || p.Project.TemplateID == nil || *p.Project.TemplateID != published.ID || len(p.Project.MemberIDs) != 1 {
				current.Fatal("selected project intent lost")
			}
			r, v = f.request("POST", "/v1/plans/"+p.ID+"/approve", token, core.NewID(), agent.PlanAction{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "本人批准准确计划"}, h)
			expect(current, r, 200, v)
			p = data[agent.Plan](current, v)
			key := core.NewID()
			action := agent.PlanAction{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "本人按已批准内容创建"}
			r, v = f.request("POST", "/v1/plans/"+p.ID+"/commit", token, key, action, h)
			expect(current, r, 200, v)
			p = data[agent.Plan](current, v)
			r, v = f.request("POST", "/v1/plans/"+p.ID+"/commit", token, key, action, h)
			expect(current, r, 200, v)
			var tasks, folders, members int
			var owner, start, due string
			if p.ProjectID == nil || p.OperationID == nil {
				current.Fatal("missing actual creation receipt")
			}
			err := f.owner.QueryRow(ctx, `SELECT owner_id,start_date::text,due_date::text,(SELECT count(*) FROM tasks WHERE project_id=p.id),(SELECT count(*) FROM folders WHERE project_id=p.id AND name='评审材料'),(SELECT count(*) FROM project_members WHERE project_id=p.id) FROM projects p WHERE id=$1`, *p.ProjectID).Scan(&owner, &start, &due, &tasks, &folders, &members)
			if err != nil || owner != f.bob || start != in.Project.StartDate || due != in.Project.DueDate || tasks != 1 || folders != 1 || members != 3 {
				current.Fatal("exact template initialization failed", err)
			}
		})
	}
	run("private metadata strict filters source content and unpublished templates", func() {
		for _, kind := range []string{"people", "templates"} {
			r, v := f.request("GET", "/v1/conversations/"+c.ID+"/project-catalog?kind="+kind, f.aliceToken, "", nil, nil)
			expect(current, r, 200, v)
			text := r.Body.String()
			for _, private := range []string{"credential", "login_key", "mfa_", "auth_version", "role_bindings"} {
				if strings.Contains(text, private) {
					current.Fatal("private account metadata exposed")
				}
			}
			for _, i := range data[agent.ProjectCatalogPage](current, v).Items {
				if i.ID == draft.ID {
					current.Fatal("draft template exposed")
				}
			}
		}
		for _, q := range []string{"kind=people&limit=0", "kind=people&limit=21", "kind=people&kind=templates", "kind=people&sql=x", "kind=unknown"} {
			r, v := f.request("GET", "/v1/conversations/"+c.ID+"/project-catalog?"+q, f.aliceToken, "", nil, nil)
			expect(current, r, 422, v)
		}
		r, v := f.request("GET", "/v1/conversations/"+c.ID+"/project-catalog/agent_project_template/"+draft.ID, f.aliceToken, "", nil, nil)
		expect(current, r, 404, v)
		r, v = f.request("GET", "/v1/conversations/"+c.ID+"/project-catalog/agent_project_template/"+published.ID+"?max_chars=100", f.aliceToken, "", nil, nil)
		expect(current, r, 200, v)
		doc := data[agent.ProjectCatalogDocument](current, v)
		if len([]rune(doc.Text)) != 100 || doc.NextOffset == nil || *doc.NextOffset != 100 || len(doc.Item.SourceSHA256) != 64 {
			current.Fatal("missing bounded exact template content")
		}
	})
	run("organization planning authority and conversation owner are required", func() {
		r, v := f.request("GET", "/v1/conversations/"+c.ID+"/project-catalog?kind=people", f.memberToken, "", nil, nil)
		expect(current, r, 404, v)
		cc := conversation(f.memberToken, nil)
		r, v = f.request("GET", "/v1/conversations/"+cc.ID+"/project-catalog?kind=templates", f.memberToken, "", nil, nil)
		expect(current, r, 404, v)
		r, v = f.request("GET", "/v1/conversations/"+c.ID+"/project-catalog?kind=templates", f.bobToken, "", nil, nil)
		expect(current, r, 404, v)
	})
	profile := core.NewID()
	exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason) VALUES($1,$2,'project-create','synthetic-142',repeat('a',64),$3,repeat('b',64),'active',10000,1000,20,600,$4,'Isolated catalog execution')`, profile, f.org, agentworker.RuntimeVersion, f.admin)
	svc := &agent.Service{DB: f.api.DB}
	start := func() (agent.Conversation, agent.ExecutionLease) {
		cc := conversation(f.aliceToken, nil)
		r, v := f.request("POST", "/v1/conversations/"+cc.ID+"/runs", f.aliceToken, core.NewID(), agent.RunCreate{ExpectedVersion: cc.Version, ContextSecurityVersion: cc.ContextSecurityVersion, ProfileID: profile, ClientMessageID: core.NewID(), Message: "先查询人员和已发布模板，再拟定项目计划"}, nil)
		expect(current, r, 202, v)
		job, e := svc.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "catalog.fixture", agent.ExecutionLeaseDuration)
		if e != nil || job == nil {
			current.Fatal("claim", e)
		}
		execution, e := svc.IssueExecution(ctx, *job)
		if e != nil {
			current.Fatal(e)
		}
		lease, e := svc.LeaseForCredential(ctx, execution.Credential)
		if e != nil {
			current.Fatal(e)
		}
		lease.Job = *job
		if e = svc.RecordVerifiedConfinement(ctx, lease, execution.Profile.ConfigurationSHA256, strings.Repeat("c", 64), time.Now().Add(time.Minute)); e != nil {
			current.Fatal(e)
		}
		if e = svc.StartExecution(ctx, lease); e != nil {
			current.Fatal(e)
		}
		return cc, lease
	}
	call := func(lease agent.ExecutionLease, key, tool string, in any) (json.RawMessage, error) {
		raw, _ := json.Marshal(in)
		return svc.ExecutionTool(ctx, lease, key, tool, raw)
	}
	stop := func(l agent.ExecutionLease) {
		if e := svc.FinishStoppedExecution(ctx, l.Job, "RUNTIME_EXECUTOR_EXITED"); e != nil {
			current.Fatal(e)
		}
	}
	decode := func(raw []byte) struct {
		Content struct {
			Kind string                   `json:"kind"`
			Page agent.ProjectCatalogPage `json:"page"`
			ID   string                   `json:"catalog_id"`
		} `json:"content"`
		Sources []agent.ResultReference `json:"source_refs"`
	} {
		var out struct {
			Content struct {
				Kind string                   `json:"kind"`
				Page agent.ProjectCatalogPage `json:"page"`
				ID   string                   `json:"catalog_id"`
			} `json:"content"`
			Sources []agent.ResultReference `json:"source_refs"`
		}
		if e := json.Unmarshal(raw, &out); e != nil {
			current.Fatal(e)
		}
		return out
	}
	run("model catalog durable observation replay and real project proposal", func() {
		cc, l := start()
		defer stop(l)
		raw, e := call(l, "catalog-people", "list_project_catalog", map[string]any{"kind": "people"})
		if e != nil {
			current.Fatal(e)
		}
		people := decode(raw)
		if len(people.Sources) != 1 || people.Sources[0].Type != "agent_project_catalog" {
			current.Fatal("page did not have one durable observation")
		}
		repeated, e := call(l, "catalog-people", "list_project_catalog", map[string]any{"kind": "people"})
		if e != nil || string(canonicalCatalogJSON(repeated)) != string(canonicalCatalogJSON(raw)) {
			current.Fatal("replay changed exact receipt", e)
		}
		raw, e = call(l, "catalog-template", "list_project_catalog", map[string]any{"kind": "templates"})
		if e != nil {
			current.Fatal(e)
		}
		templates := decode(raw)
		if len(templates.Content.Page.Items) != 1 || templates.Content.Page.Items[0].ID != published.ID {
			current.Fatal("wrong published template discovery")
		}
		if _, e = call(l, "template-text", "read_resource", map[string]any{"type": "agent_project_template", "id": published.ID}); e != nil {
			current.Fatal(e)
		}
		if _, e = call(l, "proposal", "propose_project", map[string]any{"project": map[string]any{"name": "秘书目录规划", "scope": "由真实目录选择候选", "classification": "internal", "owner_id": f.bob, "member_ids": []string{f.member}, "template_id": published.ID}, "reason": "仅拟定，等待本人批准"}); e != nil {
			current.Fatal(e)
		}
		r, v := f.request("GET", "/v1/conversations/"+cc.ID+"/project-catalog/agent_project_catalog/"+people.Content.ID, f.aliceToken, "", nil, nil)
		expect(current, r, 200, v)
		var n int
		if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_project_catalogs WHERE run_id=$1", l.RunID).Scan(&n); e != nil || n != 2 {
			current.Fatal("missing immutable page facts", e)
		}
		if _, e = f.owner.Exec(ctx, "UPDATE agent_project_catalogs SET payload='{}' WHERE run_id=$1", l.RunID); e == nil {
			current.Fatal("catalog history rewritable")
		}
	})
	run("candidate change blocks old tool replay and final completion", func() {
		_, l := start()
		defer stop(l)
		raw, e := call(l, "people-change", "list_project_catalog", map[string]any{"kind": "people", "query": "bob"})
		if e != nil {
			current.Fatal(e)
		}
		observed := decode(raw)
		if len(observed.Content.Page.Items) != 1 {
			current.Fatal("exact person query failed")
		}
		exec("UPDATE users SET display_name='bob changed',version=version+1 WHERE id=$1", f.bob)
		if _, e = call(l, "people-change", "list_project_catalog", map[string]any{"kind": "people", "query": "bob"}); e == nil {
			current.Fatal("changed person replayed")
		}
		if e = svc.CompleteExecution(ctx, l, agent.ProposedResult{Status: "pending", Summary: "固定候选已核对", SourceRefs: observed.Sources, ResourceRefs: []agent.ResultReference{}, OperationIDs: []string{}, MissingFields: []string{}, NextAction: "等待本人选择"}, agent.ExecutionUsage{}); e == nil {
			current.Fatal("changed source passed final completion")
		}
		exec("UPDATE users SET display_name='bob',version=version+1 WHERE id=$1", f.bob)
	})
	run("complete large directory pagination stays within technical source bounds", func() {
		for i := 0; i < 23; i++ {
			exec(`INSERT INTO users(id,org_id,login_key,display_name,status,credential_hash) SELECT $1,$2,$3,'目录候选','active',credential_hash FROM users WHERE id=$4`, core.NewID(), f.org, "synthetic-catalog-"+core.NewID(), f.alice)
		}
		_, l := start()
		defer stop(l)
		cursor := ""
		seen := map[string]bool{}
		pages := 0
		for {
			in := map[string]any{"kind": "people", "query": "目录候选", "limit": 20, "cursor": cursor}
			raw, e := call(l, core.NewID(), "list_project_catalog", in)
			if e != nil {
				current.Fatal(e)
			}
			page := decode(raw).Content.Page
			pages++
			for _, item := range page.Items {
				if seen[item.ID] {
					current.Fatal("duplicate candidate")
				}
				seen[item.ID] = true
			}
			if page.NextCursor == nil {
				break
			}
			cursor = *page.NextCursor
			if pages > 3 {
				current.Fatal("nonterminal pagination")
			}
		}
		if len(seen) != 23 || pages != 2 {
			current.Fatal("incomplete model directory")
		}
		if _, e := call(l, "changed-filter", "list_project_catalog", map[string]any{"kind": "people", "query": "alice", "limit": 20, "cursor": cursor}); e == nil {
			current.Fatal("cross-filter cursor accepted")
		}
	})
	run("template retirement and audit failure invalidate observations atomically", func() {
		cc, l := start()
		defer stop(l)
		raw, e := call(l, "retire-template", "list_project_catalog", map[string]any{"kind": "templates"})
		if e != nil {
			current.Fatal(e)
		}
		page := decode(raw)
		exec(`CREATE FUNCTION fail_catalog_142() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='agent.tool.read' THEN RAISE EXCEPTION 'synthetic audit fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_catalog_142 BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_catalog_142()`)
		if _, e = call(l, "audit-fault", "list_project_catalog", map[string]any{"kind": "people", "query": "alice"}); e == nil {
			current.Fatal("audit fault did not roll back")
		}
		exec("DROP TRIGGER fail_catalog_142 ON audit_events; DROP FUNCTION fail_catalog_142()")
		var n int
		if e = f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_project_catalogs WHERE run_id=$1", l.RunID).Scan(&n); e != nil || n != 1 {
			current.Fatal("unreceipted catalog escaped rollback", e)
		}
		r, v := f.request("POST", "/v1/project-templates/"+published.ID+"/retire", admin, core.NewID(), project.TemplateAction{ExpectedVersion: published.Version, ContentHash: published.ContentHash, Reason: "Explicit synthetic retirement"}, nil)
		expect(current, r, 200, v)
		if _, e = call(l, "retire-template", "list_project_catalog", map[string]any{"kind": "templates"}); e == nil {
			current.Fatal("retired template replayed")
		}
		r, v = f.request("GET", "/v1/conversations/"+cc.ID+"/project-catalog/agent_project_catalog/"+page.Content.ID, f.aliceToken, "", nil, nil)
		expect(current, r, 404, v)
	})
	run("browser_workbench", func() {
		if os.Getenv("CP_BROWSER_PROJECT_CATALOG_FIXTURE") != "1" {
			return
		}
		holdBrowserBusinessFixture(current, f, map[string]any{"fixture_kind": "project-catalog", "conversation_id": c.ID, "template_id": published.ID, "owner_id": f.bob, "member_id": f.member})
		var n int
		if e := f.owner.QueryRow(ctx, `SELECT count(*) FROM agent_plans p JOIN projects x ON x.org_id=p.org_id AND x.id=p.project_id WHERE p.conversation_id=$1 AND p.state='committed' AND x.owner_id=$2 AND x.template_id=$3 AND x.start_date='2026-11-01' AND x.due_date='2026-11-30'`, c.ID, f.bob, published.ID).Scan(&n); e != nil || n != 1 {
			current.Fatal("browser did not initialize the reviewed project", e)
		}
	})
}

func canonicalCatalogJSON(raw []byte) []byte { value, _ := core.CanonicalJSON(raw); return value }
