package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
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

func createInvite(t *testing.T, ctx context.Context, db *pgxpool.Pool, code string, initialCredits int) {
	t.Helper()

	if _, err := db.Exec(ctx, `
		INSERT INTO invites (code, initial_credits, status)
		VALUES ($1, $2, 'unused')
	`, code, initialCredits); err != nil {
		t.Fatalf("insert invite: %v", err)
	}
}

func insertAuthTestUser(t *testing.T, ctx context.Context, db *pgxpool.Pool, username, password, role, status string, credits int) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text
	`, username, string(hash), role, status, credits).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	return userID
}

func TestRegisterInitializesDailyFreeWallet(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	createInvite(t, ctx, db, "daily-free-register", 6)
	service := Service{DB: db}

	user, err := service.Register(ctx, "daily-free-user", "secret1", "daily-free-register")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.CreditBalance != 6 || user.DailyFreeCreditLimit != 6 || user.DailyFreeCreditBalance != 6 || user.PaidCreditBalance != 0 {
		t.Fatalf("user wallets total=%d free=%d/%d paid=%d, want total=6 free=6/6 paid=0",
			user.CreditBalance, user.DailyFreeCreditBalance, user.DailyFreeCreditLimit, user.PaidCreditBalance)
	}
}

func TestLoginRefreshesStaleDailyFreeWallet(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	userID := insertAuthTestUser(t, ctx, db, "stale-login", "secret1", models.RoleUser, models.UserStatusActive, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 3,
			daily_free_credit_balance = 0,
			paid_credit_balance = 2,
			credit_balance = 2,
			last_daily_free_credit_refreshed_on = CURRENT_DATE - 1
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed stale wallet: %v", err)
	}

	user, err := Service{DB: db}.Login(ctx, "stale-login", "secret1")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if user.CreditBalance != 5 || user.DailyFreeCreditBalance != 3 || user.PaidCreditBalance != 2 {
		t.Fatalf("wallets total=%d free=%d paid=%d, want 5,3,2", user.CreditBalance, user.DailyFreeCreditBalance, user.PaidCreditBalance)
	}
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

func TestValidateNewPasswordRequiresSixCharacters(t *testing.T) {
	for _, tc := range []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "six characters", password: "123456", valid: true},
		{name: "longer password", password: "secure-password", valid: true},
		{name: "five characters", password: "12345", valid: false},
		{name: "empty", password: "", valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNewPassword(tc.password)
			if tc.valid && err != nil {
				t.Fatalf("ValidateNewPassword(%q) error = %v, want nil", tc.password, err)
			}
			if !tc.valid && !errors.Is(err, ErrPasswordTooShort) {
				t.Fatalf("ValidateNewPassword(%q) error = %v, want ErrPasswordTooShort", tc.password, err)
			}
		})
	}
}

func TestChangePasswordRequiresCurrentPassword(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('password-admin', $1, $2, $3, 0)
		RETURNING id::text
	`, string(hash), models.RoleAdmin, models.UserStatusActive).Scan(&userID); err != nil {
		t.Fatalf("insert admin: %v", err)
	}

	if err := service.ChangePassword(ctx, userID, "old-password", "new-password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := service.Login(ctx, "password-admin", "new-password"); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
	if _, err := service.Login(ctx, "password-admin", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login with old password error = %v, want ErrInvalidCredentials", err)
	}
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('wrong-current-admin', $1, $2, $3, 0)
		RETURNING id::text
	`, string(hash), models.RoleAdmin, models.UserStatusActive).Scan(&userID); err != nil {
		t.Fatalf("insert admin: %v", err)
	}

	err = service.ChangePassword(ctx, userID, "bad-password", "new-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("ChangePassword error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := service.Login(ctx, "wrong-current-admin", "old-password"); err != nil {
		t.Fatalf("old password should still work: %v", err)
	}
}

func TestResetPasswordUpdatesTargetPassword(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	hash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ('reset-user', $1, $2, $3, 0)
		RETURNING id::text
	`, string(hash), models.RoleUser, models.UserStatusActive).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if err := service.ResetPassword(ctx, userID, "new-password"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if _, err := service.Login(ctx, "reset-user", "new-password"); err != nil {
		t.Fatalf("login with reset password: %v", err)
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

func TestEnsureAdminConcurrentBootstrapIsIdempotent(t *testing.T) {
	ctx, db := setupAuthTestDB(t)
	service := Service{DB: db}

	const callers = 8
	errs := make(chan error, callers)

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- service.EnsureAdmin(ctx, "root", "root-password")
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("ensure admin concurrent call returned error: %v", err)
		}
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
