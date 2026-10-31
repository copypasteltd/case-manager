package httpapi

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
)

// Real HTTP upload/hash/scan/promotion, durable native PG and ordinary commands.
// Only cloud transport, native-version attestation and elapsed clocks are synthetic.
func completedUploadFixture(t *testing.T, replacement bool, channel string) (*retirementFixture, storage.Upload) {
	f := newRetirementFixture(t, "android")
	var u storage.Upload
	if replacement {
		r, v := f.request("POST", "/v1/uploads", core.NewID(), map[string]any{"folder_id": f.f.root, "name": "Unsubmitted replacement.bin", "size_bytes": "17", "media_type": "application/octet-stream", "source_fingerprint": "completed-retirement-replacement", "file_id": f.current.File.ID, "base_version_id": f.current.Version.ID})
		expect(t, r, 201, v)
		u = readyImport(t, f.f, data[storage.Upload](t, v))
	} else {
		u, _, _ = readyUpload(f.f, "Unsubmitted new file.bin", 17)
	}
	ageCompletedUpload(t, f, u.ID)
	u = f.f.get(u.ID)
	if channel != "android" {
		retirementRenewalLogin(f, channel)
	}
	return f, u
}
func ageCompletedUpload(t *testing.T, f *retirementFixture, id string) {
	ctx := context.Background()
	tx, e := f.f.f.owner.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	for _, q := range []string{"ALTER TABLE upload_sessions DISABLE TRIGGER upload_input_immutable", "ALTER TABLE upload_sessions DISABLE TRIGGER upload_candidate_immutable", "ALTER TABLE upload_sessions DISABLE TRIGGER upload_completion_observation", "ALTER TABLE object_transfers DISABLE TRIGGER transfer_inputs_immutable", "ALTER TABLE object_transfers DISABLE TRIGGER transfer_frozen_immutable", "ALTER TABLE object_transfers DISABLE TRIGGER transfer_completion_observation"} {
		if _, e = tx.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	var raw string
	var original objectstore.Source
	if e = tx.QueryRow(ctx, "SELECT frozen_source_id FROM upload_sessions WHERE id=$1", id).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal([]byte(raw), &original); e != nil {
		t.Fatal(e)
	}
	original.VersionID = "synthetic-completed-native-draft"
	frozen, _ := json.Marshal(original)
	if _, e = tx.Exec(ctx, "UPDATE upload_sessions SET frozen_source_id=$2,frozen_native_version=$3,expires_at=clock_timestamp()-interval '3 days',completed_observed_at=clock_timestamp()-interval '4 days' WHERE id=$1", id, string(frozen), original.VersionID); e != nil {
		t.Fatal(e)
	}
	var transferID string
	var candidate objectstore.Source
	if e = tx.QueryRow(ctx, "SELECT id,frozen_source FROM object_transfers WHERE session_id=$1 AND state='complete'", id).Scan(&transferID, &raw); e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal([]byte(raw), &candidate); e != nil {
		t.Fatal(e)
	}
	candidate.VersionID = "synthetic-completed-native-candidate"
	candidateRaw, _ := json.Marshal(candidate)
	if _, e = tx.Exec(ctx, "UPDATE object_transfers SET frozen_source=$2,source_digest=$3,completed_observed_at=clock_timestamp()-interval '3 days' WHERE id=$1", transferID, candidateRaw, core.Digest(frozen)); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "UPDATE upload_sessions SET candidate_source_id=$2 WHERE id=$1", id, string(candidateRaw)); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "UPDATE jobs SET updated_at=clock_timestamp()-interval '2 days' WHERE org_id=$1 AND payload->>'session_id'=$2::text", f.f.f.org, id); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"ALTER TABLE upload_sessions ENABLE TRIGGER upload_input_immutable", "ALTER TABLE upload_sessions ENABLE TRIGGER upload_candidate_immutable", "ALTER TABLE upload_sessions ENABLE TRIGGER upload_completion_observation", "ALTER TABLE object_transfers ENABLE TRIGGER transfer_inputs_immutable", "ALTER TABLE object_transfers ENABLE TRIGGER transfer_frozen_immutable", "ALTER TABLE object_transfers ENABLE TRIGGER transfer_completion_observation"} {
		if _, e = tx.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	f.cloud.sources[original.Key] = original
	f.cloud.sources[candidate.Key] = candidate
}
func completedCandidates(t *testing.T, f *retirementFixture) []storage.RetirementCandidate {
	r, v := f.request("GET", "/v1/folders/"+f.f.root+"/completed-draft-retirement-candidates?limit=20", "", nil)
	if r.Code != 200 {
		a, _ := f.f.f.api.Identity.Authenticate(context.Background(), f.f.f.aliceToken)
		_, e := f.service.CompletedDraftCandidates(context.Background(), a, f.f.root, storage.Filter{})
		t.Logf("completed candidate diagnostic: %v", e)
	}
	expect(t, r, 200, v)
	if strings.Contains(r.Body.String(), "fixture-bucket") || strings.Contains(r.Body.String(), "synthetic-completed-native") {
		t.Fatal("private object metadata disclosed")
	}
	return data[core.Page[storage.RetirementCandidate]](t, v).Items
}

