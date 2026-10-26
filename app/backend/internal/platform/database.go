package platform

import (
	"context"
	"errors"
	"time"

	"copypaste.hk/platform/internal/core"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func OpenDatabase(ctx context.Context, connection string) (*pgxpool.Pool, error) {
	c, err := pgxpool.ParseConfig(connection)
	if err != nil {
		return nil, errors.New("invalid PostgreSQL configuration")
	}
	c.MaxConns = 10
	c.MinConns = 1
	c.MaxConnLifetime = 30 * time.Minute
	c.ConnConfig.RuntimeParams["timezone"] = "UTC"
	c.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	c.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "15000"
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, errors.New("PostgreSQL is unavailable")
	}
	return p, nil
}
func DatabaseError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Missing()
	}
	var e *pgconn.PgError
	if errors.As(err, &e) {
		switch e.Code {
		case "23505":
			return core.Fail(409, "DUPLICATE_RESOURCE", "记录已存在，请检查来源与名称")
		case "23503":
			return core.Invalid("reference", "关联资料不存在或不属于当前范围")
		case "23514", "22P02", "22007":
			return core.Invalid("value", "字段不符合资料约束")
		case "40001", "40P01":
			return core.Fail(409, "CONCURRENT_MODIFICATION", "并发修改冲突，请查询最新结果后重试")
		}
	}
	return err
}
