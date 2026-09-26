package logger

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/kurianvarkey/kunjudb"
)

// Config configures SQL query logging.
type Config struct {
	// Writer is the output destination for logs. Defaults to os.Stdout if nil.
	// To log to multiple destinations (e.g. terminal + file), pass io.MultiWriter(os.Stdout, file).
	Writer io.Writer

	// SlowThreshold defines the execution duration after which a query is logged as a slow query warning.
	// Defaults to 100ms if <= 0.
	SlowThreshold time.Duration

	// Profile enables logging for every query (not just slow queries and errors).
	// Can also be enabled via the DB_PROFILE=true environment variable.
	Profile bool

	// JSON formats log entries as structured JSON lines via log/slog.
	// When false (default), clean human-readable text is printed.
	JSON bool

	// Logger is an optional custom *slog.Logger. If provided, it takes precedence over Writer and JSON.
	Logger *slog.Logger
}

type loggingMiddleware struct {
	inner      kunjudb.SqlExecutor
	cfg        Config
	slogLogger *slog.Logger
	out        io.Writer
}

// New returns a kunjudb.Middleware that logs SQL execution according to Config.
// By default, it safely logs only slow queries (>100ms) and errors to Writer.
func New(cfg Config) kunjudb.Middleware {
	if cfg.SlowThreshold <= 0 {
		cfg.SlowThreshold = 100 * time.Millisecond
	}
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}
	if os.Getenv("DB_PROFILE") == "true" {
		cfg.Profile = true
	}

	var sl *slog.Logger
	if cfg.Logger != nil {
		sl = cfg.Logger
	} else if cfg.JSON {
		sl = slog.New(slog.NewJSONHandler(cfg.Writer, nil))
	}

	return func(next kunjudb.SqlExecutor) kunjudb.SqlExecutor {
		return &loggingMiddleware{
			inner:      next,
			cfg:        cfg,
			slogLogger: sl,
			out:        cfg.Writer,
		}
	}
}

// Default returns a logging middleware with standard production defaults:
// logs slow queries (>100ms) and query errors to os.Stdout with zero allocations for fast queries.
func Default() kunjudb.Middleware {
	return New(Config{})
}

func (m *loggingMiddleware) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := m.inner.QueryContext(ctx, query, args...)
	m.log(ctx, query, args, time.Since(start), err)
	return rows, err
}

func (m *loggingMiddleware) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	res, err := m.inner.ExecContext(ctx, query, args...)
	m.log(ctx, query, args, time.Since(start), err)
	return res, err
}

func (m *loggingMiddleware) log(ctx context.Context, query string, args []any, dur time.Duration, err error) {
	isSlow := dur > m.cfg.SlowThreshold

	// Fast path: if successful, not slow, and profiling is off, return immediately with 0 allocations.
	if err == nil && !isSlow && !m.cfg.Profile {
		return
	}

	durationMs := float64(dur.Microseconds()) / 1000.0
	formattedSQL := FormatSQL(query, args)

	if m.slogLogger != nil {
		m.logStructured(ctx, formattedSQL, durationMs, isSlow, err)
	} else {
		m.logText(formattedSQL, durationMs, isSlow, err)
	}
}

func (m *loggingMiddleware) logText(sql string, ms float64, isSlow bool, err error) {
	if err != nil {
		fmt.Fprintf(m.out, "❌ [SQL ERROR] (%.2f ms): %s | err: %v\n", ms, sql, err)
		return
	}
	if isSlow {
		fmt.Fprintf(m.out, "⚠️  [SLOW QUERY] (%.2f ms): %s\n", ms, sql)
		return
	}
	fmt.Fprintf(m.out, "✅ [%.2f ms] %s\n", ms, sql)
}

func (m *loggingMiddleware) logStructured(ctx context.Context, sql string, ms float64, isSlow bool, err error) {
	if err != nil {
		m.slogLogger.ErrorContext(ctx, "SQL Error",
			slog.String("sql", sql),
			slog.Float64("duration_ms", ms),
			slog.String("error", err.Error()),
		)
		return
	}
	if isSlow {
		m.slogLogger.WarnContext(ctx, "Slow SQL Query",
			slog.String("sql", sql),
			slog.Float64("duration_ms", ms),
			slog.Bool("slow_query", true),
		)
		return
	}
	m.slogLogger.InfoContext(ctx, "SQL Query",
		slog.String("sql", sql),
		slog.Float64("duration_ms", ms),
	)
}

// FormatSQL normalizes whitespace and safely interpolates placeholders ($1, $2 or ?) for debugging.
func FormatSQL(query string, args []any) string {
	if len(args) == 0 {
		return strings.Join(strings.Fields(query), " ")
	}

	cleanSQL := strings.Join(strings.Fields(query), " ")
	argIdx := 0

	if strings.Contains(cleanSQL, "?") {
		var sb strings.Builder
		sb.Grow(len(cleanSQL) + 16)
		for i := 0; i < len(cleanSQL); i++ {
			if cleanSQL[i] == '?' && argIdx < len(args) {
				sb.WriteString(formatArg(args[argIdx]))
				argIdx++
			} else {
				sb.WriteByte(cleanSQL[i])
			}
		}
		cleanSQL = sb.String()
	}

	for i, arg := range args {
		cleanSQL = strings.Replace(cleanSQL, fmt.Sprintf("$%d", i+1), formatArg(arg), 1)
	}

	return cleanSQL
}

func formatArg(arg any) string {
	switch v := arg.(type) {
	case nil:
		return "NULL"
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
	case []byte:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(string(v), "'", "''"))
	case time.Time:
		return fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05.000"))
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	default:
		return fmt.Sprintf("%v", v)
	}
}
