package jobqueue

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/platform"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Queue struct{ DB *pgxpool.Pool }
type Input struct {
	OrgID, Kind, Lane, DedupKey string
	PayloadVersion              int
	Payload                     json.RawMessage
	MaxAttempts                 int
}
type Job struct {
	ID, OrgID, Kind, Lane, DedupKey string
	PayloadVersion                  int
	Payload                         json.RawMessage
	Attempt, MaxAttempts            int
	FencingToken                    int64
	LeaseOwner                      string
	LeaseUntil                      time.Time
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,79}$`)
var errorCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,79}$`)
var ownerPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

func validateInput(in Input) error {
	if !core.ValidID(in.OrgID) || !identifier.MatchString(in.Kind) || !identifier.MatchString(in.Lane) || in.PayloadVersion < 1 || in.PayloadVersion > 10000 || len(in.DedupKey) < 1 || len(in.DedupKey) > 200 || strings.TrimSpace(in.DedupKey) != in.DedupKey || in.MaxAttempts < 1 || in.MaxAttempts > 16 || len(in.Payload) > 65536 || !json.Valid(in.Payload) {
		return core.Invalid("job", "任务标识、固定输入或重试上限无效")
	}
	return nil
}

// Enqueue must share the caller's transaction with an outbox consumption record
// or the corresponding command. A deduplication key cannot change fixed input.
func (q Queue) Enqueue(ctx context.Context, tx pgx.Tx, in Input) (id string, inserted bool, err error) {
	if err = validateInput(in); err != nil {
		return
	}
	id = core.NewID()
	tag, err := tx.Exec(ctx, `INSERT INTO jobs(id,org_id,kind,lane,dedup_key,payload_version,payload,max_attempts,state)
        VALUES($1,$2,$3,$4,$5,$6,$7,$8,'queued') ON CONFLICT(org_id,dedup_key) DO NOTHING`, id, in.OrgID, in.Kind, in.Lane, in.DedupKey, in.PayloadVersion, in.Payload, in.MaxAttempts)
	if err != nil {
		return "", false, platform.DatabaseError(err)
	}
	if tag.RowsAffected() == 1 {
		return id, true, nil
	}
	var same bool
	err = tx.QueryRow(ctx, `SELECT id,kind=$3 AND lane=$4 AND payload_version=$5 AND payload=$6::jsonb AND max_attempts=$7
        FROM jobs WHERE org_id=$1 AND dedup_key=$2`, in.OrgID, in.DedupKey, in.Kind, in.Lane, in.PayloadVersion, in.Payload, in.MaxAttempts).Scan(&id, &same)
	if err != nil {
		return "", false, err
	}
	if !same {
		return "", false, core.Fail(409, "JOB_DEDUP_MISMATCH", "同一任务去重标识不能更改固定输入")
	}
	return id, false, nil
}
func selection(kinds []string, lane string) error {
	if len(kinds) < 1 || len(kinds) > 64 || !identifier.MatchString(lane) {
		return core.Invalid("job", "任务领取范围无效")
	}
	for _, kind := range kinds {
		if !identifier.MatchString(kind) {
			return core.Invalid("kind", "任务类型无效")
		}
	}
	return nil
}
func leaseSeconds(owner string, lease time.Duration) (int, error) {
	if !ownerPattern.MatchString(owner) || lease < 15*time.Second || lease > 5*time.Minute {
		return 0, core.Invalid("lease", "任务租约需为15至300秒并指定执行者")
	}
	return int(lease / time.Second), nil
}

