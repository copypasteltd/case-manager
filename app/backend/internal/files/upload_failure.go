package files

import (
	"context"
	"time"

	"copypaste.hk/platform/internal/jobqueue"
	"github.com/jackc/pgx/v5"
)

func (s *UploadService) RecordUploadFailure(ctx context.Context, j jobqueue.Job, code string, permanent bool, retryAfter time.Duration) error {
	in, err := decodeUploadJob(j)
	if err != nil {
		return err
	}
	return s.jobTx(ctx, j, in, func(tx pgx.Tx, u Upload) error {
		relevant := (j.Kind == "upload.prepare.v1" && u.State == "created") || (j.Kind == "upload.finalize.v1" && u.State == "finalizing")
		relevant = relevant || (j.Kind == "upload.verify.v1" && u.State == "verifying")
		relevant = relevant || (j.Kind == "upload.promote.v1" && u.State == "verifying")
		if !relevant || u.Version != in.SessionVersion {
			return s.Queue.CompleteInTx(ctx, tx, j)
		}
		before := digest(u)
		terminal := permanent || j.Attempt >= j.MaxAttempts
		kind := "upload.retry.scheduled"
		if terminal {
			kind = "upload.failed"
			_, err = tx.Exec(ctx, "UPDATE upload_sessions SET state='failed',last_error=$2,version=version+1,updated_at=clock_timestamp() WHERE id=$1", u.ID, code)
		} else {
			_, err = tx.Exec(ctx, "UPDATE upload_sessions SET last_error=$2,updated_at=clock_timestamp() WHERE id=$1", u.ID, code)
		}
		if err != nil {
			return err
		}
		u, err = upload(ctx, tx, j.OrgID, u.ID, false)
		if err != nil {
			return err
		}
		if err = workerEvent(ctx, tx, j, u, kind, before); err != nil {
			return err
		}
		return s.Queue.FailInTx(ctx, tx, j, code, permanent, retryAfter)
	})
}
