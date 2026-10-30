package files

import (
	"context"
	"encoding/json"
	"time"

	"copypaste.hk/platform/internal/authz"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/identity"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
)

const batchColumns = `id,version,state,creator_id,input,snapshot,plan_hash,created_at,expires_at,completed_at,decision_reason,results,actor_auth_version,organization_version`

func scanBatchPlan(row pgx.Row) (FileBatchPlan, error) {
	p := FileBatchPlan{Results: []FileBatchFile{}}
	var input, snapshot, results []byte
	e := row.Scan(&p.ID, &p.Version, &p.State, &p.CreatorID, &input, &snapshot, &p.PlanHash, &p.CreatedAt, &p.ExpiresAt, &p.CompletedAt, &p.DecisionReason, &results, &p.ActorAuthVersion, &p.OrganizationVersion)
	if e != nil {
		return p, platform.DatabaseError(e)
	}
	if e = json.Unmarshal(input, &p.Input); e != nil {
		return p, e
	}
	if e = json.Unmarshal(snapshot, &p.Snapshot); e != nil {
		return p, e
	}
	if results != nil {
		e = json.Unmarshal(results, &p.Results)
	}
	return p, e
}

func batchPlan(ctx context.Context, db platform.DBTX, a core.Actor, id string, lock bool) (FileBatchPlan, error) {
	if !organizationID(id) {
		return FileBatchPlan{}, core.Missing()
	}
	q := "SELECT " + batchColumns + " FROM file_batch_plans WHERE org_id=$1 AND id=$2 AND creator_id=$3"
	if lock {
		q += " FOR UPDATE"
	}
	return scanBatchPlan(db.QueryRow(ctx, q, a.OrgID, id, a.UserID))
}

func batchReadAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, p FileBatchPlan) error {
	if e := identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return e
	}
	paths := [][]OrganizationEntry{}
	for _, item := range p.Snapshot.Items {
		f := item.File
		if e := authz.Require(ctx, tx, a, "file.read", authz.Resource{Type: "file", ID: f.ID, ProjectID: f.ProjectID, OwnerID: f.CreatorID, Classification: f.Classification}); e != nil {
			return e
		}
		if _, e := authorizeFile(ctx, tx, a, f.ID, "file.read"); e != nil {
			return e
		}
		paths = append(paths, item.Path)
	}
	if p.Snapshot.Target != nil {
		paths = append(paths, append([]OrganizationEntry{p.Snapshot.Target.Folder}, p.Snapshot.Target.Path...))
	}
	seen := map[string]bool{}
	for _, path := range paths {
		for _, entry := range path {
			if seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			current, e := authorizeFolder(ctx, tx, a, entry.ID, "folder.read")
			if e != nil {
				return e
			}
			if e = authz.Require(ctx, tx, a, "folder.read", authz.Resource{Type: "folder", ID: entry.ID, ProjectID: entry.ProjectID, OwnerID: current.CreatorID, Classification: entry.Classification}); e != nil {
				return e
			}
		}
	}
	return nil
}

func (s *Service) batchReadTx(ctx context.Context, a core.Actor) (pgx.Tx, error) {
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return nil, e
	}
	var orgVersion, userVersion int64
	var status string
	e = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&orgVersion)
	if e == nil {
		e = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", a.OrgID, a.UserID).Scan(&userVersion, &status)
	}
	if e == nil && (orgVersion != a.OrgAuthVersion || userVersion != a.AuthVersion || status != "active") {
		e = core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新读取")
	}
	if e == nil {
		e = identity.RequireInteractiveSession(ctx, tx, a, false)
	}
	if e != nil {
		tx.Rollback(context.WithoutCancel(ctx))
		return nil, e
	}
	return tx, nil
}

func (s *Service) FileBatchPlan(ctx context.Context, a core.Actor, id string) (FileBatchPlan, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, e := s.batchReadTx(ctx, a)
	if e != nil {
		return FileBatchPlan{}, e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	p, e := batchPlan(ctx, tx, a, id, false)
	if e != nil {
		return p, e
	}
	if e = batchReadAuthority(ctx, tx, a, p); e != nil {
		return FileBatchPlan{}, e
	}
	if p.State == "pending" && p.ActorAuthVersion == a.AuthVersion && p.OrganizationVersion == a.OrgAuthVersion {
		current, inspect := batchSnapshot(ctx, tx, a, p.Input)
		if inspect == nil {
			_, _, hash := batchBytes(p.Input, current)
			p.CurrentBasisMatches = hash == p.PlanHash
			p.CanExecute = p.CurrentBasisMatches && !batchBlocked(current) && time.Now().Before(p.ExpiresAt)
		}
		if inspect != nil {
			if _, check := tx.Exec(ctx, "SELECT 1"); check != nil {
				return FileBatchPlan{}, inspect
			}
		}
	}
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return FileBatchPlan{}, e
	}
	return p, platform.DatabaseError(tx.Commit(ctx))
}

