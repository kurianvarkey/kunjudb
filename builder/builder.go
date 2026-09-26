package builder

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kurianvarkey/kunjudb/dialects"
	"github.com/kurianvarkey/kunjudb/scanner"
)

// ErrNotFound is returned when a query expects a single record but none exists.
var ErrNotFound = errors.New("kunjudb: record not found")

// Executor is a narrow consumer interface for database execution.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Lifecycle Hooks
type BeforeCreator interface{ BeforeCreate() error }
type AfterCreator interface{ AfterCreate() error }
type BeforeUpdater interface{ BeforeUpdate() error }
type AfterUpdater interface{ AfterUpdate() error }
type BeforeDeleter interface{ BeforeDelete() error }
type AfterDeleter interface{ AfterDelete() error }

// SoftDeletable enables automatic soft-delete filtering when implemented by models.
type SoftDeletable interface {
	SetDeletedAt(t *time.Time)
	IsDeleted() bool
}

var softDeletableType = reflect.TypeOf((*SoftDeletable)(nil)).Elem()

func isSoftDeletable[T any]() bool {
	t := reflect.TypeOf((*T)(nil))
	if t.Implements(softDeletableType) {
		return true
	}
	if t.Elem().Implements(softDeletableType) {
		return true
	}
	return false
}

// QueryBuilder provides a type-safe fluent SQL builder.
type QueryBuilder[T any] struct {
	executor   Executor
	dialect    dialects.Dialect
	table      string
	columns    []string
	conditions []string
	args       []any
	orderBy    []string
	limit      int
	offset     int
}

// New returns a new QueryBuilder initialized for table.
func New[T any](exec Executor, dialect dialects.Dialect, table string) *QueryBuilder[T] {
	builder := &QueryBuilder[T]{
		executor: exec,
		dialect:  dialect,
		table:    table,
		columns:  []string{"*"},
	}

	if isSoftDeletable[T]() {
		builder.Where("deleted_at", "IS", nil)
	}

	return builder
}

// Select specifies which columns to retrieve.
func (q *QueryBuilder[T]) Select(cols ...string) *QueryBuilder[T] {
	if len(cols) > 0 {
		q.columns = cols
	}
	return q
}

// Where adds a WHERE condition.
func (q *QueryBuilder[T]) Where(col, op string, val any) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s %s ?", col, op))
	q.args = append(q.args, val)
	return q
}

// OrWhere adds an OR WHERE condition.
func (q *QueryBuilder[T]) OrWhere(col, op string, val any) *QueryBuilder[T] {
	if len(q.conditions) == 0 {
		return q.Where(col, op, val)
	}

	q.conditions = append(q.conditions, fmt.Sprintf("OR %s %s ?", col, op))
	q.args = append(q.args, val)
	return q
}

// WhereIn adds an IN condition.
func (q *QueryBuilder[T]) WhereIn(col string, vals ...any) *QueryBuilder[T] {
	if len(vals) == 0 {
		return q
	}

	placeholders := make([]string, len(vals))
	for i := range vals {
		placeholders[i] = "?"
	}

	q.conditions = append(q.conditions, fmt.Sprintf("%s IN (%s)", col, strings.Join(placeholders, ", ")))
	q.args = append(q.args, vals...)
	return q
}

// WhereBetween adds a BETWEEN condition.
func (q *QueryBuilder[T]) WhereBetween(col string, start, end any) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s BETWEEN ? AND ?", col))
	q.args = append(q.args, start, end)
	return q
}

// WhereLike adds a LIKE condition.
func (q *QueryBuilder[T]) WhereLike(col string, val string) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s LIKE ?", col))
	q.args = append(q.args, val)
	return q
}

// WhereILike adds a case-insensitive ILIKE condition.
func (q *QueryBuilder[T]) WhereILike(col string, val string) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s ILIKE ?", col))
	q.args = append(q.args, val)
	return q
}

// WhereNull checks if a column is NULL.
func (q *QueryBuilder[T]) WhereNull(col string) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s IS NULL", col))
	return q
}

// WhereNotNull checks if a column is NOT NULL.
func (q *QueryBuilder[T]) WhereNotNull(col string) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s IS NOT NULL", col))
	return q
}

