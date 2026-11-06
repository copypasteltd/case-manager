package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/agent"
	"copypaste.hk/platform/internal/agentworker"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/filescan"
	"copypaste.hk/platform/internal/jobqueue"
	"github.com/jackc/pgx/v5/pgconn"
)

// The native controller, ordinary upload/scan/commit and real login handlers
// are exercised against disposable PostgreSQL. Confinement and object storage
// remain explicit controlled fixtures; no real model/cloud acceptance is implied.
type round163FileSessionFixture struct {
	u       *uploadFixture
	s       *agent.Service
	cloud   *contentFixtureCloud
	hooks   *creationHookWriter
	base    storage.CommitResult
	c       agent.Conversation
	profile string
}

func round163FileSessionSetup(t *testing.T) *round163FileSessionFixture {
	t.Helper()
	u := newUploadFixture(t)
	configureStorageTargets(u)
	f := u.f
	ctx := context.Background()
	for _, command := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='agent_user'", []any{core.NewID(), f.org, f.alice}},
		{"INSERT INTO classification_policies(id,org_id,level,ai_allowed,policy_version) SELECT $1,$2,'project_restricted',true,max(policy_version)+1 FROM classification_policies WHERE org_id=$2 AND level='project_restricted'", []any{core.NewID(), f.org}},
		{"UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", []any{f.org}},
	} {
		if _, err := f.owner.Exec(ctx, command.sql, command.args...); err != nil {
			t.Fatal(err)
		}
	}
	base, cloud := commitContentFixture(t, u, "原始会话授权.md", "text/markdown", []byte("旧计划\n保留正文\n"), nil)
	hooks := &creationHookWriter{AgentDraftWriter: cloud}
	writer := &storage.AgentDraftBackend{Provider: "oss", Bucket: u.service.Backend.Bucket, Writer: hooks}
	f.api.AgentFileReaders = map[string]*storage.AgentTextBackend{"oss": {Provider: "oss", Bucket: writer.Bucket, Reader: &downloadCloud{readFn: cloud.Read}}}
	f.api.AgentDraftWriter = writer
	f.handler = f.api.Handler()
	s := &agent.Service{DB: f.api.DB, FileText: &storage.AgentTextService{Backends: f.api.AgentFileReaders}, FileDrafts: writer}
	profile := core.NewID()
	if _, err := f.owner.Exec(ctx, `INSERT INTO agent_runtime_profiles(id,org_id,skill_id,skill_version,skill_sha256,runtime_version,configuration_sha256,state,max_input_tokens,max_output_tokens,max_tool_calls,timeout_seconds,installed_by,installation_reason)
 VALUES($1,$2,'file-update','round163-controlled',repeat('a',64),$3,repeat('b',64),'active',20000,8000,20,900,$4,'Synthetic original-session boundary fixture')`, profile, f.org, agentworker.RuntimeVersion, f.admin); err != nil {
		t.Fatal(err)
	}
	r, v := f.request("POST", "/v1/conversations", f.aliceToken, core.NewID(), agent.ConversationCreate{Type: "private", ProjectID: base.File.ProjectID, Title: "文件原始登录与准确恢复", MemberIDs: []string{}}, nil)
	expect(t, r, 201, v)
	return &round163FileSessionFixture{u: u, s: s, cloud: cloud, hooks: hooks, base: base, c: data[agent.Conversation](t, v), profile: profile}
}

func (x *round163FileSessionFixture) request(t *testing.T, in agent.RunCreate) agent.Run {
	t.Helper()
	f := x.u.f.with(t)
	in.ExpectedVersion, in.ContextSecurityVersion, in.ProfileID, in.ClientMessageID = x.c.Version, x.c.ContextSecurityVersion, x.profile, core.NewID()
	in.Message = "仅操作明确选定的文件与草稿，不扩大授权，保留实际正式版本和扫描回执。"
	r, v := f.request("POST", "/v1/conversations/"+x.c.ID+"/runs", f.aliceToken, core.NewID(), in, nil)
	expect(t, r, 202, v)
	return data[agent.Run](t, v)
}

