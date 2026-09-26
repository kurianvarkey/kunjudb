package builder

import "fmt"

// OrderBy adds an `ORDER BY column direction` clause.
func (q *QueryBuilder[T]) OrderBy(column, direction string) *QueryBuilder[T] {
	q.orderBy = append(q.orderBy, fmt.Sprintf("%s %s", column, direction))
	return q
}

// Limit sets the LIMIT clause.
func (q *QueryBuilder[T]) Limit(limit int) *QueryBuilder[T] {
	q.limit = limit
	return q
}

// Offset sets the OFFSET clause.
func (q *QueryBuilder[T]) Offset(offset int) *QueryBuilder[T] {
	q.offset = offset
	return q
}

// Page sets LIMIT/OFFSET from a 1-indexed page number and page size.
func (q *QueryBuilder[T]) Page(pageNumber, pageSize int) *QueryBuilder[T] {
	if pageNumber < 1 {
		pageNumber = 1
	}
	q.limit = pageSize
	q.offset = (pageNumber - 1) * pageSize
	return q
}
