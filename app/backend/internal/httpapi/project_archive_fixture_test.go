package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/project"
	"testing"
	"time"
)

// Storage tests need a genuinely archived project, not a detached terminal head.
// Give the test administrator explicit temporary business authority and verify
// actual MFA, without changing the file owner's credentials or authority.
func archiveProjectFixture(t *testing.T, f *fixture, pid string) project.Project {
	t.Helper()
	ctx := context.Background()
	member, e := f.owner.Exec(ctx, "INSERT INTO project_members(org_id,project_id,user_id,project_role) VALUES($1,$2,$3,'manager') ON CONFLICT(project_id,user_id) DO NOTHING", f.org, pid, f.admin)
	if e != nil {
		t.Fatal(e)
	}
	if member.RowsAffected() == 1 {
		defer func() {
			if _, e := f.owner.Exec(ctx, "DELETE FROM project_members WHERE org_id=$1 AND project_id=$2 AND user_id=$3", f.org, pid, f.admin); e != nil {
				t.Error(e)
			}
		}()
	}
	ids := []string{core.NewID(), core.NewID()}
	for i, key := range []string{"project_manager", "project_archiver"} {
		if _, e := f.owner.Exec(ctx, `INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,scope_id,classification_ceiling) SELECT $1,$2,$3,id,'project',$4,'project_restricted' FROM roles WHERE org_id=$2 AND key=$5`, ids[i], f.org, f.admin, pid, key); e != nil {
			t.Fatal(e)
		}
	}
	defer func() {
		if _, e := f.owner.Exec(ctx, "DELETE FROM role_bindings WHERE org_id=$1 AND id=ANY($2::uuid[])", f.org, ids); e != nil {
			t.Error(e)
		}
	}()
	var enrolled bool
	if e := f.owner.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM mfa_credentials WHERE org_id=$1 AND user_id=$2)", f.org, f.admin).Scan(&enrolled); e != nil {
		t.Fatal(e)
	}
	var token string
	if !enrolled {
		token = f.with(t).administratorToken()
	} else {
		challenge := f.with(t).login("sysadmin", "android")
		var encrypted []byte
		var last int64
		if e := f.owner.QueryRow(ctx, "SELECT secret_cipher,last_step FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.org, f.admin).Scan(&encrypted, &last); e != nil {
			t.Fatal(e)
		}
		secret, e := identity.Open(f.api.Config.MFAKey, encrypted, f.org+":"+f.admin)
		if e != nil {
			t.Fatal("cannot open owned archive fixture credential")
		}
		if now := time.Now(); now.Unix()/30 <= last {
			time.Sleep(time.Until(time.Unix((last+1)*30, 0)) + 100*time.Millisecond)
		}
		r, v := f.with(t).request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(secret, time.Now())}, nil)
		expect(t, r, 200, v)
		token = data[identity.LoginResult](t, v).AccessToken
	}
	r, v := f.with(t).request("GET", "/v1/projects/"+pid, token, "", nil, nil)
	expect(t, r, 200, v)
	p := data[project.Project](t, v)
	for _, action := range []string{"activate", "close"} {
		if action == "activate" && p.State != "draft" || action == "close" && p.State != "active" && p.State != "on_hold" {
			continue
		}
		r, v = f.with(t).request("POST", "/v1/projects/"+pid+"/transition", token, core.NewID(), project.Transition{ExpectedVersion: p.Version, Action: action, Reason: "explicit owned fixture project lifecycle"}, nil)
		expect(t, r, 200, v)
		p = data[project.Project](t, v)
	}
	r, v = f.with(t).request("GET", "/v1/projects/"+pid+"/closeout", token, "", nil, nil)
	expect(t, r, 200, v)
	b := data[project.CloseoutPreview](t, v)
	if b.Fingerprint == nil || !b.CanArchive {
		t.Fatal("fixture is not genuinely ready to archive", b.Visibility)
	}
	r, v = f.with(t).request("POST", "/v1/projects/"+pid+"/archive", token, core.NewID(), project.ArchiveInput{ExpectedVersion: p.Version, ExpectedFingerprint: *b.Fingerprint, Summary: "Storage fixture outcomes were reviewed", LessonsLearned: "Retain file and source history after project closure", FollowUp: "No unresolved business or financial items in this fixture", BusinessVerified: true, FinancialVerified: true, FileVersionIDs: []string{}, Reason: "exact owned test project archive"}, nil)
	expect(t, r, 201, v)
	return data[project.ArchiveReceipt](t, v).Project
}
