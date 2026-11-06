package files

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/objectstore"
	"github.com/jackc/pgx/v5"
)

const TerminalObservationJobKind = "file.terminal.observation.v1"
const TerminalObservationJobLane = "terminal-observation"

func (s *ObjectRetirementService) enqueueTerminal(ctx context.Context, tx pgx.Tx, org, id string) error {
	raw, _ := json.Marshal(map[string]string{"request_id": id})
	_, _, e := s.queue().Enqueue(ctx, tx, jobqueue.Input{OrgID: org, Kind: TerminalObservationJobKind, Lane: TerminalObservationJobLane, DedupKey: TerminalObservationJobKind + "/" + id, PayloadVersion: 1, Payload: raw, MaxAttempts: 3})
	return e
}

type terminalRequest struct {
	ID, FolderID, UploadID, Basis string
	FolderVersion, UploadVersion  int64
	Actor                         core.Actor
	Snapshot                      terminalSnapshot
	ExpiresAt                     time.Time
}

func terminalRequestRead(ctx context.Context, tx pgx.Tx, org, id string) (terminalRequest, error) {
	r := terminalRequest{ID: id}
	r.Actor.OrgID = org
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT folder_id,upload_id,basis_sha256,folder_version,upload_version,actor_id,session_id,channel,actor_auth_version,organization_auth_version,snapshot,expires_at,mfa_verified_at FROM terminal_upload_observations WHERE org_id=$1 AND id=$2`, org, id).Scan(&r.FolderID, &r.UploadID, &r.Basis, &r.FolderVersion, &r.UploadVersion, &r.Actor.UserID, &r.Actor.SessionID, &r.Actor.Channel, &r.Actor.AuthVersion, &r.Actor.OrgAuthVersion, &raw, &r.ExpiresAt, &r.Actor.MFAVerifiedAt)
	if e == nil {
		e = json.Unmarshal(raw, &r.Snapshot)
	}
	return r, e
}
func (s *ObjectRetirementService) terminalWorkerCheck(ctx context.Context, j jobqueue.Job, id string) (terminalRequest, error) {
	tx, e := s.Files.DB.Begin(ctx)
	if e != nil {
		return terminalRequest{}, e
	}
	defer tx.Rollback(ctx)
	var lock string
	if e = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", j.OrgID).Scan(&lock); e != nil {
		return terminalRequest{}, e
	}
	r, e := terminalRequestRead(ctx, tx, j.OrgID, id)
	if e != nil {
		return r, e
	}
	if time.Now().After(r.ExpiresAt) {
		return r, core.Fail(409, "OBSERVATION_AUTHORIZATION_EXPIRED", "原观察授权已过期")
	}
	v, snap, e := s.terminalCandidate(ctx, tx, r.Actor, r.UploadID, true)
	if e != nil {
		return r, e
	}
	if !v.CanObserve || v.FolderID != r.FolderID || v.UploadVersion != r.UploadVersion || v.FolderVersion != r.FolderVersion || v.BasisSHA256 != r.Basis || digest(snap) != digest(r.Snapshot) {
		return r, core.Fail(409, "OBSERVATION_BASIS_CHANGED", "原观察依据或保护已变化")
	}
	if e = s.queue().CheckInTx(ctx, tx, j); e != nil {
		return r, e
	}
	return r, tx.Commit(ctx)
}
func (s *ObjectRetirementService) terminalFinish(ctx context.Context, j jobqueue.Job, r terminalRequest, values []terminalObserved, cause error) error {
	tx, e := s.Files.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var lock string
	if e = tx.QueryRow(ctx, "SELECT id FROM organizations WHERE id=$1 FOR SHARE", j.OrgID).Scan(&lock); e != nil {
		return e
	}
	if e = s.queue().CheckInTx(ctx, tx, j); e != nil {
		return e
	}
	var exists bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM terminal_upload_observation_results WHERE org_id=$1 AND request_id=$2)", j.OrgID, r.ID).Scan(&exists); e != nil {
		return e
	}
	if exists {
		if e = s.queue().CompleteInTx(ctx, tx, j); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	outcome := "observed"
	all := len(values) > 0 && len(values) == len(r.Snapshot.Coordinates)
	var code *string
	if cause == nil {
		v, snap, err := s.terminalCandidate(ctx, tx, r.Actor, r.UploadID, true)
		if err != nil {
			cause = err
		} else if !v.CanObserve || v.UploadVersion != r.UploadVersion || v.FolderVersion != r.FolderVersion || v.BasisSHA256 != r.Basis || digest(snap) != digest(r.Snapshot) || time.Now().After(r.ExpiresAt) {
			cause = core.Fail(409, "OBSERVATION_BASIS_CHANGED", "原观察依据或保护已变化")
		}
	}
	if cause != nil {
		outcome = "stopped"
		all = false
		values = []terminalObserved{}
		v := core.PublicError(cause).Code
		code = &v
	} else {
		for _, v := range values {
			if v.Outcome != "observed_present" || v.Source == nil {
				all = false
			}
		}
		if !all {
			outcome = "incomplete"
		}
	}
	raw, _ := json.Marshal(values)
	rid := core.NewID()
	if _, e = tx.Exec(ctx, `INSERT INTO terminal_upload_observation_results(id,org_id,request_id,all_present,outcome,error_code,objects) VALUES($1,$2,$3,$4,$5,$6,$7)`, rid, j.OrgID, r.ID, all, outcome, code, raw); e != nil {
		return e
	}
	payload, _ := json.Marshal(map[string]any{"request_id": r.ID, "upload_id": r.UploadID, "basis_sha256": r.Basis, "outcome": outcome, "all_present": all, "object_count": len(values), "job_id": j.ID})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,action,resource_type,resource_id,resource_version,after_digest,result,reason,session_id)
 VALUES($1,$2,$3,$3,'worker',$4,'file.terminal.observation.finished','terminal_upload_observation',$5,1,$6,$7,$8,$9)`, core.NewID(), j.OrgID, r.Actor.UserID, core.NewID(), r.ID, core.Digest(payload), map[bool]string{true: "succeeded", false: "failed"}[all], outcome, r.Actor.SessionID); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id) VALUES($1,$2,'file.terminal.observation.finished','terminal_upload_observation',$3,1,$4,$5)`, core.NewID(), j.OrgID, r.ID, payload, core.NewID()); e != nil {
		return e
	}
	if e = s.queue().CompleteInTx(ctx, tx, j); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *ObjectRetirementService) ExecuteTerminalObservation(ctx context.Context, j jobqueue.Job) error {
	var in struct {
		RequestID string `json:"request_id"`
	}
	if j.Kind != TerminalObservationJobKind || j.Lane != TerminalObservationJobLane || j.PayloadVersion != 1 || json.Unmarshal(j.Payload, &in) != nil || !core.ValidID(in.RequestID) {
		return core.Fail(409, "INVALID_JOB_INPUT", "准确原生观察任务无效")
	}
	r, e := s.terminalWorkerCheck(ctx, j, in.RequestID)
	if e != nil {
		if r.ID != "" && r.Actor.UserID != "" {
			return s.terminalFinish(ctx, j, r, nil, e)
		}
		return e
	}
	ioctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	values := []terminalObserved{}
	for _, c := range r.Snapshot.Coordinates {
		if _, e = s.terminalWorkerCheck(ctx, j, r.ID); e != nil {
			return s.terminalFinish(ctx, j, r, nil, e)
		}
		b := s.Backends[c.Provider]
		v := terminalObserved{Coordinate: c, Outcome: "failed"}
		if b == nil || b.Observer == nil || b.BindingSHA256 != c.BindingSHA256 || b.Bucket != c.Bucket || b.ExclusiveNamespace != c.ExclusiveNamespace {
			return s.terminalFinish(ctx, j, r, nil, core.Fail(409, "OBSERVATION_BACKEND_CHANGED", "准确原生观察用途或位置已变化"))
		}
		src, err := b.Observer.Head(ioctx, c.Key)
		if err != nil {
			var pe *objectstore.Error
			if errors.As(err, &pe) && pe.Code == "OBJECT_NOT_FOUND" {
				v.Outcome = "not_observed"
			}
		} else {
			u, readErr := upload(ioctx, s.Files.DB, j.OrgID, r.UploadID, false)
			if readErr != nil {
				return readErr
			}
			if src.Key == c.Key && src.Size == u.layout().Size && src.Size > 0 && validOpaque(src.ETag) && (c.Provider != "oss" || src.VersionID != "" && src.VersionID != "null") && (c.Provider != "r2" || c.ExclusiveNamespace) {
				v.Outcome = "observed_present"
				v.Source = &src
			} else {
				v.Outcome = "identity_unconfirmed"
			}
		}
		values = append(values, v)
		if ioctx.Err() != nil {
			break
		}
	}
	return s.terminalFinish(ctx, j, r, values, nil)
}
func (s *ObjectRetirementService) RunTerminalOne(ctx context.Context, owner string) (bool, error) {
	q := s.queue()
	j, e := q.Claim(ctx, []string{TerminalObservationJobKind}, TerminalObservationJobLane, owner, 2*time.Minute)
	if e != nil || j == nil {
		return false, e
	}
	e = s.ExecuteTerminalObservation(ctx, *j)
	if e != nil {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if failed := q.Fail(recovery, *j, core.PublicError(e).Code, false, 5*time.Second); failed != nil {
			return true, failed
		}
	}
	return true, e
}
