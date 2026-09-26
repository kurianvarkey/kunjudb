package executor

import (
	"context"
	"database/sql"
)

// SqlExecutor is the shared interface satisfied by *sql.DB, *sql.Tx,
// and every middleware in the kunjudb pipeline.
// It lives in its own package so both the root package and sub-packages
// (e.g. builder) can import it without creating circular dependencies.
type SqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}
