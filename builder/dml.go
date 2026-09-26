package builder

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Lifecycle hooks — implemented optionally by model types.
type BeforeCreator interface{ BeforeCreate() error }
type AfterCreator interface{ AfterCreate() error }
type BeforeUpdater interface{ BeforeUpdate() error }
type AfterUpdater interface{ AfterUpdate() error }
type BeforeDeleter interface{ BeforeDelete() error }
type AfterDeleter interface{ AfterDelete() error }

// Insert inserts a record from a column-value map.
func (q *QueryBuilder[T]) Insert(ctx context.Context, data map[string]any) (sql.Result, error) {
	keys := sortedKeys(data)
	cols := make([]string, len(keys))
	phs := make([]string, len(keys))
	args := make([]any, len(keys))
	for i, k := range keys {
		cols[i] = k
		phs[i] = q.dialect.Placeholder(i + 1)
		args[i] = data[k]
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", q.table, strings.Join(cols, ", "), strings.Join(phs, ", "))
	return q.executor.ExecContext(ctx, query, args...)
}

// Update updates rows matching the builder's WHERE conditions.
// Returns ErrMissingWhereClause if no user-supplied conditions exist unless AllowFullTable() was called.
func (q *QueryBuilder[T]) Update(ctx context.Context, data map[string]any) (sql.Result, error) {
	if !q.hasUserCond && !q.allowFullTable {
		return nil, ErrMissingWhereClause
	}
	keys := sortedKeys(data)
	setClauses := make([]string, len(keys))
	args := make([]any, 0, len(keys)+len(q.args))
	for i, k := range keys {
		setClauses[i] = fmt.Sprintf("%s = %s", k, q.dialect.Placeholder(i+1))
		args = append(args, data[k])
	}
	query := fmt.Sprintf("UPDATE %s SET %s", q.table, strings.Join(setClauses, ", "))
	if len(q.conditions) > 0 {
		where, whereArgs := q.buildWhere(len(keys))
		query += " WHERE " + where
		args = append(args, whereArgs...)
	}
	return q.executor.ExecContext(ctx, query, args...)
}

// Create inserts entity after executing BeforeCreate/AfterCreate hooks.
func (q *QueryBuilder[T]) Create(ctx context.Context, entity *T) (sql.Result, error) {
	if h, ok := any(entity).(BeforeCreator); ok {
		if err := h.BeforeCreate(); err != nil {
			return nil, fmt.Errorf("BeforeCreate hook: %w", err)
		}
	}
	res, err := q.Insert(ctx, structToMap(entity))
	if err != nil {
		return nil, err
	}
	if h, ok := any(entity).(AfterCreator); ok {
		if err := h.AfterCreate(); err != nil {
			return res, fmt.Errorf("AfterCreate hook: %w", err)
		}
	}
	return res, nil
}

// Save updates entity after executing BeforeUpdate/AfterUpdate hooks.
func (q *QueryBuilder[T]) Save(ctx context.Context, entity *T) (sql.Result, error) {
	if h, ok := any(entity).(BeforeUpdater); ok {
		if err := h.BeforeUpdate(); err != nil {
			return nil, fmt.Errorf("BeforeUpdate hook: %w", err)
		}
	}
	res, err := q.Update(ctx, structToMap(entity))
	if err != nil {
		return nil, err
	}
	if h, ok := any(entity).(AfterUpdater); ok {
		if err := h.AfterUpdate(); err != nil {
			return res, fmt.Errorf("AfterUpdate hook: %w", err)
		}
	}
	return res, nil
}

// Delete performs a soft-delete (if T implements SoftDeletable) or a hard DELETE.
// Returns ErrMissingWhereClause if no user-supplied conditions exist unless AllowFullTable() was called.
func (q *QueryBuilder[T]) Delete(ctx context.Context, entity *T) (sql.Result, error) {
	if entity != nil {
		if h, ok := any(entity).(BeforeDeleter); ok {
			if err := h.BeforeDelete(); err != nil {
				return nil, fmt.Errorf("BeforeDelete hook: %w", err)
			}
		}
	}
	res, err := q.execDelete(ctx)
	if err != nil {
		return nil, err
	}
	if entity != nil {
		if h, ok := any(entity).(AfterDeleter); ok {
			if err := h.AfterDelete(); err != nil {
				return res, fmt.Errorf("AfterDelete hook: %w", err)
			}
		}
	}
	return res, nil
}

func (q *QueryBuilder[T]) execDelete(ctx context.Context) (sql.Result, error) {
	if !q.hasUserCond && !q.allowFullTable {
		return nil, ErrMissingWhereClause
	}
	if isSoftDeletable[T]() {
		return q.Update(ctx, map[string]any{"deleted_at": time.Now()})
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "DELETE FROM %s", q.table)
	if len(q.conditions) > 0 {
		where, _ := q.buildWhere(0)
		fmt.Fprintf(&sb, " WHERE %s", where)
	}
	return q.executor.ExecContext(ctx, sb.String(), q.args...)
}
