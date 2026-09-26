package builder

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/kurianvarkey/kunjudb/dialects"
	"github.com/kurianvarkey/kunjudb/executor"
	"github.com/kurianvarkey/kunjudb/scanner"
)

// ErrNotFound is returned when a query expects a single record but none exists.
var ErrNotFound = errors.New("kunjudb: record not found")

// ErrMissingWhereClause is returned when Update or Delete is called with no user-supplied
// WHERE conditions. Call AllowFullTable() on the builder to intentionally allow bulk operations.
var ErrMissingWhereClause = errors.New("kunjudb: refusing Update/Delete with no WHERE conditions — call AllowFullTable() to override")

// SoftDeletable enables automatic soft-delete filtering when implemented by models.
type SoftDeletable interface {
	SetDeletedAt(t *time.Time)
	IsDeleted() bool
}

var softDeletableType = reflect.TypeOf((*SoftDeletable)(nil)).Elem()

// softDeletableCache caches isSoftDeletable results per type to avoid reflect overhead per call.
var softDeletableCache sync.Map // map[reflect.Type]bool

func isSoftDeletable[T any]() bool {
	t := reflect.TypeOf((*T)(nil))
	if v, ok := softDeletableCache.Load(t); ok {
		return v.(bool)
	}
	result := t.Implements(softDeletableType) || t.Elem().Implements(softDeletableType)
	softDeletableCache.Store(t, result)
	return result
}

// QueryBuilder provides a type-safe fluent SQL builder.
type QueryBuilder[T any] struct {
	executor       executor.SqlExecutor
	dialect        dialects.Dialect
	table          string
	columns        []string
	conditions     []string
	args           []any
	orderBy        []string
	limit          int
	offset         int
	hasUserCond    bool // true when at least one user-supplied WHERE condition exists
	allowFullTable bool // when true, Update/Delete without WHERE conditions is permitted
}

// New returns a QueryBuilder for the given table.
// If T implements SoftDeletable, a `deleted_at IS NULL` filter is added automatically.
// Note: the automatic filter does NOT count as a user-supplied condition for the full-table guard.
func New[T any](exec executor.SqlExecutor, dialect dialects.Dialect, table string) *QueryBuilder[T] {
	q := &QueryBuilder[T]{
		executor: exec,
		dialect:  dialect,
		table:    table,
		columns:  []string{"*"},
	}
	if isSoftDeletable[T]() {
		// Add directly — must NOT set hasUserCond so the full-table guard still fires
		// when the user forgets to add their own condition.
		q.conditions = append(q.conditions, "deleted_at IS ?")
		q.args = append(q.args, nil)
	}
	return q
}

// Get executes the SELECT and returns all matching rows.
func (q *QueryBuilder[T]) Get(ctx context.Context) ([]T, error) {
	query, args := q.build()
	rows, err := q.executor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanner.MapRaw[T](rows)
}

// First returns the first matching row, or ErrNotFound if none exists.
func (q *QueryBuilder[T]) First(ctx context.Context) (*T, error) {
	q.limit = 1
	results, err := q.Get(ctx)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNotFound
	}
	return &results[0], nil
}
