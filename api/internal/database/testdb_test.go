package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
