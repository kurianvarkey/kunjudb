package kunjudb

import (
	"context"
	"database/sql"

	"github.com/kurianvarkey/kunjudb/dialects"
)

// SqlExecutor provides standard query execution methods.
type SqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Middleware wraps an SqlExecutor to provide profiling, guards, tracing, or logging.
type Middleware func(SqlExecutor) SqlExecutor

// DB represents a database instance with configured dialect and middleware pipeline.
type DB struct {
	Pool        *sql.DB
	Dialect     dialects.Dialect
	Executor    SqlExecutor
	middlewares []Middleware
}

// New initializes a DB instance with connection pool, dialect, and optional middleware.
func New(pool *sql.DB, dialect dialects.Dialect, middlewares ...Middleware) *DB {
	var exec SqlExecutor = pool
	for _, mw := range middlewares {
		exec = mw(exec)
	}
	return &DB{Pool: pool, Dialect: dialect, Executor: exec, middlewares: middlewares}
}

// Query executes a query on the wrapped executor.
func (db *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.Executor.QueryContext(ctx, query, args...)
}

// Transaction executes a function within a transaction wrapped with the same middleware pipeline.
func (db *DB) Transaction(ctx context.Context, fn func(executor SqlExecutor) error) error {
	tx, err := db.Pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	var wrappedTx SqlExecutor = tx
	for _, mw := range db.middlewares {
		wrappedTx = mw(wrappedTx)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(wrappedTx); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}