// WhereGroup groups conditions inside parentheses.
func (q *QueryBuilder[T]) WhereGroup(fn func(sub *QueryBuilder[T])) *QueryBuilder[T] {
	sub := &QueryBuilder[T]{}
	fn(sub)

	if len(sub.conditions) > 0 {
		groupQuery := "(" + strings.Join(sub.conditions, " ") + ")"
		groupQuery = strings.Replace(groupQuery, "(AND ", "(", 1)
		groupQuery = strings.Replace(groupQuery, "(OR ", "(", 1)

		q.conditions = append(q.conditions, groupQuery)
		q.args = append(q.args, sub.args...)
	}
	return q
}

// RawWhere adds a raw condition string.
func (q *QueryBuilder[T]) RawWhere(query string, args ...any) *QueryBuilder[T] {
	q.conditions = append(q.conditions, query)
	q.args = append(q.args, args...)
	return q
}

// OrderBy adds an ORDER BY clause.
func (q *QueryBuilder[T]) OrderBy(column string, direction string) *QueryBuilder[T] {
	q.orderBy = append(q.orderBy, fmt.Sprintf("%s %s", column, direction))
	return q
}

// Limit sets the query LIMIT.
func (q *QueryBuilder[T]) Limit(limit int) *QueryBuilder[T] {
	q.limit = limit
	return q
}

// Offset sets the query OFFSET.
func (q *QueryBuilder[T]) Offset(offset int) *QueryBuilder[T] {
	q.offset = offset
	return q
}

// Page sets pagination based on page number (1-indexed) and page size.
func (q *QueryBuilder[T]) Page(pageNumber int, pageSize int) *QueryBuilder[T] {
	if pageNumber < 1 {
		pageNumber = 1
	}

	q.limit = pageSize
	q.offset = (pageNumber - 1) * pageSize
	return q
}

// ToSql returns the reconstructed SQL string.
func (q *QueryBuilder[T]) ToSql() string {
	query, args := q.build()
	return reconstructSQL(query, args)
}

func (q *QueryBuilder[T]) build() (string, []any) {
	colsPart := "*"
	if len(q.columns) > 0 && q.columns[0] != "*" {
		colsPart = strings.Join(q.columns, ", ")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("SELECT %s FROM %s", colsPart, q.table))

	if len(q.conditions) > 0 {
		sb.WriteString(" WHERE ")
		whereClause, _ := q.BuildWhere(0)
		sb.WriteString(whereClause)
	}

	if len(q.orderBy) > 0 {
		sb.WriteString(" ORDER BY " + strings.Join(q.orderBy, ", "))
	}

	if q.limit > 0 {
		sb.WriteString(fmt.Sprintf(" LIMIT %d", q.limit))
	}

	if q.offset > 0 {
		sb.WriteString(fmt.Sprintf(" OFFSET %d", q.offset))
	}

	return sb.String(), q.args
}

// BuildWhere constructs the WHERE clause using the given start placeholder index.
func (q *QueryBuilder[T]) BuildWhere(startCount int) (string, []any) {
	if len(q.conditions) == 0 {
		return "", nil
	}

	var sb strings.Builder
	for i, cond := range q.conditions {
		if i > 0 {
			upperCond := strings.ToUpper(cond)
			if !strings.HasPrefix(upperCond, "OR ") && !strings.HasPrefix(upperCond, "AND ") {
				sb.WriteString(" AND ")
			} else {
				sb.WriteString(" ")
			}
		}
		sb.WriteString(cond)
	}

	return replacePlaceholders(sb.String(), q.dialect, startCount), q.args
}

func replacePlaceholders(query string, dialect dialects.Dialect, startCount int) string {
	if !strings.Contains(query, "?") {
		return query
	}

	var sb strings.Builder
	sb.Grow(len(query) + 16)
	argCount := startCount

	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			argCount++
			sb.WriteString(dialect.Placeholder(argCount))
		} else {
			sb.WriteByte(query[i])
		}
	}

	return sb.String()
}

var queryPlaceholderRegex = regexp.MustCompile(`\$(\d+)|\?`)

func reconstructSQL(query string, args []any) string {
	argIndex := 0

	return queryPlaceholderRegex.ReplaceAllStringFunc(query, func(match string) string {
		var index int
		if match == "?" {
			argIndex++
			index = argIndex
		} else {
			fmt.Sscanf(match, "$%d", &index)
		}

		if index <= 0 || index > len(args) {
			return match
		}

		return formatArgForSQL(args[index-1])
	})
}

