package httpapi

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestRound158DerivedTextRetirement(t *testing.T) {
	fixture := newRetirementFixture(t, "android")
	f := fixture.f.f
	ctx := context.Background()
	var diagnostics bytes.Buffer
	f.api.Logger = slog.New(slog.NewTextHandler(&diagnostics, &slog.HandlerOptions{Level: slog.LevelError}))
	defer func() {
		if t.Failed() {
			t.Log(diagnostics.String())
		}
	}()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, e := f.owner.Exec(ctx, q, args...); e != nil {
			t.Fatal(e)
		}
	}
	text := "isolated old derived plaintext 香港🙂"
	exec(`INSERT INTO file_text_derivations(org_id,file_id,version_id,extractor,source_sha256) VALUES($1,$2,$3,'text-v1',$4) ON CONFLICT DO NOTHING`, f.org, fixture.old.File.ID, fixture.old.Version.ID, fixture.old.Version.SHA256)
	exec(`UPDATE file_text_derivations SET state='ready',body=$4,text_sha256=$5,character_count=char_length($4),indexed_at=clock_timestamp() WHERE org_id=$1 AND file_id=$2 AND version_id=$3 AND extractor='text-v1' AND state='queued'`, f.org, fixture.old.File.ID, fixture.old.Version.ID, text, core.Digest([]byte(text)))
	path := "/v1/files/" + fixture.current.File.ID + "/versions/" + fixture.old.Version.ID + "/derived-retirement"
	call := func(method, path, key string, body any, status int) response {
		t.Helper()
		r, v := fixture.request(method, path, key, body)
		expect(t, r, status, v)
		if strings.Contains(r.Body.String(), text) {
			t.Fatal("inventory disclosed derived body")
		}
		return v
	}
	overview := data[storage.DerivedRetirementOverview](t, call("GET", path, "", nil, 200))
	var basis storage.DerivedRetirementBasis
	if e := json.Unmarshal([]byte(overview.Basis), &basis); e != nil || basis.Eligible || len(basis.Texts) != 1 || basis.Texts[0].TextSHA256 == nil {
		t.Fatal("original protection or metadata missing", basis, e)
	}
	p := fixture.create("history")
	fixture.ready(p.ID)
	p = fixture.plan(p.ID)
	fixture.decision(p, "approve", 202)
	for n := 0; n < 3; n++ {
		if fixture.plan(p.ID).State == "completed" {
			break
		}
		fixture.run()
	}
	if fixture.plan(p.ID).State != "completed" {
		t.Fatal("ordinary original cleanup did not complete")
	}
	overview = data[storage.DerivedRetirementOverview](t, call("GET", path, "", nil, 200))
	if e := json.Unmarshal([]byte(overview.Basis), &basis); e != nil || !basis.Eligible || basis.OriginalPlanID == nil {
		t.Fatal("purged original did not permit separate derived review", basis, e)
	}
	input := storage.DerivedRetirementCreate{ExpectedFileVersion: basis.FileVersion, BasisSHA256: overview.BasisSHA256, Reason: "review known old DB text after original cleanup", Reviewed: true}
	key := core.NewID()
	plan := data[storage.DerivedRetirementPlan](t, call("POST", path, key, input, 201))
	replay := data[storage.DerivedRetirementPlan](t, call("POST", path, key, input, 201))
	if replay.ID != plan.ID {
		t.Fatal("plan replay duplicated")
	}
	decision := storage.DerivedRetirementDecision{BasisSHA256: plan.BasisSHA256, Reason: "explicit human final text clearance", Reviewed: true}
	execute := "/v1/derived-retirement-plans/" + plan.ID + "/execute"
	call("POST", execute, core.NewID(), decision, 409)
	// Synthetic time arrangement only; there is no application observation bypass.
	exec(`ALTER TABLE file_derived_retirement_plans DISABLE TRIGGER derived_retirement_plan_immutable`)
	exec(`UPDATE file_derived_retirement_plans SET created_at=clock_timestamp()-interval '2 days',ready_after=clock_timestamp()-interval '1 second',expires_at=clock_timestamp()+interval '6 days' WHERE id=$1`, plan.ID)
	exec(`ALTER TABLE file_derived_retirement_plans ENABLE TRIGGER derived_retirement_plan_immutable`)
	done := data[storage.DerivedRetirementPlan](t, call("POST", execute, core.NewID(), decision, 200))
	if done.Receipt == nil || done.Receipt.Action != "execute" || done.Receipt.RemovedCount != 1 || done.Basis != plan.Basis {
		t.Fatal("immutable exact retirement receipt missing")
	}
	var state string
	var body *string
	var hash *string
	if e := f.owner.QueryRow(ctx, `SELECT state,body,text_sha256 FROM file_text_derivations WHERE org_id=$1 AND version_id=$2 AND extractor='text-v1'`, f.org, fixture.old.Version.ID).Scan(&state, &body, &hash); e != nil || state != "blocked" || body != nil || hash != nil {
		t.Fatal("text plaintext was not cleared", e)
	}
	if _, e := f.api.DB.Exec(ctx, `INSERT INTO file_text_derivations(org_id,file_id,version_id,extractor,source_sha256) VALUES($1,$2,$3,'text-v1',$4)`, f.org, fixture.old.File.ID, fixture.old.Version.ID, fixture.old.Version.SHA256); e == nil || !strings.Contains(e.Error(), "retired version") {
		t.Fatal("retired extraction barrier missing", e)
	}
	if _, e := f.api.DB.Exec(ctx, `DELETE FROM file_derived_retirement_decisions WHERE plan_id=$1`, plan.ID); e == nil {
		t.Fatal("application deleted cleanup evidence")
	}
	if os.Getenv("CP_BROWSER_BATCH158_FIXTURE") != "" {
		holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch158-derived-retirement", "file_id": fixture.current.File.ID, "version_id": fixture.old.Version.ID, "project_id": *fixture.current.File.ProjectID, "derived_plan_id": plan.ID})
	}
}
