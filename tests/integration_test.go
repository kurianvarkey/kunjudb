package tests

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kurianvarkey/kunjudb"
	"github.com/kurianvarkey/kunjudb/dialects"
	"github.com/kurianvarkey/kunjudb/middleware/otel"

	_ "github.com/lib/pq"
)

func TestIntegration_Postgres(t *testing.T) {
	// 1. Connect with proper error checking
	dsn := "postgres://user:password@localhost:5432/testdb?sslmode=disable"
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("Failed to open connection: %v", err)
	}
	defer conn.Close()

	// 2. Initialize KunjuDB
	// Ensure NewTracingMiddleware matches the Middleware interface: func(SqlExecutor) SqlExecutor
	db := kunjudb.New(conn, dialects.PostgreSql{}, otel.NewTracingMiddleware)

	// 3. Setup and Teardown
	ctx := context.Background()
	db.Pool.Exec("DROP TABLE IF EXISTS users") // Cleanup before start

	_, err = db.Pool.Exec("CREATE TABLE users (id serial primary key, email text, deleted_at timestamp)")
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// 4. Test Create/Insert logic
	_, err = db.Pool.Exec("INSERT INTO users (email) VALUES ($1)", "test@example.com")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// 5. Test the Generic QueryBuilder and Scanner
	// Note: If Get returns *User, results is a pointer, not a slice.
	user, err := kunjudb.Table[User](db, "users").
		Select("id", "email").
		Where("email", "=", "test@example.com").
		First(ctx)

	// 6. Assertions
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if user == nil {
		t.Fatal("Expected user result, got nil")
	}
	if user.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", user.Email)
	}
}
