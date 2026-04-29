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

	var passwordHash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, user.ID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if passwordHash == "alice-password" {
		t.Fatal("password_hash stored plaintext password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("alice-password")); err != nil {
		t.Fatalf("password_hash does not verify with bcrypt: %v", err)
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

func TestEnsureAdminRejectsNormalUserConflict(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("user-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('root', $1, $2, $3, 0)
	`, string(hash), models.RoleUser, models.UserStatusActive)
	if err != nil {
		t.Fatalf("insert normal user: %v", err)
	}

	err = service.EnsureAdmin(ctx, "root", "root-password")
	if !errors.Is(err, ErrAdminConflict) {
		t.Fatalf("ensure admin error = %v, want ErrAdminConflict", err)
	}

	var role string
	if err := db.QueryRow(ctx, `SELECT role FROM users WHERE username = 'root'`).Scan(&role); err != nil {
		t.Fatalf("query role: %v", err)
	}
	if role != models.RoleUser {
		t.Fatalf("role = %q, want unchanged normal user role", role)
	}
}

func TestEnsureAdminRejectsDisabledAdminConflict(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("admin-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('root', $1, $2, $3, 0)
	`, string(hash), models.RoleAdmin, models.UserStatusDisabled)
	if err != nil {
		t.Fatalf("insert disabled admin: %v", err)
	}

	err = service.EnsureAdmin(ctx, "root", "root-password")
	if !errors.Is(err, ErrAdminConflict) {
		t.Fatalf("ensure admin error = %v, want ErrAdminConflict", err)
	}
}

func TestSessionCodecRoundTrip(t *testing.T) {
	codec := NewSessionCodec("test-secret")

	cookieValue, err := codec.Sign("user-123")
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
	if cookieValue == "user-123" {
		t.Fatal("signed cookie value must not be raw user ID")
	}

	userID, ok := codec.Verify(cookieValue)
	if !ok {
		t.Fatal("verify signed cookie returned false")
	}
	if userID != "user-123" {
		t.Fatalf("verified user ID = %q, want user-123", userID)
	}
}

func TestSessionCodecRejectsTamperedCookie(t *testing.T) {
	codec := NewSessionCodec("test-secret")

	cookieValue, err := codec.Sign("user-123")
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}
	tampered := cookieValue + "x"

	if userID, ok := codec.Verify(tampered); ok {
		t.Fatalf("tampered cookie verified as user %q", userID)
	}
}

func TestSessionCodecRejectsWrongSecret(t *testing.T) {
	cookieValue, err := NewSessionCodec("test-secret").Sign("user-123")
	if err != nil {
		t.Fatalf("sign session: %v", err)
	}

	if userID, ok := NewSessionCodec("other-secret").Verify(cookieValue); ok {
		t.Fatalf("cookie verified with wrong secret as user %q", userID)
	}
}
