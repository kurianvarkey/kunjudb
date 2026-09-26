package kunjudb

import (
	"context"
	"database/sql"

	"github.com/kurianvarkey/kunjudb/dialects"
	"github.com/kurianvarkey/kunjudb/executor"
)

// SqlExecutor is a type alias for executor.SqlExecutor, preserved for backward compatibility.
// Prefer importing executor.SqlExecutor directly in new code.
type SqlExecutor = executor.SqlExecutor

// Middleware wraps a SqlExecutor to provide profiling, guards, tracing, or logging.
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

// Use appends a middleware to the pipeline and rebuilds the executor chain.
// Returns *DB for fluent chaining. Must be called before the DB is used concurrently.
//
//	db := kunjudb.New(pool, dialect)
//	db.Use(logger.Default())
//	if isProd {
//	    db.Use(guard.Profile(guard.ProfileStrict))
//	}
func (db *DB) Use(mw Middleware) *DB {
	db.middlewares = append(db.middlewares, mw)
	var exec SqlExecutor = db.Pool
	for _, m := range db.middlewares {
		exec = m(exec)
	}
	db.Executor = exec
	return db
}

// Query executes a query on the wrapped executor.
func (db *DB) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.Executor.QueryContext(ctx, query, args...)
}

// Transaction executes fn within a database transaction wrapped with the full middleware pipeline.
// The transaction is automatically rolled back on error or panic; committed on success.
// Must be called before any concurrent use of the returned executor.
func (db *DB) Transaction(ctx context.Context, fn func(SqlExecutor) error) error {
	tx, err := db.Pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	// defer Rollback is a no-op after a successful Commit (sql.ErrTxDone is silently discarded).
	// It also runs on panic, ensuring the connection is never leaked.
	defer tx.Rollback() //nolint:errcheck

	var wrappedTx SqlExecutor = tx
	for _, mw := range db.middlewares {
		wrappedTx = mw(wrappedTx)
	}

	if err := fn(wrappedTx); err != nil {
		return err
	}

	return tx.Commit()
}
