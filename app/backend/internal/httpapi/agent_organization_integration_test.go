package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/files"
)

type organizationFixture struct {
	t    *testing.T
	f    *fixture
	c    agent.Conversation
	root string
	file files.File
}

func newOrganizationFixture(t *testing.T) *organizationFixture {
	u := newUploadFixture(t)
	configureStorageTargets(u)
	f := u.f
	ctx := context.Background()
	for _, role := range []string{"agent_user", "agent_plan_operator"} {
		if _, e := f.owner.Exec(ctx, "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key=$4", core.NewID(), f.org, f.alice, role); e != nil {
			t.Fatal(e)
		}
	}
	for _, level := range []string{"internal", "project_restricted"} {
		if _, e := f.owner.Exec(ctx, "INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,$3,true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level=$3", core.NewID(), f.org, level); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := f.owner.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org); e != nil {
		t.Fatal(e)
	}
	if _, e := f.owner.Exec(ctx, "UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.alice); e != nil {
		t.Fatal(e)
	}
	base, _ := commitContentFixture(t, u, "待整理资料.md", "text/markdown", []byte("Only namespace metadata may change.\n"), nil)
	r, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: base.File.ProjectID, Title: "资料整理私密会话", MemberIDs: []string{}}, nil)
	expect(t, r, 201, res)
	return &organizationFixture{t: t, f: f, c: data[agent.Conversation](t, res), root: u.root, file: base.File}
}
func (h *organizationFixture) exec(sql string, args ...any) {
	h.t.Helper()
	if _, e := h.f.owner.Exec(context.Background(), sql, args...); e != nil {
		h.t.Fatal(e)
	}
}
func (h *organizationFixture) count(sql string, args ...any) int {
	h.t.Helper()
	var n int
	if e := h.f.owner.QueryRow(context.Background(), sql, args...).Scan(&n); e != nil {
		h.t.Fatal(e)
	}
	return n
}
func (h *organizationFixture) folder(name string) files.Folder {
	h.t.Helper()
	r, res := h.f.request("POST", "/v1/folders", h.f.aliceToken, core.NewID(), files.FolderCreate{ParentID: &h.root, ProjectID: h.c.ProjectID, Name: name, Classification: "project_restricted"}, nil)
	expect(h.t, r, 201, res)
	return data[files.Folder](h.t, res)
}
func (h *organizationFixture) plan(actions []files.OrganizationAction) agent.OrganizationPlan {
	h.t.Helper()
	r, res := h.f.request("POST", "/v1/conversations/"+h.c.ID+"/organization-plans", h.f.aliceToken, core.NewID(), agent.OrganizationPlanCreate{ExpectedVersion: h.c.Version, ContextSecurityVersion: h.c.ContextSecurityVersion, Actions: actions, Reason: "按核对后的准确位置整理资料"}, nil)
	expect(h.t, r, 201, res)
	return data[agent.OrganizationPlan](h.t, res)
}
func (h *organizationFixture) act(p agent.OrganizationPlan, action string, status int) agent.OrganizationPlan {
	h.t.Helper()
	r, res := h.f.request("POST", "/v1/organization-plans/"+p.ID+"/"+action, h.f.aliceToken, core.NewID(), agent.PlanAction{ExpectedVersion: p.Version, PlanHash: p.PlanHash, Reason: "本人明确确认 " + action}, nil)
	expect(h.t, r, status, res)
	if status != 200 {
		return agent.OrganizationPlan{}
	}
	return data[agent.OrganizationPlan](h.t, res)
}
func ptr[T any](v T) *T { return &v }