// A file is a durable discovery point for every owned batch containing it.
// Historical labels require both their pinned scope and all current sources.
func (s *Service) FileBatchPlans(ctx context.Context, a core.Actor, fileID string, f Filter) (core.Page[FileBatchPlan], error) {
	out := core.Page[FileBatchPlan]{Items: []FileBatchPlan{}}
	if !organizationID(fileID) { return out, core.Missing() }
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, e := s.batchReadTx(ctx, a)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, e = authorizeFile(ctx, tx, a, fileID, "file.read"); e != nil {
		return out, e
	}
	at, before, limit, binding, e := cleanupPage(s, a, "file-batch-plans/"+fileID, f)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, "SELECT "+batchColumns+` FROM file_batch_plans WHERE org_id=$1 AND creator_id=$2
 AND EXISTS(SELECT 1 FROM jsonb_array_elements(snapshot::jsonb->'items') i WHERE i->'file'->>'id'=$3)
 AND ($4::timestamptz IS NULL OR (created_at,id)<($4,$5::uuid)) ORDER BY created_at DESC,id DESC LIMIT $6`, a.OrgID, a.UserID, fileID, at, before, limit+1)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		p, read := scanBatchPlan(rows)
		if read != nil {
			e = read
			break
		}
		out.Items = append(out.Items, p)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, p := range out.Items {
		if e = batchReadAuthority(ctx, tx, a, p); e != nil {
			return core.Page[FileBatchPlan]{}, e
		}
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		cleanupNext(s, &out, limit, binding, last.ID, last.CreatedAt)
	}
	return out, platform.DatabaseError(tx.Commit(ctx))
}

func (s *Service) batchReplay(ctx context.Context, a core.Actor, out core.Result, err error) (core.Result, error) {
	if err != nil || !out.Replayed {
		return out, err
	}
	raw, e := json.Marshal(out.Data)
	if e != nil {
		return core.Result{}, e
	}
	var p FileBatchPlan
	if e = json.Unmarshal(raw, &p); e != nil {
		return core.Result{}, e
	}
	p, e = s.FileBatchPlan(ctx, a, p.ID)
	if e != nil {
		return core.Result{}, e
	}
	out.Data = p
	return out, nil
}

