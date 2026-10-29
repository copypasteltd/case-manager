package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"copypaste.hk/platform/internal/auditlog"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
)

func TestAuditConsoleIntegration(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	admin := f.administratorToken()
	var diagnostics bytes.Buffer
	f.api.Logger = slog.New(slog.NewJSONHandler(&diagnostics, &slog.HandlerOptions{Level: slog.LevelError}))
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(diagnostics.String())
		}
	})
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	filt := auditlog.Filter{From: "2026-10-01T00:00:00+08:00", To: "2026-10-02T00:00:00+08:00", Action: "fixture.audit.v1"}
	query := func(f auditlog.Filter) string {
		raw, _ := json.Marshal(f)
		var values map[string]string
		if e := json.Unmarshal(raw, &values); e != nil {
			t.Fatal(e)
		}
		q := url.Values{}
		for k, v := range values {
			q.Set(k, v)
		}
		return "/v1/audit/events?" + q.Encode()
	}
	get := func(t *testing.T, path, token string, status int) response {
		t.Helper()
		r, v := f.with(t).request("GET", path, token, "", nil, nil)
		expect(t, r, status, v)
		return v
	}
	post := func(t *testing.T, token, key string, in any, status int) response {
		t.Helper()
		r, v := f.with(t).request("POST", "/v1/audit/exports", token, key, in, nil)
		expect(t, r, status, v)
		return v
	}
	secret := "synthetic reason must not escape audit metadata"
	ids := []string{core.NewID(), core.NewID(), core.NewID()}
	resource := f.createProject("Synthetic restricted audit resource", nil).ID
	for _, id := range ids {
		exec(`INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,resource_version,result,reason,occurred_at)
 VALUES($1,$2,$3,$3,'web','synthetic-request','fixture.audit.v1','project',$4,9007199254740993,'succeeded',$5,'2026-10-01T04:00:00Z')`, id, f.org, f.alice, resource, secret)
	}
	foreignOrg, foreignID := core.NewID(), core.NewID()
	if _, err := f.owner.Exec(ctx, `INSERT INTO organizations(id,name) VALUES($1,'Synthetic other organization')`, foreignOrg); err == nil {
		t.Fatal("single-organization invariant was lost")
	}
	if _, err := f.owner.Exec(ctx, `INSERT INTO audit_events(id,org_id,channel,request_id,action,resource_type,result) VALUES($1,$2,'web','synthetic-foreign','fixture.audit.v1','project','succeeded')`, foreignID, foreignOrg); err == nil {
		t.Fatal("foreign organization audit insert was accepted")
	}
	binary := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		req.Header.Set("Authorization", "Bearer "+admin)
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, req)
		return rec
	}
	input := auditlog.ExportCreate{Filters: filt, Reason: "Synthetic audit review"}
	var record auditlog.Export
	t.Run("dedicated authority, tenant and metadata boundary", func(t *testing.T) {
		get(t, query(filt), f.memberToken, 404)
		page := data[auditlog.Page](t, get(t, query(filt), admin, 200))
		if len(page.Items) != 3 || page.Disclosure != auditlog.Disclosure {
			t.Fatal("missing tenant metadata")
		}
		for _, e := range page.Items {
			if e.ResourceVersion == nil || *e.ResourceVersion != "9007199254740993" || e.ID == foreignID {
				t.Fatal("numeric precision or tenant boundary lost")
			}
		}
		r, v := f.with(t).request("GET", "/v1/audit/events/"+ids[0], admin, "", nil, nil)
		expect(t, r, 200, v)
		if bytes.Contains(r.Body.Bytes(), []byte(secret)) || bytes.Contains(r.Body.Bytes(), []byte("session_id")) || bytes.Contains(r.Body.Bytes(), []byte("reason")) {
			t.Fatal("arbitrary content exposed")
		}
		get(t, "/v1/audit/events/"+foreignID, admin, 404)
		get(t, "/v1/projects/"+resource, admin, 404)
	})
	t.Run("exact filters, keyset ties and bound cursors", func(t *testing.T) {
		seen := map[string]bool{}
		path := query(filt) + "&limit=1"
		for i := 0; i < 3; i++ {
			p := data[auditlog.Page](t, get(t, path, admin, 200))
			if len(p.Items) != 1 || seen[p.Items[0].ID] {
				t.Fatal("lost or repeated tie")
			}
			seen[p.Items[0].ID] = true
			if i < 2 {
				if p.NextCursor == nil {
					t.Fatal("missing next cursor")
				}
				path = query(filt) + "&limit=1&cursor=" + url.QueryEscape(*p.NextCursor)
			} else if p.NextCursor != nil {
				t.Fatal("unexpected tail")
			}
		}
		p := data[auditlog.Page](t, get(t, query(filt)+"&limit=1", admin, 200))
		changed := filt
		changed.Result = "denied"
		get(t, query(changed)+"&cursor="+url.QueryEscape(*p.NextCursor), admin, 400)
		get(t, query(filt)+"&cursor=invalid", admin, 400)
		bad := []string{query(filt) + "&action=other", query(filt) + "&ignored=x", query(filt) + "&limit=01", "/v1/audit/events?from=bad&to=bad", "/v1/audit/events/" + ids[0] + "?ignored=x"}
		for _, path := range bad {
			get(t, path, admin, 422)
		}
		filtered := filt
		filtered.ActorID = f.bob
		if len(data[auditlog.Page](t, get(t, query(filtered), admin, 200)).Items) != 0 {
			t.Fatal("actor filter ignored")
		}
		filtered = filt
		filtered.From = "2026-10-01T04:00:00Z"
		filtered.To = "2026-10-01T04:00:00.000001Z"
		if len(data[auditlog.Page](t, get(t, query(filtered), admin, 200)).Items) != 3 {
			t.Fatal("inclusive start lost")
		}
		filtered.From = "2026-10-01T03:00:00Z"
		filtered.To = "2026-10-01T04:00:00Z"
		if len(data[auditlog.Page](t, get(t, query(filtered), admin, 200)).Items) != 0 {
			t.Fatal("exclusive end lost")
		}
	})
	t.Run("four human channels and independent export permission", func(t *testing.T) {
		role := core.NewID()
		exec(`INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'synthetic_auditor','Synthetic metadata auditor')`, role, f.org)
		exec(`INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'audit.read')`, f.org, role)
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')`, core.NewID(), f.org, f.alice, role)
		for _, channel := range []string{"web", "wechat", "douyin", "android"} {
			if channel == "web" {
				r, v := f.with(t).request("POST", "/v1/auth/login", "", "", identity.LoginInput{Login: "alice", Password: "member-test-password-long", Device: "web"}, map[string]string{"Origin": "http://localhost:5178"})
				expect(t, r, 200, v)
				headers := map[string]string{"Origin": "http://localhost:5178"}
				cookies := []string{}
				for _, cookie := range r.Result().Cookies() {
					cookies = append(cookies, cookie.Name+"="+cookie.Value)
					if cookie.Name == "cp_csrf" {
						headers["X-CSRF-Token"] = cookie.Value
					}
				}
				headers["Cookie"] = strings.Join(cookies, "; ")
				r, v = f.with(t).request("GET", query(filt), "", "", nil, headers)
				expect(t, r, 200, v)
				r, v = f.with(t).request("POST", "/v1/audit/exports", "", core.NewID(), input, headers)
				expect(t, r, 404, v)
				delete(headers, "X-CSRF-Token")
				r, v = f.with(t).request("POST", "/v1/audit/exports", "", core.NewID(), input, headers)
				expect(t, r, 403, v)
				continue
			}
			token := f.login("alice", channel).AccessToken
			if channel == "android" {
				f.aliceToken = token
			}
			if token == "" {
				t.Fatal("synthetic channel login failed")
			}
			get(t, query(filt), token, 200)
			post(t, token, core.NewID(), input, 404)
		}
	})
	t.Run("fixed export, replay, late rows and byte integrity", func(t *testing.T) {
		key := core.NewID()
		record = data[auditlog.Export](t, post(t, admin, key, input, 201))
		if record.RowCount != 3 || record.State != "ready" || !record.CanDownload {
			t.Fatal("invalid export receipt")
		}
		replay := data[auditlog.Export](t, post(t, admin, key, input, 201))
		if replay.ID != record.ID {
			t.Fatal("idempotent export duplicated")
		}
		changed := input
		changed.Reason = "Another purpose"
		post(t, admin, key, changed, 409)
		path := "/v1/audit/exports/" + record.ID
		r := binary(path + "/download")
		if r.Code != 200 {
			t.Fatalf("CSV download status %d: %s", r.Code, r.Body.String())
		}
		original := append([]byte(nil), r.Body.Bytes()...)
		if core.Digest(original) != record.CSVSHA256 || len(original) != record.CSVBytes || bytes.Contains(original, []byte(secret)) {
			t.Fatal("wrong CSV integrity or content boundary")
		}
		exec(`INSERT INTO audit_events(id,org_id,channel,request_id,action,resource_type,result,occurred_at) VALUES($1,$2,'web','synthetic-late','fixture.audit.v1','project','succeeded','2026-10-01T03:00:00Z')`, core.NewID(), f.org)
		r = binary(path + "/download")
		if r.Code != 200 || !bytes.Equal(original, r.Body.Bytes()) {
			t.Fatal("late event changed sealed export")
		}
		chunk := get(t, path+"/chunks?offset=0&length="+fmt.Sprint(record.CSVBytes), admin, 200)
		var part struct {
			Data string `json:"data_base64"`
			SHA  string `json:"chunk_sha256"`
		}
		if json.Unmarshal(chunk.Data, &part) != nil {
			t.Fatal("invalid chunk")
		}
		decoded, e := base64.StdEncoding.DecodeString(part.Data)
		if e != nil || !bytes.Equal(original, decoded) || part.SHA != record.CSVSHA256 {
			t.Fatal("chunk integrity lost")
		}
		for _, suffix := range []string{"/chunks?offset=00&length=1", "/chunks?offset=0&length=524289", "/chunks?offset=0&length=1&extra=x", "/download?token=invalid"} {
			get(t, path+suffix, admin, 422)
		}
		get(t, path+"/chunks?offset="+fmt.Sprint(record.CSVBytes)+"&length=1", admin, 416)
		get(t, path, f.aliceToken, 404)
		role := core.NewID()
		exec(`INSERT INTO roles(id,org_id,key,name) VALUES($1,$2,'synthetic_exporter','Synthetic independent exporter')`, role, f.org)
		exec(`INSERT INTO role_actions(org_id,role_id,action) VALUES($1,$2,'audit.read'),($1,$2,'audit.export')`, f.org, role)
		exec(`INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')`, core.NewID(), f.org, f.bob, role)
		get(t, path, f.bobToken, 404)
		if len(data[core.Page[auditlog.Export]](t, get(t, "/v1/audit/exports", f.bobToken, 200)).Items) != 0 {
			t.Fatal("another auditor saw private export history")
		}
	})
	t.Run("fresh MFA and no agent executor access", func(t *testing.T) {
		exec(`UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE user_id=$1`, f.admin)
		post(t, admin, core.NewID(), input, 403)
		get(t, "/v1/audit/exports/"+record.ID+"/download", admin, 403)
		exec(`UPDATE sessions SET mfa_verified_at=clock_timestamp() WHERE user_id=$1`, f.admin)
		a, err := f.api.Identity.Authenticate(ctx, admin)
		if err != nil {
			t.Fatal(err)
		}
		a.RunID = core.NewID()
		service := &auditlog.Service{DB: f.api.DB}
		if _, err = service.Event(ctx, a, ids[0]); err == nil {
			t.Fatal("agent read the human audit console")
		}
	})
	t.Run("row cap rejects entire export and empty export works", func(t *testing.T) {
		exec(`INSERT INTO audit_events(id,org_id,channel,request_id,action,resource_type,result,occurred_at) SELECT gen_random_uuid(),$1,'worker','synthetic-batch','fixture.audit.batch','project','succeeded','2026-10-01T04:00:00Z' FROM generate_series(1,10001)`, f.org)
		big := input
		big.Filters.Action = "fixture.audit.batch"
		post(t, admin, core.NewID(), big, 409)
		empty := input
		empty.Filters.Action = "fixture.audit.none"
		v := data[auditlog.Export](t, post(t, admin, core.NewID(), empty, 201))
		if v.RowCount != 0 || v.CSVBytes <= 0 {
			t.Fatal("empty export omitted header")
		}
		expired := core.NewID()
		exec(`INSERT INTO audit_exports(id,org_id,requester_id,channel,auth_version,authorization_version,filters,disclosure,reason,row_count,csv_bytes,csv_sha256,created_at,expires_at) SELECT $1,org_id,requester_id,channel,auth_version,authorization_version,filters,disclosure,reason,0,csv_bytes,csv_sha256,clock_timestamp()-interval '48 hours',clock_timestamp()-interval '24 hours' FROM audit_exports WHERE id=$2`, expired, v.ID)
		old := data[auditlog.Export](t, get(t, "/v1/audit/exports/"+expired, admin, 200))
		if old.State != "expired" || old.CanDownload {
			t.Fatal("expired export remained downloadable")
		}
		get(t, "/v1/audit/exports/"+expired+"/download", admin, 409)
	})
	t.Run("database immutability and exact complete reference set", func(t *testing.T) {
		for _, q := range []string{`UPDATE audit_exports SET row_count=0 WHERE id=$1`, `DELETE FROM audit_export_events WHERE export_id=$1`, `UPDATE audit_events SET reason='changed' WHERE id=$1`} {
			id := record.ID
			if strings.Contains(q, "UPDATE audit_events") {
				id = ids[0]
			}
			if _, e := f.api.DB.Exec(ctx, q, id); e == nil {
				t.Fatal("application rewrote immutable evidence")
			}
		}
		if _, e := f.api.DB.Exec(ctx, `INSERT INTO audit_export_events(org_id,export_id,position,event_id) VALUES($1,$2,4,$3)`, f.org, record.ID, foreignID); e == nil {
			t.Fatal("sealed set allowed an append")
		}
		if _, e := f.api.DB.Exec(ctx, `INSERT INTO audit_exports(id,org_id,requester_id,channel,auth_version,authorization_version,filters,disclosure,reason,row_count,csv_bytes,csv_sha256) SELECT $1,org_id,requester_id,channel,auth_version,authorization_version,filters,disclosure,reason,1,csv_bytes,csv_sha256 FROM audit_exports WHERE id=$2`, core.NewID(), record.ID); e == nil {
			t.Fatal("incomplete set committed")
		}
	})
	t.Run("audit failure rolls back header references and idempotency", func(t *testing.T) {
		exec(`CREATE FUNCTION fail_audit_export_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='audit.export.created' THEN RAISE EXCEPTION 'synthetic atomic audit failure';END IF;RETURN NEW;END $$;CREATE TRIGGER fail_audit_export_fixture BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION fail_audit_export_fixture()`)
		defer exec(`DROP TRIGGER fail_audit_export_fixture ON audit_events;DROP FUNCTION fail_audit_export_fixture()`)
		var before int
		if e := f.owner.QueryRow(ctx, `SELECT count(*) FROM audit_exports`).Scan(&before); e != nil {
			t.Fatal(e)
		}
		key := core.NewID()
		post(t, admin, key, input, 500)
		var after, receipts int
		if e := f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM audit_exports),(SELECT count(*) FROM idempotency_records WHERE idempotency_key=$1)`, key).Scan(&after, &receipts); e != nil || after != before || receipts != 0 {
			t.Fatal("partial audit export transaction survived")
		}
	})
	t.Run("authorization change invalidates old artifacts and logout denies download", func(t *testing.T) {
		exec(`UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1`, f.org)
		// Validate using the current service actor to avoid creating another MFA credential.
		var a core.Actor
		var session string
		if e := f.owner.QueryRow(ctx, `SELECT s.id,s.user_id,s.auth_version,o.authorization_version,s.mfa_verified_at FROM sessions s JOIN organizations o ON o.id=s.org_id WHERE s.org_id=$1 AND s.user_id=$2 AND s.revoked_at IS NULL ORDER BY s.created_at DESC LIMIT 1`, f.org, f.admin).Scan(&session, &a.UserID, &a.AuthVersion, &a.OrgAuthVersion, &a.MFAVerifiedAt); e != nil {
			t.Fatal(e)
		}
		a.OrgID = f.org
		a.SessionID = session
		a.Channel = "android"
		service := &auditlog.Service{DB: f.api.DB}
		stored, e := service.Export(ctx, a, record.ID)
		if e != nil || stored.State != "authorization_changed" || stored.CanDownload {
			t.Fatal("old export retained authority")
		}
		if _, e = service.Download(ctx, a, record.ID, core.NewID()); e == nil {
			t.Fatal("download ignored changed authority")
		}
		exec(`UPDATE sessions SET revoked_at=clock_timestamp() WHERE id=$1`, session)
		if _, e = service.Event(ctx, a, ids[0]); e == nil {
			t.Fatal("stale actor remained authorized after logout")
		}
	})
}
