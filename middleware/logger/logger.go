package logger

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kurianvarkey/kunjudb"
)

const (
	// DefaultSlowThreshold is the default execution duration after which a query is logged as a slow query warning.
	DefaultSlowThreshold = 100 * time.Millisecond
)

// WriterGroup is implemented by compound writers (such as logger.MultiWriter)
// that expose their underlying destinations for per-target format inspection.
type WriterGroup interface {
	Writers() []io.Writer
}

type multiWriter struct {
	writers []io.Writer
}

// Config configures SQL query logging.
type Config struct {
	// Writer is the output destination for logs. Defaults to os.Stdout if nil.
	// You can pass a single writer (os.Stdout, *os.File) or multiple writers
	// via logger.MultiWriter(os.Stdout, logFile).
	Writer io.Writer

	// SlowThreshold defines the execution duration after which a query is logged as a slow query warning.
	// Defaults to 100ms if <= 0.
	SlowThreshold time.Duration

	// Profile enables logging for every query (not just slow queries and errors).
	//
	// WARNING: Do NOT enable Profile in production — it allocates on every query
	// and will flood your logs under load. Use SlowThreshold instead.
	Profile bool

	// JSON formats log entries as structured JSON lines via log/slog.
	// When false (default), clean human-readable text is printed to Writer(s).
	JSON bool

	// Logger is an optional custom *slog.Logger. If provided, it takes precedence over Writer and JSON.
	Logger *slog.Logger
}

type targetWriter struct {
	writer io.Writer
	isFile bool
}

type loggingMiddleware struct {
	inner      kunjudb.SqlExecutor
	cfg        Config
	slogLogger *slog.Logger
	targets    []targetWriter
	mu         sync.Mutex // Synchronizes writes across concurrent goroutines
}

// New returns a kunjudb.Middleware that logs SQL execution according to Config.
// Each destination writer is pre-classified once at initialization:
//   - Disk files (*os.File with regular file mode) receive timestamped Laravel-style lines.
//   - Terminal devices (os.Stdout, os.Stderr) or in-memory buffers receive emoji-tagged lines.
func New(cfg Config) kunjudb.Middleware {
	if cfg.SlowThreshold <= 0 {
		cfg.SlowThreshold = DefaultSlowThreshold
	}

	var rawWriters []io.Writer
	if cfg.Writer == nil {
		rawWriters = []io.Writer{os.Stdout}
	} else if wg, ok := cfg.Writer.(WriterGroup); ok {
		rawWriters = wg.Writers()
	} else {
		rawWriters = []io.Writer{cfg.Writer}
	}

	targets := make([]targetWriter, 0, len(rawWriters))
	destWriters := make([]io.Writer, 0, len(rawWriters))
	for _, w := range rawWriters {
		targets = append(targets, targetWriter{
			writer: w,
			isFile: isRegularFile(w), // Evaluated once at setup, eliminating per-query syscalls
		})
		destWriters = append(destWriters, w)
	}

	var sl *slog.Logger
	if cfg.Logger != nil {
		sl = cfg.Logger
	} else if cfg.JSON {
		var dest io.Writer
		if len(destWriters) == 1 {
			dest = destWriters[0]
		} else {
			dest = io.MultiWriter(destWriters...)
		}
		sl = slog.New(slog.NewJSONHandler(dest, nil))
	}

	return func(next kunjudb.SqlExecutor) kunjudb.SqlExecutor {
		return &loggingMiddleware{
			inner:      next,
			cfg:        cfg,
			slogLogger: sl,
			targets:    targets,
		}
	}
}

func (m multiWriter) Write(p []byte) (int, error) {
	for _, w := range m.writers {
		n, err := w.Write(p)
		if err != nil {
			return n, err
		}
		if n != len(p) {
			return n, io.ErrShortWrite
		}
	}
	return len(p), nil
}

func (m multiWriter) Writers() []io.Writer {
	return m.writers
}

// MultiWriter creates an io.Writer that fans out to multiple writers while allowing
// kunjudb logger to inspect each target's type and format output accordingly
// (e.g. terminal emojis for stdout vs timestamped text for files).
func MultiWriter(writers ...io.Writer) io.Writer {
	all := make([]io.Writer, 0, len(writers))
	for _, w := range writers {
		if wg, ok := w.(WriterGroup); ok {
			all = append(all, wg.Writers()...)
		} else if w != nil {
			all = append(all, w)
		}
	}
	return multiWriter{writers: all}
}

