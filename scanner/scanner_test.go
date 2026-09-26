package scanner

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type TestUser struct {
	ID        int        `db:"id"`
	Name      string     `db:"name"`
	Email     string     `db:"email"`
	Password  string     `db:"-"` // Must be ignored
	CreatedAt time.Time  `db:"created_at"`
	DeletedAt *time.Time `db:"deleted_at"`
}

func TestMapRaw_BasicAndUnmappedColumn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	now := time.Now().Truncate(time.Second)
	rows := sqlmock.NewRows([]string{"id", "unmapped_col", "name", "email", "Password", "created_at"}).
		AddRow(1, "extra_value_1", "Alice", "alice@example.com", "hacked_password", now).
		AddRow(2, "extra_value_2", "Bob", "bob@example.com", "hacked_password_2", now)

	mock.ExpectQuery("SELECT id, unmapped_col, name, email, Password, created_at FROM users").WillReturnRows(rows)

	sqlRows, err := db.Query("SELECT id, unmapped_col, name, email, Password, created_at FROM users")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	users, err := MapRaw[TestUser](sqlRows)
	if err != nil {
		t.Fatalf("MapRaw failed: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Name != "Alice" || users[0].Email != "alice@example.com" {
		t.Errorf("unexpected user 0 data: %+v", users[0])
	}
	// Password has db:"-" so it must NOT be populated from the query column
	if users[0].Password != "" {
		t.Errorf("expected Password with db:\"-\" to remain empty, got %q", users[0].Password)
	}
	if !users[0].CreatedAt.Equal(now) {
		t.Errorf("expected CreatedAt %v, got %v", now, users[0].CreatedAt)
	}
}

func TestMapRaw_RowsErr(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	expectedErr := errors.New("network stream closed")
	rows := sqlmock.NewRows([]string{"id", "name", "email"}).
		AddRow(1, "Alice", "alice@example.com").
		RowError(0, expectedErr)

	mock.ExpectQuery("SELECT id, name, email FROM users").WillReturnRows(rows)

	sqlRows, err := db.Query("SELECT id, name, email FROM users")
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}

	_, scanErr := MapRaw[TestUser](sqlRows)
	if scanErr == nil {
		t.Fatalf("expected rows error, got nil")
	}
}
