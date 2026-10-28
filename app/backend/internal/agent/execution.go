package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"slices"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const ExecutionJobKind = "agent.execute.v1"
const ExecutionLeaseDuration = 90 * time.Second

type Execution struct {
	RunID           string         `json:"id"`
	Channel         string         `json:"channel"`
	AuthorityStamp  string         `json:"authority_stamp"`
	SkillSHA256     string         `json:"skill_sha256"`
	ExpiresAt       time.Time      `json:"expires_at"`
	Credential      string         `json:"task_credential"`
	Input           map[string]any `json:"input"`
	PreparedContext string         `json:"prepared_context"`
	MaxToolCalls    int            `json:"max_tool_calls"`
	MaxInputTokens  int            `json:"max_input_tokens"`
	MaxOutputTokens int            `json:"max_output_tokens"`
	Profile         RuntimeProfile `json:"profile"`
}
type ExecutionLease struct {
	RunID, OrgID, AuthorityStamp string
	credentialSHA256             string
	Job                          jobqueue.Job
	ExpiresAt                    time.Time
}
type executionState struct {
	Run          Run
	Actor        core.Actor
	Conversation Conversation
	Profile      RuntimeProfile
	Message      string
	Sources      []SourceSnapshot
}

func runtimeFailure(code string) error {
	return core.Fail(409, code, "秘书执行范围、租约或停止状态已变化，请核对任务结果")
}
func (s *Service) Queue() jobqueue.Queue { return jobqueue.Queue{DB: s.DB} }

func (s *Service) executionMapping(ctx context.Context, j jobqueue.Job) (string, error) {
	if j.Kind != ExecutionJobKind || j.Lane != "agent" || j.PayloadVersion != 1 || j.MaxAttempts != 1 || j.Attempt != 1 {
		return "", runtimeFailure("RUNTIME_JOB_INVALID")
	}
	var input struct {
		RunID string `json:"run_id"`
	}
	if json.Unmarshal(j.Payload, &input) != nil || !core.ValidID(input.RunID) {
		return "", runtimeFailure("RUNTIME_JOB_INVALID")
	}
	var id string
	err := s.DB.QueryRow(ctx, "SELECT run_id FROM agent_run_jobs WHERE org_id=$1 AND job_id=$2 AND run_id=$3", j.OrgID, j.ID, input.RunID).Scan(&id)
	return id, platform.DatabaseError(err)
}

