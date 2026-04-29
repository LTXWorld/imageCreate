package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"imagecreate/api/internal/models"
)

var (
	ErrInvalidInvite      = errors.New("invalid invite")
	ErrDuplicateUsername  = errors.New("duplicate username")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrDisabledUser       = errors.New("disabled user")
	ErrInvalidInput       = errors.New("invalid input")
)

type Service struct {
	DB *pgxpool.Pool
}

type User struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	CreditBalance int    `json:"credit_balance"`
}

func (s Service) Register(ctx context.Context, username, password, inviteCode string) (User, error) {
	if username == "" || password == "" || inviteCode == "" {
		return User{}, ErrInvalidInput
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
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

	var user User
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, username, role, status, credit_balance
	`, username, string(passwordHash), models.RoleUser, models.UserStatusActive, initialCredits).Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
	); err != nil {
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

func (s Service) Login(ctx context.Context, username, password string) (User, error) {
	if username == "" || password == "" {
		return User{}, ErrInvalidCredentials
	}

	var user User
	var passwordHash string
	if err := s.DB.QueryRow(ctx, `
		SELECT id::text, username, password_hash, role, status, credit_balance
		FROM users
		WHERE username = $1
	`, username).Scan(
		&user.ID,
		&user.Username,
		&passwordHash,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
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

	return user, nil
}

func (s Service) EnsureAdmin(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return ErrInvalidInput
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = s.DB.Exec(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ($1, $2, $3, $4, 0)
		ON CONFLICT (username) DO NOTHING
	`, username, string(passwordHash), models.RoleAdmin, models.UserStatusActive)
	return err
}

func (s Service) userByID(ctx context.Context, id string) (User, error) {
	var user User
	if err := s.DB.QueryRow(ctx, `
		SELECT id::text, username, role, status, credit_balance
		FROM users
		WHERE id = $1
	`, id).Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	if user.Status == models.UserStatusDisabled {
		return User{}, ErrDisabledUser
	}
	return user, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