// Claim acquires one job. SKIP LOCKED lets independent workers/lanes progress;
// every reclaim increments the fence even when the same worker returns.
func (q Queue) Claim(ctx context.Context, kinds []string, lane, owner string, lease time.Duration) (*Job, error) {
	if err := selection(kinds, lane); err != nil {
		return nil, err
	}
	seconds, err := leaseSeconds(owner, lease)
	if err != nil {
		return nil, err
	}
	var j Job
	err = q.DB.QueryRow(ctx, `WITH chosen AS (
        SELECT id FROM jobs WHERE kind=ANY($1::text[]) AND lane=$2 AND attempt<max_attempts
        AND ((state IN ('queued','retry_wait') AND available_at<=clock_timestamp())
            OR (state='running' AND lease_until<=clock_timestamp()))
        ORDER BY available_at,created_at,id FOR UPDATE SKIP LOCKED LIMIT 1
    ) UPDATE jobs j SET state='running',attempt=attempt+1,fencing_token=fencing_token+1,
        lease_owner=$3,lease_until=clock_timestamp()+($4::int*interval '1 second'),updated_at=clock_timestamp()
        FROM chosen WHERE j.id=chosen.id
        RETURNING j.id,j.org_id,j.kind,j.lane,j.dedup_key,j.payload_version,j.payload,j.attempt,j.max_attempts,j.fencing_token,j.lease_owner,j.lease_until`,
		kinds, lane, owner, seconds).Scan(&j.ID, &j.OrgID, &j.Kind, &j.Lane, &j.DedupKey, &j.PayloadVersion, &j.Payload, &j.Attempt, &j.MaxAttempts, &j.FencingToken, &j.LeaseOwner, &j.LeaseUntil)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, platform.DatabaseError(err)
	}
	return &j, nil
}
func lost() error {
	return core.Fail(409, "JOB_LEASE_LOST", "任务租约已失效，当前执行者不能提交结果")
}
func (q Queue) Renew(ctx context.Context, j Job, lease time.Duration) (time.Time, error) {
	seconds, err := leaseSeconds(j.LeaseOwner, lease)
	if err != nil {
		return time.Time{}, err
	}
	var until time.Time
	err = q.DB.QueryRow(ctx, `UPDATE jobs SET lease_until=clock_timestamp()+($5::int*interval '1 second'),updated_at=clock_timestamp()
        WHERE org_id=$1 AND id=$2 AND lease_owner=$3 AND fencing_token=$4 AND state='running' AND lease_until>clock_timestamp()
        RETURNING lease_until`, j.OrgID, j.ID, j.LeaseOwner, j.FencingToken, seconds).Scan(&until)
	if err == pgx.ErrNoRows {
		return time.Time{}, lost()
	}
	return until, err
}

// CheckInTx locks the job after the domain's organization/project/resource
// locks. It prevents a reclaim while the guarded business transaction commits.
func (q Queue) CheckInTx(ctx context.Context, tx pgx.Tx, j Job) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM jobs WHERE org_id=$1 AND id=$2 AND lease_owner=$3 AND fencing_token=$4
        AND state='running' AND lease_until>clock_timestamp() FOR UPDATE`, j.OrgID, j.ID, j.LeaseOwner, j.FencingToken).Scan(&id)
	if err == pgx.ErrNoRows {
		return lost()
	}
	return err
}
func (q Queue) CompleteInTx(ctx context.Context, tx pgx.Tx, j Job) error {
	tag, err := tx.Exec(ctx, `UPDATE jobs SET state='succeeded',lease_until=NULL,lease_owner=NULL,updated_at=clock_timestamp()
        WHERE org_id=$1 AND id=$2 AND lease_owner=$3 AND fencing_token=$4 AND state='running' AND lease_until>clock_timestamp()`, j.OrgID, j.ID, j.LeaseOwner, j.FencingToken)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return lost()
	}
	return nil
}
func (q Queue) Fail(ctx context.Context, j Job, code string, permanent bool, retryAfter time.Duration) error {
	return fail(ctx, q.DB, j, code, permanent, retryAfter)
}

// FailInTx keeps domain failure state and the retry/dead decision atomic.
func (q Queue) FailInTx(ctx context.Context, tx pgx.Tx, j Job, code string, permanent bool, retryAfter time.Duration) error {
	return fail(ctx, tx, j, code, permanent, retryAfter)
}
func fail(ctx context.Context, db platform.DBTX, j Job, code string, permanent bool, retryAfter time.Duration) error {
	if !errorCode.MatchString(code) || retryAfter < time.Second || retryAfter > 24*time.Hour {
		return core.Invalid("job", "任务错误码或重试间隔无效")
	}
	tag, err := db.Exec(ctx, `UPDATE jobs SET state=CASE WHEN $5 OR attempt>=max_attempts THEN 'dead' ELSE 'retry_wait' END,
        available_at=clock_timestamp()+($6::int*interval '1 second'),last_error=$7,first_failed_at=coalesce(first_failed_at,clock_timestamp()),
        lease_until=NULL,lease_owner=NULL,updated_at=clock_timestamp()
        WHERE org_id=$1 AND id=$2 AND lease_owner=$3 AND fencing_token=$4 AND state='running' AND lease_until>clock_timestamp()`,
		j.OrgID, j.ID, j.LeaseOwner, j.FencingToken, permanent, int(retryAfter/time.Second), code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return lost()
	}
	return nil
}
func (q Queue) SweepExhausted(ctx context.Context, kinds []string, lane string) (int64, error) {
	if err := selection(kinds, lane); err != nil {
		return 0, err
	}
	tag, err := q.DB.Exec(ctx, `UPDATE jobs SET state='dead',last_error='ATTEMPTS_EXHAUSTED',lease_owner=NULL,lease_until=NULL,
        first_failed_at=coalesce(first_failed_at,clock_timestamp()),updated_at=clock_timestamp()
        WHERE kind=ANY($1::text[]) AND lane=$2 AND attempt>=max_attempts
        AND (state IN ('queued','retry_wait') OR (state='running' AND lease_until<=clock_timestamp()))`, kinds, lane)
	return tag.RowsAffected(), err
}
