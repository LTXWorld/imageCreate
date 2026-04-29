package credits

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/models"
)

var (
	ErrInsufficientCredits = errors.New("insufficient credits")
	ErrUserNotFound        = errors.New("user not found")
)

type Service struct {
	DB *pgxpool.Pool
}

func (s Service) Adjust(ctx context.Context, userID string, amount int, reason string, actorUserID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin credit adjustment: %w", err)
	}
	defer tx.Rollback(ctx)

	var balanceAfter int
	err = tx.QueryRow(ctx, `
		UPDATE users
		SET credit_balance = credit_balance + $2,
			updated_at = now()
		WHERE id = $1::uuid
			AND credit_balance + $2 >= 0
		RETURNING credit_balance
	`, userID, amount).Scan(&balanceAfter)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1::uuid)`, userID).Scan(&exists); err != nil {
			return fmt.Errorf("check user exists: %w", err)
		}
		if !exists {
			return ErrUserNotFound
		}
		return ErrInsufficientCredits
	}
	if err != nil {
		return fmt.Errorf("update credit balance: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, type, amount, balance_after, reason, actor_user_id)
		VALUES ($1::uuid, $2, $3, $4, $5, NULLIF($6, '')::uuid)
	`, userID, models.LedgerAdminAdjustment, amount, balanceAfter, reason, actorUserID); err != nil {
		return fmt.Errorf("insert credit ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit credit adjustment: %w", err)
	}
	return nil
}

func (s Service) RefundGeneration(ctx context.Context, tx pgx.Tx, userID string, taskID string, reason string) error {
	var balanceAfter int
	if err := tx.QueryRow(ctx, `
		UPDATE users
		SET credit_balance = credit_balance + 1,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING credit_balance
	`, userID).Scan(&balanceAfter); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf("refund generation credit: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, task_id, type, amount, balance_after, reason)
		VALUES ($1::uuid, $2::uuid, $3, 1, $4, $5)
	`, userID, taskID, models.LedgerGenerationRefund, balanceAfter, reason); err != nil {
		return fmt.Errorf("insert refund ledger: %w", err)
	}

	return nil
}