func (x *round163FileSessionFixture) run(t *testing.T, mode string) agent.Run {
	t.Helper()
	in := agent.RunCreate{}
	if mode == "create" {
		r, v := x.u.f.with(t).request("GET", "/v1/folders/"+x.base.File.FolderID, x.u.f.aliceToken, "", nil, nil)
		expect(t, r, 200, v)
		folder := data[storage.Folder](t, v)
		in.FileCreate = &agent.FileCreateInput{FolderID: folder.ID, ExpectedFolderVersion: folder.Version, Name: "准确新建-" + core.NewID() + ".md", MediaType: "text/markdown"}
	} else {
		in.FileUpdate = &agent.FileUpdateInput{FileID: x.base.File.ID, BaseVersionID: x.base.Version.ID, ExpectedFileVersion: x.base.File.Version}
	}
	return x.request(t, in)
}

func (x *round163FileSessionFixture) start(t *testing.T, run agent.Run) agent.ExecutionLease {
	t.Helper()
	ctx := context.Background()
	j, err := x.s.Queue().Claim(ctx, []string{agent.ExecutionJobKind}, "agent", "round163.file-session", agent.ExecutionLeaseDuration)
	if err != nil || j == nil {
		t.Fatal("claim exact file execution", err)
	}
	issued, err := x.s.IssueExecution(ctx, *j)
	if err != nil || issued.RunID != run.ID {
		t.Fatal("issue exact file execution", err)
	}
	l, err := x.s.LeaseForCredential(ctx, issued.Credential)
	if err != nil {
		t.Fatal(err)
	}
	l.Job = *j
	if err = x.s.RecordVerifiedConfinement(ctx, l, issued.Profile.ConfigurationSHA256, strings.Repeat("c", 64), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = x.s.StartExecution(ctx, l); err != nil {
		t.Fatal(err)
	}
	return l
}

func (x *round163FileSessionFixture) tool(l agent.ExecutionLease, name string, args any) (map[string]json.RawMessage, error) {
	raw, _ := json.Marshal(args)
	result, err := x.s.ExecutionTool(context.Background(), l, "round163:"+core.NewID(), name, raw)
	if err != nil {
		return nil, err
	}
	var out map[string]json.RawMessage
	err = json.Unmarshal(result, &out)
	return out, err
}

func (x *round163FileSessionFixture) stage(t *testing.T, run agent.Run, l agent.ExecutionLease) agent.FileUpdateView {
	t.Helper()
	name := "create_file_draft"
	args := map[string]any{"file_id": x.base.File.ID, "base_version_id": x.base.Version.ID, "edits": []storage.AgentTextEdit{{Start: 0, End: 3, Text: "新计划"}}}
	if run.FileCreate != nil {
		name = "stage_file_create"
		args = map[string]any{"folder_id": run.FileCreate.Target.FolderID, "name": run.FileCreate.Target.Name, "content": "准确新建内容\n"}
	}
	out, err := x.tool(l, name, args)
	if err != nil {
		t.Fatal("stage exact file bytes", err)
	}
	var view agent.FileUpdateView
	if json.Unmarshal(out["content"], &view) != nil || len(view.Drafts) != 1 || view.AuthorizationState != "current" {
		t.Fatal("staged view must retain exact live session state")
	}
	return view
}

func (x *round163FileSessionFixture) proposal(t *testing.T, run agent.Run, l agent.ExecutionLease, draft agent.FileDraftView) (string, map[string]any) {
	t.Helper()
	name, commit := "propose_file_update", "commit_file_update"
	args := map[string]any{"file_id": x.base.File.ID, "base_version_id": x.base.Version.ID, "draft_id": draft.ID}
	if run.FileCreate != nil {
		name, commit = "propose_file_create", "commit_file_create"
		args = map[string]any{"folder_id": run.FileCreate.Target.FolderID, "name": run.FileCreate.Target.Name, "draft_id": draft.ID}
	}
	out, err := x.tool(l, name, args)
	var view agent.FileUpdateView
	if err != nil || json.Unmarshal(out["content"], &view) != nil || len(view.Drafts) != 1 || view.Drafts[0].PlanHash == nil {
		t.Fatal("immutable exact draft proposal", err)
	}
	args["plan_hash"] = *view.Drafts[0].PlanHash
	return commit, args
}

func (x *round163FileSessionFixture) storageJob(t *testing.T, mode string) jobqueue.Job {
	t.Helper()
	ctx := context.Background()
	consumer, kinds, lane := storage.UploadConsumer(), storage.UploadJobKinds, storage.UploadLane
	if mode == "verify" {
		consumer, kinds, lane = storage.UploadIntegrityConsumer(), storage.VerifyJobKinds, storage.VerifySmallLane
	} else if mode == "promote" {
		consumer, kinds, lane = storage.UploadPromotionConsumer(), storage.PromotionJobKinds, storage.PromoteSmallLane
	}
	for range 20 {
		delivery, err := x.u.service.Queue.DispatchOne(ctx, consumer)
		if err != nil {
			t.Fatal(err)
		}
		if delivery == nil {
			break
		}
	}
	j, err := x.u.service.Queue.Claim(ctx, kinds, lane, "round163.storage."+mode, 2*time.Minute)
	if err != nil || j == nil {
		t.Fatal("claim exact native storage job", mode, err)
	}
	return *j
}

func (x *round163FileSessionFixture) storage(t *testing.T, mode string, scanner filescan.Scanner) error {
	t.Helper()
	j := x.storageJob(t, mode)
	ctx := context.Background()
	switch mode {
	case "verify":
		if scanner == nil {
			scanner = cleanIntegrityScanner()
		}
		return x.u.service.RunIntegrityJob(ctx, j, x.cloud, scanner)
	case "promote":
		return x.u.service.RunPromotionJob(ctx, j, x.cloud, x.cloud)
	default:
		return x.u.service.RunUploadJob(ctx, j, x.cloud)
	}
}

func (x *round163FileSessionFixture) original(t *testing.T, run string) core.Actor {
	t.Helper()
	a, err := x.u.f.api.Identity.Authenticate(context.Background(), x.u.f.aliceToken)
	if err != nil {
		t.Fatal(err)
	}
	var session, channel string
	if err = x.u.f.owner.QueryRow(context.Background(), "SELECT session_id::text,origin_channel FROM agent_file_intents WHERE run_id=$1", run).Scan(&session, &channel); err != nil || session != a.SessionID || channel != a.Channel {
		t.Fatal("new file intent did not bind the actual original login", err)
	}
	return a
}

func (x *round163FileSessionFixture) history(t *testing.T, run, state string) agent.Run {
	t.Helper()
	r, v := x.u.f.with(t).request("GET", "/v1/runs/"+run, x.u.f.aliceToken, "", nil, nil)
	expect(t, r, 200, v)
	view := data[agent.Run](t, v)
	actual := ""
	if view.FileUpdate != nil {
		actual = view.FileUpdate.AuthorizationState
	} else if view.FileCreate != nil {
		actual = view.FileCreate.AuthorizationState
	}
	if actual != state {
		t.Fatalf("historical authorization state %q, want %q", actual, state)
	}
	if strings.Contains(string(v.Data), "session_id") || strings.Contains(string(v.Data), "origin_channel") {
		t.Fatal("private original session provenance leaked into a run receipt")
	}
	return view
}

func (x *round163FileSessionFixture) noCommit(t *testing.T, run string) {
	t.Helper()
	var count int
	if err := x.u.f.owner.QueryRow(context.Background(), `SELECT
 (SELECT count(*) FROM agent_file_commits WHERE run_id=$1)
 +(SELECT count(*) FROM file_versions WHERE org_id=$2 AND source='agent')
 +(SELECT count(*) FROM idempotency_records WHERE org_id=$2 AND state='succeeded'
 AND idempotency_key IN ('agent-file-update:'||$1::text,'agent-file-create:'||$1::text))`, run, x.u.f.org).Scan(&count); err != nil || count != 0 {
		t.Fatal("revoked delegation left a formal version/ordinary command/commit receipt", count, err)
	}
}

func TestRound163FileOriginalSessions(t *testing.T) {
	ctx := context.Background()
	for _, mode := range []string{"update", "create"} {
		t.Run("live_"+mode+"_keeps_ordinary_receipt", func(t *testing.T) {
			x := round163FileSessionSetup(t)
			run := x.run(t, mode)
			x.original(t, run.ID)
			l := x.start(t, run)
			view := x.stage(t, run, l)
			commit, args := x.proposal(t, run, l, view.Drafts[0])
			for _, stage := range []string{"control", "verify", "promote"} {
				if err := x.storage(t, stage, nil); err != nil {
					t.Fatal("live original-session pipeline", stage, err)
				}
			}
			out, err := x.tool(l, commit, args)
			if err != nil || string(out["committed"]) != "true" {
				t.Fatal("live original session failed exact ordinary commit", err)
			}
			x.history(t, run.ID, "current")
		})
		t.Run("logout_and_new_login_cannot_revive_"+mode, func(t *testing.T) {
			x := round163FileSessionSetup(t)
			run := x.run(t, mode)
			a := x.original(t, run.ID)
			l := x.start(t, run)
			view := x.stage(t, run, l)
			commit, args := x.proposal(t, run, l, view.Drafts[0])
			for _, stage := range []string{"control", "verify", "promote"} {
				if err := x.storage(t, stage, nil); err != nil {
					t.Fatal(err)
				}
			}
			if err := x.u.f.api.Identity.Logout(ctx, a, core.NewID()); err != nil {
				t.Fatal(err)
			}
			x.u.f.aliceToken = x.u.f.login("alice", "android").AccessToken
			if _, err := x.tool(l, commit, args); core.PublicError(err).Code != "AUTHORIZATION_CHANGED" {
				t.Fatal("another login revived revoked file authority", err)
			}
			x.history(t, run.ID, "changed")
			x.noCommit(t, run.ID)
			// Bypass the controller using the trusted lower ordinary boundary:
			// it must independently reject the same revoked original session.
			a.RunID = run.ID
			var uploadVersion int64
			if err := x.u.f.owner.QueryRow(ctx, "SELECT version FROM upload_sessions WHERE id=$1", view.Drafts[0].ID).Scan(&uploadVersion); err != nil {
				t.Fatal(err)
			}
			in := storage.UploadCommit{ExpectedVersion: uploadVersion, SHA256: view.Drafts[0].ContentSHA256}
			route, prefix := "/internal/agent-files/create", "agent-file-create:"
			if mode == "update" {
				in.BaseVersionID = &x.base.Version.ID
				route, prefix = "/internal/agent-files/commit", "agent-file-update:"
			}
			body, _ := json.Marshal(in)
			_, err := x.u.service.CommitAgentDraft(ctx, core.Command{Actor: a, RequestID: core.NewID(), Route: route, Key: prefix + run.ID, Body: body}, view.Drafts[0].ID, in)
			if core.PublicError(err).Code != "AUTHORIZATION_CHANGED" {
				t.Fatal("trusted ordinary upload boundary ignored original logout", err)
			}
			x.noCommit(t, run.ID)
		})
	}

	t.Run("logout_during_external_draft_io_stops_stage_receipt", func(t *testing.T) {
		x := round163FileSessionSetup(t)
		run := x.run(t, "create")
		a := x.original(t, run.ID)
		l := x.start(t, run)
		x.hooks.after = func() {
			if err := x.u.f.api.Identity.Logout(ctx, a, core.NewID()); err != nil {
				t.Fatal(err)
			}
		}
		_, err := x.tool(l, "stage_file_create", map[string]any{"folder_id": run.FileCreate.Target.FolderID, "name": run.FileCreate.Target.Name, "content": "隔离草稿字节\n"})
		if core.PublicError(err).Code != "AUTHORIZATION_CHANGED" {
			t.Fatal("external I/O callback published under a revoked login", err)
		}
		var count int
		if err = x.u.f.owner.QueryRow(ctx, "SELECT count(*) FROM agent_file_stages WHERE run_id=$1", run.ID).Scan(&count); err != nil || count != 0 {
			t.Fatal("stage survived revoked external I/O", count, err)
		}
		x.noCommit(t, run.ID)
	})

	t.Run("expiry_during_scan_stops_worker_and_sql_evidence", func(t *testing.T) {
		x := round163FileSessionSetup(t)
		run := x.run(t, "update")
		a := x.original(t, run.ID)
		l := x.start(t, run)
		view := x.stage(t, run, l)
		if err := x.storage(t, "control", nil); err != nil {
			t.Fatal(err)
		}
		scanner := integrityScanner(func(ctx context.Context, source io.ReadCloser, size int64) (filescan.Result, error) {
			result, err := cleanIntegrityScanner().Scan(ctx, source, size)
			if _, e := x.u.f.owner.Exec(ctx, "UPDATE sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1", a.SessionID); e != nil {
				t.Fatal(e)
			}
			return result, err
		})
		if err := x.storage(t, "verify", scanner); core.PublicError(err).Code != "AUTHORIZATION_CHANGED" {
			t.Fatal("scan callback advanced an expired delegation", err)
		}
		var clean int
		if err := x.u.f.owner.QueryRow(ctx, "SELECT count(*) FROM upload_sessions WHERE id=$1 AND (state='ready' OR scan_state='clean')", view.Drafts[0].ID).Scan(&clean); err != nil || clean != 0 {
			t.Fatal("expired scan published positive evidence", clean, err)
		}
		// Application INSERT privileges cannot forge new evidence after expiry.
		_, err := x.u.f.api.DB.Exec(ctx, `INSERT INTO agent_file_drafts(id,org_id,run_id,invocation_key,input_sha256,edits,content_sha256,size_bytes,diff,source_fingerprint)
 SELECT $1,org_id,run_id,$2,input_sha256,edits,content_sha256,size_bytes,diff,source_fingerprint FROM agent_file_drafts WHERE id=$3`, core.NewID(), core.NewID(), view.Drafts[0].ID)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "23514" || !strings.Contains(pg.Message, "original session") {
			t.Fatal("SQL did not reject expired original-session evidence", err)
		}
		x.u.f.aliceToken = x.u.f.login("alice", "android").AccessToken
		x.history(t, run.ID, "changed")
		x.noCommit(t, run.ID)
	})

	t.Run("logout_during_promotion_stops_ready_publication", func(t *testing.T) {
		x := round163FileSessionSetup(t)
		run := x.run(t, "create")
		a := x.original(t, run.ID)
		l := x.start(t, run)
		view := x.stage(t, run, l)
		for _, stage := range []string{"control", "verify"} {
			if err := x.storage(t, stage, nil); err != nil {
				t.Fatal(err)
			}
		}
		x.cloud.onWrite = func() {
			if err := x.u.f.api.Identity.Logout(ctx, a, core.NewID()); err != nil {
				t.Fatal(err)
			}
		}
		if err := x.storage(t, "promote", nil); core.PublicError(err).Code != "AUTHORIZATION_CHANGED" {
			t.Fatal("promotion callback advanced a revoked delegation", err)
		}
		var ready int
		if err := x.u.f.owner.QueryRow(ctx, "SELECT count(*) FROM upload_sessions WHERE id=$1 AND state='ready'", view.Drafts[0].ID).Scan(&ready); err != nil || ready != 0 {
			t.Fatal("revoked promotion published a ready candidate", ready, err)
		}
		x.u.f.aliceToken = x.u.f.login("alice", "android").AccessToken
		x.history(t, run.ID, "changed")
		x.noCommit(t, run.ID)
	})

	t.Run("legacy_bytes_remain_readable_and_explicit_recovery_binds_new_login", func(t *testing.T) {
		x := round163FileSessionSetup(t)
		run := x.run(t, "update")
		l := x.start(t, run)
		view := x.stage(t, run, l)
		var before string
		if err := x.u.f.owner.QueryRow(ctx, "SELECT intent||intent_sha256 FROM agent_file_intents WHERE run_id=$1", run.ID).Scan(&before); err != nil {
			t.Fatal(err)
		}
		// Privileged isolated history simulation only: emulate columns added to
		// pre129 rows without changing any original intent/proof bytes. The
		// production INSERT/immutable guards are never relaxed for the app role.
		tx, err := x.u.f.owner.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, "SET LOCAL session_replication_role=replica"); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, "UPDATE agent_file_intents SET session_id=NULL,origin_channel=NULL WHERE run_id=$1", run.ID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err = x.tool(l, "inspect_operation", map[string]any{"run_id": run.ID}); core.PublicError(err).Code != "AUTHORIZATION_CHANGED" {
			t.Fatal("legacy unbound intent continued execution", err)
		}
		x.history(t, run.ID, "legacy_unbound")
		var version int
		if err = x.u.f.owner.QueryRow(ctx, "SELECT version FROM agent_runs WHERE id=$1", run.ID).Scan(&version); err != nil {
			t.Fatal(err)
		}
		response, body := x.u.f.with(t).request("POST", "/v1/runs/"+run.ID+"/cancel", x.u.f.aliceToken, core.NewID(), map[string]any{"expected_version": version, "action": "cancel", "reason": "明确停止旧运行后，以新登录恢复准确草稿"}, nil)
		expect(t, response, 200, body)
		if err = x.s.FinishStoppedExecution(ctx, l.Job, "RUNTIME_AUTHORITY_CHANGED"); err != nil {
			t.Fatal(err)
		}
		x.u.f.aliceToken = x.u.f.login("alice", "android").AccessToken
		recovered := x.request(t, agent.RunCreate{FileRecovery: &agent.FileRecoveryInput{SourceRunID: run.ID, DraftID: view.Drafts[0].ID, ContentSHA256: view.Drafts[0].ContentSHA256, Reviewed: true}})
		a := x.original(t, recovered.ID)
		var recoverySession, after string
		if err = x.u.f.owner.QueryRow(ctx, "SELECT recovery_body::jsonb->>'session_id' FROM agent_file_recoveries WHERE run_id=$1", recovered.ID).Scan(&recoverySession); err != nil || recoverySession != a.SessionID {
			t.Fatal("recovery did not bind the fresh explicit login", err)
		}
		if err = x.u.f.owner.QueryRow(ctx, "SELECT intent||intent_sha256 FROM agent_file_intents WHERE run_id=$1", run.ID).Scan(&after); err != nil || before != after {
			t.Fatal("legacy original intent bytes were rewritten", err)
		}
		newLease := x.start(t, recovered)
		out, err := x.tool(newLease, "recover_file_draft", map[string]any{"run_id": recovered.ID, "recovery_sha256": recovered.FileUpdate.Recovery.RecoverySHA256})
		var fresh agent.FileUpdateView
		if err != nil || json.Unmarshal(out["content"], &fresh) != nil || len(fresh.Drafts) != 1 || fresh.Drafts[0].ID == view.Drafts[0].ID || fresh.Drafts[0].ContentSHA256 != view.Drafts[0].ContentSHA256 || fresh.AuthorizationState != "current" {
			t.Fatal("new session did not restage the exact legacy saved bytes", err)
		}
		x.noCommit(t, recovered.ID)
	})
}

