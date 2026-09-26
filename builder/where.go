package builder

import (
	"fmt"
	"strings"
)

// Select specifies which columns to retrieve.
func (q *QueryBuilder[T]) Select(cols ...string) *QueryBuilder[T] {
	if len(cols) > 0 {
		q.columns = cols
	}
	return q
}

// Where adds an AND WHERE condition: `col op ?`.
func (q *QueryBuilder[T]) Where(col, op string, val any) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s %s ?", col, op))
	q.args = append(q.args, val)
	q.hasUserCond = true
	return q
}

// OrWhere adds an OR WHERE condition. Falls back to Where when no prior condition exists.
func (q *QueryBuilder[T]) OrWhere(col, op string, val any) *QueryBuilder[T] {
	if len(q.conditions) == 0 {
		return q.Where(col, op, val)
	}
	q.conditions = append(q.conditions, fmt.Sprintf("OR %s %s ?", col, op))
	q.args = append(q.args, val)
	q.hasUserCond = true
	return q
}

// WhereIn adds a `col IN (?, ?, ...)` condition.
func (q *QueryBuilder[T]) WhereIn(col string, vals ...any) *QueryBuilder[T] {
	if len(vals) == 0 {
		return q
	}
	ph := strings.TrimSuffix(strings.Repeat("?, ", len(vals)), ", ")
	q.conditions = append(q.conditions, fmt.Sprintf("%s IN (%s)", col, ph))
	q.args = append(q.args, vals...)
	q.hasUserCond = true
	return q
}

// WhereBetween adds a `col BETWEEN ? AND ?` condition.
func (q *QueryBuilder[T]) WhereBetween(col string, start, end any) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s BETWEEN ? AND ?", col))
	q.args = append(q.args, start, end)
	q.hasUserCond = true
	return q
}

// WhereLike adds a `col LIKE ?` condition.
func (q *QueryBuilder[T]) WhereLike(col, val string) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s LIKE ?", col))
	q.args = append(q.args, val)
	q.hasUserCond = true
	return q
}

// WhereILike adds a case-insensitive `col ILIKE ?` condition.
func (q *QueryBuilder[T]) WhereILike(col, val string) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s ILIKE ?", col))
	q.args = append(q.args, val)
	q.hasUserCond = true
	return q
}

// WhereNull adds a `col IS NULL` condition.
func (q *QueryBuilder[T]) WhereNull(col string) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s IS NULL", col))
	q.hasUserCond = true
	return q
}

// WhereNotNull adds a `col IS NOT NULL` condition.
func (q *QueryBuilder[T]) WhereNotNull(col string) *QueryBuilder[T] {
	q.conditions = append(q.conditions, fmt.Sprintf("%s IS NOT NULL", col))
	q.hasUserCond = true
	return q
}

// WhereGroup groups sub-conditions inside parentheses.
func (q *QueryBuilder[T]) WhereGroup(fn func(sub *QueryBuilder[T])) *QueryBuilder[T] {
	sub := &QueryBuilder[T]{}
	fn(sub)
	if len(sub.conditions) == 0 {
		return q
	}
	inner := strings.TrimPrefix(strings.TrimPrefix(strings.Join(sub.conditions, " "), "AND "), "OR ")
	q.conditions = append(q.conditions, "("+inner+")")
	q.args = append(q.args, sub.args...)
	q.hasUserCond = true
	return q
}

// RawWhere appends a raw SQL fragment with its bind arguments.
//
// WARNING: Never interpolate user input directly into the query string — always
// pass untrusted values as bind arguments to prevent SQL injection:
//
//	q.RawWhere("status = ? AND age > ?", userStatus, userAge)  // ✅ safe
//	q.RawWhere("status = '" + input + "'")                     // ❌ injection risk
func (q *QueryBuilder[T]) RawWhere(query string, args ...any) *QueryBuilder[T] {
	q.conditions = append(q.conditions, query)
	q.args = append(q.args, args...)
	q.hasUserCond = true
	return q
}

// AllowFullTable permits Update or Delete to run without any WHERE conditions.
// Use only for intentional bulk operations (e.g. bulk deactivation, table wipes in tests).
func (q *QueryBuilder[T]) AllowFullTable() *QueryBuilder[T] {
	q.allowFullTable = true
	return q
}

// Unscoped removes the automatic soft-delete filter so all rows are visible.
func (q *QueryBuilder[T]) Unscoped() *QueryBuilder[T] {
	out := q.conditions[:0]
	for _, c := range q.conditions {
		if !strings.Contains(c, "deleted_at IS") {
			out = append(out, c)
		}
	}
	q.conditions = out
	return q
}