// Locks organization, actor, conversation, run, then job. The short organization
// write lock fences business/source changes through the existing command lock order.
// No model or network I/O lives in this transaction; first-phase scope is three users.
func (s *Service) executionTx(ctx context.Context, lease ExecutionLease, authority bool) (pgx.Tx, executionState, error) {
	var st executionState
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, st, err
	}
	fail := func(e error) (pgx.Tx, executionState, error) { _ = tx.Rollback(ctx); return nil, st, e }
	var orgVersion int64
	if err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR UPDATE", lease.OrgID).Scan(&orgVersion); err != nil {
		return fail(err)
	}
	var actorVersion, userVersion, runOrgVersion, security int64
	var status, revision string
	if err = tx.QueryRow(ctx, `SELECT actor_id,actor_auth_version,organization_auth_version,channel,conversation_revision_id,context_security_version
 FROM agent_runs WHERE org_id=$1 AND id=$2`, lease.OrgID, lease.RunID).Scan(&st.Actor.UserID, &actorVersion, &runOrgVersion, &st.Actor.Channel, &revision, &security); err != nil {
		return fail(platform.DatabaseError(err))
	}
	st.Actor.OrgID = lease.OrgID
	st.Actor.AuthVersion = actorVersion
	st.Actor.OrgAuthVersion = orgVersion
	if err = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", lease.OrgID, st.Actor.UserID).Scan(&userVersion, &status); err != nil {
		return fail(err)
	}
	var conversationID string
	if err = tx.QueryRow(ctx, "SELECT c.id FROM conversations c JOIN agent_runs r ON r.org_id=c.org_id AND r.conversation_id=c.id WHERE r.org_id=$1 AND r.id=$2 FOR SHARE OF c", lease.OrgID, lease.RunID).Scan(&conversationID); err != nil {
		return fail(err)
	}
	st.Conversation, err = metadata(ctx, tx, lease.OrgID, conversationID, false)
	if err != nil {
		return fail(err)
	}
	st.Run, err = runRecord(ctx, tx, lease.OrgID, lease.RunID, true)
	if err != nil {
		return fail(err)
	}
	if err = s.Queue().CheckInTx(ctx, tx, lease.Job); err != nil {
		return fail(err)
	}
	var expires time.Time
	if err = tx.QueryRow(ctx, "SELECT expires_at FROM agent_runs WHERE org_id=$1 AND id=$2", lease.OrgID, lease.RunID).Scan(&expires); err != nil {
		return fail(err)
	}
	if !slices.Contains([]string{"queued", "running", "awaiting_approval", "committing"}, st.Run.State) {
		return fail(runtimeFailure("RUNTIME_TERMINAL"))
	}
	if authority {
		if status != "active" || userVersion != actorVersion || orgVersion != runOrgVersion || st.Conversation.CurrentRevisionID != revision || st.Conversation.ContextSecurityVersion != security || st.Conversation.State != "active" {
			return fail(runtimeFailure("RUNTIME_AUTHORITY_CHANGED"))
		}
		if st.Run.CancelRequestedAt != nil {
			return fail(runtimeFailure("RUNTIME_CANCEL_REQUESTED"))
		}
		var live bool
		if err = tx.QueryRow(ctx, "SELECT $1::timestamptz>clock_timestamp()", expires).Scan(&live); err != nil {
			return fail(err)
		}
		if !live {
			return fail(runtimeFailure("RUNTIME_DEADLINE_EXCEEDED"))
		}
		if err = telegramExecutionAuthority(ctx, tx, st); err != nil {
			return fail(err)
		}
		if err = require(ctx, tx, st.Actor, st.Conversation, "agent.run"); err != nil {
			return fail(runtimeFailure("RUNTIME_AUTHORITY_CHANGED"))
		}
		members, e := aiAudience(ctx, tx, st.Actor, st.Conversation)
		if e != nil {
			return fail(runtimeFailure("RUNTIME_AUTHORITY_CHANGED"))
		}
		if !slices.Equal(members, st.Run.audience) {
			return fail(runtimeFailure("RUNTIME_AUTHORITY_CHANGED"))
		}
		st.Profile, err = scanProfile(tx.QueryRow(ctx, "SELECT "+profileColumns+" FROM agent_runtime_profiles WHERE org_id=$1 AND id=$2 AND state='active'", lease.OrgID, st.Run.ProfileID))
		if err != nil {
			return fail(runtimeFailure("RUNTIME_PROFILE_RETIRED"))
		}
		var raw []byte
		if st.Actor.Channel == "telegram" {
			var digest *string
			if err = tx.QueryRow(ctx, "SELECT telegram_configuration_sha256 FROM agent_runtime_profiles WHERE org_id=$1 AND id=$2", lease.OrgID, st.Run.ProfileID).Scan(&digest); err != nil {
				return fail(err)
			}
			if digest == nil || (st.Profile.SkillID != "project-briefing" && st.Profile.SkillID != "incident-summary") {
				return fail(runtimeFailure("RUNTIME_PROFILE_RETIRED"))
			}
			st.Profile.ConfigurationSHA256 = *digest
		}
		if err = tx.QueryRow(ctx, "SELECT body,source_snapshots FROM conversation_messages WHERE org_id=$1 AND run_id=$2 AND role='user'", lease.OrgID, lease.RunID).Scan(&st.Message, &raw); err != nil {
			return fail(err)
		}
		if err = json.Unmarshal(raw, &st.Sources); err != nil {
			return fail(err)
		}
		if err = executionSources(ctx, tx, st, st.Sources); err != nil {
			return fail(err)
		}
		if _, err = executionMutationState(ctx, tx, st); err != nil {
			return fail(err)
		}
		if _, err = s.executionFileIntent(ctx, tx, st); err != nil {
			return fail(err)
		}
		if _, err = executionOARequestState(ctx, tx, st); err != nil {
			return fail(err)
		}
		if _, err = executionPostWriteState(ctx, tx, st); err != nil {
			return fail(err)
		}
	}
	return tx, st, nil
}
func executionSources(ctx context.Context, tx pgx.Tx, st executionState, snapshots []SourceSnapshot) error {
	if p, e := readPostWrite(ctx, tx, st.Actor.OrgID, st.Run.ID); e != nil {
		return e
	} else if p != nil {
		if e = postWriteReferences(ctx, tx, st.Actor, p, snapshots); e != nil {
			return e
		}
	}
	refs := make([]SourceReference, 0, len(snapshots))
	for _, v := range snapshots {
		refs = append(refs, v.SourceReference)
	}
	current, err := sourceSnapshots(ctx, tx, st.Actor, st.Conversation, refs, st.Run.audience, true)
	if err != nil {
		return runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	a, _ := json.Marshal(current)
	b, _ := json.Marshal(snapshots)
	if string(a) != string(b) {
		return runtimeFailure("RUNTIME_SOURCE_CHANGED")
	}
	return nil
}
func (s *Service) IssueExecution(ctx context.Context, j jobqueue.Job) (Execution, error) {
	var out Execution
	runID, err := s.executionMapping(ctx, j)
	if err != nil {
		return out, err
	}
	lease := ExecutionLease{RunID: runID, OrgID: j.OrgID, Job: j}
	tx, st, err := s.executionTx(ctx, lease, true)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if st.Run.State != "queued" {
		return out, runtimeFailure("RUNTIME_ALREADY_STARTED")
	}
	token := make([]byte, 48)
	if _, err = rand.Read(token); err != nil {
		return out, err
	}
	credential := hex.EncodeToString(token)
	stampData, _ := json.Marshal([]any{j.ID, j.FencingToken, j.LeaseOwner, st.Run.ID, st.Actor.OrgID, st.Actor.UserID, st.Actor.AuthVersion, st.Actor.OrgAuthVersion, st.Conversation.CurrentRevisionID, st.Conversation.ContextSecurityVersion, st.Profile.ID, st.Profile.SkillSHA256, core.Digest(token)})
	stamp := core.Digest(stampData)
	_, err = tx.Exec(ctx, `INSERT INTO agent_execution_leases(run_id,org_id,job_id,fencing_token,lease_owner,credential_sha256,authority_stamp,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, runID, j.OrgID, j.ID, j.FencingToken, j.LeaseOwner, core.Digest([]byte(credential)), stamp, st.Run.ExpiresAt)
	if err != nil {
		return out, platform.DatabaseError(err)
	}
	// Only explicitly selected sources are prepared here; old native or conversation context is never resumed.
	preparedValue := map[string]any{"run_id": st.Run.ID, "conversation_id": st.Conversation.ID, "project_id": st.Conversation.ProjectID, "source_refs": st.Sources, "untrusted_data": true}
	if p, e := executionPostWriteState(ctx, tx, st); e != nil {
		return Execution{}, e
	} else if p != nil {
		preparedValue["authorized_post_write"] = p.PostWriteView
	}
	if r, e := executionOARequestState(ctx, tx, st); e != nil {
		return Execution{}, e
	} else if r != nil {
		preparedValue["authorized_oa_request"] = r.OARequestView
	}
	if m, e := executionMutationState(ctx, tx, st); e != nil {
		return Execution{}, e
	} else if m != nil {
		preparedValue["authorized_mutation"] = m.MutationView
	}
	if st.Actor.Channel == "telegram" {
		preparedValue["channel"] = "telegram"
		preparedValue["delivery_mode"] = "portal_only"
	}
	if st.Profile.SkillID == "incident-summary" {
		var serverTime time.Time
		if err = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&serverTime); err != nil {
			return Execution{}, err
		}
		preparedValue["server_time"] = serverTime
	}
	prepared, _ := json.Marshal(preparedValue)
	if i, e := s.executionFileIntent(ctx, tx, st); e != nil {
		return Execution{}, e
	} else if i != nil {
		if i.Mode == "create" {
			preparedValue["authorized_file_create"] = fileCreateView(i.FileUpdateView)
		} else {
			preparedValue["authorized_file_update"] = i.FileUpdateView
		}
		prepared, _ = json.Marshal(preparedValue)
	}
	input := map[string]any{"request": st.Message}
	if st.Conversation.ProjectID != nil {
		input["context_project_id"] = *st.Conversation.ProjectID
	}
	out = Execution{RunID: runID, Channel: st.Actor.Channel, AuthorityStamp: stamp, SkillSHA256: st.Profile.SkillSHA256, ExpiresAt: st.Run.ExpiresAt, Credential: credential, Input: input, PreparedContext: string(prepared), MaxToolCalls: st.Profile.MaxToolCalls, MaxInputTokens: st.Profile.MaxInputTokens, MaxOutputTokens: st.Profile.MaxOutputTokens, Profile: st.Profile}
	if err = executionAudit(ctx, tx, st, "agent.execution.issued", map[string]any{"profile_id": st.Profile.ID}); err != nil {
		return Execution{}, err
	}
	return out, tx.Commit(ctx)
}

func (s *Service) LeaseForCredential(ctx context.Context, credential string) (ExecutionLease, error) {
	var out ExecutionLease
	if len(credential) != 96 {
		return out, runtimeFailure("RUNTIME_CREDENTIAL_INVALID")
	}
	if _, err := hex.DecodeString(credential); err != nil {
		return out, runtimeFailure("RUNTIME_CREDENTIAL_INVALID")
	}
	err := s.DB.QueryRow(ctx, `SELECT l.run_id,l.org_id,l.authority_stamp,l.job_id,l.fencing_token,l.lease_owner,l.expires_at
 FROM agent_execution_leases l WHERE l.credential_sha256=$1 AND l.expires_at>clock_timestamp()`, core.Digest([]byte(credential))).Scan(&out.RunID, &out.OrgID, &out.AuthorityStamp, &out.Job.ID, &out.Job.FencingToken, &out.Job.LeaseOwner, &out.ExpiresAt)
	if err != nil {
		return out, runtimeFailure("RUNTIME_CREDENTIAL_INVALID")
	}
	out.Job.OrgID = out.OrgID
	out.credentialSHA256 = core.Digest([]byte(credential))
	return out, nil
}
func (s *Service) CheckExecution(ctx context.Context, lease ExecutionLease) error {
	tx, _, err := s.executionTx(ctx, lease, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	return tx.Commit(ctx)
}
func (s *Service) StartExecution(ctx context.Context, lease ExecutionLease) error {
	tx, st, err := s.executionTx(ctx, lease, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var confined bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_execution_verifications WHERE org_id=$1 AND run_id=$2 AND authority_stamp=$3 AND configuration_sha256=$4 AND expires_at>clock_timestamp())`, lease.OrgID, lease.RunID, lease.AuthorityStamp, st.Profile.ConfigurationSHA256).Scan(&confined); err != nil {
		return err
	}
	if !confined {
		return runtimeFailure("RUNTIME_CONFINEMENT_UNVERIFIED")
	}
	if st.Run.State == "running" {
		return tx.Commit(ctx)
	}
	if st.Run.State != "queued" {
		return runtimeFailure("RUNTIME_ALREADY_STARTED")
	}
	if err = executionTransition(ctx, tx, st, "running", "run.started", nil, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) RenewExecution(ctx context.Context, lease ExecutionLease) error {
	tx, _, err := s.executionTx(ctx, lease, true)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE jobs SET lease_until=least(clock_timestamp()+interval '90 seconds',$5::timestamptz),updated_at=clock_timestamp()
 WHERE org_id=$1 AND id=$2 AND lease_owner=$3 AND fencing_token=$4`, lease.OrgID, lease.Job.ID, lease.Job.LeaseOwner, lease.Job.FencingToken, lease.ExpiresAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func executionTransition(ctx context.Context, tx pgx.Tx, st executionState, state, kind string, code, outcome *string) error {
	_, err := tx.Exec(ctx, `UPDATE agent_runs SET state=$3,version=version+1,last_event_seq=last_event_seq+1,safe_error_code=$4,outcome=$5,updated_at=clock_timestamp() WHERE org_id=$1 AND id=$2`, st.Actor.OrgID, st.Run.ID, state, code, outcome)
	if err != nil {
		return err
	}
	if err = appendRunEvent(ctx, tx, st.Actor.OrgID, st.Run.ID, st.Run.LastEventSeq+1, kind, state, code); err != nil {
		return err
	}
	st.Run.Version++
	st.Run.LastEventSeq++
	return executionAudit(ctx, tx, st, kind, map[string]any{"state": state, "safe_code": code})
}
func executionAudit(ctx context.Context, tx pgx.Tx, st executionState, action string, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	requestID := core.NewID()
	_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,run_id,action,resource_type,resource_id,resource_version,after_digest,result)
 VALUES($1,$2,$3,$3,$4,$5,$6,$7,'agent_run',$6,$8,$9,'succeeded')`, core.NewID(), st.Actor.OrgID, st.Actor.UserID, st.Actor.Channel, requestID, st.Run.ID, action, st.Run.Version, core.Digest(raw))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
 VALUES($1,$2,$3,'agent_run',$4,$5,$6,$7)`, core.NewID(), st.Actor.OrgID, action, st.Run.ID, st.Run.Version, raw, requestID)
	return err
}

// Called by the trusted controller only after its executor has exited. Runtime-originated interruption reports cannot prove process teardown.
func (s *Service) FinishStoppedExecution(ctx context.Context, j jobqueue.Job, code string) error {
	runID, err := s.executionMapping(ctx, j)
	if err != nil {
		return err
	}
	lease := ExecutionLease{RunID: runID, OrgID: j.OrgID, Job: j}
	tx, st, err := s.executionTx(ctx, lease, false)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if !slices.Contains(runtimeCodes, code) {
		code = "RUNTIME_UNAVAILABLE"
	}
	state, kind := executionStopState(st)
	if st.Run.State == "committing" {
		code = "COMMIT_RECONCILIATION_REQUIRED"
	}
	if state == "cancelled" {
		code = "RUNTIME_CANCEL_REQUESTED"
	}
	if err = executionTransition(ctx, tx, st, state, kind, &code, nil); err != nil {
		return err
	}
	if state == "cancelled" {
		_, err = tx.Exec(ctx, "UPDATE jobs SET state='cancelled',lease_until=NULL,lease_owner=NULL,updated_at=clock_timestamp() WHERE id=$1", j.ID)
	} else {
		err = s.Queue().FailInTx(ctx, tx, j, code, true, time.Second)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var runtimeCodes = []string{"RUNTIME_UNAVAILABLE", "RUNTIME_AUTHORITY_CHANGED", "RUNTIME_SOURCE_CHANGED", "RUNTIME_PROFILE_RETIRED", "RUNTIME_DEADLINE_EXCEEDED", "RUNTIME_CANCEL_REQUESTED", "RUNTIME_LEASE_LOST", "RUNTIME_EXECUTOR_EXITED", "RUNTIME_OUTPUT_INVALID", "RUNTIME_USAGE_BUDGET_EXCEEDED", "RUNTIME_TOOL_BUDGET_EXCEEDED", "RUNTIME_CONFINEMENT_UNVERIFIED", "RUNTIME_PROVIDER_FAILED", "RUNTIME_NATIVE_TOOL_FORBIDDEN", "RUNTIME_STREAM_INCOMPLETE"}
