package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"imagecreate/api/internal/credits"
	"imagecreate/api/internal/models"
)

var (
	ErrInvalidInvite      = errors.New("invalid invite")
	ErrDuplicateUsername  = errors.New("duplicate username")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrDisabledUser       = errors.New("disabled user")
	ErrInvalidInput       = errors.New("invalid input")
	ErrAdminConflict      = errors.New("admin username conflict")
	ErrPasswordTooShort   = errors.New("password too short")
	ErrUserNotFound       = errors.New("user not found")
)

const MinPasswordLength = 6

type Service struct {
	DB *pgxpool.Pool
}

type User struct {
	ID                     string `json:"id"`
	Username               string `json:"username"`
	Role                   string `json:"role"`
	Status                 string `json:"status"`
	CreditBalance          int    `json:"credit_balance"`
	DailyFreeCreditLimit   int    `json:"daily_free_credit_limit"`
	DailyFreeCreditBalance int    `json:"daily_free_credit_balance"`
	PaidCreditBalance      int    `json:"paid_credit_balance"`
}

func ValidateNewPassword(password string) error {
	if len(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	return nil
}

func hashPassword(password string) (string, error) {
	if err := ValidateNewPassword(password); err != nil {
		return "", err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(passwordHash), nil
}

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
		&user.DailyFreeCreditLimit,
		&user.DailyFreeCreditBalance,
		&user.PaidCreditBalance,
	)
	return user, err
}

func (s Service) Register(ctx context.Context, username, password, inviteCode string) (User, error) {
	if username == "" || password == "" || inviteCode == "" {
		return User{}, ErrInvalidInput
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var inviteID string
	var initialCredits int
	var inviteStatus string
	if err := tx.QueryRow(ctx, `
		SELECT id::text, initial_credits, status
		FROM invites
		WHERE code = $1
		FOR UPDATE
	`, inviteCode).Scan(&inviteID, &initialCredits, &inviteStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInvalidInvite
		}
		return User{}, err
	}
	if inviteStatus != "unused" {
		return User{}, ErrInvalidInvite
	}

	user, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (
			username,
			password_hash,
			role,
			status,
			credit_balance,
			daily_free_credit_limit,
			daily_free_credit_balance,
			paid_credit_balance,
			last_daily_free_credit_refreshed_on
		)
		VALUES ($1, $2, $3, $4, $5, $5, $5, 0, CURRENT_DATE)
		RETURNING id::text, username, role, status, credit_balance, daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance
	`, username, passwordHash, models.RoleUser, models.UserStatusActive, initialCredits))
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrDuplicateUsername
		}
		return User{}, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE invites
		SET status = 'used', used_by = $1, used_at = now()
		WHERE id = $2
	`, user.ID, inviteID); err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrInvalidInvite
		}
		return User{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, type, amount, balance_after, reason)
		VALUES ($1, $2, $3, $4, $5)
	`, user.ID, models.LedgerInviteGrant, initialCredits, initialCredits, "invite registration grant"); err != nil {
		return User{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}

	return user, nil
}

func (s Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := s.ChangePasswordTx(ctx, tx, userID, currentPassword, newPassword); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s Service) ResetPassword(ctx context.Context, userID, newPassword string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := s.ResetPasswordTx(ctx, tx, userID, newPassword); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s Service) ChangePasswordTx(ctx context.Context, tx pgx.Tx, userID, currentPassword, newPassword string) error {
	if err := ValidateNewPassword(newPassword); err != nil {
		return err
	}

	var passwordHash string
	if err := tx.QueryRow(ctx, `
		SELECT password_hash
		FROM users
		WHERE id = $1::uuid
		FOR UPDATE
	`, userID).Scan(&passwordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	return updatePasswordHash(ctx, tx, userID, passwordHash)
}

func (s Service) ResetPasswordTx(ctx context.Context, tx pgx.Tx, userID, newPassword string) error {
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	return updatePasswordHash(ctx, tx, userID, passwordHash)
}

func updatePasswordHash(ctx context.Context, tx pgx.Tx, userID, passwordHash string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, updated_at = now()
		WHERE id = $1::uuid
	`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s Service) Login(ctx context.Context, username, password string) (User, error) {
	if username == "" || password == "" {
		return User{}, ErrInvalidCredentials
	}

	var user User
	var passwordHash string
	if err := s.DB.QueryRow(ctx, `
		SELECT id::text,
			username,
			password_hash,
			role,
			status,
			credit_balance,
			daily_free_credit_limit,
			daily_free_credit_balance,
			paid_credit_balance
		FROM users
		WHERE username = $1
	`, username).Scan(
		&user.ID,
		&user.Username,
		&passwordHash,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
		&user.DailyFreeCreditLimit,
		&user.DailyFreeCreditBalance,
		&user.PaidCreditBalance,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}
	if user.Status == models.UserStatusDisabled {
		return User{}, ErrDisabledUser
	}

	creditService := credits.Service{DB: s.DB}
	if _, err := creditService.RefreshDailyFreeCredits(ctx, user.ID); err != nil {
		return User{}, err
	}
	return s.userByID(ctx, user.ID)
}

func (s Service) EnsureAdmin(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return ErrInvalidInput
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return err
	}

	_, err = s.DB.Exec(ctx, `
		INSERT INTO users (
			username,
			password_hash,
			role,
			status,
			credit_balance,
			daily_free_credit_limit,
			daily_free_credit_balance,
			paid_credit_balance,
			last_daily_free_credit_refreshed_on
		)
		VALUES ($1, $2, $3, $4, 0, 0, 0, 0, CURRENT_DATE)
		ON CONFLICT (username) DO NOTHING
	`, username, passwordHash, models.RoleAdmin, models.UserStatusActive)
	if err != nil {
		return err
	}

	var role string
	var status string
	if err := s.DB.QueryRow(ctx, `
		SELECT role, status
		FROM users
		WHERE username = $1
	`, username).Scan(&role, &status); err != nil {
		return err
	}
	if role == models.RoleAdmin && status == models.UserStatusActive {
		return nil
	}
	return ErrAdminConflict
}

func (s Service) userByID(ctx context.Context, id string) (User, error) {
	user, err := s.scanUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	if user.Status == models.UserStatusDisabled {
		return User{}, ErrDisabledUser
	}
	if user.Status == models.UserStatusActive {
		creditService := credits.Service{DB: s.DB}
		if _, err := creditService.RefreshDailyFreeCredits(ctx, user.ID); err != nil {
			return User{}, err
		}
		user, err = s.scanUserByID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return User{}, ErrInvalidCredentials
			}
			return User{}, err
		}
		if user.Status == models.UserStatusDisabled {
			return User{}, ErrDisabledUser
		}
	}
	return user, nil
}

func (s Service) scanUserByID(ctx context.Context, id string) (User, error) {
	return scanUser(s.DB.QueryRow(ctx, `
		SELECT id::text, username, role, status, credit_balance, daily_free_credit_limit, daily_free_credit_balance, paid_credit_balance
		FROM users
		WHERE id = $1::uuid
	`, id))
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
