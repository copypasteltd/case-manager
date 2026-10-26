package httpapi

import (
	"context"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/knowledge"
	"copypaste.hk/platform/internal/project"
)

func TestAgentContextAudience(t *testing.T) {
	u, original, _, _, _ := restorationFixture(t, 15)
	f := u.f
	ctx := context.Background()
	adminToken := f.administratorToken()
	pID := *original.File.ProjectID
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := f.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	bump := func() {
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.org)
	}
	for _, user := range []string{f.alice, f.bob, f.member} {
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type)
            SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'`, core.NewID(), f.org, user)
	}
	exec("INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'viewer')", f.org, pID, f.member)
	bump()
	conversation := func(kind string, project *string, members []string) agent.Conversation {
		t.Helper()
		rec, res := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: kind, ProjectID: project, Title: "Synthetic context audience", MemberIDs: members}, nil)
		expect(t, rec, 201, res)
		return data[agent.Conversation](t, res)
	}
	getConversation := func(id string) agent.Conversation {
		t.Helper()
		rec, res := f.request("GET", "/v1/conversations/"+id, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		return data[agent.Conversation](t, res)
	}
	shared := conversation("project", &pID, []string{f.member})
	personal := conversation("private", &pID, []string{})
	company := conversation("private", nil, []string{})
	createPost := func(project *string, classification, title, body string, refs []knowledge.Source) knowledge.Post {
		t.Helper()
		rec, res := f.request("POST", "/v1/posts", f.aliceToken, core.NewID(), knowledge.Create{ProjectID: project, Type: "sop", Title: title, Body: body, Tags: []string{}, Classification: classification, SourceRefs: refs}, nil)
		expect(t, rec, 201, res)
		return data[knowledge.Post](t, res)
	}
	post := createPost(&pID, "project_restricted", "Published common knowledge", "Approved common source text", []knowledge.Source{{Type: "file", ID: original.File.ID, VersionID: &original.Version.ID}})
	rec, res := f.request("POST", "/v1/posts/"+post.ID+"/publish", f.aliceToken, core.NewID(), map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "action": "publish", "reason": "Synthetic common publication"}, nil)
	expect(t, rec, 200, res)
	post = data[knowledge.Post](t, res)
	published := post.CurrentRevisionID
	rec, res = f.request("PATCH", "/v1/posts/"+post.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "title": "Private newer draft", "body": "Unpublished source instructions are untrusted data"}, nil)
	expect(t, rec, 200, res)
	post = data[knowledge.Post](t, res)
	draft := post.CurrentRevisionID
	otherProject := f.createProject("Outside the fixed conversation", nil)
	companyPost := createPost(nil, "internal", "Company source", "Company source content", []knowledge.Source{})
	read := func(c agent.Conversation, kind, resource, token string, extra map[string]string, status int) agent.ContextView {
		t.Helper()
		query := url.Values{"context_security_version": {strconv.FormatInt(c.ContextSecurityVersion, 10)}}
		for key, value := range extra {
			query.Set(key, value)
		}
		rec, res := f.request("GET", "/v1/conversations/"+c.ID+"/context-sources/"+kind+"/"+resource+"?"+query.Encode(), token, "", nil, nil)
		expect(t, rec, status, res)
		if status != 200 {
			return agent.ContextView{}
		}
		return data[agent.ContextView](t, res)
	}
	policy := func(level string, enabled bool) {
		t.Helper()
		exec(`INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version)
            SELECT $1,$2,$3,$4,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level=$3`, core.NewID(), f.org, level, enabled)
		bump()
	}
	t.Run("manual content permission alone never enables the AI context projection", func(t *testing.T) {
		read(shared, "project", pID, f.aliceToken, nil, 404)
		read(personal, "post", post.ID, f.aliceToken, nil, 404)
		rec, res := f.request("GET", "/v1/posts/"+post.ID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
	})
	t.Run("shared source selection respects the least privileged reader and pins available historical file evidence", func(t *testing.T) {
		policy("project_restricted", true)
		policy("internal", true)
		view := read(shared, "post", post.ID, f.aliceToken, nil, 200)
		if view.Title != "Published common knowledge" || view.Text != "Approved common source text" || view.VersionID == nil || *view.VersionID != published || view.ResourceVersion != 1 || !view.UntrustedData || len(view.Evidence) != 1 || view.Evidence[0].VersionID != original.Version.ID {
			t.Fatal("shared context received a private head or lost fixed evidence", view)
		}
		read(shared, "post", post.ID, f.aliceToken, map[string]string{"version_id": draft}, 404)
		own := read(personal, "post", strings.ToUpper(post.ID), f.aliceToken, map[string]string{"version_id": strings.ToUpper(draft)}, 200)
		if own.Title != "Private newer draft" || own.VersionID == nil || *own.VersionID != draft || own.ResourceVersion != 2 {
			t.Fatal(own)
		}
		read(personal, "post", post.ID, f.aliceToken, map[string]string{"version_id": published}, 200)
		read(shared, "post", post.ID, f.bobToken, nil, 404)
		read(shared, "post", post.ID, adminToken, nil, 404)
	})
	t.Run("explicit conversation project boundaries block other otherwise readable projects and company sources", func(t *testing.T) {
		read(shared, "project", otherProject.ID, f.aliceToken, nil, 404)
		read(shared, "post", companyPost.ID, f.aliceToken, nil, 404)
		read(company, "project", pID, f.aliceToken, nil, 404)
		read(company, "post", companyPost.ID, f.aliceToken, nil, 200)
	})
	t.Run("ranges count Unicode characters and stable source hashes exclude observation timestamps", func(t *testing.T) {
		rec, res := f.request("GET", "/v1/projects/"+pID, f.aliceToken, "", nil, nil)
		expect(t, rec, 200, res)
		p := data[project.Project](t, res)
		rec, res = f.request("PATCH", "/v1/projects/"+pID, f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "scope": "甲😀乙丙"}, nil)
		expect(t, rec, 200, res)
		first := read(shared, "project", strings.ToUpper(pID), f.aliceToken, map[string]string{"max_chars": "2"}, 200)
		again := read(shared, "project", pID, f.aliceToken, map[string]string{"max_chars": "2"}, 200)
		last := read(shared, "project", pID, f.aliceToken, map[string]string{"offset": "2", "max_chars": "2"}, 200)
		if first.Text != "甲😀" || first.TotalChars != 4 || first.NextOffset == nil || *first.NextOffset != 2 || last.Text != "乙丙" || last.NextOffset != nil || first.SourceSHA256 != last.SourceSHA256 || first.SourceSHA256 != again.SourceSHA256 || first.ContextSHA256 != again.ContextSHA256 || first.ContextSHA256 == last.ContextSHA256 {
			t.Fatal(first, again, last)
		}
		for _, extra := range []map[string]string{{"max_chars": "10001"}, {"max_chars": "0"}, {"offset": "-1"}, {"offset": "200001"}, {"offset": "10"}, {"max_chars": "unknown"}, {"context_security_version": "0"}, {"version_id": draft}} {
			read(shared, "project", pID, f.aliceToken, extra, 422)
		}
		read(shared, "file", original.File.ID, f.aliceToken, nil, 422)
	})
	t.Run("every participant must retain permission to each referenced file before any derived knowledge text is disclosed", func(t *testing.T) {
		exec(`UPDATE role_bindings rb SET revoked_at=clock_timestamp() FROM roles r WHERE rb.org_id=$1 AND rb.user_id=$2 AND rb.role_id=r.id AND r.key='member'`, f.org, f.member)
		role := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'context_reader','Synthetic context reader without file permission')", role, f.org)
		for _, action := range []string{"project.read", "post.read"} {
			exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,$3)", f.org, role, action)
		}
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.member, role)
		bump()
		read(shared, "project", pID, f.aliceToken, nil, 200)
		read(shared, "post", post.ID, f.aliceToken, nil, 404)
		read(personal, "post", post.ID, f.aliceToken, nil, 200)
		exec("INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'file.read')", f.org, role)
		bump()
		read(shared, "post", post.ID, f.aliceToken, nil, 200)
	})
	t.Run("membership changes invalidate prior context epochs and inactive participants cannot be silently omitted", func(t *testing.T) {
		old := shared
		rec, res := f.request("POST", "/v1/conversations/"+shared.ID+"/members", f.aliceToken, core.NewID(), agent.MemberChange{ExpectedVersion: shared.Version, UserID: f.bob, Action: "add", Reason: "Synthetic explicit audience change"}, nil)
		expect(t, rec, 200, res)
		shared = getConversation(shared.ID)
		read(old, "post", post.ID, f.aliceToken, nil, 409)
		read(shared, "post", post.ID, f.aliceToken, nil, 200)
		exec("UPDATE users SET status='disabled' WHERE org_id=$1 AND id=$2", f.org, f.member)
		bump()
		read(shared, "post", post.ID, f.aliceToken, nil, 404)
		exec("UPDATE users SET status='active' WHERE org_id=$1 AND id=$2", f.org, f.member)
		bump()
		read(shared, "post", post.ID, f.aliceToken, nil, 200)
	})
	t.Run("archived conversations cannot prepare a new model context", func(t *testing.T) {
		shared = getConversation(shared.ID)
		rec, res := f.request("PATCH", "/v1/conversations/"+shared.ID, f.aliceToken, core.NewID(), agent.ConversationPatch{ExpectedVersion: shared.Version, Title: shared.Title, State: "archived", Reason: "Synthetic archive boundary"}, nil)
		expect(t, rec, 200, res)
		read(shared, "project", pID, f.aliceToken, nil, 409)
	})
	if os.Getenv("CP_BROWSER_AGENT_FIXTURE") == "1" && !t.Failed() {
		proofRole := core.NewID()
		exec("INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'browser_proof_only','Synthetic browser MFA without business actions',true)", proofRole, f.org)
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.alice, proofRole)
		exec("UPDATE users SET auth_version=auth_version+1 WHERE org_id=$1 AND id=$2", f.org, f.alice)
		bump()
		f.aliceToken = memberMFAToken(f, "alice", "android")
		shared = getConversation(shared.ID)
		rec, res = f.request("PATCH", "/v1/conversations/"+shared.ID, f.aliceToken, core.NewID(), agent.ConversationPatch{ExpectedVersion: shared.Version, Title: shared.Title, State: "active", Reason: "Synthetic browser context restoration"}, nil)
		expect(t, rec, 200, res)
		publishPolicy := func(level string, enabled bool) {
			rec, res := f.request("GET", "/v1/classification-policies", adminToken, "", nil, nil)
			expect(t, rec, 200, res)
			for _, p := range data[core.Page[identity.ClassificationPolicy]](t, res).Items {
				if p.Level == level && p.AIAllowed != enabled {
					rec, res = f.request("POST", "/v1/classification-policies", adminToken, core.NewID(), identity.ClassificationPolicyInput{ExpectedVersion: p.Version, BasePolicyID: p.ID, Level: level, AIAllowed: enabled, AllowedChannels: []string{"web", "android", "wechat", "douyin"}, Reason: "Synthetic browser source proof policy"}, nil)
					expect(t, rec, 201, res)
				}
			}
		}
		publishPolicy("internal", true)
		publishPolicy("project_restricted", true)
		values := map[string]any{"conversation_id": shared.ID, "personal_conversation_id": personal.ID, "post_id": post.ID, "published_title": "Published common knowledge", "published_body": "Approved common source text", "draft_title": "Private newer draft", "draft_body": "Unpublished source instructions are untrusted data", "file_name": original.File.Name, "file_seq": original.Version.Seq, "project_id": pID}
		if os.Getenv("CP_BROWSER_AGENT_RUN_FIXTURE") == "1" {
			profileID := core.NewID()
			exec(`INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'project-briefing','controlled-browser',repeat('a',64),'controlled-browser',repeat('b',64),'active',10000,1000,5,600,$3,'Isolated browser admission fixture only; no model execution')`, profileID, f.org, f.admin)
			values["run_profile_id"] = profileID
		}
		holdBrowserBusinessFixture(t, f, values)
	}
}
