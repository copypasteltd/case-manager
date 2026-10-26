package httpapi

import (
	"context"
	"io"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/filescan"
	"copypaste.hk/platform/internal/identity"
)

func memberMFAToken(f *fixture, login, channel string) string {
	f.t.Helper()
	challenge := f.login(login, channel)
	rec, result := f.request("POST", "/v1/auth/mfa/enroll", "", "", map[string]string{"challenge": challenge.Challenge}, nil)
	expect(f.t, rec, 200, result)
	enrollment := data[identity.LoginResult](f.t, result)
	rec, result = f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(enrollment.MFASecret, time.Now())}, nil)
	expect(f.t, rec, 200, result)
	return data[identity.LoginResult](f.t, result).AccessToken
}
func configureWaiverPolicy(f *uploadFixture, reviewer string) storage.WaiverPolicy {
	f.f.t.Helper()
	var projectID string
	if err := f.f.owner.QueryRow(context.Background(), "SELECT project_id FROM folders WHERE id=$1", f.root).Scan(&projectID); err != nil {
		f.f.t.Fatal(err)
	}
	if _, err := f.f.owner.Exec(context.Background(), `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling)
		SELECT $1,$2,$3,id,'project',$4,'project_restricted' FROM roles WHERE org_id=$2 AND key='file_scan_reviewer'`,
		core.NewID(), f.f.org, reviewer, projectID); err != nil {
		f.f.t.Fatal(err)
	}
	admin := f.f.administratorToken()
	rec, result := f.f.request("POST", "/v1/storage/scan-waiver-policies", admin, core.NewID(), map[string]any{
		"expected_version": 0, "classification": "project_restricted", "enabled": true, "media_types": []string{"application/octet-stream"},
		"allowed_codes": []string{"SCANNER_NOT_CONFIGURED"}, "max_bytes": "120000000000", "reviewer_id": reviewer, "reason": "isolated explicit policy fixture; no production approval"}, nil)
	expect(f.f.t, rec, 201, result)
	policy := data[storage.WaiverPolicy](f.f.t, result)
	if reviewer != f.f.alice {
		f.f.aliceToken = f.f.login("alice", "android").AccessToken
	}
	return policy
}
func blockedWaiverUpload(f *uploadFixture) (storage.Upload, *integrityCloud) {
	f.f.t.Helper()
	u := f.frozen("waiver.bin", 15)
	cloud := &integrityCloud{uploadCloudFixture: f.cloud}
	if worked, err := integrityHost(f, cloud, filescan.Disabled{}).RunOne(context.Background()); err != nil || !worked {
		f.f.t.Fatal(err)
	}
	return f.get(u.ID), cloud
}
func waiverInput(u storage.Upload) map[string]any {
	return map[string]any{"expected_version": u.Version, "scan_report_id": *u.ScanResultID, "sha256": *u.SHA256, "reason": "source inspected separately; accept documented unscannable risk", "acknowledged_risk": true}
}
func requestWaiver(f *uploadFixture, u storage.Upload) storage.ScanWaiver {
	f.f.t.Helper()
	rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/waivers", f.f.aliceToken, core.NewID(), waiverInput(u), nil)
	expect(f.f.t, rec, 201, result)
	return data[storage.ScanWaiver](f.f.t, result)
}
func waiverDecision(w storage.ScanWaiver) map[string]any {
	return map[string]any{"expected_version": w.Version, "upload_version": w.UploadVersion, "scan_report_id": w.ScanReportID, "sha256": w.SHA256, "decision": "approve", "reason": "independent bounded exception fixture", "acknowledged_risk": true}
}
func TestScanWaiverBoundaries(t *testing.T) {
	ctx := context.Background()
	t.Run("default deny and independent recent MFA approval preserves waived evidence through commit", func(t *testing.T) {
		f := newUploadFixture(t)
		u, cloud := blockedWaiverUpload(f)
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/waivers", f.f.aliceToken, core.NewID(), waiverInput(u), nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "SCAN_WAIVER_POLICY_REQUIRED" {
			t.Fatal("default policy did not deny", result.Error)
		}
		policy := configureWaiverPolicy(f, f.f.bob)
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE scan_waiver_policies SET enabled=false WHERE id=$1", policy.ID); err == nil {
			t.Fatal("policy history was mutable")
		}
		w := requestWaiver(f, u)
		if w.OwnerID == w.ReviewerID || w.PolicyID != policy.ID || w.State != "pending" {
			t.Fatal("request snapshot invalid", w)
		}
		rec, result = f.f.request("POST", "/v1/scan-waivers/"+w.ID+"/decisions", f.f.aliceToken, core.NewID(), waiverDecision(w), nil)
		expect(t, rec, 403, result)
		bob := memberMFAToken(f.f, "bob", "wechat")
		rec, result = f.f.request("GET", "/v1/scan-waivers?state=pending", bob, "", nil, nil)
		expect(t, rec, 200, result)
		page := data[core.Page[storage.ScanWaiver]](t, result)
		if len(page.Items) != 1 || page.Items[0].ID != w.ID {
			t.Fatal("reviewer could not discover request", page)
		}
		member := f.f.login("member", "android").AccessToken
		rec, result = f.f.request("GET", "/v1/scan-waivers", member, "", nil, nil)
		expect(t, rec, 200, result)
		if len(data[core.Page[storage.ScanWaiver]](t, result).Items) != 0 {
			t.Fatal("unauthorized waiver listed")
		}
		bad := waiverDecision(w)
		bad["sha256"] = core.Digest([]byte("unread-confirmation"))
		rec, result = f.f.request("POST", "/v1/scan-waivers/"+w.ID+"/decisions", bob, core.NewID(), bad, nil)
		expect(t, rec, 409, result)
		key := core.NewID()
		rec, result = f.f.request("POST", "/v1/scan-waivers/"+w.ID+"/decisions", bob, key, waiverDecision(w), nil)
		expect(t, rec, 200, result)
		approved := data[storage.ScanWaiver](t, result)
		if approved.State != "approved" {
			t.Fatal("approval not committed", approved)
		}
		rec, result = f.f.request("POST", "/v1/scan-waivers/"+w.ID+"/decisions", bob, key, waiverDecision(w), nil)
		expect(t, rec, 200, result)
		if rec.Header().Get("X-Idempotent-Replay") != "true" {
			t.Fatal("decision replay repeated")
		}
		u = f.get(u.ID)
		if u.ScanState != "waived" || u.ScanWaiverRef == nil || *u.ScanWaiverRef != w.ID || u.State != "verifying" {
			t.Fatal("waiver falsely converted to clean", u)
		}
		to := newPromotionCloud(f)
		if worked, err := promotionHost(f, cloud, to).RunOne(ctx); err != nil || !worked {
			t.Fatal("waiver promotion not executed", err)
		}
		u = f.get(u.ID)
		if u.State != "ready" {
			t.Fatal("valid waiver did not permit candidate", u)
		}
		configureStorageTargets(f)
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/commit", f.f.aliceToken, core.NewID(), commitInput(u), nil)
		expect(t, rec, 201, result)
		v := data[storage.CommitResult](t, result).Version
		if v.ScanState != "waived" || v.ScanWaiverRef == nil || *v.ScanWaiverRef != w.ID {
			t.Fatal("formal version lost risk exception", v)
		}
		wireDownloads(f)
		grant := createDownload(t, f, v.ID)
		if grant.ScanState != "waived" || len(grant.Warnings) != 2 || grant.Warnings[0] != "SCAN_WAIVED_NOT_CLEAN" {
			t.Fatal("download hid original scanning risk")
		}
		rec, result = f.f.request("POST", "/v1/file-versions/"+v.ID+"/download", f.f.aliceToken, core.NewID(), map[string]string{"disposition": "inline"}, nil)
		expect(t, rec, 422, result)
		var reportState string
		if err := f.f.owner.QueryRow(ctx, "SELECT state FROM upload_scan_reports WHERE id=$1", w.ScanReportID).Scan(&reportState); err != nil || reportState != "unscannable" {
			t.Fatal("original scan report rewritten", reportState, err)
		}
		if _, err := f.f.api.DB.Exec(ctx, "UPDATE scan_waiver_decisions SET decision='reject' WHERE waiver_id=$1", w.ID); err == nil {
			t.Fatal("decision evidence mutable")
		}
	})
	t.Run("requester cannot become their own exception reviewer", func(t *testing.T) {
		f := newUploadFixture(t)
		u, _ := blockedWaiverUpload(f)
		configureWaiverPolicy(f, f.f.alice)
		f.f.aliceToken = memberMFAToken(f.f, "alice", "android")
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/waivers", f.f.aliceToken, core.NewID(), waiverInput(u), nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "INDEPENDENT_REVIEWER_REQUIRED" {
			t.Fatal("self review not explicitly denied", result.Error)
		}
	})
	t.Run("policy revision and rescan invalidate pending approval", func(t *testing.T) {
		f := newUploadFixture(t)
		u, _ := blockedWaiverUpload(f)
		p := configureWaiverPolicy(f, f.f.bob)
		w := requestWaiver(f, u)
		bob := memberMFAToken(f.f, "bob", "android")
		if _, err := f.f.owner.Exec(ctx, `INSERT INTO scan_waiver_policies(id,org_id,classification,version,enabled,media_types,allowed_codes,max_bytes,reviewer_id,reason,published_by)
			SELECT $2,org_id,classification,version+1,false,media_types,allowed_codes,max_bytes,reviewer_id,'controlled revised fixture',published_by
			FROM scan_waiver_policies WHERE id=$1`, p.ID, core.NewID()); err != nil {
			t.Fatal(err)
		}
		rec, result := f.f.request("POST", "/v1/scan-waivers/"+w.ID+"/decisions", bob, core.NewID(), waiverDecision(w), nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "SCAN_WAIVER_POLICY_CHANGED" {
			t.Fatal("pending policy snapshot bypassed", result.Error)
		}
		u = f.get(u.ID)
		rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/rescan", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": u.Version}, nil)
		expect(t, rec, 202, result)
		rec, result = f.f.request("POST", "/v1/scan-waivers/"+w.ID+"/decisions", bob, core.NewID(), waiverDecision(w), nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "SCAN_EVIDENCE_MISMATCH" {
			t.Fatal("stale scan accepted", result.Error)
		}
	})
	t.Run("decision audit failure rolls back waiver upload and event while cancellation stays explicit", func(t *testing.T) {
		f := newUploadFixture(t)
		u, _ := blockedWaiverUpload(f)
		configureWaiverPolicy(f, f.f.bob)
		w := requestWaiver(f, u)
		bob := memberMFAToken(f.f, "bob", "android")
		if _, err := f.f.owner.Exec(ctx, `CREATE FUNCTION reject_waiver_fixture() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN IF NEW.action='upload.scan.waived' THEN RAISE EXCEPTION 'fixture'; END IF; RETURN NEW; END; $$;
			CREATE TRIGGER reject_waiver_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_waiver_fixture();`); err != nil {
			t.Fatal(err)
		}
		rec, result := f.f.request("POST", "/v1/scan-waivers/"+w.ID+"/decisions", bob, core.NewID(), waiverDecision(w), nil)
		expect(t, rec, 500, result)
		u = f.get(u.ID)
		var decisions int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM scan_waiver_decisions WHERE waiver_id=$1", w.ID).Scan(&decisions); err != nil || decisions != 0 {
			t.Fatal("decision partially committed", decisions, err)
		}
		if u.State != "blocked" || u.ScanState != "unscannable" || u.ScanWaiverRef != nil {
			t.Fatal("audit failure released content", u)
		}
		rec, result = f.f.request("POST", "/v1/scan-waivers/"+w.ID+"/cancel", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": w.Version, "action": "cancel", "reason": "withdraw fixture"}, nil)
		expect(t, rec, 200, result)
		if data[storage.ScanWaiver](t, result).State != "cancelled" {
			t.Fatal("explicit cancellation missing")
		}
	})
	t.Run("malware verdict cannot enter the exception workflow", func(t *testing.T) {
		f := newUploadFixture(t)
		u := f.frozen("blocked-malware.bin", 15)
		scanner := integrityScanner(func(ctx context.Context, body io.ReadCloser, size int64) (filescan.Result, error) {
			body.Close()
			return filescan.Result{State: "quarantined", Code: "MALWARE_DETECTED"}, nil
		})
		if worked, err := integrityHost(f, &integrityCloud{uploadCloudFixture: f.cloud}, scanner).RunOne(ctx); err != nil || !worked {
			t.Fatal(err)
		}
		u = f.get(u.ID)
		rec, result := f.f.request("POST", "/v1/uploads/"+u.ID+"/waivers", f.f.aliceToken, core.NewID(), waiverInput(u), nil)
		expect(t, rec, 409, result)
		if result.Error.Code != "SCAN_WAIVER_NOT_ELIGIBLE" {
			t.Fatal("malware exception allowed", result.Error)
		}
	})
}
