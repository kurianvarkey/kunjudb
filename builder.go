package kunjudb

import (
	"github.com/kurianvarkey/kunjudb/builder"
)

// ErrNotFound is returned when a query expects a single record but none exists.
var ErrNotFound = builder.ErrNotFound

// QueryBuilder is an alias to the builder implementation.
type QueryBuilder[T any] = builder.QueryBuilder[T]

// SoftDeletable is an alias to the model soft-delete interface.
type SoftDeletable = builder.SoftDeletable

// Table is the entry point for type-safe fluent queries.
func Table[T any](db *DB, tableName string) *QueryBuilder[T] {
	return builder.New[T](db.Executor, db.Dialect, tableName)
}
