package httpapi

import (
	"context"
	"testing"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
)

func TestWaiverFixedReport(t *testing.T) {
	f := newUploadFixture(t)
	u, _ := blockedWaiverUpload(f)
	configureWaiverPolicy(f, f.f.bob)
	w := requestWaiver(f, u)
	bob := memberMFAToken(f.f, "bob", "wechat")
	path := "/v1/scan-waivers/" + w.ID + "/scan-report"
	for _, token := range []string{f.f.aliceToken, bob} {
		rec, result := f.f.request("GET", path, token, "", nil, nil)
		expect(t, rec, 200, result)
		report := data[storage.ScanReport](t, result)
		if report.ID != w.ScanReportID || report.SHA256 != w.SHA256 || report.UploadID != u.ID {
			t.Fatal("waiver resolved another report")
		}
	}
	rec, result := f.f.request("GET", path, f.f.memberToken, "", nil, nil)
	expect(t, rec, 404, result)
	current := f.get(u.ID)
	rec, result = f.f.request("POST", "/v1/uploads/"+u.ID+"/rescan", f.f.aliceToken, core.NewID(), map[string]any{"expected_version": current.Version}, nil)
	expect(t, rec, 202, result)
	rec, result = f.f.request("GET", path, bob, "", nil, nil)
	expect(t, rec, 200, result)
	if data[storage.ScanReport](t, result).ID != w.ScanReportID {
		t.Fatal("rescan rebound the fixed application evidence")
	}
	if _, err := f.f.owner.Exec(context.Background(), "UPDATE role_bindings SET revoked_at=now() WHERE org_id=$1 AND user_id=$2", f.f.org, f.f.bob); err != nil {
		t.Fatal(err)
	}
	rec, result = f.f.request("GET", path, bob, "", nil, nil)
	expect(t, rec, 404, result)
}
