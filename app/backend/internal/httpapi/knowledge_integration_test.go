package httpapi

import (
	"context"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/knowledge"
)

func TestKnowledgeRevisionsAndComments(t *testing.T) {
	f := setup(t)
	p := f.createProject("Knowledge boundary", []string{f.bob})
	ctx := context.Background()
	role := core.NewID()
	if _, err := f.owner.Exec(ctx, "DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2", f.org, f.bob); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'published_reader','Published reader with comments')", role, f.org); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"project.read", "task.read", "post.read", "comment.write"} {
		if _, err := f.owner.Exec(ctx, "INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,$3)", f.org, role, action); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", core.NewID(), f.org, f.bob, role); err != nil {
		t.Fatal(err)
	}
	bob := f.login("bob", "android")
	create := map[string]any{"project_id": p.ID, "type": "sop", "title": "已审核知识", "body": "公开给项目成员的正文", "summary": "已审核摘要", "tags": []string{"项目知识", "release"}, "classification": "project_restricted"}
	rec, result := f.request("POST", "/v1/posts", f.aliceToken, core.NewID(), create, nil)
	expect(t, rec, 201, result)
	post := data[knowledge.Post](t, result)
	first := post.CurrentRevisionID
	publishedCommentKey := core.NewID()
	t.Run("drafts and unpublished revisions are filtered before search and comments", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("GET", "/v1/posts/"+post.ID, bob.AccessToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.request("GET", "/v1/posts?q=项目知识", bob.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[knowledge.Post]](t, result).Items) != 0 {
			t.Fatal("draft leaked through tags")
		}
		action := map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "action": "publish", "reason": "已完成内部审核"}
		rec, result = f.request("POST", "/v1/posts/"+post.ID+"/publish", f.aliceToken, core.NewID(), action, nil)
		expect(t, rec, 200, result)
		post = data[knowledge.Post](t, result)
		rec, result = f.request("POST", "/v1/posts/"+post.ID+"/comments", bob.AccessToken, publishedCommentKey, map[string]any{"body": "发布版的阅读反馈", "expected_context_revision_id": first}, nil)
		expect(t, rec, 201, result)
		publishedComment := data[knowledge.Comment](t, result)
		patch := map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "title": "尚未发布的机密修订", "body": "unpublished-secret-phrase", "tags": []string{"未发布关键词"}}
		rec, result = f.request("PATCH", "/v1/posts/"+post.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 200, result)
		post = data[knowledge.Post](t, result)
		rec, result = f.request("POST", "/v1/posts/"+post.ID+"/comments", f.aliceToken, core.NewID(), map[string]any{"body": "draft-only-comment", "expected_context_revision_id": post.CurrentRevisionID}, nil)
		expect(t, rec, 201, result)
		draftComment := data[knowledge.Comment](t, result)
		rec, result = f.request("GET", "/v1/posts/"+post.ID, bob.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		visible := data[knowledge.Post](t, result)
		if visible.Title != "已审核知识" || visible.Body != "公开给项目成员的正文" || visible.CurrentRevisionID != first || visible.HasUnpublishedChanges {
			t.Fatal("reader received draft metadata or body")
		}
		for _, q := range []string{"unpublished-secret-phrase", "未发布关键词"} {
			rec, result = f.request("GET", "/v1/posts?q="+q, bob.AccessToken, "", nil, nil)
			expect(t, rec, 200, result)
			if len(data[core.Page[knowledge.Post]](t, result).Items) != 0 {
				t.Fatal("draft leaked through text search")
			}
		}
		rec, result = f.request("GET", "/v1/posts/"+post.ID+"/comments", bob.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		var comments struct {
			Items []knowledge.Comment `json:"items"`
		}
		comments = data[struct {
			Items []knowledge.Comment `json:"items"`
		}](t, result)
		if len(comments.Items) != 1 || comments.Items[0].ID != publishedComment.ID {
			t.Fatal("draft discussion leaked through published post")
		}
		rec, result = f.request("GET", "/v1/comments/"+draftComment.ID+"/history", bob.AccessToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.request("GET", "/v1/posts/"+post.ID+"/revisions", bob.AccessToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.request("PATCH", "/v1/posts/"+post.ID, f.aliceToken, core.NewID(), patch, nil)
		expect(t, rec, 409, result)
	})
	t.Run("history restoration creates a new revision and publish checks its exact base", func(t *testing.T) {
		f := f.with(t)
		stale := map[string]any{"expected_version": post.Version, "base_revision_id": first, "action": "publish", "reason": "Cannot publish an old base"}
		rec, result := f.request("POST", "/v1/posts/"+post.ID+"/publish", f.aliceToken, core.NewID(), stale, nil)
		expect(t, rec, 409, result)
		rec, result = f.request("POST", "/v1/posts/"+post.ID+"/restore", f.aliceToken, core.NewID(), map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "revision_id": first, "reason": "恢复已审核版本作为新草稿"}, nil)
		expect(t, rec, 200, result)
		post = data[knowledge.Post](t, result)
		if post.CurrentRevisionID == first || post.Body != "公开给项目成员的正文" {
			t.Fatal("restore overwrote history instead of creating a new revision")
		}
		rec, result = f.request("GET", "/v1/posts/"+post.ID+"/revisions", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		history := data[core.Page[knowledge.Revision]](t, result)
		if len(history.Items) != 3 {
			t.Fatal("immutable revision history missing", len(history.Items))
		}
		action := map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "action": "publish", "reason": "发布恢复后的新修订"}
		rec, result = f.request("POST", "/v1/posts/"+post.ID+"/publish", f.aliceToken, core.NewID(), action, nil)
		expect(t, rec, 200, result)
		post = data[knowledge.Post](t, result)
		rec, result = f.request("GET", "/v1/posts/"+post.ID, bob.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		if data[knowledge.Post](t, result).CurrentRevisionID != post.CurrentRevisionID {
			t.Fatal("published revision did not advance")
		}
		var count int
		if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE org_id=$1 AND resource_id=$2 AND action='post.restored' AND before_digest IS NOT NULL AND session_id IS NOT NULL", f.org, post.ID).Scan(&count); err != nil || count != 1 {
			t.Fatal("restoration audit lacks actor session or prior digest", count, err)
		}
	})
	t.Run("comments bind the displayed revision and page within caller and revision", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("POST", "/v1/posts/"+post.ID+"/comments", bob.AccessToken, core.NewID(), map[string]any{"body": "缺少正文依据"}, nil)
		expect(t, rec, 422, result)
		rec, result = f.request("POST", "/v1/posts/"+post.ID+"/comments", bob.AccessToken, core.NewID(), map[string]any{"body": "对旧正文的评论", "expected_context_revision_id": first}, nil)
		expect(t, rec, 409, result)
		rec, result = f.request("POST", "/v1/posts/"+post.ID+"/comments", bob.AccessToken, publishedCommentKey, map[string]any{"body": "发布版的阅读反馈", "expected_context_revision_id": first}, nil)
		expect(t, rec, 409, result)
		for _, body := range []string{"当前阅读反馈一", "当前阅读反馈二", "当前阅读反馈三"} {
			rec, result = f.request("POST", "/v1/posts/"+post.ID+"/comments", bob.AccessToken, core.NewID(), map[string]any{"body": body, "expected_context_revision_id": post.CurrentRevisionID}, nil)
			expect(t, rec, 201, result)
			if data[knowledge.Comment](t, result).AuthorDisplayName == "" {
				t.Fatal("comment author display name is missing")
			}
		}
		rec, result = f.request("GET", "/v1/posts/"+post.ID+"/comments?limit=2", bob.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		page := data[core.Page[knowledge.Comment]](t, result)
		if len(page.Items) != 2 || page.NextCursor == nil {
			t.Fatal("discussion first page has incorrect bounds")
		}
		rec, result = f.request("GET", "/v1/posts/"+post.ID+"/comments?limit=2&cursor="+*page.NextCursor, bob.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		next := data[core.Page[knowledge.Comment]](t, result)
		if len(next.Items) != 1 || next.NextCursor != nil || next.Items[0].ID == page.Items[0].ID || next.Items[0].ID == page.Items[1].ID {
			t.Fatal("discussion pagination duplicated or skipped a current revision comment")
		}
		rec, result = f.request("GET", "/v1/posts/"+post.ID+"/comments?cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
		expect(t, rec, 400, result)
	})
	t.Run("comment edits preserve history and enforce author and version", func(t *testing.T) {
		f := f.with(t)
		task := f.createTask(p, "Discussion context", false)
		rec, result := f.request("POST", "/v1/tasks/"+task.ID+"/comments", f.aliceToken, core.NewID(), map[string]any{"body": "原始评论"}, nil)
		expect(t, rec, 201, result)
		comment := data[knowledge.Comment](t, result)
		key := core.NewID()
		input := map[string]any{"expected_revision": comment.Revision, "body": "修订后的评论"}
		rec, result = f.request("PATCH", "/v1/comments/"+comment.ID, f.aliceToken, key, input, nil)
		expect(t, rec, 201, result)
		newComment := data[knowledge.Comment](t, result)
		rec, result = f.request("PATCH", "/v1/comments/"+comment.ID, f.aliceToken, key, input, nil)
		expect(t, rec, 201, result)
		if data[knowledge.Comment](t, result).ID != newComment.ID {
			t.Fatal("comment replay created another revision")
		}
		rec, result = f.request("PATCH", "/v1/comments/"+comment.ID, f.aliceToken, core.NewID(), input, nil)
		expect(t, rec, 409, result)
		rec, result = f.request("GET", "/v1/comments/"+newComment.ID+"/history", f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		history := data[struct {
			Items []knowledge.Comment `json:"items"`
		}](t, result)
		if len(history.Items) != 2 || history.Items[1].Body != "原始评论" {
			t.Fatal("comment revision overwrote its original")
		}
		rec, result = f.request("GET", "/v1/tasks/"+task.ID, bob.AccessToken, "", nil, nil)
		expect(t, rec, 200, result)
		rec, result = f.request("PATCH", "/v1/comments/"+newComment.ID, bob.AccessToken, core.NewID(), map[string]any{"expected_revision": newComment.Revision, "body": "冒名修改"}, nil)
		expect(t, rec, 404, result)
	})
	t.Run("scope changes, unreadable references and archived writes are rejected", func(t *testing.T) {
		f := f.with(t)
		rec, result := f.request("PATCH", "/v1/posts/"+post.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "classification": "internal"}, nil)
		expect(t, rec, 422, result)
		rec, result = f.request("POST", "/v1/posts", f.aliceToken, core.NewID(), map[string]any{"project_id": p.ID, "type": "article", "title": "Missing reference", "body": "body", "classification": "project_restricted", "source_refs": []knowledge.Source{{Type: "file", ID: core.NewID()}}}, nil)
		expect(t, rec, 404, result)
		rec, result = f.request("POST", "/v1/posts/"+post.ID+"/publish", f.aliceToken, core.NewID(), map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "action": "archive", "reason": "完成后归档"}, nil)
		expect(t, rec, 200, result)
		post = data[knowledge.Post](t, result)
		rec, result = f.request("POST", "/v1/posts/"+post.ID+"/comments", f.aliceToken, core.NewID(), map[string]any{"body": "不能评论归档内容", "expected_context_revision_id": post.CurrentRevisionID}, nil)
		expect(t, rec, 409, result)
		rec, result = f.request("PATCH", "/v1/posts/"+post.ID, f.aliceToken, core.NewID(), map[string]any{"expected_version": post.Version, "base_revision_id": post.CurrentRevisionID, "body": "不能改写归档内容"}, nil)
		expect(t, rec, 409, result)
		rec, result = f.request("GET", "/v1/posts/"+post.ID, f.memberToken, "", nil, nil)
		expect(t, rec, 404, result)
	})
}
