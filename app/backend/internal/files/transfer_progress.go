package files

import (
	"context"
	"encoding/json"

	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"github.com/jackc/pgx/v5"
)

// guardedTransferTx rechecks the domain and lease before each durable receipt.
// Cloud operations run outside this transaction.
type guardedTransferTx func(func(pgx.Tx) error) error

func loadTransferHash(ctx context.Context, id string, state *objectstore.CopyState, checkpoint guardedTransferTx) error {
	return checkpoint(func(tx pgx.Tx) error {
		var sha *string
		err := tx.QueryRow(ctx, `SELECT source_digest,runtime_version,completed_bytes,hash_state,checksum,sha256
            FROM object_transfer_hashes WHERE transfer_id=$1`, id).Scan(&state.Hash.SourceDigest,
			&state.Hash.RuntimeVersion, &state.Hash.CompletedBytes, &state.Hash.State, &state.Hash.Checksum, &sha)
		if err == pgx.ErrNoRows {
			return nil
		}
		if sha != nil {
			state.Hash.SHA256 = *sha
		}
		return err
	})
}
func transferHooks(ctx context.Context, id string, fence int64, checkpoint guardedTransferTx) objectstore.CopyHooks {
	return objectstore.CopyHooks{
		Check: func() error { return checkpoint(func(pgx.Tx) error { return nil }) },
		Part: func(p objectstore.Part) error {
			return checkpoint(func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO object_transfer_parts(transfer_id,part_no,size_bytes,etag,fencing_token)
                    VALUES($1,$2,$3,$4,$5) ON CONFLICT(transfer_id,part_no) DO UPDATE SET size_bytes=excluded.size_bytes,etag=excluded.etag,
                    fencing_token=excluded.fencing_token,updated_at=clock_timestamp()`, id, p.Number, p.Size, p.ETag, fence)
				return err
			})
		},
		Seal: func(parts []objectstore.Part) error {
			return checkpoint(func(tx pgx.Tx) error {
				raw, _ := json.Marshal(parts)
				_, err := tx.Exec(ctx, "UPDATE object_transfers SET manifest=$2,state='sealed',updated_at=clock_timestamp() WHERE id=$1", id, raw)
				return err
			})
		},
		Frozen: func(source objectstore.Source) error {
			return checkpoint(func(tx pgx.Tx) error {
				raw, _ := json.Marshal(source)
				_, err := tx.Exec(ctx, "UPDATE object_transfers SET frozen_source=$2,state='complete',updated_at=clock_timestamp() WHERE id=$1", id, raw)
				return err
			})
		},
		Hash: func(p objectstore.HashCheckpoint) error {
			return checkpoint(func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, `INSERT INTO object_transfer_hashes(transfer_id,source_digest,runtime_version,completed_bytes,hash_state,checksum,sha256,fencing_token)
                    VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8)
                    ON CONFLICT(transfer_id) DO UPDATE SET runtime_version=excluded.runtime_version,completed_bytes=excluded.completed_bytes,hash_state=excluded.hash_state,
                    checksum=excluded.checksum,sha256=excluded.sha256,fencing_token=excluded.fencing_token,updated_at=clock_timestamp()
                    WHERE object_transfer_hashes.source_digest=excluded.source_digest`, id, p.SourceDigest, p.RuntimeVersion, p.CompletedBytes, p.State, p.Checksum, p.SHA256, fence)
				if err == nil && tag.RowsAffected() != 1 {
					return core.Fail(409, "SOURCE_CHECKPOINT_MISMATCH", "目标摘要来源已改变")
				}
				return err
			})
		},
	}
}
