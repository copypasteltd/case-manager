package httpapi

import (
	"context"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/knowledge"
	"copypaste.hk/platform/internal/notification"
)

func taskMentionFor(t *testing.T, f *fixture, token, taskID string) []notification.Notification {
	t.Helper()
	rec, res := f.request("GET", "/v1/notifications?category=mention&limit=50", token, "", nil, nil)
	expect(t, rec, 200, res)
	out := []notification.Notification{}
	for _, n := range data[core.Page[notification.Notification]](t, res).Items {
		if n.Target.ID == taskID {
			out = append(out, n)
		}
	}
	return out
}

func TestTaskCommentMentions(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := f.createProject("Explicit task discussion recipients", []string{f.bob})
	task := f.createTask(p, "A discussion is independent of assignment", false)
	create := func(body map[string]any) knowledge.Comment {
		t.Helper()
		rec, res := f.request("POST", "/v1/tasks/"+task.ID+"/comments", f.aliceToken, core.NewID(), body, nil)
		expect(t, rec, 201, res)
		return data[knowledge.Comment](t, res)
	}
	edit := func(c knowledge.Comment, body map[string]any) knowledge.Comment {
		t.Helper()
		body["expected_revision"] = c.Revision
		rec, res := f.request("PATCH", "/v1/comments/"+c.ID, f.aliceToken, core.NewID(), body, nil)
		expect(t, rec, 201, res)
		return data[knowledge.Comment](t, res)
	}
	rec, res := f.request("GET", "/v1/tasks/"+task.ID+"/mention-candidates?limit=1", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	candidates := data[core.Page[knowledge.MentionCandidate]](t, res)
	if len(candidates.Items) != 1 || candidates.Items[0].ID != f.bob {
		t.Fatal("candidate selection must use eligible project members", candidates)
	}
	create(map[string]any{"body": "Literal @Bob has no implicit recipient"})
	drainNotifications(t, f)
	if len(taskMentionFor(t, f, f.bobToken, task.ID)) != 0 {
		t.Fatal("freeform prose generated a mention")
	}
	first := create(map[string]any{"body": "Please review this decision", "mentioned_user_ids": []string{f.bob}})
	drainNotifications(t, f)
	notices := taskMentionFor(t, f, f.bobToken, task.ID)
	if len(notices) != 1 || notices[0].Target.CommentID == nil || *notices[0].Target.CommentID != first.ID || !notices[0].AttentionRequired || notices[0].Read {
		t.Fatal("explicit mention was not delivered to non-assignee", notices)
	}
	originalNotice := notices[0].ID
	second := edit(first, map[string]any{"body": "Clarified, preserving recipients"})
	drainNotifications(t, f)
	notices = taskMentionFor(t, f, f.bobToken, task.ID)
	if len(second.MentionedUserIDs) != 1 || len(notices) != 1 || notices[0].ID != originalNotice || *notices[0].Target.CommentID != second.ID || notices[0].DeliverySeq != 1 {
		t.Fatal("text edit repeated or lost a mention", notices)
	}
	rec, res = f.request("GET", "/v1/comments/"+second.ID, f.bobToken, "", nil, nil)
	expect(t, rec, 200, res)
	if data[knowledge.Comment](t, res).Body != second.Body {
		t.Fatal("exact comment read did not return current target")
	}
	rec, res = f.request("POST", "/v1/notifications/"+originalNotice+"/read", f.bobToken, core.NewID(), map[string]any{"delivery_seq": 1}, nil)
	expect(t, rec, 200, res)
	third := edit(second, map[string]any{"body": "No longer requesting review", "mentioned_user_ids": []string{}})
	drainNotifications(t, f)
	if len(taskMentionFor(t, f, f.bobToken, task.ID)) != 0 {
		t.Fatal("removed mention still visible")
	}
	fourth := edit(third, map[string]any{"body": "A new explicit request", "mentioned_user_ids": []string{f.bob}})
	drainNotifications(t, f)
	notices = taskMentionFor(t, f, f.bobToken, task.ID)
	if len(notices) != 1 || notices[0].ID == originalNotice || *notices[0].Target.CommentID != fourth.ID {
		t.Fatal("re-mention resurrected the removed notification", notices)
	}
	rec, res = f.request("GET", "/v1/notifications/"+originalNotice, f.bobToken, "", nil, nil)
	expect(t, rec, 404, res)
	rec, res = f.request("GET", "/v1/comments/"+fourth.ID+"/history?limit=2", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	history := data[core.Page[knowledge.Comment]](t, res)
	if len(history.Items) != 2 || history.NextCursor == nil {
		t.Fatal("revision history lost pagination", history)
	}
	rec, res = f.request("GET", "/v1/comments/"+fourth.ID+"/history?limit=2&cursor="+*history.NextCursor, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	if len(data[core.Page[knowledge.Comment]](t, res).Items) != 2 {
		t.Fatal("history continuation lost original revisions")
	}
	// Queue delivery, then remove that recipient before any worker sees the event.
	queued := create(map[string]any{"body": "Withdraw before delivery", "mentioned_user_ids": []string{f.bob}})
	edit(queued, map[string]any{"body": "Withdrawn", "mentioned_user_ids": []string{}})
	drainNotifications(t, f)
	if len(taskMentionFor(t, f, f.bobToken, task.ID)) != 1 {
		t.Fatal("stale queued mention was delivered")
	}
	var count int
	if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_deliveries WHERE org_id=$1 AND recipient_id=$2", f.org, f.bob).Scan(&count); err != nil || count != 2 {
		t.Fatal("unexpected duplicate delivery rows", count, err)
	}
}

func TestTaskCommentMentionAuthority(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := f.createProject("Mention authority", []string{f.bob})
	task := f.createTask(p, "Mention target", false)
	for _, ids := range [][]string{{f.alice}, {f.bob, f.bob}, {f.member}, {core.NewID()}} {
		rec, res := f.request("POST", "/v1/tasks/"+task.ID+"/comments", f.aliceToken, core.NewID(), map[string]any{"body": "Invalid recipient", "mentioned_user_ids": ids}, nil)
		if rec.Code < 400 || rec.Code >= 500 {
			t.Fatal("invalid recipient was not rejected", rec.Code, res)
		}
	}
	key := core.NewID()
	body := map[string]any{"body": "Keep immutable recipients", "mentioned_user_ids": []string{f.bob}}
	rec, res := f.request("POST", "/v1/tasks/"+task.ID+"/comments", f.aliceToken, key, body, nil)
	expect(t, rec, 201, res)
	first := data[knowledge.Comment](t, res)
	// Domain mutation uses the authorization barrier and invalidates Bob's session.
	rec, res = f.request("POST", "/v1/projects/"+p.ID+"/members", f.aliceToken, core.NewID(), map[string]any{"expected_version": p.Version, "user_id": f.bob, "action": "remove", "reason": "Mention authority regression"}, nil)
	expect(t, rec, 200, res)
	drainNotifications(t, f)
	var delivered int
	if err := f.owner.QueryRow(ctx, "SELECT count(*) FROM notification_deliveries WHERE org_id=$1 AND recipient_id=$2", f.org, f.bob).Scan(&delivered); err != nil || delivered != 0 {
		t.Fatal("queued mention bypassed revoked membership", delivered, err)
	}
	rec, res = f.request("GET", "/v1/comments/"+first.ID, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	filtered := data[knowledge.Comment](t, res)
	if !filtered.MentionsFiltered || len(filtered.MentionedUserIDs) != 0 {
		t.Fatal("ineligible original recipient leaked", filtered)
	}
	// Text-only revision keeps the original array in storage, but filters its receipt.
	editBody := map[string]any{"body": "Text edit after revocation", "expected_revision": first.Revision}
	editKey := core.NewID()
	rec, res = f.request("PATCH", "/v1/comments/"+first.ID, f.aliceToken, editKey, editBody, nil)
	expect(t, rec, 201, res)
	edited := data[knowledge.Comment](t, res)
	if !edited.MentionsFiltered || len(edited.MentionedUserIDs) != 0 {
		t.Fatal("filtered mutation receipt leaked recipients")
	}
	var stored []string
	if err := f.owner.QueryRow(ctx, "SELECT mentioned_user_ids FROM comments WHERE org_id=$1 AND id=$2", f.org, edited.ID).Scan(&stored); err != nil || len(stored) != 1 || stored[0] != f.bob {
		t.Fatal("text-only edit lost hidden immutable recipients", stored, err)
	}
	rec, res = f.request("PATCH", "/v1/comments/"+first.ID, f.aliceToken, editKey, editBody, nil)
	expect(t, rec, 201, res)
	if c := data[knowledge.Comment](t, res); c.ID != edited.ID || !c.MentionsFiltered {
		t.Fatal("replay bypassed visibility")
	}
	rec, res = f.request("POST", "/v1/tasks/"+task.ID+"/comments", f.aliceToken, key, body, nil)
	expect(t, rec, 403, res)
	rec, res = f.request("PATCH", "/v1/comments/"+edited.ID, f.aliceToken, core.NewID(), map[string]any{"body": "Filtered list must not replace hidden originals", "expected_revision": edited.Revision, "mentioned_user_ids": []string{}}, nil)
	expect(t, rec, 403, res)
	// The immutable table guard also rejects bypasses through direct database writes.
	if _, err := f.owner.Exec(ctx, "UPDATE comments SET mentioned_user_ids='{}' WHERE org_id=$1 AND id=$2", f.org, first.ID); err == nil {
		t.Fatal("stored comment mentions were mutable")
	}
	if _, err := f.owner.Exec(ctx, `INSERT INTO comments(id,org_id,resource_type,resource_id,author_id,body,revision,mentioned_user_ids) VALUES($1,$2,'task',$3,$4,'ineligible direct write',1,$5)`, core.NewID(), f.org, task.ID, f.alice, []string{f.bob}); err == nil {
		t.Fatal("database admitted revoked new recipient")
	}
}

func TestTaskMentionCandidateBoundaries(t *testing.T) {
	f := setup(t)
	p := f.createProject("Signed mention candidate pages", []string{f.bob, f.member})
	task := f.createTask(p, "Candidate cursor binding", false)
	other := f.createTask(p, "Another task", false)
	rec, res := f.request("GET", "/v1/tasks/"+task.ID+"/mention-candidates?limit=1", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	page := data[core.Page[knowledge.MentionCandidate]](t, res)
	if len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("eligible candidate continuation missing", page)
	}
	rec, res = f.request("GET", "/v1/tasks/"+task.ID+"/mention-candidates?limit=1&cursor="+*page.NextCursor, f.aliceToken, "", nil, nil)
	expect(t, rec, 200, res)
	next := data[core.Page[knowledge.MentionCandidate]](t, res)
	if len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID || next.NextCursor != nil {
		t.Fatal("candidate continuation repeated or invented members", next)
	}
	for _, path := range []string{
		"/v1/tasks/" + other.ID + "/mention-candidates?limit=1&cursor=" + *page.NextCursor,
		"/v1/tasks/" + task.ID + "/mention-candidates?limit=1&q=Bob&cursor=" + *page.NextCursor,
	} {
		rec, res = f.request("GET", path, f.aliceToken, "", nil, nil)
		if rec.Code < 400 || rec.Code >= 500 {
			t.Fatal("candidate cursor crossed its task or query boundary", rec.Code, res)
		}
	}
	rec, res = f.request("POST", "/v1/posts", f.aliceToken, core.NewID(), map[string]any{"project_id": p.ID, "type": "meeting_note", "classification": "project_restricted", "title": "Post discussions do not create task mentions", "body": "Context"}, nil)
	expect(t, rec, 201, res)
	post := data[knowledge.Post](t, res)
	rec, res = f.request("POST", "/v1/posts/"+post.ID+"/comments", f.aliceToken, core.NewID(), map[string]any{"body": "Unsupported post mention", "expected_context_revision_id": post.CurrentRevisionID, "mentioned_user_ids": []string{f.bob}}, nil)
	expect(t, rec, 422, res)
	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = core.NewID()
	}
	rec, res = f.request("POST", "/v1/tasks/"+task.ID+"/comments", f.aliceToken, core.NewID(), map[string]any{"body": "Exceeds fixed recipient bound", "mentioned_user_ids": tooMany}, nil)
	expect(t, rec, 422, res)
}