func formatArgForSQL(arg any) string {
	if arg == nil {
		return "NULL"
	}

	switch v := arg.(type) {
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
	case time.Time:
		return fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05"))
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Insert inserts a record using the given column-value map.
func (q *QueryBuilder[T]) Insert(ctx context.Context, data map[string]any) (sql.Result, error) {
	columnLen := len(data)
	columns := make([]string, 0, columnLen)
	placeholders := make([]string, 0, columnLen)
	args := make([]any, 0, columnLen)

	keys := make([]string, 0, columnLen)
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, column := range keys {
		columns = append(columns, column)
		placeholders = append(placeholders, q.dialect.Placeholder(i+1))
		args = append(args, data[column])
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		q.table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	return q.executor.ExecContext(ctx, query, args...)
}

// Update updates records matching the Builder's conditions.
func (q *QueryBuilder[T]) Update(ctx context.Context, data map[string]any) (sql.Result, error) {
	columnLen := len(data)
	setClauses := make([]string, 0, columnLen)
	setArgs := make([]any, 0, columnLen+len(q.args))

	keys := make([]string, 0, columnLen)
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	paramIndex := 0
	for _, column := range keys {
		paramIndex++
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", column, q.dialect.Placeholder(paramIndex)))
		setArgs = append(setArgs, data[column])
	}

	query := fmt.Sprintf("UPDATE %s SET %s", q.table, strings.Join(setClauses, ", "))

	if len(q.conditions) > 0 {
		whereClause, whereArgs := q.BuildWhere(paramIndex)
		query += " WHERE " + whereClause
		setArgs = append(setArgs, whereArgs...)
	}

	return q.executor.ExecContext(ctx, query, setArgs...)
}

// Create creates an entity, executing hooks and inserting fields.
func (q *QueryBuilder[T]) Create(ctx context.Context, table string, entity any) (sql.Result, error) {
	if h, ok := entity.(BeforeCreator); ok {
		if err := h.BeforeCreate(); err != nil {
			return nil, err
		}
	}

	res, err := q.Insert(ctx, structToMap(entity))
	if err != nil {
		return nil, err
	}

	if h, ok := entity.(AfterCreator); ok {
		if err := h.AfterCreate(); err != nil {
			return res, err
		}
	}

	return res, nil
}

// Save updates an entity, executing hooks.
func (q *QueryBuilder[T]) Save(ctx context.Context, entity *T) (sql.Result, error) {
	if h, ok := any(entity).(BeforeUpdater); ok {
		if err := h.BeforeUpdate(); err != nil {
			return nil, err
		}
	}

	res, err := q.Update(ctx, structToMap(entity))
	if err != nil {
		return nil, err
	}

	if h, ok := any(entity).(AfterUpdater); ok {
		if err := h.AfterUpdate(); err != nil {
			return res, err
		}
	}

	return res, nil
}

// Delete executes a soft delete or hard delete.
func (q *QueryBuilder[T]) Delete(ctx context.Context, entity *T) (sql.Result, error) {
	if entity != nil {
		if h, ok := any(entity).(BeforeDeleter); ok {
			if err := h.BeforeDelete(); err != nil {
				return nil, err
			}
		}
	}

	var result sql.Result
	var err error

	if isSoftDeletable[T]() {
		now := time.Now()
		result, err = q.Update(ctx, map[string]any{"deleted_at": now})
	} else {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("DELETE FROM %s", q.table))

		if len(q.conditions) > 0 {
			whereClause, _ := q.BuildWhere(0)
			sb.WriteString(" WHERE " + whereClause)
		}

		result, err = q.executor.ExecContext(ctx, sb.String(), q.args...)
	}

	if err != nil {
		return nil, err
	}

	if entity != nil {
		if h, ok := any(entity).(AfterDeleter); ok {
			if err := h.AfterDelete(); err != nil {
				return result, err
			}
		}
	}

	return result, nil
}

// Unscoped removes the automatic soft-delete filter from the conditions.
func (q *QueryBuilder[T]) Unscoped() *QueryBuilder[T] {
	newConditions := make([]string, 0, len(q.conditions))
	for _, w := range q.conditions {
		if !strings.Contains(w, "deleted_at IS") {
			newConditions = append(newConditions, w)
		}
	}

	q.conditions = newConditions
	return q
}

// Get executes the SELECT query and maps results using MapRaw.
func (q *QueryBuilder[T]) Get(ctx context.Context) ([]T, error) {
	query, args := q.build()
	rows, err := q.executor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	return scanner.MapRaw[T](rows)
}

// First fetches a single record or returns ErrNotFound.
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

func structToMap(entity any) map[string]any {
	out := make(map[string]any)
	v := reflect.Indirect(reflect.ValueOf(entity))
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		out[tag] = v.Field(i).Interface()
	}

	return out
}
