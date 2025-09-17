package testutil

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"assisted-venue-approval/pkg/database"
)

// DBTest provides a real DB connection for integration tests with helpers for isolation.
// It uses DATABASE_URL_TEST if set, otherwise DATABASE_URL. Tests are skipped if missing.
// Keep this small and pragmatic.
type DBTest struct {
	T   *testing.T
	DB  *database.DB
	SQL *sql.DB
}

func NewDBTest(t *testing.T) *DBTest {
	t.Helper()
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		t.Skip("DATABASE_URL_TEST or DATABASE_URL not set; skipping integration tests")
	}
	db, err := database.New(url)
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	return &DBTest{T: t, DB: db, SQL: db.Conn()}
}

func (d *DBTest) Close() {
	_ = d.DB.Close()
}

// Truncate wipes relevant tables. Best effort as schema may evolve.
func (d *DBTest) Truncate() {
	// TODO: implement table cleanup when schema fixtures are available.
	// For now we rely on unique IDs per test and transactional cleanup.
}

// WithTx runs fn inside a transaction and rolls back by default.
func (d *DBTest) WithTx(fn func(tx *sql.Tx)) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		d.T.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	fn(tx)
}
