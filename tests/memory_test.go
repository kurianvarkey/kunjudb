package tests

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/kurianvarkey/kunjudb"
)

type User struct {
	ID    int    `db:"id"`
	Email string `db:"email"`
	Age   int    `db:"age"`
}

func TestMemoryGrowth(t *testing.T) {
	db := setupTestDB(t)
	for i := 0; i < 100000; i++ {
		// Run complex queries repeatedly
		_ = kunjudb.Table[User](db, "users").Where("id", "=", i).ToSql()

		if i%10000 == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			fmt.Printf("Alloc = %v MiB", m.Alloc/1024/1024)
		}
	}
}
