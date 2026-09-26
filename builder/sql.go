package builder

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kurianvarkey/kunjudb/dialects"
)

// ToSql returns the SQL string with values inlined — for debugging only, never for execution.
func (q *QueryBuilder[T]) ToSql() string {
	query, args := q.build()
	return reconstructSQL(query, args)
}

func (q *QueryBuilder[T]) build() (string, []any) {
	cols := "*"
	if len(q.columns) > 0 && q.columns[0] != "*" {
		cols = strings.Join(q.columns, ", ")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "SELECT %s FROM %s", cols, q.table)
	if len(q.conditions) > 0 {
		where, _ := q.buildWhere(0)
		fmt.Fprintf(&sb, " WHERE %s", where)
	}
	if len(q.orderBy) > 0 {
		fmt.Fprintf(&sb, " ORDER BY %s", strings.Join(q.orderBy, ", "))
	}
	if q.limit > 0 {
		fmt.Fprintf(&sb, " LIMIT %d", q.limit)
	}
	if q.offset > 0 {
		fmt.Fprintf(&sb, " OFFSET %d", q.offset)
	}
	return sb.String(), q.args
}

// buildWhere constructs the WHERE clause, offsetting placeholders by startCount.
func (q *QueryBuilder[T]) buildWhere(startCount int) (string, []any) {
	if len(q.conditions) == 0 {
		return "", nil
	}
	var sb strings.Builder
	for i, cond := range q.conditions {
		if i > 0 {
			upper := strings.ToUpper(cond)
			if strings.HasPrefix(upper, "OR ") || strings.HasPrefix(upper, "AND ") {
				sb.WriteByte(' ')
			} else {
				sb.WriteString(" AND ")
			}
		}
		sb.WriteString(cond)
	}
	return replacePlaceholders(sb.String(), q.dialect, startCount), q.args
}

// BuildWhere is the exported shim for external packages (e.g. join builders).
func (q *QueryBuilder[T]) BuildWhere(startCount int) (string, []any) {
	return q.buildWhere(startCount)
}

func replacePlaceholders(query string, dialect dialects.Dialect, startCount int) string {
	if !strings.Contains(query, "?") {
		return query
	}
	var sb strings.Builder
	sb.Grow(len(query) + 16)
	n := startCount
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			sb.WriteString(dialect.Placeholder(n))
		} else {
			sb.WriteByte(query[i])
		}
	}
	return sb.String()
}

var placeholderRegex = regexp.MustCompile(`\$(\d+)|\?`)

func reconstructSQL(query string, args []any) string {
	idx := 0
	return placeholderRegex.ReplaceAllStringFunc(query, func(match string) string {
		var i int
		if match == "?" {
			idx++
			i = idx
		} else {
			fmt.Sscanf(match, "$%d", &i)
		}
		if i <= 0 || i > len(args) {
			return match
		}
		return formatArg(args[i-1])
	})
}

func formatArg(arg any) string {
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
