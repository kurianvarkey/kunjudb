package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kurianvarkey/kunjudb"
	"github.com/kurianvarkey/kunjudb/dialects"
	"github.com/kurianvarkey/kunjudb/middleware/guard"
	"github.com/kurianvarkey/kunjudb/middleware/logger"
	"github.com/kurianvarkey/kunjudb/middleware/otel"
	"github.com/kurianvarkey/kunjudb/middleware/profiler"
	_ "github.com/lib/pq"
)

type User struct {
	ID    int    `db:"id"`
	Email string `db:"email"`
	Age   int    `db:"age"`
}

func main() {
	dsn := "postgres://user:password@localhost:5432/testdb?sslmode=disable"
	pool, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to open test database: %v", err)
	}

	if err := pool.Ping(); err != nil {
		log.Fatalf("Test database is unreachable: %v", err)
	}

	env := os.Getenv("APP_ENV")
	var securityMiddleware kunjudb.Middleware
	if env == "production" {
		securityMiddleware = guard.Profile(guard.ProfileStrict)
	} else {
		securityMiddleware = guard.Profile(guard.ProfileDevelopment)
	}

	db := kunjudb.New(pool, dialects.PostgreSql{},
		otel.NewTracingMiddleware,
		securityMiddleware,
		//logger.Default(),
		profiler.New(pool, profiler.Config{
			SlowThreshold:      100 * time.Millisecond,
			EnableIndexAdvisor: true,
		}),
	)

	// Open the log file
	logFile, err := os.OpenFile("sql.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	db.Use(logger.New(logger.Config{
		Writer:  logger.MultiWriter(os.Stdout, logFile),
		Profile: true,
	}))

	// Using the high-speed scanner on a raw query
	rows, err := db.Executor.QueryContext(context.Background(), "SELECT id, email FROM users WHERE id = $1", 1)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}
	users, err := kunjudb.MapRaw[User](rows)
	if err != nil {
		log.Fatalf("Mapping failed: %v", err)
	}

	fmt.Println(users)
}
