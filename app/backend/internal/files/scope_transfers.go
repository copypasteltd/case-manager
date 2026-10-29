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
	"github.com/jackc/pgx/v5/pgconn"
)

const scopeTransferColumns = `id,version,state,creator_id,input,snapshot,plan_hash,created_at,expires_at,completed_at,decision_reason,actor_auth_version,organization_version,origin_session_id,origin_channel`

func scanScopeTransfer(row pgx.Row) (ScopeTransfer, error) {
	var out ScopeTransfer
	var input, snapshot []byte
	e := row.Scan(&out.ID, &out.Version, &out.State, &out.CreatorID, &input, &snapshot, &out.PlanHash, &out.CreatedAt, &out.ExpiresAt, &out.CompletedAt, &out.DecisionReason, &out.ActorAuthVersion, &out.OrganizationVersion, &out.OriginalSessionID, &out.OriginalChannel)
	if e != nil {
		return out, platform.DatabaseError(e)
	}
	if e = json.Unmarshal(input, &out.Input); e != nil {
		return out, e
	}
	e = json.Unmarshal(snapshot, &out.Snapshot)
	return out, e
}

func scopeTransfer(ctx context.Context, db platform.DBTX, a core.Actor, id string, lock bool) (ScopeTransfer, error) {
	if !organizationID(id) {
		return ScopeTransfer{}, core.Missing()
	}
	query := "SELECT " + scopeTransferColumns + " FROM file_scope_transfers WHERE org_id=$1 AND id=$2 AND creator_id=$3"
	if lock {
		query += " FOR UPDATE"
	}
	return scanScopeTransfer(db.QueryRow(ctx, query, a.OrgID, id, a.UserID))
}

