package kunjudb

import (
	"database/sql"

	"github.com/kurianvarkey/kunjudb/scanner"
)

// MapRaw maps sql.Rows to a slice of struct T using the high-performance cached scanner.
func MapRaw[T any](rows *sql.Rows) ([]T, error) {
	return scanner.MapRaw[T](rows)
}
