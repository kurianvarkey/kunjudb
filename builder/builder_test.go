package builder

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/kurianvarkey/kunjudb/dialects"
)

type TestUser struct {
	ID    int    `db:"id"`
	Name  string `db:"name"`
	Email string `db:"email"`
}

type SoftUser struct {
	ID        int        `db:"id"`
	Name      string     `db:"name"`
	DeletedAt *time.Time `db:"deleted_at"`
}

func (s *SoftUser) SetDeletedAt(t *time.Time) {
	s.DeletedAt = t
}

func (s *SoftUser) IsDeleted() bool {
	return s.DeletedAt != nil
}

func TestBuilder_UpdatePlaceholderOffset(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer rawDB.Close()

	b := New[TestUser](rawDB, dialects.PostgreSql{}, "users")

	expectedSQL := `UPDATE users SET age = \$1, name = \$2 WHERE id = \$3`
	mock.ExpectExec(expectedSQL).
		WithArgs(30, "Alice", 10).
		WillReturnResult(sqlmock.NewResult(1, 1))

	_, updateErr := b.Where("id", "=", 10).
		Update(context.Background(), map[string]any{
			"name": "Alice",
			"age":  30,
		})

	if updateErr != nil {
		t.Fatalf("Update failed: %v", updateErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestBuilder_InsertDeterministicSortedColumns(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer rawDB.Close()

	b := New[TestUser](rawDB, dialects.PostgreSql{}, "users")

	expectedSQL := regexp.QuoteMeta("INSERT INTO users (email, name) VALUES ($1, $2)")
	mock.ExpectExec(expectedSQL).
		WithArgs("alice@example.com", "Alice").
		WillReturnResult(sqlmock.NewResult(1, 1))

	_, insertErr := b.Insert(context.Background(), map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
	})

	if insertErr != nil {
		t.Fatalf("Insert failed: %v", insertErr)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %s", err)
	}
}

func TestBuilder_FirstNotFound(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer rawDB.Close()

	b := New[TestUser](rawDB, dialects.PostgreSql{}, "users")

	rows := sqlmock.NewRows([]string{"id", "name", "email"})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM users WHERE id = $1 LIMIT 1")).
		WithArgs(999).
		WillReturnRows(rows)

	user, err := b.Where("id", "=", 999).First(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if user != nil {
		t.Fatalf("expected nil user, got %v", user)
	}
}

func TestBuilder_SoftDeletableInterface(t *testing.T) {
	if !isSoftDeletable[SoftUser]() {
		t.Errorf("expected SoftUser to be soft deletable")
	}
	if isSoftDeletable[TestUser]() {
		t.Errorf("expected TestUser to not be soft deletable")
	}
}