func (s *Service) CreateFileBatch(ctx context.Context, c core.Command, in FileBatchCreate) (core.Result, error) {
	in, e := batchInput(in)
	if e != nil {
		return core.Result{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c.ExclusiveOrganizationLock = true
	out, e := s.Commands.Do(ctx, c, func(tx pgx.Tx) error { return batchAuthority(ctx, tx, c.Actor, in, false) }, func(tx pgx.Tx) (core.Result, error) {
		snapshot, e := batchSnapshot(ctx, tx, c.Actor, in)
		if e != nil {
			return core.Result{}, e
		}
		input, raw, hash := batchBytes(in, snapshot)
		if len(input)+len(raw) > 512*1024 {
			return core.Result{}, core.Invalid("files", "固定清单超过512KiB，请缩小范围")
		}
		var now time.Time
		var boundary *time.Time
		if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
			return core.Result{}, e
		}
		e = tx.QueryRow(ctx, `SELECT min(t) FROM (
 SELECT valid_from t FROM role_bindings WHERE org_id=$1 AND revoked_at IS NULL AND valid_from>clock_timestamp()
 UNION ALL SELECT valid_until FROM role_bindings WHERE org_id=$1 AND revoked_at IS NULL AND valid_until>clock_timestamp()
 UNION ALL SELECT expires_at FROM resource_grants WHERE org_id=$1 AND expires_at>clock_timestamp()) b`, c.Actor.OrgID).Scan(&boundary)
		if e != nil {
			return core.Result{}, e
		}
		expires := now.Add(30 * time.Minute)
		if boundary != nil && boundary.Before(expires) {
			expires = *boundary
		}
		if !expires.After(now.Add(time.Second)) {
			return core.Result{}, core.Fail(409, "BATCH_BASIS_CHANGED", "授权边界已变化，请重新拟定清单")
		}
		id := core.NewID()
		_, e = tx.Exec(ctx, `INSERT INTO file_batch_plans(id,org_id,creator_id,origin_session_id,origin_channel,actor_auth_version,organization_version,input,snapshot,plan_hash,created_at,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, c.Actor.OrgID, c.Actor.UserID, c.Actor.SessionID, c.Actor.Channel, c.Actor.AuthVersion, c.Actor.OrgAuthVersion, string(input), string(raw), hash, now, expires)
		if e != nil {
			return core.Result{}, e
		}
		p, e := batchPlan(ctx, tx, c.Actor, id, false)
		if e != nil {
			return core.Result{}, e
		}
		p.CurrentBasisMatches = true
		p.CanExecute = !batchBlocked(snapshot)
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "file.batch.proposed", ResourceType: "file_batch_plan", ResourceID: id, Version: 1, Payload: map[string]any{"reason": in.Reason, "plan_hash": hash, "action": in.Action, "count": len(in.Files)}}}, nil
	})
	return s.batchReplay(ctx, c.Actor, out, e)
}

func (s *Service) DecideFileBatch(ctx context.Context, c core.Command, id string, in FileBatchDecision, execute bool) (core.Result, error) {
	if !organizationID(id) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || !scopeHashValid(in.PlanHash) || core.Name(in.Reason) != nil || !in.Acknowledged {
		return core.Result{}, core.Invalid("decision", "请核对固定清单的版本、摘要、原因并明确确认")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c.ExclusiveOrganizationLock = true
	out, e := s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if e := identity.RequireInteractiveSession(ctx, tx, c.Actor, execute); e != nil {
			return e
		}
		p, e := batchPlan(ctx, tx, c.Actor, id, false)
		if e != nil {
			return e
		}
		if e = batchReadAuthority(ctx, tx, c.Actor, p); e != nil {
			return e
		}
		if execute {
			return batchAuthority(ctx, tx, c.Actor, p.Input, true)
		}
		return nil
	}, func(tx pgx.Tx) (core.Result, error) {
		p, e := batchPlan(ctx, tx, c.Actor, id, true)
		if e != nil {
			return core.Result{}, e
		}
		if p.State != "pending" || p.Version != in.ExpectedVersion || p.PlanHash != in.PlanHash {
			return core.Result{}, core.Fail(409, "BATCH_PLAN_CHANGED", "计划已处理或依据不同，请重新读取")
		}
		state, event := "cancelled", "file.batch.cancelled"
		var results *string
		if execute {
			var valid bool
			if e = tx.QueryRow(ctx, "SELECT $1::timestamptz>clock_timestamp()", p.ExpiresAt).Scan(&valid); e != nil {
				return core.Result{}, e
			}
			if !valid || p.ActorAuthVersion != c.Actor.AuthVersion || p.OrganizationVersion != c.Actor.OrgAuthVersion {
				return core.Result{}, core.Fail(409, "BATCH_BASIS_CHANGED", "计划已过期或权限配置已变化，请重新拟定")
			}
			current, e := batchSnapshot(ctx, tx, c.Actor, p.Input)
			if e != nil {
				return core.Result{}, e
			}
			_, _, hash := batchBytes(p.Input, current)
			if hash != p.PlanHash || batchBlocked(current) {
				return core.Result{}, core.Fail(409, "BATCH_BASIS_CHANGED", "文件、目录、保留约束或名称冲突已变化，请重新拟定")
			}
			child := *s
			child.Commands.Tx = tx
			completed := []FileBatchFile{}
			for _, item := range current.Items {
				f := item.File
				next := c
				next.Route = c.Route + "/composed/" + f.ID
				next.Key = "file-batch:" + core.Digest([]byte(c.Route+"\n"+c.Key+"\n"+f.ID))
				var r core.Result
				switch p.Input.Action {
				case "move":
					body := Move{ExpectedVersion: f.Version, TargetFolderID: &p.Input.Target.ID, Reason: in.Reason}
					next.Body, _ = json.Marshal(body)
					r, e = child.MoveFile(ctx, next, f.ID, body)
				case "trash":
					body := Transition{ExpectedVersion: f.Version, Action: "trash", Reason: in.Reason}
					next.Body, _ = json.Marshal(body)
					r, e = child.Trash(ctx, next, f.ID, body)
				case "restore":
					body := Restore{ExpectedVersion: f.Version, Mode: "untrash", Reason: in.Reason}
					next.Body, _ = json.Marshal(body)
					r, e = child.Restore(ctx, next, f.ID, body)
				}
				if e != nil {
					return core.Result{}, e
				}
				completed = append(completed, batchFile(r.Data.(File)))
			}
			raw, _ := json.Marshal(completed)
			encoded := string(raw)
			results = &encoded
			state, event = "executed", "file.batch.executed"
		}
		if e = identity.RequireInteractiveSession(ctx, tx, c.Actor, execute); e != nil {
			return core.Result{}, e
		}
		_, e = tx.Exec(ctx, `UPDATE file_batch_plans SET state=$4,version=version+1,completed_at=clock_timestamp(),decision_reason=$5,decision_session_id=$6,decision_channel=$7,results=$8
 WHERE org_id=$1 AND id=$2 AND creator_id=$3`, c.Actor.OrgID, id, c.Actor.UserID, state, in.Reason, c.Actor.SessionID, c.Actor.Channel, results)
		if e != nil {
			return core.Result{}, e
		}
		p, e = batchPlan(ctx, tx, c.Actor, id, false)
		if e != nil {
			return core.Result{}, e
		}
		return core.Result{Data: p, Event: core.Event{Type: event, ResourceType: "file_batch_plan", ResourceID: id, Version: p.Version, Payload: map[string]any{"reason": in.Reason, "plan_hash": p.PlanHash, "action": p.Input.Action, "count": len(p.Input.Files), "acknowledged": true}}}, nil
	})
	return s.batchReplay(ctx, c.Actor, out, e)
}