func transferReadAuthority(ctx context.Context, tx pgx.Tx, a core.Actor, p ScopeTransfer) error {
	if e := scopeTransferAuthority(ctx, tx, a, false); e != nil {
		return e
	}
	// Historical labels and paths still require the old scope and the current
	// object scope. Being the owner of a plan does not preserve removed access.
	for _, item := range p.Snapshot.Items {
		r := authz.Resource{Type: item.Kind, ID: item.ID, ProjectID: item.ProjectID, OwnerID: item.CreatorID, Classification: item.Classification}
		if e := authz.Require(ctx, tx, a, item.Kind+".read", r); e != nil {
			return e
		}
		if item.Kind == "file" {
			current, e := file(ctx, tx, a.OrgID, item.ID, false)
			if e != nil {
				return e
			}
			if e = authz.Require(ctx, tx, a, "file.read", fileResource(current)); e != nil {
				return e
			}
		} else {
			current, e := folder(ctx, tx, a.OrgID, item.ID, false)
			if e != nil {
				return e
			}
			if e = authz.Require(ctx, tx, a, "folder.read", folderResource(current)); e != nil {
				return e
			}
		}
	}
	for _, path := range [][]OrganizationEntry{p.Snapshot.SourcePath, append([]OrganizationEntry{p.Snapshot.Target}, p.Snapshot.TargetPath...)} {
		for _, entry := range path {
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

func (s *Service) transferReadTx(ctx context.Context, a core.Actor) (pgx.Tx, error) {
	tx, e := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if e != nil {
		return nil, e
	}
	var orgVersion int64
	e = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1 FOR SHARE", a.OrgID).Scan(&orgVersion)
	if e == nil && orgVersion != a.OrgAuthVersion {
		e = core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新读取")
	}
	if e == nil {
		e = scopeTransferAuthority(ctx, tx, a, false)
	}
	if e != nil {
		tx.Rollback(context.WithoutCancel(ctx))
		return nil, e
	}
	return tx, nil
}

func (s *Service) ScopeTransfer(ctx context.Context, a core.Actor, id string) (ScopeTransfer, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, e := s.transferReadTx(ctx, a)
	if e != nil {
		return ScopeTransfer{}, e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	p, e := scopeTransfer(ctx, tx, a, id, false)
	if e != nil {
		return p, e
	}
	if e = transferReadAuthority(ctx, tx, a, p); e != nil {
		return ScopeTransfer{}, e
	}
	if p.State == "pending" && p.ActorAuthVersion == a.AuthVersion && p.OrganizationVersion == a.OrgAuthVersion {
		// Analysis errors invalidate execution. The immutable plan is still a
		// readable historical proposal; never replace its selected inputs.
		current, inspect := scopeSnapshot(ctx, tx, a, p.Input)
		if inspect == nil {
			p.CurrentBasisMatches = scopePlanHash(p.Input, current) == p.PlanHash
		}
		if inspect != nil {
			if _, e = tx.Exec(ctx, "SELECT 1"); e != nil {
				return ScopeTransfer{}, inspect
			}
		}
		p.CanExecute = p.CurrentBasisMatches && !transferBlocked(p.Snapshot) && time.Now().Before(p.ExpiresAt)
	}
	if e = identity.RequireInteractiveSession(ctx, tx, a, false); e != nil {
		return ScopeTransfer{}, e
	}
	e = tx.Commit(ctx)
	return p, e
}

func (s *Service) ScopeTransfers(ctx context.Context, a core.Actor, kind, id string, filter Filter) (core.Page[ScopeTransfer], error) {
	out := core.Page[ScopeTransfer]{Items: []ScopeTransfer{}}
	if (kind != "file" && kind != "folder") || !organizationID(id) {
		return out, core.Missing()
	}
	tx, e := s.transferReadTx(ctx, a)
	if e != nil {
		return out, e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, e = OrganizationMetadata(ctx, tx, a, kind, id, kind+".read"); e != nil {
		return out, e
	}
	at, before, limit, binding, e := cleanupPage(s, a, "file-scope-transfers/"+kind+"/"+id, filter)
	if e != nil {
		return out, e
	}
	rows, e := tx.Query(ctx, "SELECT "+scopeTransferColumns+` FROM file_scope_transfers WHERE org_id=$1 AND creator_id=$2 AND resource_kind=$3 AND resource_id=$4
 AND ($5::timestamptz IS NULL OR (created_at,id)<($5,$6::uuid)) ORDER BY created_at DESC,id DESC LIMIT $7`, a.OrgID, a.UserID, kind, id, at, before, limit+1)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		p, read := scanScopeTransfer(rows)
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
		if e = transferReadAuthority(ctx, tx, a, p); e != nil {
			return core.Page[ScopeTransfer]{}, e
		}
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		cleanupNext(s, &out, limit, binding, last.ID, last.CreatedAt)
	}
	return out, tx.Commit(ctx)
}

func (s *Service) CreateScopeTransfer(ctx context.Context, c core.Command, in ScopeTransferCreate) (core.Result, error) {
	if e := scopeTransferInput(in); e != nil {
		return core.Result{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if e := scopeTransferAuthority(ctx, tx, c.Actor, false); e != nil {
			return e
		}
		permission := "file.move"
		if in.Kind == "folder" {
			permission = "folder.update"
		}
		if _, e := OrganizationMetadata(ctx, tx, c.Actor, in.Kind, in.ResourceID, permission); e != nil {
			return e
		}
		_, e := OrganizationMetadata(ctx, tx, c.Actor, "folder", in.TargetFolderID, "folder.create")
		return e
	}, func(tx pgx.Tx) (core.Result, error) {
		var boundary *time.Time
		e := tx.QueryRow(ctx, `SELECT min(t) FROM (
 SELECT valid_from t FROM role_bindings WHERE org_id=$1 AND revoked_at IS NULL AND valid_from>transaction_timestamp()
 UNION ALL SELECT valid_until FROM role_bindings WHERE org_id=$1 AND revoked_at IS NULL AND valid_until>transaction_timestamp()
 UNION ALL SELECT expires_at FROM resource_grants WHERE org_id=$1 AND expires_at>transaction_timestamp()
 ) b`, c.Actor.OrgID).Scan(&boundary)
		if e != nil {
			return core.Result{}, e
		}
		snapshot, e := scopeSnapshot(ctx, tx, c.Actor, in)
		if e != nil {
			return core.Result{}, e
		}
		input, _ := json.Marshal(in)
		raw, _ := json.Marshal(snapshot)
		if len(raw) > 2*1024*1024 {
			return core.Result{}, core.Fail(422, "TRANSFER_SCOPE_TOO_LARGE", "权限变化清单超过单份计划上限，请缩小实际整理范围")
		}
		var now time.Time
		if e = tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); e != nil {
			return core.Result{}, e
		}
		expires := now.Add(30 * time.Minute)
		if boundary != nil && boundary.Before(expires) {
			expires = *boundary
		}
		if !expires.After(now.Add(time.Second)) {
			return core.Result{}, core.Fail(409, "TRANSFER_BASIS_CHANGED", "分析期间授权已到达变更边界，请重新拟定")
		}
		id, hash := core.NewID(), scopePlanHash(in, snapshot)
		_, e = tx.Exec(ctx, `INSERT INTO file_scope_transfers(id,org_id,creator_id,origin_session_id,origin_channel,actor_auth_version,organization_version,resource_kind,resource_id,input,snapshot,plan_hash,created_at,expires_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, c.Actor.OrgID, c.Actor.UserID, c.Actor.SessionID, c.Actor.Channel, c.Actor.AuthVersion, c.Actor.OrgAuthVersion, in.Kind, in.ResourceID, input, raw, hash, now, expires)
		if e != nil {
			return core.Result{}, e
		}
		p, e := scopeTransfer(ctx, tx, c.Actor, id, false)
		if e != nil {
			return core.Result{}, e
		}
		p.CurrentBasisMatches, p.CanExecute = true, !transferBlocked(snapshot)
		if e = identity.RequireInteractiveSession(ctx, tx, c.Actor, false); e != nil {
			return core.Result{}, e
		}
		return core.Result{Status: 201, Data: p, Event: core.Event{Type: "file.scope_transfer.proposed", ResourceType: "file_scope_transfer", ResourceID: id, Version: 1, Payload: map[string]any{"reason": in.Reason, "plan_hash": hash, "resource_kind": in.Kind, "resource_id": in.ResourceID}}}, nil
	})
}

func (s *Service) DecideScopeTransfer(ctx context.Context, c core.Command, id string, in ScopeTransferDecision, execute bool) (core.Result, error) {
	if !organizationID(id) {
		return core.Result{}, core.Missing()
	}
	if core.RequireVersion(in.ExpectedVersion) != nil || !scopeHashValid(in.PlanHash) || core.Name(in.Reason) != nil {
		return core.Result{}, core.Invalid("decision", "请核对计划准确版本、摘要及本次原因")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c.ExclusiveOrganizationLock = true
	return s.Commands.Do(ctx, c, func(tx pgx.Tx) error {
		if e := scopeTransferAuthority(ctx, tx, c.Actor, execute); e != nil {
			return e
		}
		p, e := scopeTransfer(ctx, tx, c.Actor, id, false)
		if e != nil {
			return e
		}
		return transferReadAuthority(ctx, tx, c.Actor, p)
	}, func(tx pgx.Tx) (core.Result, error) {
		p, e := scopeTransfer(ctx, tx, c.Actor, id, true)
		if e != nil {
			return core.Result{}, e
		}
		if p.Version != in.ExpectedVersion || p.PlanHash != in.PlanHash {
			return core.Result{}, core.Conflict()
		}
		if p.State != "pending" {
			return core.Result{}, core.UnexpectedState(p.State, "pending")
		}
		state, event := "cancelled", "file.scope_transfer.cancelled"
		if execute {
			if p.ActorAuthVersion != c.Actor.AuthVersion || p.OrganizationVersion != c.Actor.OrgAuthVersion {
				return core.Result{}, core.Fail(412, "AUTHORIZATION_CHANGED", "拟定后权限已改变，请重新拟定计划")
			}
			// A plan can be reviewed on another current human client. Execution
			// still needs a live origin session and the executing client's MFA.
			origin := c.Actor
			origin.SessionID, origin.Channel = p.OriginalSessionID, p.OriginalChannel
			if e = identity.RequireInteractiveSession(ctx, tx, origin, false); e != nil {
				return core.Result{}, e
			}
			var fresh bool
			if e = tx.QueryRow(ctx, "SELECT clock_timestamp()<$1", p.ExpiresAt).Scan(&fresh); e != nil {
				return core.Result{}, e
			}
			if !fresh {
				return core.Result{}, core.Fail(409, "TRANSFER_EXPIRED", "计划已到期，请重新拟定并核对权限变化")
			}
			current, inspect := scopeSnapshot(ctx, tx, c.Actor, p.Input)
			if inspect != nil {
				return core.Result{}, inspect
			}
			if scopePlanHash(p.Input, current) != p.PlanHash {
				return core.Result{}, core.Fail(409, "TRANSFER_BASIS_CHANGED", "资料、目录或权限分析已变化，请重新拟定")
			}
			if transferBlocked(current) {
				return core.Result{}, core.Fail(409, "TRANSFER_BLOCKED", "计划包含未解决的业务引用、保留、在途任务或分类约束")
			}
			if e = executeScopeTransfer(ctx, tx, c.Actor, p); e != nil {
				return core.Result{}, e
			}
			if e = identity.RequireInteractiveSession(ctx, tx, c.Actor, true); e != nil {
				return core.Result{}, e
			}
			if e = tx.QueryRow(ctx, "SELECT clock_timestamp()<$1", p.ExpiresAt).Scan(&fresh); e != nil {
				return core.Result{}, e
			}
			if !fresh {
				return core.Result{}, core.Fail(409, "TRANSFER_EXPIRED", "执行期间计划已到达有效边界，未提交变更")
			}
			// Invalidate old assistant contexts, permits and client cursors which
			// relied on the previous project/classification visibility.
			if _, e = tx.Exec(ctx, "UPDATE organizations SET authorization_version=authorization_version+1,updated_at=clock_timestamp() WHERE id=$1", c.Actor.OrgID); e != nil {
				return core.Result{}, e
			}
			state, event = "executed", "file.scope_transfer.executed"
		}
		_, e = tx.Exec(ctx, "UPDATE file_scope_transfers SET state=$3,version=version+1,completed_at=clock_timestamp(),decision_reason=$4,decision_session_id=$5,decision_channel=$6 WHERE org_id=$1 AND id=$2", c.Actor.OrgID, id, state, in.Reason, c.Actor.SessionID, c.Actor.Channel)
		if e != nil {
			return core.Result{}, e
		}
		p, e = scopeTransfer(ctx, tx, c.Actor, id, false)
		return core.Result{Data: p, Event: core.Event{Type: event, ResourceType: "file_scope_transfer", ResourceID: id, Version: p.Version, Payload: map[string]any{"reason": in.Reason, "plan_hash": p.PlanHash, "resource_kind": p.Input.Kind, "resource_id": p.Input.ResourceID, "item_count": len(p.Snapshot.Items)}}}, e
	})
}

func executeScopeTransfer(ctx context.Context, tx pgx.Tx, a core.Actor, p ScopeTransfer) error {
	nameKey, e := core.StorageNameKey(p.Snapshot.NewName)
	if e != nil {
		return e
	}
	for _, item := range p.Snapshot.Items {
		var tag pgconn.CommandTag
		if item.Kind == "folder" {
			tag, e = tx.Exec(ctx, `UPDATE folders SET project_id=$3,root_scope=$4,classification=$5,
 parent_id=CASE WHEN id=$6 THEN $7::uuid ELSE parent_id END,name=CASE WHEN id=$6 THEN $8 ELSE name END,
 name_key=CASE WHEN id=$6 THEN $9 ELSE name_key END,version=version+1,updated_at=clock_timestamp()
 WHERE org_id=$1 AND id=$2 AND version=$10`, a.OrgID, item.ID, p.Snapshot.Target.ProjectID, scopeRoot(p.Snapshot.Target.ProjectID), item.NewClassification, p.Input.ResourceID, p.Snapshot.Target.ID, p.Snapshot.NewName, nameKey, item.Version)
		} else {
			tag, e = tx.Exec(ctx, `UPDATE files SET classification=$3,
 folder_id=CASE WHEN id=$4 THEN $5::uuid ELSE folder_id END,name=CASE WHEN id=$4 THEN $6 ELSE name END,
 name_key=CASE WHEN id=$4 THEN $7 ELSE name_key END,version=version+1,updated_at=clock_timestamp()
 WHERE org_id=$1 AND id=$2 AND version=$8`, a.OrgID, item.ID, item.NewClassification, p.Input.ResourceID, p.Snapshot.Target.ID, p.Snapshot.NewName, nameKey, item.Version)
		}
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return core.Conflict()
		}
	}
	return nil
}

func scopeRoot(project *string) string {
	if project == nil {
		return "company"
	}
	return "project:" + *project
}
