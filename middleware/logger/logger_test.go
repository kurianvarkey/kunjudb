package logger

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestFormatSQL(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		args     []any
		expected string
	}{
		{
			name:     "PostgreSQL Placeholders ($1, $2)",
			query:    "SELECT *   FROM users WHERE id = $1 AND name = $2",
			args:     []any{42, "Alice"},
			expected: "SELECT * FROM users WHERE id = 42 AND name = 'Alice'",
		},
		{
			name:     "MySQL Placeholders (?)",
			query:    "INSERT INTO users (name, age) VALUES (?, ?)",
			args:     []any{"Bob", 30},
			expected: "INSERT INTO users (name, age) VALUES ('Bob', 30)",
		},
		{
			name:     "Nil and Boolean args",
			query:    "UPDATE users SET active = $1, deleted_at = $2 WHERE id = $3",
			args:     []any{true, nil, 10},
			expected: "UPDATE users SET active = TRUE, deleted_at = NULL WHERE id = 10",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatSQL(tc.query, tc.args)
			if got != tc.expected {
				t.Errorf("FormatSQL() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestLogger_Default(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Writer:        &buf,
		SlowThreshold: 10 * time.Millisecond,
	}

	mw := New(cfg)(&mockExecutor{delay: 1 * time.Millisecond})
	ctx := context.Background()

	// 1. Fast query should NOT log anything by default (production hygiene)
	_, _ = mw.QueryContext(ctx, "SELECT * FROM users WHERE id = $1", 1)
	if buf.Len() > 0 {
		t.Errorf("expected 0 output for fast query by default, got: %s", buf.String())
	}

	// 2. Slow query should log warning
	mwSlow := New(cfg)(&mockExecutor{delay: 15 * time.Millisecond})
	_, _ = mwSlow.QueryContext(ctx, "SELECT * FROM large_table", nil)
	if !strings.Contains(buf.String(), "⚠️  [SLOW QUERY]") {
		t.Errorf("expected slow query warning, got: %s", buf.String())
	}

	// 3. Error should log error
	buf.Reset()
	mwErr := New(cfg)(&mockExecutor{err: errors.New("connection failed")})
	_, _ = mwErr.QueryContext(ctx, "SELECT 1", nil)
	if !strings.Contains(buf.String(), "❌ [SQL ERROR]") {
		t.Errorf("expected SQL error log, got: %s", buf.String())
	}
}

func TestLogger_Profile(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Writer:        &buf,
		SlowThreshold: 10 * time.Millisecond,
		Profile:       true, // Log all queries
	}

	mw := New(cfg)(&mockExecutor{delay: 1 * time.Millisecond})
	ctx := context.Background()

	_, _ = mw.QueryContext(ctx, "SELECT * FROM users WHERE id = $1", 1)
	output := buf.String()
	if !strings.Contains(output, "✅ [") || !strings.Contains(output, "SELECT * FROM users WHERE id = 1") {
		t.Errorf("expected profiled query output, got: %s", output)
	}
}

func TestLogger_JSON(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		Writer:        &buf,
		SlowThreshold: 10 * time.Millisecond,
		Profile:       true,
		JSON:          true,
	}

	mw := New(cfg)(&mockExecutor{delay: 1 * time.Millisecond})
	ctx := context.Background()

	_, _ = mw.QueryContext(ctx, "SELECT id FROM users WHERE email = $1", "test@example.com")
	output := buf.String()
	if !strings.Contains(output, `"sql":"SELECT id FROM users WHERE email = 'test@example.com'"`) {
		t.Errorf("expected JSON with formatted SQL, got: %s", output)
	}
	if !strings.Contains(output, `"duration_ms":`) {
		t.Errorf("expected duration_ms in JSON log, got: %s", output)
	}
}

func TestLogger_MultiWriter(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_sql.log")

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer file.Close()

	var termBuf bytes.Buffer
	multi := io.MultiWriter(&termBuf, file)

	cfg := Config{
		Writer:        multi,
		SlowThreshold: 5 * time.Millisecond,
	}

	mw := New(cfg)(&mockExecutor{delay: 10 * time.Millisecond})
	ctx := context.Background()
	_, _ = mw.QueryContext(ctx, "SELECT * FROM multi_test", nil)

	file.Close()

	// Verify terminal output
	if !strings.Contains(termBuf.String(), "⚠️  [SLOW QUERY]") {
		t.Errorf("expected slow query in terminal buffer, got: %s", termBuf.String())
	}

	// Verify file content
	fileBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(fileBytes), "⚠️  [SLOW QUERY]") {
		t.Errorf("expected slow query in log file, got: %s", string(fileBytes))
	}
}

func BenchmarkLogger_FastPath(b *testing.B) {
	var buf bytes.Buffer
	cfg := Config{
		Writer:        &buf,
		SlowThreshold: 100 * time.Millisecond,
	}

	exec := &mockExecutor{}
	mw := New(cfg)(exec)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = mw.QueryContext(ctx, "SELECT * FROM users WHERE id = $1", 1)
	}
}
