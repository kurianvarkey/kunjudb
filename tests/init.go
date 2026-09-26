package tests

import (
	"database/sql"
	"os"
	"testing"

	"github.com/kurianvarkey/kunjudb"
	"github.com/kurianvarkey/kunjudb/dialects"
)

func setupTestDB(t *testing.T) *kunjudb.DB {
	// 1. Connection string (usually from an environment variable)
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgres://user:password@localhost:5432/testdb?sslmode=disable"
	}

	// 2. Open standard sql.DB
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// 3. Verify connection
	if err := conn.Ping(); err != nil {
		t.Fatalf("Test database is unreachable: %v", err)
	}

	// 4. Wrap in your library's DB struct
	// We usually don't need logging/guards during unit benchmarks
	return kunjudb.New(conn, dialects.PostgreSql{})
}
