package httpapi

import (
	"context"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/knowledge"
)

func TestCommentAttachmentPreservation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	p := f.createProject("Comment attachment preservation", []string{f.bob})
	var root string
	if err := f.owner.QueryRow(ctx, "SELECT id FROM folders WHERE org_id=$1 AND project_id=$2 AND parent_id IS NULL", f.org, p.ID).Scan(&root); err != nil {
		t.Fatal(err)
	}
	fileID, versionID := core.NewID(), core.NewID()
	for _, query := range []string{
		"INSERT INTO storage_epochs(org_id,epoch,active_primary,changed_by,fence_evidence) VALUES($1,1,'oss',$2,'comment metadata fixture')",
	} {
		if _, err := f.owner.Exec(ctx, query, f.org, f.admin); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.owner.Exec(ctx, "INSERT INTO files(id,org_id,folder_id,name,name_key,classification,state,creator_id) VALUES($1,$2,$3,'comment.txt','comment.txt','project_restricted','active',$4)", fileID, f.org, root, f.alice); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, `INSERT INTO file_versions(id,org_id,file_id,seq,object_key,provider_source_id,storage_epoch,size_bytes,sha256,media_type,source,author_id,executor_id,scan_state,availability,original_name)
		VALUES($1,$2,$3,1,'comment/fixture','metadata-only',1,12,$4,'text/plain','comment-test',$5,$5,'clean','available','comment.txt')`, versionID, f.org, fileID, strings.Repeat("a", 64), f.alice); err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(ctx, "UPDATE files SET current_version_id=$3 WHERE org_id=$1 AND id=$2", f.org, fileID, versionID); err != nil {
		t.Fatal(err)
	}
	rec, result := f.request("POST", "/v1/posts", f.aliceToken, core.NewID(), map[string]any{"project_id": p.ID, "type": "meeting_note", "classification": "project_restricted", "title": "Attachment discussion", "body": "Immutable context"}, nil)
	expect(t, rec, 201, result)
	post := data[knowledge.Post](t, result)
	rec, result = f.request("POST", "/v1/posts/"+post.ID+"/comments", f.aliceToken, core.NewID(), map[string]any{"body": "Original", "expected_context_revision_id": post.CurrentRevisionID, "attachment_versions": []string{versionID}}, nil)
	expect(t, rec, 201, result)
	first := data[knowledge.Comment](t, result)
	if len(first.AttachmentVersions) != 1 {
		t.Fatal("missing initial attachment")
	}
	if _, err := f.owner.Exec(ctx, "UPDATE files SET state='trashed',trashed_at=now() WHERE org_id=$1 AND id=$2", f.org, fileID); err != nil {
		t.Fatal(err)
	}
	rec, result = f.request("GET", "/v1/posts/"+post.ID+"/comments", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, result)
	page := data[core.Page[knowledge.Comment]](t, result)
	if len(page.Items) != 1 || len(page.Items[0].AttachmentVersions) != 0 {
		t.Fatal("hidden attachment exposed")
	}
	body := map[string]any{"body": "Text only; preserve originals", "expected_revision": first.Revision, "expected_context_revision_id": post.CurrentRevisionID}
	key := core.NewID()
	rec, result = f.request("PATCH", "/v1/comments/"+first.ID, f.aliceToken, key, body, nil)
	expect(t, rec, 201, result)
	second := data[knowledge.Comment](t, result)
	if second.ID == first.ID || second.Revision != 2 || len(second.AttachmentVersions) != 0 {
		t.Fatal("unsafe mutation response", second)
	}
	var stored []string
	if err := f.owner.QueryRow(ctx, "SELECT attachment_versions FROM comments WHERE org_id=$1 AND id=$2", f.org, second.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0] != versionID {
		t.Fatal("text edit lost an inaccessible original")
	}
	// The idempotent receipt must reapply current visibility to its stored data.
	rec, result = f.request("PATCH", "/v1/comments/"+first.ID, f.aliceToken, key, body, nil)
	expect(t, rec, 201, result)
	replay := data[knowledge.Comment](t, result)
	if replay.ID != second.ID || len(replay.AttachmentVersions) != 0 {
		t.Fatal("replay leaked hidden pins or created another edit")
	}
	replacement := map[string]any{"body": "Explicit clear", "expected_revision": second.Revision, "expected_context_revision_id": post.CurrentRevisionID, "attachment_versions": []string{}}
	rec, result = f.request("PATCH", "/v1/comments/"+second.ID, f.aliceToken, core.NewID(), replacement, nil)
	expect(t, rec, 404, result)
	if _, err := f.owner.Exec(ctx, "UPDATE files SET state='active',trashed_at=NULL WHERE org_id=$1 AND id=$2", f.org, fileID); err != nil {
		t.Fatal(err)
	}
	rec, result = f.request("GET", "/v1/posts/"+post.ID+"/comments", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, result)
	page = data[core.Page[knowledge.Comment]](t, result)
	if len(page.Items) != 1 || page.Items[0].ID != second.ID || len(page.Items[0].AttachmentVersions) != 1 || page.Items[0].AttachmentVersions[0] != versionID {
		t.Fatal("preserved original did not reappear")
	}
	rec, result = f.request("PATCH", "/v1/comments/"+second.ID, f.aliceToken, core.NewID(), replacement, nil)
	expect(t, rec, 201, result)
	third := data[knowledge.Comment](t, result)
	if third.Revision != 3 || len(third.AttachmentVersions) != 0 {
		t.Fatal("explicit visible clear failed")
	}
	rec, result = f.request("GET", "/v1/comments/"+third.ID+"/history", f.aliceToken, "", nil, nil)
	expect(t, rec, 200, result)
	history := data[core.Page[knowledge.Comment]](t, result)
	if len(history.Items) != 3 {
		t.Fatal("immutable edit history was lost")
	}
	for _, item := range history.Items {
		if item.ID == first.ID && (item.Body != "Original" || len(item.AttachmentVersions) != 1) {
			t.Fatal("original revision changed")
		}
	}
}
