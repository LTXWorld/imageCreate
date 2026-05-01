package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsDisposableTestDatabaseName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "test", want: true},
		{name: "test_app", want: true},
		{name: "app_test", want: true},
		{name: "app_test_db", want: true},
		{name: "APP_TEST_DB", want: true},
		{name: "latest", want: false},
		{name: "contest", want: false},
		{name: "attestation", want: false},
		{name: "production", want: false},
		{name: "mytestdb", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isDisposableTestDatabaseName(tt.name); got != tt.want {
				t.Fatalf("isDisposableTestDatabaseName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMigrationsAddDailyFreeCreditWalletColumns(t *testing.T) {
	ctx := context.Background()
	db := RequireTestDB(t)
	databaseURL := os.Getenv("TEST_DATABASE_URL")

	if err := RunMigrations(databaseURL, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('wallet-migration-user', 'hash', 'user', 'active', 4)
		RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatalf("insert migrated user shape: %v", err)
	}

	if _, err := db.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, type, amount, balance_after, reason, wallet_type, business_date)
		VALUES ($1::uuid, 'daily_free_refresh', 0, 4, 'daily refresh', 'daily_free', NULL)
	`, userID); err == nil {
		t.Fatalf("insert daily_free_refresh with NULL business_date succeeded, want constraint failure")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != "credit_ledger_daily_free_refresh_business_date_check" {
			t.Fatalf("insert daily_free_refresh with NULL business_date error = %v, want credit_ledger_daily_free_refresh_business_date_check", err)
		}
	}

	var freeLimit, freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeLimit, &freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallet columns: %v", err)
	}
	if freeLimit != 0 || freeBalance != 0 || paidBalance != 0 || total != 4 {
		t.Fatalf("new user defaults freeLimit=%d freeBalance=%d paidBalance=%d total=%d, want 0,0,0,4", freeLimit, freeBalance, paidBalance, total)
	}
}
