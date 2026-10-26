package platform

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Tx lets a trusted business coordinator compose commands atomically. The caller
// owns commit/rollback and must roll back the entire transaction on any error.
// Authorization, idempotency, audit and outbox checks still run for every command.
type Commands struct {
	DB *pgxpool.Pool
	Tx pgx.Tx
}

func (c Commands) Do(ctx context.Context, command core.Command, authorize func(pgx.Tx) error, fn func(pgx.Tx) (core.Result, error)) (core.Result, error) {
	var result core.Result
	if len(command.Key) < 16 || len(command.Key) > 200 {
		return result, core.Invalid("Idempotency-Key", "请提供16至200字符的幂等标识")
	}
	if command.Actor.UserID == "" {
		return result, core.Forbidden()
	}
	canonical, err := core.CanonicalJSON(command.Body)
	if err != nil {
		return result, core.Invalid("body", "请求不是有效JSON")
	}
	hash := core.Digest(append([]byte(command.Route+"\n"), canonical...))
	tx := c.Tx
	owned := tx == nil
	if owned {
		tx, err = c.DB.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return result, err
		}
		defer tx.Rollback(ctx)
	}
	commit := func() error {
		if owned {
			return tx.Commit(ctx)
		}
		return nil
	}
	var orgVersion, userVersion int64
	var status string
	organizationLock := " FOR SHARE"
	if command.AuthorizationMutation {
		organizationLock = " FOR UPDATE"
	}
	err = tx.QueryRow(ctx, "SELECT authorization_version FROM organizations WHERE id=$1"+organizationLock, command.Actor.OrgID).Scan(&orgVersion)
	if err != nil {
		return result, DatabaseError(err)
	}
	err = tx.QueryRow(ctx, "SELECT auth_version,status FROM users WHERE org_id=$1 AND id=$2 FOR SHARE", command.Actor.OrgID, command.Actor.UserID).Scan(&userVersion, &status)
	if err != nil || status != "active" || userVersion != command.Actor.AuthVersion || orgVersion != command.Actor.OrgAuthVersion {
		return result, core.Fail(412, "AUTHORIZATION_CHANGED", "权限已变更，请重新获取授权")
	}
	if authorize == nil {
		return result, errors.New("every command requires authorization before idempotent replay")
	}
	if err = authorize(tx); err != nil {
		return result, err
	}
	operationID := core.NewID()
	tag, err := tx.Exec(ctx, `INSERT INTO idempotency_records(id,org_id,actor_id,route_key,idempotency_key,request_hash,state,expires_at)
		VALUES($1,$2,$3,$4,$5,$6,'in_progress',now()+interval '7 days') ON CONFLICT(org_id,actor_id,route_key,idempotency_key) DO NOTHING`,
		operationID, command.Actor.OrgID, command.Actor.UserID, command.Route, command.Key, hash)
	if err != nil {
		return result, err
	}
	if tag.RowsAffected() == 0 {
		var storedHash, state string
		var response []byte
		var statusCode int
		err = tx.QueryRow(ctx, `SELECT id,request_hash,state,response_status,response_data FROM idempotency_records
			WHERE org_id=$1 AND actor_id=$2 AND route_key=$3 AND idempotency_key=$4 FOR UPDATE`,
			command.Actor.OrgID, command.Actor.UserID, command.Route, command.Key).Scan(&operationID, &storedHash, &state, &statusCode, &response)
		if err != nil {
			return result, err
		}
		if storedHash != hash {
			return result, core.Fail(409, "IDEMPOTENCY_MISMATCH", "同一幂等标识不能用于不同请求")
		}
		if state != "succeeded" {
			return result, core.Fail(409, "OPERATION_IN_PROGRESS", "操作结果尚在确认，请查询操作状态")
		}
		result = core.Result{Status: statusCode, Data: json.RawMessage(response), OperationID: operationID, Replayed: true}
		return result, commit()
	}
	result, err = fn(tx)
	if err != nil {
		return core.Result{}, DatabaseError(err)
	}
	if result.Status == 0 {
		result.Status = 200
	}
	response, err := json.Marshal(result.Data)
	if err != nil {
		return core.Result{}, err
	}
	for _, event := range append([]core.Event{result.Event}, result.Events...) {
		if event.Type == "" || event.ResourceID == "" || event.ResourceType == "" {
			return core.Result{}, errors.New("business command must provide audit/outbox event")
		}
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			return core.Result{}, err
		}
		eventID := core.NewID()
		reason, _ := event.Payload["reason"].(string)
		_, err = tx.Exec(ctx, `INSERT INTO audit_events(id,org_id,actor_id,executor_id,channel,request_id,run_id,action,resource_type,resource_id,resource_version,after_digest,result,reason,session_id,before_digest)
			VALUES($1,$2,$3,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9,$10,$11,'succeeded',NULLIF($12,''),NULLIF($13,'')::uuid,NULLIF($14,''))`,
			core.NewID(), command.Actor.OrgID, command.Actor.UserID, command.Actor.Channel, command.RequestID, command.Actor.RunID, event.Type, event.ResourceType, event.ResourceID, event.Version, core.Digest(response), reason, command.Actor.SessionID, event.BeforeDigest)
		if err != nil {
			return core.Result{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,org_id,event_type,aggregate_type,aggregate_id,aggregate_version,payload,request_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, eventID, command.Actor.OrgID, event.Type, event.ResourceType, event.ResourceID, event.Version, payload, command.RequestID)
		if err != nil {
			return core.Result{}, err
		}
	}
	_, err = tx.Exec(ctx, "UPDATE idempotency_records SET state='succeeded',response_status=$2,response_data=$3,completed_at=$4 WHERE id=$1", operationID, result.Status, response, time.Now().UTC())
	if err != nil {
		return core.Result{}, err
	}
	if err = commit(); err != nil {
		return core.Result{}, DatabaseError(err)
	}
	result.OperationID = operationID
	return result, nil
}