func isRegularFile(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		if fi, err := f.Stat(); err == nil {
			return fi.Mode().IsRegular()
		}
	}
	return false
}

// Default returns a logging middleware with standard production defaults:
// logs slow queries (>100ms) and query errors to os.Stdout with zero allocations for fast queries.
func Default() kunjudb.Middleware {
	return New(Config{})
}

func (m *loggingMiddleware) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := m.inner.QueryContext(ctx, query, args...)
	m.log(ctx, query, args, start, err)
	return rows, err
}

func (m *loggingMiddleware) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	res, err := m.inner.ExecContext(ctx, query, args...)
	m.log(ctx, query, args, start, err)
	return res, err
}

func (m *loggingMiddleware) log(ctx context.Context, query string, args []any, start time.Time, err error) {
	dur := time.Since(start)
	isSlow := dur > m.cfg.SlowThreshold

	// Fast path: if successful, not slow, and profiling is off, return immediately with 0 allocations and no lock.
	if err == nil && !isSlow && !m.cfg.Profile {
		return
	}

	ms := float64(dur.Microseconds()) / 1000.0
	formattedSQL := FormatSQL(query, args)

	if m.slogLogger != nil {
		m.logStructured(ctx, formattedSQL, ms, isSlow, err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, t := range m.targets {
		if t.isFile {
			m.logFile(t.writer, formattedSQL, ms, isSlow, err, start)
		} else {
			m.logTerminal(t.writer, formattedSQL, ms, isSlow, err, start)
		}
	}
}

// logTerminal writes emoji-decorated output to the terminal/buffer writer.
func (m *loggingMiddleware) logTerminal(w io.Writer, query string, ms float64, isSlow bool, err error, start time.Time) {
	ts := start.Format("2006-01-02 15:04:05")
	switch {
	case err != nil:
		fmt.Fprintf(w, "❌ [%s] [ERROR] %s (%.2f ms) | err: %v\n", ts, query, ms, err)
	case isSlow:
		fmt.Fprintf(w, "⚠️ [%s] [SLOW] %s (%.2f ms)\n", ts, query, ms)
	default:
		fmt.Fprintf(w, "✅ [%s] %s (%.2f ms)\n", ts, query, ms)
	}
}

// logFile writes a structured Laravel-style line to the file writer.
// Uses the query start time as the timestamp, not the log-write time.
func (m *loggingMiddleware) logFile(w io.Writer, query string, ms float64, isSlow bool, err error, start time.Time) {
	ts := start.Format("2006-01-02 15:04:05")
	switch {
	case err != nil:
		fmt.Fprintf(w, "[%s] ERROR: %s (%.2f ms) | err: %v\n", ts, query, ms, err)
	case isSlow:
		fmt.Fprintf(w, "[%s] WARNING: %s (%.2f ms)\n", ts, query, ms)
	default:
		fmt.Fprintf(w, "[%s] INFO: %s (%.2f ms)\n", ts, query, ms)
	}
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
// Compaction runs in a single pass without allocating a slice of tokens.
func FormatSQL(query string, args []any) string {
	cleanSQL := compactWhitespace(query)
	if len(args) == 0 {
		return cleanSQL
	}

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

	if strings.Contains(cleanSQL, "$") {
		for i, arg := range args {
			cleanSQL = strings.Replace(cleanSQL, fmt.Sprintf("$%d", i+1), formatArg(arg), 1)
		}
	}

	return cleanSQL
}

// compactWhitespace collapses consecutive whitespace in a single pass without slice allocation.
func compactWhitespace(s string) string {
	// Skip leading whitespace
	start := 0
	for start < len(s) && isSpace(s[start]) {
		start++
	}

	// Skip trailing whitespace
	end := len(s)
	for end > start && isSpace(s[end-1]) {
		end--
	}

	if start >= end {
		return ""
	}

	var sb strings.Builder
	sb.Grow(end - start)
	inSpace := false

	for i := start; i < end; i++ {
		b := s[i]
		if isSpace(b) {
			if !inSpace {
				sb.WriteByte(' ')
				inSpace = true
			}
		} else {
			sb.WriteByte(b)
			inSpace = false
		}
	}

	return sb.String()
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
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
