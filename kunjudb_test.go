package kunjudb

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kurianvarkey/kunjudb/dialects"
)

type User struct {
	ID    int    `db:"id"`
	Email string `db:"email"`
}

func TestFacade_TableAndScanner(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer rawDB.Close()

	db := New(rawDB, dialects.PostgreSql{})

	rows := sqlmock.NewRows([]string{"id", "email"}).
		AddRow(1, "user@example.com")
	mock.ExpectQuery("SELECT id, email FROM users WHERE id = \\$1").
		WithArgs(1).
		WillReturnRows(rows)

	user, err := Table[User](db, "users").
		Select("id", "email").
		Where("id", "=", 1).
		First(context.Background())

	if err != nil {
		t.Fatalf("facade query failed: %v", err)
	}
	if user.Email != "user@example.com" {
		t.Errorf("unexpected user email: %s", user.Email)
	}
}