// Separate opt-in browser fixture; omitted from ordinary native execution.
// The caller owns and closes the returned HTTP server through the existing
// /__fixture/stop route. It includes no provider/model/runtime integration claim.
func TestRound163FileSessionBrowserFixture(t *testing.T) {
	if os.Getenv("CP_BROWSER_ROUND163") != "1" || os.Getenv("CP_BROWSER_BUSINESS_FIXTURE") != "1" {
		t.Skip("explicit isolated round163 browser fixture only")
	}
	x := round163FileSessionSetup(t)
	run := x.run(t, "create")
	a := x.original(t, run.ID)
	l := x.start(t, run)
	view := x.stage(t, run, l)
	if err := x.u.f.api.Identity.Logout(context.Background(), a, core.NewID()); err != nil {
		t.Fatal(err)
	}
	if err := x.s.FinishStoppedExecution(context.Background(), l.Job, "RUNTIME_AUTHORITY_CHANGED"); err != nil {
		t.Fatal(err)
	}
	// The shared browser harness requires a genuine enrolled MFA account.
	// Grant only an actionless MFA role, then use the normal enrollment/verify
	// handlers; never manufacture mfa_verified_at or reset the replay counter.
	f := x.u.f
	ctx := context.Background()
	proof := core.NewID()
	for _, command := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO roles(id,org_id,key,name,requires_mfa) VALUES($1,$2,'round163_browser_proof','Actionless browser MFA proof',true)", []any{proof, f.org}},
		{"INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) VALUES($1,$2,$3,$4,'org')", []any{core.NewID(), f.org, f.alice, proof}},
		{"UPDATE users SET auth_version=auth_version+1 WHERE id=$1", []any{f.alice}},
		{"UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", []any{f.org}},
	} {
		if _, err := f.owner.Exec(ctx, command.sql, command.args...); err != nil {
			t.Fatal(err)
		}
	}
	f.aliceToken = memberMFAToken(f, "alice", "android")
	x.history(t, run.ID, "changed")
	holdBrowserBusinessFixture(t, f, map[string]any{"fixture_kind": "batch163-file-original-session", "round163_file_session": true, "project_id": *x.base.File.ProjectID, "file_id": x.base.File.ID, "source_version_id": x.base.Version.ID, "conversation_id": x.c.ID, "profile_id": x.profile, "authorization_run_id": run.ID, "authorization_draft_id": view.Drafts[0].ID, "authorization_state": "changed"})
}
