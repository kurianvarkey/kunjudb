package logger

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kurianvarkey/kunjudb"
)

// Logger is the interface for logging executed SQL queries.
type Logger interface {
	Log(ctx context.Context, sql string, duration time.Duration, err error)
}

type loggingMiddleware struct {
	inner  kunjudb.SqlExecutor
	logger Logger
}

// New creates a kunjudb.Middleware that logs executed queries using the provided Logger.
func New(logger Logger) kunjudb.Middleware {
	return func(next kunjudb.SqlExecutor) kunjudb.SqlExecutor {
		return &loggingMiddleware{
			inner:  next,
			logger: logger,
		}
	}
}

func (m *loggingMiddleware) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := m.inner.QueryContext(ctx, query, args...)
	m.logger.Log(ctx, query, time.Since(start), err)
	return rows, err
}

func (m *loggingMiddleware) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	res, err := m.inner.ExecContext(ctx, query, args...)
	m.logger.Log(ctx, query, time.Since(start), err)
	return res, err
}

// DefaultLogger prints query execution status to standard output.
type DefaultLogger struct{}

func (l DefaultLogger) Log(ctx context.Context, query string, duration time.Duration, err error) {
	if err != nil {
		fmt.Printf("❌ ERROR: %s | err: %v\n", query, err)
	} else if duration > 200*time.Millisecond {
		fmt.Printf("⚠️ SLOW (%v): %s\n", duration, query)
	} else {
		fmt.Printf("✅ OK (%v): %s\n", duration, query)
	}
}

// FileLogger is a file-backed query logger.
type FileLogger struct {
	file     *os.File
	internal *log.Logger
}

// NewFileLogger initializes a logger that writes to a specific file path.
func NewFileLogger(filename string) (*FileLogger, error) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return nil, err
	}

	return &FileLogger{
		file:     file,
		internal: log.New(file, "", log.LstdFlags),
	}, nil
}

func (l *FileLogger) Log(ctx context.Context, query string, duration time.Duration, err error) {
	status := "OK"
	if err != nil {
		status = fmt.Sprintf("ERROR: %v", err)
	}
	l.internal.Printf("%s | %v | %s\n", status, duration, query)
}

// Close closes the underlying log file descriptor.
func (l *FileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
