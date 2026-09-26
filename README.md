# KunjuDB

[![Go Reference](https://pkg.go.dev/badge/github.com/kurianvarkey/kunjudb.svg)](https://pkg.go.dev/github.com/kurianvarkey/kunjudb)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**KunjuDB** is an ultra-lightweight, high-performance database toolkit and lightweight ORM for Go 1.21+. Designed with a zero-allocation philosophy in the query hot-path, it combines the developer ergonomics of modern ORMs with the raw speed and low memory footprint of handwritten SQL.

---

## 🚀 Key Features

- **⚡ Near-Zero Overhead Scanning (`MapRaw`)**: Pre-computed metadata caching (`sync.Map`) and recycled pointer pools (`sync.Pool`) achieve scanning performance within ~13 ns/row of handwritten `rows.Scan()`, while allocating **less memory** than standard manual scanning.
- **🛡️ 0-Byte Safe Discarding**: Safely ignores unmapped database columns without allocating memory or throwing scan errors using a 0-byte `discardScanner`.
- **🚨 Slow Query Profiler**: Configurable execution threshold monitoring that logs slow queries with standard structured logging (`log/slog`).
- **💡 PostgreSQL Index Advisor**: When slow queries are detected, an optional non-blocking `EXPLAIN (FORMAT JSON)` inspects execution plans for unindexed sequential scans (`Seq Scan`) and recommends missing indexes.
- **🔌 Zero-Dependency Telemetry (`QueryObserver`)**: Pluggable metrics hook allowing applications to stream query telemetry to Prometheus, OpenTelemetry, Datadog, or custom metrics engines without burdening the core package with third-party dependencies.
- **🧱 Deterministic Query Caching**: Automatic column sorting ensures prepared statements and query plan caches are preserved in PostgreSQL/MySQL.
- **🔒 SQL Guard Firewall**: Middleware that blocks destructive commands (`DROP TABLE`, `TRUNCATE`, `ALTER`) with customizable security profiles (`Development`, `ReadOnly`, `Strict`).
- **🌱 Fluent Type-Safe Builder**: Generics-based fluent API for `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `WHERE`, `ORDER BY`, `LIMIT`, `OFFSET`, and soft-deletes.
- **🪝 Lifecycle Hooks & Soft Deletes**: Automatic hook execution (`BeforeCreate`, `AfterCreate`, `BeforeUpdate`, etc.) and zero-allocation `SoftDeletable` interface checks.

---

## 📦 Installation

```bash
go get github.com/kurianvarkey/kunjudb
```

---

## 🛠️ Quick Start

### 1. Define Models
Use `db` tags to map struct fields to database columns:

```go
package main

import "time"

type User struct {
	ID        int        `db:"id"`
	Name      string     `db:"name"`
	Email     string     `db:"email"`
	Age       int        `db:"age"`
	DeletedAt *time.Time `db:"deleted_at"`
}

// Optional: Implement SoftDeletable to automatically enable soft-deletes
func (u *User) SetDeletedAt(t *time.Time) { u.DeletedAt = t }
func (u *User) IsDeleted() bool          { return u.DeletedAt != nil }
```

### 2. Initialize with Middleware Stack

```go
package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/kurianvarkey/kunjudb"
	"github.com/kurianvarkey/kunjudb/dialects"
	"github.com/kurianvarkey/kunjudb/middleware/guard"
	"github.com/kurianvarkey/kunjudb/middleware/profiler"
	_ "github.com/lib/pq"
)

func main() {
	pool, err := sql.Open("postgres", "postgres://user:pass@localhost:5432/mydb?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}

	// Initialize DB with modular middlewares
	db := kunjudb.New(pool, dialects.PostgreSql{},
		// 1. Profiler & Slow Query Monitor
		profiler.New(pool, profiler.Config{
			SlowThreshold:      100 * time.Millisecond,
			EnableIndexAdvisor: true, // Suggests indexes on slow queries
			Observer: profiler.QueryObserverFunc(func(ctx context.Context, query string, dur time.Duration, err error) {
				// Pipe duration to your metrics backend (e.g. Prometheus)
			}),
		}),
		// 2. SQL Firewall (blocks destructive queries)
		guard.Profile(guard.ProfileStrict),
	)

	_ = db
}
```

---

## 📖 Usage Guide

### Fluent Queries & Scanning

```go
ctx := context.Background()

// 1. Fetch multiple records using Fluent Builder
users, err := kunjudb.Table[User](db, "users").
	Where("age", ">=", 21).
	WhereILike("email", "%@example.com").
	OrderBy("id DESC").
	Limit(20).
	Get(ctx)

// 2. Fetch a single record (returns kunjudb.ErrNotFound if absent)
user, err := kunjudb.Table[User](db, "users").
	Where("id", "=", 42).
	First(ctx)
if errors.Is(err, kunjudb.ErrNotFound) {
	log.Println("User not found")
}

// 3. High-Performance Raw Scanning (sqlc / manual query style)
rows, err := db.Executor.QueryContext(ctx, "SELECT id, name, email FROM users WHERE age > $1", 18)
if err != nil {
	log.Fatal(err)
}
users, err = kunjudb.MapRaw[User](rows)
```

### CRUD Operations & Hooks

```go
// INSERT (Keys are deterministically sorted for prepared statement caching)
res, err := kunjudb.Table[User](db, "users").Insert(ctx, map[string]any{
	"name":  "Alice",
	"email": "alice@example.com",
	"age":   30,
})

// UPDATE with condition
res, err = kunjudb.Table[User](db, "users").
	Where("id", "=", 42).
	Update(ctx, map[string]any{
		"age": 31,
	})

// DELETE (Automatic soft-delete if model implements SoftDeletable)
err = kunjudb.Table[User](db, "users").
	Where("id", "=", 42).
	Delete(ctx, nil)

// UNSCOPED: Query including soft-deleted rows
allUsers, err := kunjudb.Table[User](db, "users").
	Unscoped().
	Where("age", ">", 18).
	Get(ctx)
```

### Safe Transactions

KunjuDB transactions wrap the full middleware stack (including logging and guards) and handle automatic rollback on panic or error:

```go
err := db.Transaction(ctx, func(exec kunjudb.SqlExecutor) error {
	_, err := exec.ExecContext(ctx, "UPDATE accounts SET balance = balance - 100 WHERE id = $1", 1)
	if err != nil {
		return err // Automatically triggers tx.Rollback()
	}

	_, err = exec.ExecContext(ctx, "UPDATE accounts SET balance = balance + 100 WHERE id = $2", 2)
	return err // Returning nil triggers tx.Commit()
})
```

---

## 📝 SQL Query Logging

The `middleware/logger` package provides query logging with safe parameter interpolation (`$1, $2` or `?`), execution timing, and zero-allocation fast-paths designed for production.

### Production Default Behavior
By default, the logger adheres to strict production hygiene:
- **Fast Queries**: Executed with **zero allocations (~77ns)** and emit **no log output**, preventing disk flood and log noise.
- **Slow Queries (>100ms)** & **Errors**: Automatically logged with full query text, parameter values, and execution time.

```go
import "github.com/kurianvarkey/kunjudb/middleware/logger"

// Standard production default (logs slow queries >100ms and errors to os.Stdout)
db := kunjudb.New(pool, dialects.PostgreSql{}, logger.Default())
```

---

### Configuration Options (`logger.Config`)

Configure behavior using standard Go types (`io.Writer` and `*slog.Logger`):

```go
type Config struct {
    Writer        io.Writer     // Destination (os.Stdout, file, or io.MultiWriter)
    SlowThreshold time.Duration // Slow query threshold (defaults to 100ms)
    Profile       bool          // If true, logs EVERY query (or via DB_PROFILE=true)
    JSON          bool          // If true, outputs structured JSON via log/slog
    Logger        *slog.Logger  // Optional custom slog.Logger (takes precedence)
}
```

#### 1. Local Development (Log Every Query to Console)
Turn on profiling to inspect every executed query and its interpolated parameters:
```go
db.Use(logger.New(logger.Config{
    Profile: true, // or run with DB_PROFILE=true in local .env
}))
```
**Sample Console Output:**
```text
✅ [1.45 ms] SELECT * FROM users WHERE id = 1 AND active = TRUE
```

#### 2. Production Structured JSON (`log/slog`)
Recommended for cloud collectors (Datadog, AWS CloudWatch, Grafana Loki, ELK). Fast queries are silent; only slow queries and failures are emitted:
```go
db.Use(logger.New(logger.Config{
    JSON:          true,                   // structured JSON lines
    SlowThreshold: 100 * time.Millisecond, // customizable threshold
}))
```
**Sample JSON Output:**
```json
{"time":"2026-09-26T12:00:00Z","level":"WARN","msg":"Slow SQL Query","sql":"SELECT * FROM orders WHERE status = 'pending'","duration_ms":152.4,"slow_query":true}
```

#### 3. Terminal + File Dual Logging
Stream logs to both the terminal (`os.Stdout`) and a log file using standard `io.MultiWriter` and proper resource cleanup:
```go
logFile, err := os.OpenFile("sql.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
if err != nil {
    log.Fatal(err)
}
defer logFile.Close() // Proper resource hygiene

db.Use(logger.New(logger.Config{
    Writer:        io.MultiWriter(os.Stdout, logFile),
    SlowThreshold: 100 * time.Millisecond,
}))
```

#### 4. Custom Application Logger Injection
Pass your existing service `*slog.Logger` to inherit application log levels, context handlers, and formats:
```go
db.Use(logger.New(logger.Config{
    Logger: myAppLogger,
}))
```

### Environment Variable
- `DB_PROFILE=true`: Forces all queries to be logged across any environment without code changes.

## 🔍 Profiler & Automated Index Advisor

When enabled, queries exceeding `SlowThreshold` are logged via `log/slog`. If attached to PostgreSQL, the profiler runs a non-blocking `EXPLAIN (FORMAT JSON)` with a 100ms timeout to detect sequential scans on filtered columns:

```json
{
  "time": "2026-09-26T11:00:00Z",
  "level": "WARN",
  "msg": "🚨 Slow Database Query Detected",
  "duration": "245.1ms",
  "threshold": "100ms",
  "query": "SELECT * FROM orders WHERE status = $1",
  "index_recommendation": "Sequential scan detected on table 'orders' (filter: (status = 'pending')). Consider adding an index on table 'orders'."
}
```

---

## 📊 Benchmark & Memory Performance

Head-to-head comparison scanning 100 records:

| Scanner Implementation | Speed (`ns/op`) | Memory (`B/op`) | Allocs (`allocs/op`) | Type-Safety |
| :--- | :--- | :--- | :--- | :--- |
| **KunjuDB `MapRaw`** | **26,129 ns** | **12,139 B** | **108 allocs** | **Automatic & Dynamic** |
| Handwritten `rows.Scan()` | 24,821 ns | 12,594 B | 111 allocs | Manual & Error-Prone |
| Naive Reflection ORM | ~45,000 ns | ~45,000 B | ~350 allocs | Automatic |

- **Zero-Allocation Hot Path**: Reflection overhead is a mere **~13 nanoseconds per row** (~5% difference compared to manual code).
- **Less Memory Than Manual Code**: Pre-allocated slice capacities and pointer recycling avoid slice growth reallocation churn, consuming **455 fewer bytes** and **3 fewer allocations** than handwritten `rows.Scan`.

---

## 🧪 Testing & Verification

### Running Unit Tests
```bash
go test -v -count=1 ./...
```

### Running Benchmarks
```bash
go test -run='^$' -bench='BenchmarkGenericScanner|BenchmarkStandardScanner' -benchmem ./tests
```

### Running Memory Profiler (`pprof`)
```bash
go test -run='^$' -bench=BenchmarkGenericScanner -memprofile=mem.pprof ./tests
go tool pprof -alloc_space mem.pprof
# Inside pprof:
(pprof) list MapRaw
```

---

## 🛡️ SQL Guard Firewall Profiles

Protect your database from accidental or malicious destructive commands:

```go
// Strict (Production Recommended): Blocks DROP, TRUNCATE, ALTER, GRANT, REVOKE
db := kunjudb.New(pool, dialects.PostgreSql{}, guard.Profile(guard.ProfileStrict))

// Read-Only: Blocks all mutations (INSERT, UPDATE, DELETE, DDL)
db := kunjudb.New(pool, dialects.PostgreSql{}, guard.Profile(guard.ProfileReadOnly))

// Development: Blocks catastrophic commands (DROP DATABASE)
db := kunjudb.New(pool, dialects.PostgreSql{}, guard.Profile(guard.ProfileDevelopment))

// Custom Keywords
db := kunjudb.New(pool, dialects.PostgreSql{}, guard.New("DROP TABLE", "DELETE FROM users"))
```

---

## 🤝 Contributing

1. Fork the repository.
2. Ensure all changes maintain zero allocations in the hot path.
3. Run tests and benchmarks:
   ```bash
   go test -v ./...
   go test -bench=. -benchmem ./...
   ```
4. Submit a Pull Request.

---

## 📜 License

Distributed under the MIT License. See [LICENSE](LICENSE) for details.