func TestAgentOrganizationPlans(t *testing.T) {
	h := newOrganizationFixture(t)
	f := h.f
	ctx := context.Background()
	caseOf := func(name string, fn func(*testing.T)) {
		t.Run(name, func(child *testing.T) {
			h.t = child
			h.f = f.with(child)
			defer func() { h.t = t; h.f = f }()
			fn(child)
		})
	}
	root := func() files.Folder {
		r, res := h.f.request("GET", "/v1/folders/"+h.root, h.f.aliceToken, "", nil, nil)
		expect(h.t, r, 200, res)
		return data[files.Folder](h.t, res)
	}
	caseOf("exact batch requires approval then creates renames and moves with immutable receipts", func(t *testing.T) {
		source, target := h.folder("原目录"), h.folder("归档资料")
		p := h.plan([]files.OrganizationAction{{Kind: "folder.create", ID: h.root, ExpectedVersion: root().Version, Name: ptr("项目月报")}, {Kind: "folder.move", ID: source.ID, ExpectedVersion: source.Version, Name: ptr("交付资料")}, {Kind: "file.move", ID: h.file.ID, ExpectedVersion: h.file.Version, TargetFolderID: &target.ID, ExpectedTargetVersion: &target.Version, Name: ptr("已归档资料.md")}})
		if p.State != "proposed" || !p.CanApprove || p.CanCommit || len(p.Preview) != 3 || len(p.Results) != 0 {
			t.Fatal("proposal incorrectly implies committed work")
		}
		h.act(p, "commit", 409)
		approved := h.act(p, "approve", 200)
		done := h.act(approved, "commit", 200)
		if done.State != "committed" || done.OperationID == nil || len(done.Results) != 3 || *done.OperationID != done.Results[0].OperationID {
			t.Fatal("missing atomic per-item receipts")
		}
		if h.count("SELECT count(*) FROM agent_organization_events WHERE plan_id=$1", p.ID) != 3 {
			t.Fatal("missing durable plan history")
		}
		r, res := f.request("GET", "/v1/files/"+h.file.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, res)
		current := data[files.File](t, res)
		if current.FolderID != target.ID || current.Name != "已归档资料.md" || current.CurrentVersionID == nil || *current.CurrentVersionID != *h.file.CurrentVersionID {
			t.Fatal("metadata organization changed the original bytes or location is wrong")
		}
		h.file = current
		before := h.count("SELECT count(*) FROM idempotency_records WHERE org_id=$1 AND route_key LIKE '/internal/agent-organization/%'", f.org)
		replay := h.act(approved, "commit", 200)
		if *replay.OperationID != *done.OperationID || before != h.count("SELECT count(*) FROM idempotency_records WHERE org_id=$1 AND route_key LIKE '/internal/agent-organization/%'", f.org) {
			t.Fatal("commit replay duplicated business work")
		}
		if h.count("SELECT count(*) FROM outbox_events WHERE org_id=$1 AND aggregate_id=$2 AND event_type='agent.organization.committed'", f.org, p.ID) != 1 || h.count("SELECT count(*) FROM outbox_events WHERE org_id=$1 AND aggregate_id=$2 AND event_type='agent.organization.reconciled'", f.org, p.ID) != 1 {
			t.Fatal("receipt reconciliation emitted a second business commit event")
		}
		if _, e := f.api.DB.Exec(ctx, "UPDATE agent_organization_plans SET reason='tamper' WHERE id=$1", p.ID); e == nil {
			t.Fatal("mutable approved plan")
		}
		if _, e := f.api.DB.Exec(ctx, "DELETE FROM agent_organization_events WHERE plan_id=$1", p.ID); e == nil {
			t.Fatal("mutable event history")
		}
	})
	caseOf("MFA wrong hash other actor and revoked plans cannot authorize a batch", func(t *testing.T) {
		p := h.plan([]files.OrganizationAction{{Kind: "folder.create", ID: h.root, ExpectedVersion: root().Version, Name: ptr("人工验证目录")}})
		h.exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		h.act(p, "approve", 403)
		h.exec("UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2", f.org, f.alice)
		r, res := f.request("POST", "/v1/organization-plans/"+p.ID+"/approve", f.aliceToken, core.NewID(), agent.PlanAction{ExpectedVersion: 1, PlanHash: strings.Repeat("0", 64), Reason: "wrong hash"}, nil)
		expect(t, r, 409, res)
		r, res = f.request("GET", "/v1/organization-plans/"+p.ID, f.bobToken, "", nil, nil)
		expect(t, r, 404, res)
		approved := h.act(p, "approve", 200)
		revoked := h.act(approved, "revoke", 200)
		h.act(revoked, "commit", 409)
		if h.count("SELECT count(*) FROM folders WHERE org_id=$1 AND name='人工验证目录'", f.org) != 0 {
			t.Fatal("unapproved work persisted")
		}
	})
	caseOf("source target and context changes fail before any namespace write", func(t *testing.T) {
		source, target := h.folder("待调整目录"), h.folder("目标目录")
		p := h.plan([]files.OrganizationAction{{Kind: "folder.move", ID: source.ID, ExpectedVersion: source.Version, TargetFolderID: &target.ID, ExpectedTargetVersion: &target.Version}})
		approved := h.act(p, "approve", 200)
		r, res := f.request("POST", "/v1/folders/"+target.ID+"/move", f.aliceToken, core.NewID(), files.Move{ExpectedVersion: target.Version, NewName: ptr("由员工改名的目录"), Reason: "competing change"}, nil)
		expect(t, r, 200, res)
		h.act(approved, "commit", 409)
		if h.count("SELECT count(*) FROM folders WHERE id=$1 AND parent_id=$2 AND version=1", source.ID, h.root) != 1 {
			t.Fatal("stale plan partially moved source")
		}
		p = h.plan([]files.OrganizationAction{{Kind: "folder.move", ID: source.ID, ExpectedVersion: source.Version, Name: ptr("不能旧版本改名")}})
		r, res = f.request("POST", "/v1/folders/"+source.ID+"/move", f.aliceToken, core.NewID(), files.Move{ExpectedVersion: source.Version, NewName: ptr("人工改名"), Reason: "competing source"}, nil)
		expect(t, r, 200, res)
		h.act(p, "approve", 409)
	})
	caseOf("a later naming conflict rolls back earlier commands receipts audit and plan head", func(t *testing.T) {
		existing := h.folder("不可覆盖目录")
		p := h.plan([]files.OrganizationAction{{Kind: "folder.create", ID: h.root, ExpectedVersion: root().Version, Name: ptr("应完整回滚目录")}, {Kind: "folder.create", ID: h.root, ExpectedVersion: root().Version, Name: &existing.Name}})
		approved := h.act(p, "approve", 200)
		h.act(approved, "commit", 409)
		if h.count("SELECT count(*) FROM folders WHERE org_id=$1 AND name='应完整回滚目录'", f.org) != 0 || h.count("SELECT count(*) FROM idempotency_records WHERE idempotency_key LIKE $1", "agent-organization:"+p.ID+":%") != 0 || h.count("SELECT count(*) FROM agent_organization_events WHERE plan_id=$1", p.ID) != 2 {
			t.Fatal("partial batch or receipt escaped rollback")
		}
		r, res := f.request("GET", "/v1/organization-plans/"+p.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 200, res)
		after := data[agent.OrganizationPlan](t, res)
		if after.State != "approved" || after.OperationID != nil || len(after.Results) != 0 {
			t.Fatal("failed batch appears committed")
		}
	})
	caseOf("bounded listing and plan scope exclude other projects cycles and closed projects", func(t *testing.T) {
		r, res := f.request("GET", "/v1/conversations/"+h.c.ID+"/directories/"+h.root, f.aliceToken, "", nil, nil)
		expect(t, r, 200, res)
		listing := data[files.DirectoryListing](t, res)
		if listing.Folder.ID != h.root || len(listing.Items) == 0 {
			t.Fatal("missing real namespace projection")
		}
		other := f.createProject("Out of conversation scope", []string{})
		var otherRoot string
		if e := f.owner.QueryRow(ctx, "SELECT id FROM folders WHERE org_id=$1 AND project_id=$2 AND parent_id IS NULL", f.org, other.ID).Scan(&otherRoot); e != nil {
			t.Fatal(e)
		}
		r, res = f.request("GET", "/v1/conversations/"+h.c.ID+"/directories/"+otherRoot, f.aliceToken, "", nil, nil)
		expect(t, r, 404, res)
		child := h.folder("循环检查")
		r, res = f.request("POST", "/v1/conversations/"+h.c.ID+"/organization-plans", f.aliceToken, core.NewID(), agent.OrganizationPlanCreate{ExpectedVersion: h.c.Version, ContextSecurityVersion: h.c.ContextSecurityVersion, Reason: "invalid cycle", Actions: []files.OrganizationAction{{Kind: "folder.move", ID: child.ID, ExpectedVersion: child.Version, TargetFolderID: &child.ID, ExpectedTargetVersion: &child.Version}}}, nil)
		expect(t, r, 422, res)
		// A current source of authority does not survive a conversation revision.
		p := h.plan([]files.OrganizationAction{{Kind: "folder.create", ID: h.root, ExpectedVersion: root().Version, Name: ptr("会话修订前目录")}})
		r, res = f.request("PATCH", "/v1/conversations/"+h.c.ID, f.aliceToken, core.NewID(), agent.ConversationPatch{ExpectedVersion: h.c.Version, Title: "会话已修订", State: "active", Reason: "scope refresh"}, nil)
		expect(t, r, 200, res)
		h.act(p, "approve", 409)
	})
}

