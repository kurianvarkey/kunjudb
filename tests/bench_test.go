package tests

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kurianvarkey/kunjudb"
)

// go test -bench=. -benchmem ./pkg/ndb/

// BenchmarkGenericScanner tests your MapRaw implementation
func BenchmarkGenericScannerMapRaw(b *testing.B) {
	for b.Loop() {
		// 1. Setup inside loop because rows are "consumed" once read
		b.StopTimer()
		rows := setupMockRows(100)
		b.StartTimer()

		_, err := kunjudb.MapRaw[User](rows)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStandardScanner tests your manual scanning
func BenchmarkStandardScanner(b *testing.B) {
	for b.Loop() {
		b.StopTimer()
		rows := setupMockRows(100)
		b.StartTimer()

		var users []User
		for rows.Next() {
			var u User
			// Manual scanning - our baseline
			err := rows.Scan(&u.ID, &u.Email, &u.Age)
			if err != nil {
				b.Fatal(err)
			}
			users = append(users, u)
		}
		rows.Close()
	}
}

func BenchmarkQueryBuilding(b *testing.B) {
	db := setupTestDB(nil) // Your helper to init DB

	b.ResetTimer()
	b.ReportAllocs() // This tells Go to track memory allocations

	for i := 0; i < b.N; i++ {
		_ = kunjudb.Table[User](db, "users").
			Select("id", "email").
			Where("id", ">", 10).
			Limit(1).
			ToSql()
	}
}

func setupMockRows(count int) *sql.Rows {
	// 1. Create a mock database connection
	db, mock, err := sqlmock.New()
	if err != nil {
		panic("could not create sqlmock")
	}

	// 2. Define the columns that match your BenchUser struct
	columns := []string{"id", "email", "age"}
	rows := sqlmock.NewRows(columns)

	// 3. Fill with dummy data
	for i := 0; i < count; i++ {
		rows.AddRow(
			i,
			fmt.Sprintf("user-%d@example.com", i),
			20+(i%50),
		)
	}

	// 4. Tell the mock to return these rows for the next query
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	// 5. Execute a query to get the actual *sql.Rows object
	realRows, err := db.Query("SELECT * FROM users")
	if err != nil {
		panic(err)
	}

	return realRows
}