// Additional abandoned promotion completions are synthetic journal fixtures;
// they exercise inventory of every known completion, including non-selected keys.
func addKnownCompletedCandidate(t *testing.T, f *retirementFixture, session string) {
	ctx := context.Background()
	tx, e := f.f.f.owner.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	var fileID string
	if e = tx.QueryRow(ctx, "SELECT file_id FROM object_transfers WHERE session_id=$1 LIMIT 1", session).Scan(&fileID); e != nil {
		t.Fatal(e)
	}
	version := core.NewID()
	key, _ := objectstore.VersionKey(f.f.f.org, fileID, version)
	source := objectstore.Source{Key: key, ETag: "synthetic-abandoned-candidate", VersionID: "synthetic-native-" + version, Size: 17}
	raw, _ := json.Marshal(source)
	if _, e = tx.Exec(ctx, "ALTER TABLE object_transfers DISABLE TRIGGER transfer_completion_observation"); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO object_transfers(id,org_id,session_id,job_id,file_id,version_id,source_digest,object_key,provider_upload_id,frozen_source,state,fencing_token,target_provider,target_bucket,completed_observed_at)
 SELECT gen_random_uuid(),org_id,session_id,job_id,file_id,$2,source_digest,$3,'synthetic-completed', $4,'complete',1,target_provider,target_bucket,clock_timestamp()-interval '3 days' FROM object_transfers WHERE session_id=$1 ORDER BY created_at,id LIMIT 1`, session, version, key, raw); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, "ALTER TABLE object_transfers ENABLE TRIGGER transfer_completion_observation"); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	f.cloud.sources[key] = source
}
func createCompletedPlan(t *testing.T, f *retirementFixture, u storage.Upload) storage.RetirementPlan {
	candidates := completedCandidates(t, f)
	index := slices.IndexFunc(candidates, func(v storage.RetirementCandidate) bool { return v.TargetID == u.ID })
	if index < 0 || !candidates[index].Eligible {
		t.Fatalf("not eligible: %+v", candidates)
	}
	c := candidates[index]
	r, v := f.request("POST", "/v1/folders/"+f.f.root+"/completed-draft-retirement-plans", core.NewID(), storage.CompletedDraftRetirementCreate{ExpectedFolderVersion: c.FolderVersion, ExpectedUploadVersion: c.UploadVersion, TargetID: c.TargetID, Reason: "Remove exact expired uncommitted objects"})
	expect(t, r, 201, v)
	return data[storage.RetirementPlan](t, v)
}
func TestCompletedUploadRetirementFourClients(t *testing.T) {
	for _, channel := range []string{"web", "android", "wechat", "douyin"} {
		t.Run(channel, func(t *testing.T) {
			for _, replacement := range []bool{false, true} {
				name := "new"
				if replacement {
					name = "replacement"
				}
				t.Run(name, func(t *testing.T) {
					f, u := completedUploadFixture(t, replacement, channel)
					if os.Getenv("CP_BROWSER_COMPLETED_UPLOAD_FIXTURE") == "1" && !replacement && channel == "android" {
						holdBrowserBusinessFixture(t, f.f.f, map[string]any{"fixture_kind": "completed-upload-retirement", "folder_id": f.f.root, "upload_id": u.ID, "fixture_scope": "Native disposable PG; real upload/hash/scan/promotion, synthetic native versions and elapsed clocks; no live cloud deletion."})
						var count int
						if e := f.f.f.owner.QueryRow(context.Background(), "SELECT count(*) FROM object_retirement_plans WHERE scope_folder_id=$1 AND target_id=$2", f.f.root, u.ID).Scan(&count); e != nil || count != 1 {
							t.Fatal("browser did not create one durable folder plan", count, e)
						}
						return
					}
					p := createCompletedPlan(t, f, u)
					if p.Snapshot.FolderID != f.f.root || len(p.Items) != 2 || p.Items[0].Provider != p.Items[1].Provider || p.Items[0].Role != "upload_draft" || p.Items[1].Role != "promotion_candidate" || ((p.Snapshot.FileID != "") != replacement) {
						t.Fatal("wrong fixed scope", p)
					}
					// Unelapsed observation rejects, even though retention has elapsed.
					f.decision(p, "approve", 409)
					f.ready(p.ID)
					p = f.decision(p, "approve", 202)
					var marker string
					var version int64
					if e := f.f.f.owner.QueryRow(context.Background(), "SELECT retirement_plan_id,version FROM upload_sessions WHERE id=$1", u.ID).Scan(&marker, &version); e != nil || marker != p.ID || version != u.Version+1 {
						t.Fatal("session not permanently fenced", e)
					}
					for range 2 {
						f.run()
					}
					p = f.plan(p.ID)
					if p.State != "completed" || f.cloud.calls != 2 {
						t.Fatal("not completed exactly once", p, f.cloud.calls)
					}
					// Metadata and formal head remain; no artificial file/quota fact is created.
					var formal int
					var head string
					if e := f.f.f.owner.QueryRow(context.Background(), "SELECT count(*) FROM file_versions WHERE org_id=$1", f.f.f.org).Scan(&formal); e != nil || formal != 2 {
						t.Fatal("formal history changed", formal, e)
					}
					if e := f.f.f.owner.QueryRow(context.Background(), "SELECT current_version_id FROM files WHERE id=$1", f.current.File.ID).Scan(&head); e != nil || head != f.current.Version.ID {
						t.Fatal("head changed", head, e)
					}
					r, v := f.request("GET", "/v1/folders/"+f.f.root+"/completed-draft-retirement-plans?limit=1", "", nil)
					expect(t, r, 200, v)
					if rows := data[core.Page[storage.RetirementPlan]](t, v); len(rows.Items) != 1 || rows.Items[0].State != "completed" {
						t.Fatal(rows)
					}
				})
			}
		})
	}
}
func TestCompletedUploadRetirementFencesAndRenewal(t *testing.T) {
	f, u := completedUploadFixture(t, true, "android")
	for range 2 {
		addKnownCompletedCandidate(t, f, u.ID)
	}
	p := createCompletedPlan(t, f, u)
	if len(p.Items) != 4 {
		t.Fatal("older completed candidates omitted", p)
	}
	f.ready(p.ID)
	p = f.decision(p, "approve", 202)
	// Direct stale metadata writes and a new physical promotion cannot bypass the API.
	for _, q := range []string{"UPDATE upload_sessions SET retirement_plan_id=NULL,version=version+1 WHERE id=$1", "UPDATE upload_sessions SET state='committed',version=version+1 WHERE id=$1", "UPDATE object_transfers SET state='starting' WHERE session_id=$1"} {
		if _, e := f.f.f.owner.Exec(context.Background(), q, u.ID); e == nil {
			t.Fatal("retired session write accepted", q)
		}
	}
	r, v := f.request("POST", "/v1/uploads/"+u.ID+"/commit", core.NewID(), commitInput(u))
	expect(t, r, 409, v)
	f.run()
	p = f.plan(p.ID)
	stopRetirementAuthority(f, p)
	retirementRenewalLogin(f, "android")
	p = f.plan(p.ID)
	if f.cloud.calls != 1 || !slices.ContainsFunc(p.Items, func(v storage.RetirementItem) bool { return v.State == "blocked" }) {
		t.Fatal("partial result lost", p)
	}
	r, v = f.request("POST", "/v1/object-retirement-plans/"+p.ID+"/reauthorize", core.NewID(), retirementRenewalInput(p))
	expect(t, r, 202, v)
	p = data[storage.RetirementPlan](t, v)
	if p.Authorization == nil || len(p.Authorization.Items) != 3 {
		t.Fatal("remaining objects lost", p)
	}
	for range 3 {
		f.run()
	}
	p = f.plan(p.ID)
	if p.State != "completed" || f.cloud.calls != 4 {
		t.Fatal("confirmed object repeated", p)
	}
}
func TestCompletedUploadRetirementProtection(t *testing.T) {
	for _, scenario := range []string{"retention", "active-job", "native-version", "uncertain-transfer", "legal-hold", "stale-upload", "changed-after-plan", "cancelled-barrier", "approval-rollback"} {
		t.Run(scenario, func(t *testing.T) {
			f, u := completedUploadFixture(t, true, "android")
			ctx := context.Background()
			exec := func(q string, args ...any) {
				if _, e := f.f.f.owner.Exec(ctx, q, args...); e != nil {
					t.Fatal(e)
				}
			}
			if scenario == "stale-upload" {
				c := completedCandidates(t, f)[0]
				r, v := f.request("POST", "/v1/folders/"+f.f.root+"/completed-draft-retirement-plans", core.NewID(), storage.CompletedDraftRetirementCreate{ExpectedFolderVersion: c.FolderVersion, ExpectedUploadVersion: c.UploadVersion + 1, TargetID: u.ID, Reason: "stale"})
				expect(t, r, 409, v)
				return
			}
			if scenario == "changed-after-plan" || scenario == "cancelled-barrier" || scenario == "approval-rollback" {
				p := createCompletedPlan(t, f, u)
				f.ready(p.ID)
				if scenario == "changed-after-plan" {
					exec("UPDATE upload_sessions SET version=version+1 WHERE id=$1", u.ID)
					f.decision(p, "approve", 409)
					return
				}
				if scenario == "approval-rollback" {
					exec("CREATE FUNCTION reject_completed_retirement_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='file.original.retirement.approved' THEN RAISE EXCEPTION 'isolated audit rejection';END IF;RETURN NEW;END $$")
					exec("CREATE TRIGGER reject_completed_retirement_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_completed_retirement_audit()")
					f.decision(p, "approve", 500)
					var fenced bool
					if e := f.f.f.owner.QueryRow(ctx, "SELECT retirement_plan_id IS NOT NULL FROM upload_sessions WHERE id=$1", u.ID).Scan(&fenced); e != nil || fenced {
						t.Fatal("approval failed without rollback", e)
					}
					return
				}
				p = f.decision(p, "approve", 202)
				f.decision(p, "cancel", 200)
				f.run()
				if f.cloud.calls != 0 {
					t.Fatal("cancelled cleanup executed")
				}
				exec("SELECT 1")
				if _, e := f.f.f.owner.Exec(ctx, "UPDATE upload_sessions SET retirement_plan_id=NULL WHERE id=$1", u.ID); e == nil {
					t.Fatal("cancel resurrected upload")
				}
				return
			}
			expected := ""
			switch scenario {
			case "retention":
				expected = "RETENTION_PERIOD_ACTIVE"
				exec("INSERT INTO retention_policies(id,org_id,version,kind,enabled,retain_seconds,observation_seconds,published_by,reason) VALUES(gen_random_uuid(),$1,2,'draft',true,31536000,86400,$2,'isolated longer retention')", f.f.f.org, f.f.f.admin)
			case "active-job":
				expected = "UPLOAD_JOB_ACTIVE"
				exec("UPDATE jobs SET updated_at=clock_timestamp() WHERE org_id=$1 AND payload->>'session_id'=$2::text", f.f.f.org, u.ID)
			case "native-version":
				expected = "NATIVE_OBJECT_VERSION_REQUIRED"
				exec("ALTER TABLE upload_sessions DISABLE TRIGGER upload_input_immutable")
				exec("UPDATE upload_sessions SET frozen_source_id=jsonb_set(frozen_source_id::jsonb,'{VersionID}','\"\"')::text WHERE id=$1", u.ID)
				exec("ALTER TABLE upload_sessions ENABLE TRIGGER upload_input_immutable")
			case "uncertain-transfer":
				expected = "UNCERTAIN_OBJECT_PROTECTED"
				exec("UPDATE object_transfers SET state='uncertain' WHERE session_id=$1", u.ID)
			case "legal-hold":
				expected = "RETENTION_HOLD"
				exec("UPDATE files SET legal_hold=true WHERE id=$1", f.current.File.ID)
			}
			candidates := completedCandidates(t, f)
			if len(candidates) != 1 || candidates[0].Eligible || !slices.Contains(candidates[0].BlockingReasons, expected) {
				t.Fatal("protection missing", expected, candidates)
			}
			r, v := f.request("POST", "/v1/folders/"+f.f.root+"/completed-draft-retirement-plans", core.NewID(), storage.CompletedDraftRetirementCreate{ExpectedFolderVersion: candidates[0].FolderVersion, ExpectedUploadVersion: u.Version, TargetID: u.ID, Reason: "protected"})
			expect(t, r, 409, v)
		})
	}
}