func TestAgentOrganizationBrowserFixture(t *testing.T) {
	h := newOrganizationFixture(t)
	target := h.folder("归档资料")
	source := h.folder("原始目录")
	// Real interactive credentials are only used by the explicitly enabled,
	// disposable browser fixture. They never configure a production runtime.
	if os.Getenv("CP_BROWSER_ORGANIZATION_FIXTURE") == "1" {
		h.f.aliceToken = memberMFAToken(h.f, "alice", "android")
		_ = h.f.administratorToken()
	}
	h.plan([]files.OrganizationAction{{Kind: "folder.move", ID: source.ID, ExpectedVersion: source.Version, Name: ptr("移动端已核对目录")}, {Kind: "file.move", ID: h.file.ID, ExpectedVersion: h.file.Version, TargetFolderID: &target.ID, ExpectedTargetVersion: &target.Version}})
	if os.Getenv("CP_BROWSER_ORGANIZATION_FIXTURE") == "1" && !t.Failed() {
		holdBrowserBusinessFixture(t, h.f, map[string]any{"organization_fixture": true, "conversation_id": h.c.ID, "project_id": *h.c.ProjectID, "root_id": h.root, "source_id": source.ID, "target_id": target.ID, "file_id": h.file.ID})
	}
}

func TestAgentOrganizationGuards(t *testing.T) {
	h := newOrganizationFixture(t)
	f := h.f
	ctx := context.Background()
	var version int64
	if e := f.owner.QueryRow(ctx, "SELECT version FROM folders WHERE id=$1", h.root).Scan(&version); e != nil {
		t.Fatal(e)
	}
	action := files.OrganizationAction{Kind: "folder.create", ID: h.root, ExpectedVersion: version, Name: ptr("需人工核对的目录")}
	t.Run("enumeration stays bounded when the namespace exceeds fifty direct items", func(t *testing.T) {
		for i := 0; i < 51; i++ {
			h.folder(fmt.Sprintf("有限盘点目录%02d", i))
		}
		r, res := f.request("GET", "/v1/conversations/"+h.c.ID+"/directories/"+h.root, f.aliceToken, "", nil, nil)
		expect(t, r, 200, res)
		listing := data[files.DirectoryListing](t, res)
		if !listing.HasMore || len(listing.Items) != 50 {
			t.Fatal("unbounded or misleading directory inventory")
		}
	})
	t.Run("an expired immutable intent cannot be revived by a current MFA session", func(t *testing.T) {
		p := h.plan([]files.OrganizationAction{action})
		var raw string
		if e := f.owner.QueryRow(ctx, "SELECT intent FROM agent_organization_plans WHERE id=$1", p.ID).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		var intent map[string]any
		if e := json.Unmarshal([]byte(raw), &intent); e != nil {
			t.Fatal(e)
		}
		id := core.NewID()
		created := p.CreatedAt.Add(-2 * time.Hour)
		expires := created.Add(30 * time.Minute)
		intent["id"], intent["created_at"], intent["expires_at"] = id, created, expires
		b, _ := json.Marshal(intent)
		canonical, e := core.CanonicalJSON(b)
		if e != nil {
			t.Fatal(e)
		}
		hash := core.Digest(canonical)
		tx, e := f.owner.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		_, e = tx.Exec(ctx, `INSERT INTO agent_organization_plans(id,org_id,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,origin_run_id,project_id,intent,plan_hash,preview,reason,created_at,expires_at)
 SELECT $2,org_id,conversation_id,conversation_revision_id,context_security_version,actor_id,actor_auth_version,organization_auth_version,origin_run_id,project_id,$3,$4,preview,reason,$5,$6 FROM agent_organization_plans WHERE id=$1`, p.ID, id, string(canonical), hash, created, expires)
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(ctx, "INSERT INTO agent_organization_events(id,org_id,plan_id,version,kind,plan_hash,actor_id,reason) VALUES($1,$2,$3,1,'proposed',$4,$5,'Expired fixture intent')", core.NewID(), f.org, id, hash, f.alice)
		if e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		p.ID, p.PlanHash = id, hash
		h.act(p, "approve", 409)
		r, res := f.request("GET", "/v1/organization-plans/"+id, f.aliceToken, "", nil, nil)
		expect(t, r, 200, res)
		view := data[agent.OrganizationPlan](t, res)
		if view.BlockedReason == nil || *view.BlockedReason != "PLAN_EXPIRED" || view.CanApprove || !view.CanRevoke {
			t.Fatal("expired grant presented as runnable")
		}
	})
	t.Run("a changed head cannot commit without a matching durable event", func(t *testing.T) {
		p := h.plan([]files.OrganizationAction{action})
		tx, e := f.api.DB.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, "UPDATE agent_organization_plans SET state='approved',version=2 WHERE id=$1", p.ID); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e == nil {
			t.Fatal("plan head committed without human approval history")
		}
	})
	t.Run("plan limits and parent dependent batches are rejected", func(t *testing.T) {
		child := h.folder("需要独立计划的父目录")
		cases := [][]files.OrganizationAction{
			{{Kind: "folder.move", ID: child.ID, ExpectedVersion: child.Version, Name: ptr("新父目录名称")}, {Kind: "folder.create", ID: child.ID, ExpectedVersion: child.Version, Name: ptr("不得同批新建子目录")}},
		}
		tooMany := []files.OrganizationAction{}
		for i := 0; i < 11; i++ {
			v := action
			v.Name = ptr(fmt.Sprintf("超量%02d", i))
			tooMany = append(tooMany, v)
		}
		cases = append(cases, tooMany)
		for _, actions := range cases {
			r, res := f.request("POST", "/v1/conversations/"+h.c.ID+"/organization-plans", f.aliceToken, core.NewID(), agent.OrganizationPlanCreate{ExpectedVersion: h.c.Version, ContextSecurityVersion: h.c.ContextSecurityVersion, Actions: actions, Reason: "reject unsafe expansion"}, nil)
			expect(t, r, 422, res)
		}
	})
	t.Run("an AI classification revocation hides captured plan data and blocks execution", func(t *testing.T) {
		p := h.act(h.plan([]files.OrganizationAction{action}), "approve", 200)
		h.exec("INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',false,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", core.NewID(), f.org)
		h.exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
		r, res := f.request("GET", "/v1/organization-plans/"+p.ID, f.aliceToken, "", nil, nil)
		expect(t, r, 404, res)
		h.act(p, "commit", 404)
		if h.count("SELECT count(*) FROM folders WHERE org_id=$1 AND name=$2", f.org, *action.Name) != 0 {
			t.Fatal("revoked AI scope still executed")
		}
	})
}
