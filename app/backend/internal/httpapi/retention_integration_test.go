package httpapi

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/knowledge"
)

func TestRetentionManagement(t *testing.T) {
	ctx := context.Background()
	f, old, current, _, _ := restorationFixture(t, 15)
	basePath := "/v1/files/" + current.File.ID
	holdInput := map[string]any{"expected_version": current.File.Version, "version_id": old.Version.ID, "kind": "legal", "reason": "fixed historical evidence"}
	rec, result := f.f.request("POST", basePath+"/retention-holds", f.f.aliceToken, core.NewID(), holdInput, nil)
	expect(t, rec, 403, result)
	if _, err := f.f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling)
        SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='file_retention_manager'`, core.NewID(), f.f.org, f.f.alice); err != nil {
		t.Fatal(err)
	}
	rec, result = f.f.request("POST", basePath+"/retention-holds", f.f.aliceToken, core.NewID(), holdInput, nil)
	expect(t, rec, 401, result) // A newly required MFA role rejects the old unverified session.
	f.f.aliceToken = memberMFAToken(f.f, "alice", "android")
	var hold storage.RetentionHold
	t.Run("fixed history hold blocks trash and release is immutable idempotent evidence", func(t *testing.T) {
		key := core.NewID()
		rec, result := f.f.request("POST", basePath+"/retention-holds", f.f.aliceToken, key, holdInput, nil)
		expect(t, rec, 201, result)
		hold = data[storage.RetentionHold](t, result)
		rec, result = f.f.request("POST", basePath+"/retention-holds", f.f.aliceToken, key, holdInput, nil)
		expect(t, rec, 201, result)
		if data[storage.RetentionHold](t, result).ID != hold.ID {
			t.Fatal("hold replay duplicated")
		}
		rec, result = f.f.request("GET", basePath+"/retention", f.f.bobToken, "", nil, nil)
		expect(t, rec, 200, result)
		view := data[storage.RetentionOverview](t, result)
		if view.ActiveHoldCount != 1 || view.CanManage || view.ExecutionAvailable {
			t.Fatal(view)
		}
		rec, result = f.f.request("GET", basePath+"/retention-holds", f.f.bobToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.f.request("POST", basePath+"/trash", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": current.File.Version, "action": "trash", "reason": "must remain retained"}, nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "RETENTION_HOLD" {
			t.Fatal(result.Error)
		}
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE retention_holds SET reason='tampered' WHERE id=$1", hold.ID); err == nil {
			t.Fatal("hold modified")
		}
		rec, result = f.f.request("POST", "/v1/file-versions/"+old.Version.ID+"/retention-assessments", f.f.aliceToken, core.NewID(), map[string]any{"expected_file_version": current.File.Version, "reason": "before release"}, nil)
		expect(t, rec, 201, result)
		assessment := data[storage.RetentionAssessment](t, result)
		if !slices.Contains(assessment.Snapshot.BlockingReasons, "RETENTION_HOLD") || !slices.Contains(assessment.Snapshot.BlockingReasons, "POLICY_REQUIRED") {
			t.Fatal(assessment)
		}
		releaseKey := core.NewID()
		release := map[string]string{"reason": "case resolved; preserve release evidence"}
		for range 2 {
			rec, result = f.f.request("POST", "/v1/retention-holds/"+hold.ID+"/release", f.f.aliceToken, releaseKey, release, nil)
			expect(t, rec, 200, result)
		}
		released := data[storage.RetentionHold](t, result)
		if released.Active || released.ReleasedAt == nil || released.ReleaseReason == nil {
			t.Fatal(released)
		}
		var n int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM retention_hold_releases WHERE hold_id=$1", hold.ID).Scan(&n); err != nil || n != 1 {
			t.Fatal(n, err)
		}
		rec, result = f.f.request("GET", "/v1/retention-assessments/"+assessment.ID, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		if data[storage.RetentionAssessment](t, result).Snapshot.ActiveHoldCount != 1 {
			t.Fatal("past assessment rewritten after release")
		}
		if _, err := f.f.api.DB.Exec(ctx, "DELETE FROM retention_hold_releases WHERE hold_id=$1", hold.ID); err == nil {
			t.Fatal("release evidence removed")
		}
	})
	t.Run("knowledge fixed revisions and comment attachments register in their business transaction", func(t *testing.T) {
		rec, result := f.f.request("POST", "/v1/posts", f.f.aliceToken, core.NewID(), map[string]any{
			"project_id": current.File.ProjectID, "type": "article", "title": "Retained attachment", "body": "fixed original evidence",
			"classification": "project_restricted", "source_refs": []map[string]string{{"type": "file", "id": old.File.ID, "version_id": old.Version.ID}}}, nil)
		expect(t, rec, 201, result)
		post := data[knowledge.Post](t, result)
		rec, result = f.f.request("POST", "/v1/posts/"+post.ID+"/comments", f.f.aliceToken, core.NewID(), map[string]any{
			"body": "fixed comment evidence", "expected_context_revision_id": post.CurrentRevisionID, "attachment_versions": []string{old.Version.ID}}, nil)
		expect(t, rec, 201, result)
		rec, result = f.f.request("POST", "/v1/file-versions/"+old.Version.ID+"/retention-assessments", f.f.aliceToken, core.NewID(), map[string]any{"expected_file_version": current.File.Version, "reason": "reference inventory"}, nil)
		expect(t, rec, 201, result)
		assessment := data[storage.RetentionAssessment](t, result)
		if assessment.Snapshot.ReferenceCounts["knowledge_revision"] != 1 || assessment.Snapshot.ReferenceCounts["comment"] != 1 ||
			!slices.Contains(assessment.Snapshot.BlockingReasons, "BUSINESS_REFERENCE") || assessment.Snapshot.ExecutionAvailable {
			t.Fatal(assessment)
		}
		raw, _ := json.Marshal(assessment.Snapshot)
		canonical, _ := core.CanonicalJSON(raw)
		if core.Digest(canonical) != assessment.SnapshotSHA256 {
			t.Fatal("assessment digest not canonical")
		}
		if _, err := f.f.api.DB.Exec(ctx, "DELETE FROM file_version_references WHERE version_id=$1", old.Version.ID); err == nil {
			t.Fatal("pinned evidence removed")
		}
		before := int64(0)
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE version_id=$1", old.Version.ID).Scan(&before); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", "/v1/posts", f.f.aliceToken, core.NewID(), map[string]any{
			"project_id": current.File.ProjectID, "type": "article", "title": "Follow current", "body": "resolves at read time",
			"classification": "project_restricted", "source_refs": []map[string]string{{"type": "file", "id": old.File.ID}}}, nil)
		expect(t, rec, 201, result)
		var after int64
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM file_version_references WHERE version_id=$1", old.Version.ID).Scan(&after); err != nil || before != after {
			t.Fatal("follow-current unexpectedly pinned old bytes", before, after, err)
		}
	})
	t.Run("policy publication requires recent MFA and creates immutable independent versions", func(t *testing.T) {
		admin := f.f.administratorToken()
		input := map[string]any{"expected_version": 0, "kind": "formal_history", "enabled": true, "retain_seconds": int64(86400), "observation_seconds": int64(86400), "reason": "isolated explicit assessment policy, no production approval"}
		rec, result := f.f.request("POST", "/v1/storage/retention-policies", admin, core.NewID(), input, nil)
		expect(t, rec, 201, result)
		policy := data[storage.RetentionPolicy](t, result)
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE retention_policies SET enabled=false WHERE id=$1", policy.ID); err == nil {
			t.Fatal("policy mutated")
		}
		rec, result = f.f.request("POST", "/v1/storage/retention-policies", admin, core.NewID(), input, nil)
		expect(t, rec, 409, result)
		rec, result = f.f.request("POST", "/v1/file-versions/"+current.Version.ID+"/retention-assessments", f.f.aliceToken, core.NewID(), map[string]any{"expected_file_version": current.File.Version, "reason": "new version protected"}, nil)
		expect(t, rec, 201, result)
		assessment := data[storage.RetentionAssessment](t, result)
		if assessment.Snapshot.Policy == nil || assessment.Snapshot.Policy.ID != policy.ID ||
			!slices.Contains(assessment.Snapshot.BlockingReasons, "CURRENT_VERSION") || !slices.Contains(assessment.Snapshot.BlockingReasons, "RETENTION_PERIOD_ACTIVE") {
			t.Fatal(assessment)
		}
		input["expected_version"] = 1
		input["enabled"] = false
		rec, result = f.f.request("POST", "/v1/storage/retention-policies", admin, core.NewID(), input, nil)
		expect(t, rec, 201, result)
		rec, result = f.f.request("POST", "/v1/file-versions/"+old.Version.ID+"/retention-assessments", f.f.aliceToken, core.NewID(), map[string]any{"expected_file_version": current.File.Version, "reason": "latest policy required"}, nil)
		expect(t, rec, 201, result)
		assessment = data[storage.RetentionAssessment](t, result)
		if assessment.Snapshot.Policy.Version != 2 || !slices.Contains(assessment.Snapshot.BlockingReasons, "POLICY_DISABLED") {
			t.Fatal(assessment)
		}
		rec, result = f.f.request("GET", "/v1/storage/retention-policies", admin, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[storage.RetentionPolicy]](t, result).Items) != 1 {
			t.Fatal("unpublished policy kinds invented")
		}
	})
	t.Run("trash adds its own retention policy without replacing the formal history protection", func(t *testing.T) {
		rec, result := f.f.request("POST", basePath+"/trash", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": current.File.Version, "action": "trash", "reason": "verify combined policies"}, nil)
		expect(t, rec, 200, result)
		trashed := data[storage.File](t, result)
		rec, result = f.f.request("POST", "/v1/file-versions/"+old.Version.ID+"/retention-assessments", f.f.aliceToken, core.NewID(), map[string]any{"expected_file_version": trashed.Version, "reason": "both independent retention windows apply"}, nil)
		expect(t, rec, 201, result)
		a := data[storage.RetentionAssessment](t, result)
		if a.Snapshot.Policy == nil || a.Snapshot.Policy.Kind != "formal_history" || a.Snapshot.Policy.Version != 2 || a.Snapshot.TrashPolicy != nil ||
			!slices.Contains(a.Snapshot.BlockingReasons, "POLICY_REQUIRED") || !slices.Contains(a.Snapshot.BlockingReasons, "POLICY_DISABLED") {
			t.Fatal(a)
		}
		rec, result = f.f.request("POST", basePath+"/restore", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": trashed.Version, "mode": "untrash", "reason": "finish isolated policy check"}, nil)
		expect(t, rec, 200, result)
		current.File = data[storage.File](t, result)
		holdInput["expected_version"] = current.File.Version
	})
	t.Run("hold creation and assessment audit failures roll back all business evidence", func(t *testing.T) {
		if _, err := f.f.owner.Exec(ctx, `CREATE FUNCTION reject_retention_fixture() RETURNS trigger LANGUAGE plpgsql AS $x$
            BEGIN IF NEW.action IN ('file.retention_hold.created','file.retention.assessed','file.retention_hold.released') THEN RAISE EXCEPTION 'isolated retention audit failure'; END IF; RETURN NEW; END $x$;
            CREATE TRIGGER reject_retention_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_retention_fixture()`); err != nil {
			t.Fatal(err)
		}
		var before, after int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM retention_holds WHERE file_id=$1", current.File.ID).Scan(&before); err != nil {
			t.Fatal(err)
		}
		key := core.NewID()
		rec, result := f.f.request("POST", basePath+"/retention-holds", f.f.aliceToken, key, holdInput, nil)
		expect(t, rec, 500, result)
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM retention_holds WHERE file_id=$1", current.File.ID).Scan(&after); err != nil || before != after {
			t.Fatal("partial hold", before, after, err)
		}
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM retention_assessments WHERE file_id=$1", current.File.ID).Scan(&before); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", "/v1/file-versions/"+old.Version.ID+"/retention-assessments", f.f.aliceToken, core.NewID(), map[string]any{"expected_file_version": current.File.Version, "reason": "must roll back"}, nil)
		expect(t, rec, 500, result)
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM retention_assessments WHERE file_id=$1", current.File.ID).Scan(&after); err != nil || before != after {
			t.Fatal("partial assessment", before, after, err)
		}
		if _, err := f.f.owner.Exec(ctx, "DROP TRIGGER reject_retention_fixture ON audit_events; DROP FUNCTION reject_retention_fixture()"); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", basePath+"/retention-holds", f.f.aliceToken, key, holdInput, nil)
		expect(t, rec, 201, result)
		hold = data[storage.RetentionHold](t, result)
	})
	t.Run("concurrent release has one final immutable outcome and preserves global legal hold", func(t *testing.T) {
		if _, err := f.f.owner.Exec(ctx, "UPDATE files SET legal_hold=true WHERE org_id=$1 AND id=$2", f.f.org, current.File.ID); err != nil {
			t.Fatal(err)
		}
		statuses := make(chan int, 2)
		var group sync.WaitGroup
		for range 2 {
			group.Add(1)
			go func() {
				defer group.Done()
				rec, _ := f.f.request("POST", "/v1/retention-holds/"+hold.ID+"/release", f.f.aliceToken, core.NewID(), map[string]string{"reason": "concurrent release"}, nil)
				statuses <- rec.Code
			}()
		}
		group.Wait()
		close(statuses)
		seen := map[int]int{}
		for n := range statuses {
			seen[n]++
		}
		if seen[200] != 1 || seen[409] != 1 {
			t.Fatal(seen)
		}
		rec, result := f.f.request("GET", basePath+"/retention", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		view := data[storage.RetentionOverview](t, result)
		if !view.LegalHold || view.ActiveHoldCount != 0 {
			t.Fatal(view)
		}
		rec, result = f.f.request("POST", basePath+"/trash", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": current.File.Version, "action": "trash", "reason": "global legal flag remains"}, nil)
		expect(t, rec, 409, result)
	})
	t.Run("revoked managers cannot replay a saved assessment and hold paging binds identity", func(t *testing.T) {
		rec, result := f.f.request("GET", basePath+"/retention-holds?limit=1", f.f.aliceToken, "", nil, nil)
		expect(t, rec, 200, result)
		page := data[core.Page[storage.RetentionHold]](t, result)
		if len(page.Items) != 1 || page.NextCursor == nil {
			t.Fatal("hold paging", page)
		}
		key := core.NewID()
		input := map[string]any{"expected_file_version": current.File.Version, "reason": "before revoke"}
		rec, result = f.f.request("POST", "/v1/file-versions/"+old.Version.ID+"/retention-assessments", f.f.aliceToken, key, input, nil)
		expect(t, rec, 201, result)
		assessment := data[storage.RetentionAssessment](t, result)
		if _, err := f.f.owner.Exec(ctx, `UPDATE role_bindings SET revoked_at=clock_timestamp() WHERE org_id=$1 AND user_id=$2
            AND role_id IN (SELECT id FROM roles WHERE org_id=$1 AND key='file_retention_manager')`, f.f.org, f.f.alice); err != nil {
			t.Fatal(err)
		}
		rec, result = f.f.request("POST", "/v1/file-versions/"+old.Version.ID+"/retention-assessments", f.f.aliceToken, key, input, nil)
		expect(t, rec, 404, result)
		rec, result = f.f.request("GET", "/v1/retention-assessments/"+assessment.ID, f.f.aliceToken, "", nil, nil)
		expect(t, rec, 404, result)
		rec, result = f.f.request("GET", basePath+"/retention-holds?cursor="+*page.NextCursor, f.f.bobToken, "", nil, nil)
		expect(t, rec, 404, result)
	})
}
