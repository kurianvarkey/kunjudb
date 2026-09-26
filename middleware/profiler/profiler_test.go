package profiler

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type mockExecutor struct {
	delay time.Duration
	err   error
}

func (m *mockExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return nil, m.err
}

func (m *mockExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return nil, m.err
}

func TestProfilerMiddleware_FastQuery(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	var observedCount int32
	observer := QueryObserverFunc(func(ctx context.Context, query string, duration time.Duration, err error) {
		atomic.AddInt32(&observedCount, 1)
	})

	cfg := Config{
		SlowThreshold: 50 * time.Millisecond,
		Logger:        logger,
		Observer:      observer,
	}

	exec := &mockExecutor{delay: 1 * time.Millisecond}
	mw := New(nil, cfg)(exec)

	ctx := context.Background()
	_, _ = mw.QueryContext(ctx, "SELECT id FROM users WHERE id = $1", 1)
	_, _ = mw.ExecContext(ctx, "UPDATE users SET age = 30 WHERE id = $1", 1)

	if atomic.LoadInt32(&observedCount) != 2 {
		t.Fatalf("expected observer to be called twice, got %d", observedCount)
	}

	if strings.Contains(logBuf.String(), "Slow Database Query Detected") {
		t.Errorf("fast queries should not produce slow query warnings, got log: %s", logBuf.String())
	}
}

func TestProfilerMiddleware_SlowQuery(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	cfg := Config{
		SlowThreshold: 10 * time.Millisecond,
		Logger:        logger,
	}

	exec := &mockExecutor{delay: 20 * time.Millisecond}
	mw := New(nil, cfg)(exec)

	ctx := context.Background()
	_, _ = mw.QueryContext(ctx, "SELECT * FROM large_table WHERE name = $1", "test")

	if !strings.Contains(logBuf.String(), "Slow Database Query Detected") {
		t.Errorf("expected slow query warning log, got: %s", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "SELECT * FROM large_table") {
		t.Errorf("log should contain query, got: %s", logBuf.String())
	}
}

func TestProfilerMiddleware_QueryError(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	cfg := Config{
		SlowThreshold: 5 * time.Millisecond,
		Logger:        logger,
	}

	queryErr := errors.New("connection failed")
	exec := &mockExecutor{delay: 10 * time.Millisecond, err: queryErr}
	mw := New(nil, cfg)(exec)

	ctx := context.Background()
	_, err := mw.QueryContext(ctx, "SELECT 1", nil)
	if !errors.Is(err, queryErr) {
		t.Fatalf("expected error %v, got %v", queryErr, err)
	}

	if !strings.Contains(logBuf.String(), "connection failed") {
		t.Errorf("expected error attribute in log, got: %s", logBuf.String())
	}
}

func TestFindSeqScanRecommendation(t *testing.T) {
	tests := []struct {
		name     string
		plan     map[string]any
		expected string
	}{
		{
			name: "Direct Seq Scan with Filter",
			plan: map[string]any{
				"Node Type":     "Seq Scan",
				"Relation Name": "users",
				"Filter":        "(email = 'test@example.com')",
			},
			expected: "Sequential scan detected on table 'users' (filter: (email = 'test@example.com')). Consider adding an index on table 'users'.",
		},
		{
			name: "Index Scan (No Recommendation)",
			plan: map[string]any{
				"Node Type":     "Index Scan",
				"Relation Name": "users",
			},
			expected: "",
		},
		{
			name: "Nested Plan with Seq Scan",
			plan: map[string]any{
				"Node Type": "Nested Loop",
				"Plans": []any{
					map[string]any{
						"Node Type":     "Seq Scan",
						"Relation Name": "orders",
						"Filter":        "(status = 'pending')",
					},
				},
			},
			expected: "Sequential scan detected on table 'orders' (filter: (status = 'pending')). Consider adding an index on table 'orders'.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findSeqScanRecommendation(tc.plan)
			if got != tc.expected {
				t.Errorf("findSeqScanRecommendation() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestIsSelectQuery(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"SELECT * FROM users", true},
		{"select id from users", true},
		{"   SELECT count(*) FROM t", true},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", true},
		{"with data as (select 1) select * from data", true},
		{"INSERT INTO users (name) VALUES ('a')", false},
		{"UPDATE users SET name = 'b'", false},
		{"DELETE FROM users", false},
		{"SEL", false},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			got := isSelectQuery(tc.query)
			if got != tc.expected {
				t.Errorf("isSelectQuery(%q) = %v, want %v", tc.query, got, tc.expected)
			}
		})
	}
}
