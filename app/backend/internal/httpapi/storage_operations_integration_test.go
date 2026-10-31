package httpapi

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/identity"
)

func TestStorageOperations(t *testing.T) {
	f, committed, primary, secondary, id := committedReplicaFixture(t, 15)
	ctx := context.Background()
	current := t
	exec := func(q string, args ...any) {
		current.Helper()
		if _, err := f.f.owner.Exec(ctx, q, args...); err != nil {
			current.Fatal(err)
		}
	}
	run := func(name string, fn func()) {
		if !t.Run(name, func(st *testing.T) { current = st; f.f.t = st; defer func() { current = t; f.f.t = t }(); fn() }) {
			t.FailNow()
		}
	}
	run("ordinary_file_authority_does_not_grant_organization_inventory", func() {
		var otherRoleActions int
		if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM role_actions a JOIN roles r ON r.org_id=a.org_id AND r.id=a.role_id WHERE r.org_id=$1 AND r.key='agent_runtime_manager' AND a.action IN('storage.operations.read','storage.operations.retry')", f.f.org).Scan(&otherRoleActions); err != nil || otherRoleActions != 0 {
			current.Fatal("unrelated bootstrap role gained storage authority", otherRoleActions, err)
		}
		r, v := f.f.request("GET", "/v1/storage/replicas", f.f.aliceToken, "", nil, nil)
		expect(current, r, 404, v)
	})
	bind := func(user, role string) string {
		bid := core.NewID()
		exec("INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key=$4", bid, f.f.org, user, role)
		return bid
	}
	operator := bind(f.f.alice, "storage_operations_operator")
	bind(f.f.member, "storage_operations_reader")
	exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.f.org)
	f.f.aliceToken = memberMFAToken(f.f, "alice", "android")
	reader := f.f.login("member", "android").AccessToken
	admin := f.f.administratorToken()
	login := func(device string) (string, map[string]string) {
		var cipher []byte
		if err := f.f.owner.QueryRow(ctx, "SELECT secret_cipher FROM mfa_credentials WHERE org_id=$1 AND user_id=$2", f.f.org, f.f.alice).Scan(&cipher); err != nil {
			current.Fatal(err)
		}
		secret, err := identity.Open(f.f.api.Config.MFAKey, cipher, f.f.org+":"+f.f.alice)
		if err != nil {
			current.Fatal("owned credential unavailable")
		}
		exec("UPDATE mfa_credentials SET last_step=$3 WHERE org_id=$1 AND user_id=$2", f.f.org, f.f.alice, time.Now().Unix()/30-1)
		challenge := f.f.login("alice", device)
		headers := map[string]string{}
		if device == "web" {
			headers["Origin"] = "http://localhost:5178"
		}
		r, v := f.f.request("POST", "/v1/auth/mfa/verify", "", "", map[string]string{"challenge": challenge.Challenge, "code": authenticatorCode(secret, time.Now())}, headers)
		expect(current, r, 200, v)
		l := data[identity.LoginResult](current, v)
		if device == "web" {
			headers["X-CSRF-Token"] = l.CSRFToken
			for _, c := range r.Result().Cookies() {
				if c.Name == "cp_session" {
					headers["Cookie"] = "cp_session=" + c.Value
				}
			}
			return "", headers
		}
		return l.AccessToken, headers
	}
	get := func(token string, headers map[string]string) storage.StorageReplicaDetails {
		r, v := f.f.request("GET", "/v1/storage/replicas/"+id, token, "", nil, headers)
		expect(current, r, 200, v)
		return data[storage.StorageReplicaDetails](current, v)
	}
	input := func(v storage.StorageReplicaDetails) storage.StorageReplicaRetry {
		return storage.StorageReplicaRetry{ExpectedVersion: v.Item.Version, BasisSHA256: v.Item.BasisSHA256, Reviewed: true, Reason: "已核对固定副本与来源，安排人工重试"}
	}
	path := "/v1/storage/replicas/" + id + "/retry"
	secondary.corrupt = true
	host := replicaHost(f, primary, secondary, 15)
	runReplica(t, host)
	run("metadata_reader_sees_failure_and_fixed_ids_without_content_or_provider_details", func() {
		r, v := f.f.request("GET", "/v1/storage/replicas?provider=r2&state=failed", reader, "", nil, nil)
		expect(current, r, 200, v)
		list := data[core.Page[storage.StorageReplicaOperation]](current, v)
		if len(list.Items) != 1 || list.Items[0].ID != id || list.Items[0].Job == nil || list.Items[0].Job.State != "dead" {
			current.Fatal("missing real failed replica metadata")
		}
		for _, key := range []string{"bucket", "object_key", "source_id", "sha256", "name", "payload", "last_error", "reason", "session_id"} {
			if strings.Contains(r.Body.String(), `"`+key+`"`) {
				current.Fatal("private field exposed", key)
			}
		}
		if get(reader, nil).CanRetry {
			current.Fatal("metadata reader received retry authority")
		}
		r, v = f.f.request("POST", path, reader, core.NewID(), input(get(reader, nil)), nil)
		expect(current, r, 403, v)
	})
	run("organization_operator_does_not_gain_ordinary_file_content_or_retry_authority", func() {
		v := get(admin, nil)
		if v.CanOpen || v.CanRetry {
			current.Fatal("system administrator gained private project content authority")
		}
		r, res := f.f.request("GET", "/v1/files/"+committed.File.ID, admin, "", nil, nil)
		expect(current, r, 404, res)
		r, res = f.f.request("POST", path, admin, core.NewID(), input(v), nil)
		expect(current, r, 404, res)
	})
	run("unknown_provider_error_is_replaced_with_fixed_public_code", func() {
		exec("UPDATE file_replicas SET last_error='PRIVATE_PROVIDER_DETAIL fixture-bucket/private-key' WHERE id=$1", id)
		v := get(f.f.aliceToken, nil)
		if v.Item.ErrorCode == nil || *v.Item.ErrorCode != "OTHER_STORAGE_FAILURE" {
			current.Fatal("unsafe provider error was not masked")
		}
	})
	run("review_binds_job_progress_even_without_replica_version_change", func() {
		before := get(f.f.aliceToken, nil)
		exec("UPDATE jobs SET updated_at=clock_timestamp()+interval '1 millisecond' WHERE id=$1", before.Item.Job.ID)
		r, v := f.f.request("POST", path, f.f.aliceToken, core.NewID(), input(before), nil)
		expect(current, r, 409, v)
		after := get(f.f.aliceToken, nil)
		if after.Item.Version != before.Item.Version || after.Item.BasisSHA256 == before.Item.BasisSHA256 {
			current.Fatal("job progress was not included in review basis")
		}
	})
	run("strict_filters_and_explicit_review_reject_ambiguous_inputs", func() {
		for _, q := range []string{"?provider=oss&provider=r2", "?q=private", "?job_state=unknown"} {
			r, v := f.f.request("GET", "/v1/storage/replicas"+q, f.f.aliceToken, "", nil, nil)
			expect(current, r, 422, v)
		}
		in := input(get(f.f.aliceToken, nil))
		in.Reviewed = false
		r, v := f.f.request("POST", path, f.f.aliceToken, core.NewID(), in, nil)
		expect(current, r, 422, v)
		r, v = f.f.request("GET", "/v1/storage/replicas/"+core.NewID(), f.f.aliceToken, "", nil, nil)
		expect(current, r, 404, v)
	})
	var first storage.StorageReplicaReceipt
	var firstInput storage.StorageReplicaRetry
	var firstKey string
	var webHeaders map[string]string
	for i, channel := range []string{"web", "wechat", "douyin", "android"} {
		run(channel+"_reviewed_retry_receipt_and_replay_preserve_ordinary_authority", func() {
			token, headers := login(channel)
			before := get(token, headers)
			in := input(before)
			key := core.NewID()
			if !before.CanRetry || !before.CanOpen {
				current.Fatal("ordinary file operator cannot review known failed copy")
			}
			if channel == "web" {
				exec("CREATE FUNCTION reject_storage_operation_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated receipt insertion rejection' USING ERRCODE='23514'; END; $$")
				exec("CREATE TRIGGER reject_storage_operation_fixture BEFORE INSERT ON storage_replica_retries FOR EACH ROW EXECUTE FUNCTION reject_storage_operation_fixture()")
				r, v := f.f.request("POST", path, token, key, in, headers)
				expect(current, r, 422, v)
				state, version, _ := replicaState(current, f, id)
				if state != "failed" || version != before.Item.Version {
					current.Fatal("failed coordinator did not roll back ordinary retry")
				}
				var n int
				if err := f.f.owner.QueryRow(ctx, "SELECT count(*) FROM idempotency_records WHERE org_id=$1 AND idempotency_key IN($2,$3)", f.f.org, key, "storage-retry:"+core.Digest([]byte(key))).Scan(&n); err != nil || n != 0 {
					current.Fatal("failed receipt left command keys", n, err)
				}
				exec("DROP TRIGGER reject_storage_operation_fixture ON storage_replica_retries")
				exec("DROP FUNCTION reject_storage_operation_fixture()")
			}
			r, v := f.f.request("POST", path, token, key, in, headers)
			expect(current, r, 200, v)
			receipt := data[storage.StorageReplicaReceipt](current, v)
			if receipt.ReplicaID != id || receipt.Before.Version != before.Item.Version || receipt.After.Version != before.Item.Version+1 || receipt.After.State != "queued" || receipt.After.Job != nil || receipt.BasisSHA256 != in.BasisSHA256 || receipt.BusinessOperationID == "" {
				current.Fatal("incorrect ordinary retry receipt")
			}
			r, v = f.f.request("POST", path, token, key, in, headers)
			expect(current, r, 200, v)
			if r.Header().Get("X-Idempotent-Replay") != "true" || data[storage.StorageReplicaReceipt](current, v).ID != receipt.ID {
				current.Fatal("retry did not preserve receipt")
			}
			if i == 0 {
				first, firstInput, firstKey, webHeaders = receipt, in, key, headers
			}
			if i < 3 {
				runReplica(current, host)
			} else {
				f.f.aliceToken = token
			}
		})
	}
	run("worker_verification_and_exact_original_retry_receipt_are_separate_facts", func() {
		secondary.corrupt = false
		runReplica(current, host)
		v := get(f.f.aliceToken, nil)
		if v.Item.State != "verified" || v.CanRetry {
			current.Fatal("copy never completed verification")
		}
		r, res := f.f.request("POST", path, "", firstKey, firstInput, webHeaders)
		expect(current, r, 200, res)
		saved := data[storage.StorageReplicaReceipt](current, res)
		if saved.ID != first.ID || saved.After.State != "queued" || saved.After.Version != first.After.Version {
			current.Fatal("historical receipt was relabeled with later worker state")
		}
	})
	run("immutable_history_pages_are_complete_and_cursor_bound_to_current_reader", func() {
		r, v := f.f.request("GET", "/v1/storage/replica-retries?replica_id="+id+"&limit=2", reader, "", nil, nil)
		expect(current, r, 200, v)
		page := data[core.Page[storage.StorageReplicaReceipt]](current, v)
		if len(page.Items) != 2 || page.NextCursor == nil {
			current.Fatal("missing bounded receipt history")
		}
		r, v = f.f.request("GET", "/v1/storage/replica-retries?replica_id="+id+"&limit=2&cursor="+*page.NextCursor, reader, "", nil, nil)
		expect(current, r, 200, v)
		rest := data[core.Page[storage.StorageReplicaReceipt]](current, v)
		if len(rest.Items) != 2 || rest.NextCursor != nil {
			current.Fatal("incomplete history")
		}
		r, v = f.f.request("GET", "/v1/storage/replica-retries?replica_id="+id+"&cursor="+*page.NextCursor, f.f.aliceToken, "", nil, nil)
		expect(current, r, 400, v)
		for _, q := range []string{"UPDATE storage_replica_retries SET reason='changed' WHERE id=$1", "DELETE FROM storage_replica_retries WHERE id=$1"} {
			if _, err := f.f.owner.Exec(ctx, q, first.ID); err == nil {
				current.Fatal("immutable receipt changed")
			}
		}
	})
	run("cached_receipts_recheck_recent_mfa_and_revoked_organization_authority", func() {
		exec("UPDATE sessions SET mfa_verified_at=clock_timestamp()-interval '11 minutes' WHERE org_id=$1 AND user_id=$2 AND device_type='web'", f.f.org, f.f.alice)
		r, v := f.f.request("POST", path, "", firstKey, firstInput, webHeaders)
		expect(current, r, 403, v)
		exec("DELETE FROM role_bindings WHERE id=$1", operator)
		exec("UPDATE organizations SET authorization_version=authorization_version+1 WHERE id=$1", f.f.org)
		token, h := login("android")
		r, v = f.f.request("POST", path, token, firstKey, firstInput, h)
		expect(current, r, 404, v)
	})
	t.Run("browser_storage_operations", func(t *testing.T) {
		if os.Getenv("CP_BROWSER_STORAGE_OPERATIONS_FIXTURE") != "1" {
			return
		}
		b, c, p, s, rid := committedReplicaFixture(t, 15)
		s.corrupt = true
		runReplica(t, replicaHost(b, p, s, 15))
		if _, err := b.f.owner.Exec(ctx, "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type) SELECT $1,$2,$3,id,'org' FROM roles WHERE org_id=$2 AND key='storage_operations_operator'", core.NewID(), b.f.org, b.f.alice); err != nil {
			t.Fatal(err)
		}
		_ = memberMFAToken(b.f, "alice", "android")
		holdBrowserBusinessFixture(t, b.f, map[string]any{"fixture_kind": "storage-operations", "replica_id": rid, "file_id": c.File.ID})
	})
}
