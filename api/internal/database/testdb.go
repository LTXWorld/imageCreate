package database

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const truncateAppTablesSQL = "TRUNCATE audit_logs, credit_ledger, generation_tasks, invites, users RESTART IDENTITY CASCADE"

func RequireTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	requireDisposableDatabase(t, databaseURL)

	pool, err := Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}

	t.Cleanup(pool.Close)
	truncateAppTables(t, pool)

	return pool
}

func requireDisposableDatabase(t *testing.T, databaseURL string) {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}

	databaseName := strings.ToLower(cfg.ConnConfig.Database)
	if !isDisposableTestDatabaseName(databaseName) {
		t.Fatalf("refusing to use TEST_DATABASE_URL database %q: database name must be exactly \"test\", start with \"test_\", end in \"_test\", or contain \"_test_\"", cfg.ConnConfig.Database)
	}
}

func isDisposableTestDatabaseName(name string) bool {
	name = strings.ToLower(name)
	return name == "test" ||
		strings.HasPrefix(name, "test_") ||
		strings.HasSuffix(name, "_test") ||
		strings.Contains(name, "_test_")
}

func truncateAppTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(context.Background(), truncateAppTablesSQL); err != nil {
		var pgErr *pgconn.PgError
		// Migration tests may call RequireTestDB before app tables exist.
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			return
		}
		t.Fatalf("truncate test database tables: %v", err)
	}
}
