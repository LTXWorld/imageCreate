package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"imagecreate/api/internal/database"
	"imagecreate/api/internal/models"
)

func setupAuthTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	db := database.RequireTestDB(t)

	if err := database.RunMigrations(databaseURL, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return context.Background(), db
}

func TestRegisterConsumesInviteAndGrantsCredits(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	_, err := db.Exec(ctx, `
		INSERT INTO invites (code, initial_credits, status)
		VALUES ($1, $2, 'unused')
	`, "alice-invite", 5)
	if err != nil {
		t.Fatalf("insert invite: %v", err)
	}

	user, err := service.Register(ctx, "alice", "alice-password", "alice-invite")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if user.Username != "alice" || user.Role != models.RoleUser || user.Status != models.UserStatusActive || user.CreditBalance != 5 {
		t.Fatalf("registered user = %+v, want active user alice with 5 credits", user)
	}

	var inviteStatus string
	var usedBy string
	var usedAtSet bool
	if err := db.QueryRow(ctx, `
		SELECT status, used_by::text, used_at IS NOT NULL
		FROM invites
		WHERE code = $1
	`, "alice-invite").Scan(&inviteStatus, &usedBy, &usedAtSet); err != nil {
		t.Fatalf("query invite: %v", err)
	}
	if inviteStatus != "used" || usedBy != user.ID || !usedAtSet {
		t.Fatalf("invite status=%q used_by=%q used_at_set=%v, want used by %q", inviteStatus, usedBy, usedAtSet, user.ID)
	}

	var balance int
	if err := db.QueryRow(ctx, `SELECT credit_balance FROM users WHERE id = $1`, user.ID).Scan(&balance); err != nil {
		t.Fatalf("query user balance: %v", err)
	}
	if balance != 5 {
		t.Fatalf("credit_balance = %d, want 5", balance)
	}

	var ledgerRows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1 AND type = $2 AND amount = 5 AND balance_after = 5
	`, user.ID, models.LedgerInviteGrant).Scan(&ledgerRows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if ledgerRows != 1 {
		t.Fatalf("invite_grant ledger rows = %d, want 1", ledgerRows)
	}
}

func TestRegisterRejectsUsedInvite(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("existing-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	var existingUserID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('existing', $1, $2, $3, 0)
		RETURNING id::text
	`, string(hash), models.RoleUser, models.UserStatusActive).Scan(&existingUserID); err != nil {
		t.Fatalf("insert existing user: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO invites (code, initial_credits, status, used_by, used_at)
		VALUES ($1, 5, 'used', $2, now())
	`, "used-invite", existingUserID)
	if err != nil {
		t.Fatalf("insert used invite: %v", err)
	}

	_, err = service.Register(ctx, "bob", "bob-password", "used-invite")
	if !errors.Is(err, ErrInvalidInvite) {
		t.Fatalf("register error = %v, want ErrInvalidInvite", err)
	}

	var userRows int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userRows); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userRows != 1 {
		t.Fatalf("user rows = %d, want 1", userRows)
	}
}

func TestLoginRejectsDisabledUser(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("secret-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	_, err = db.Exec(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('disabled-alice', $1, $2, $3, 0)
	`, string(hash), models.RoleUser, models.UserStatusDisabled)
	if err != nil {
		t.Fatalf("insert disabled user: %v", err)
	}

	_, err = service.Login(ctx, "disabled-alice", "secret-password")
	if !errors.Is(err, ErrDisabledUser) {
		t.Fatalf("login error = %v, want ErrDisabledUser", err)
	}
}

func TestEnsureAdminCreatesConfiguredAdminOnce(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	if err := service.EnsureAdmin(ctx, "root", "root-password"); err != nil {
		t.Fatalf("ensure admin first call: %v", err)
	}
	if err := service.EnsureAdmin(ctx, "root", "root-password"); err != nil {
		t.Fatalf("ensure admin second call: %v", err)
	}

	var adminRows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM users
		WHERE username = 'root' AND role = $1 AND status = $2
	`, models.RoleAdmin, models.UserStatusActive).Scan(&adminRows); err != nil {
		t.Fatalf("count admin rows: %v", err)
	}
	if adminRows != 1 {
		t.Fatalf("admin rows = %d, want 1", adminRows)
	}
}
