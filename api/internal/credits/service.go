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
	ErrTaskNotFound        = errors.New("task not found")
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
		SET paid_credit_balance = paid_credit_balance + $2,
			credit_balance = daily_free_credit_balance + paid_credit_balance + $2,
			updated_at = now()
		WHERE id = $1::uuid
			AND paid_credit_balance + $2 >= 0
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
		INSERT INTO credit_ledger (user_id, type, wallet_type, amount, balance_after, reason, actor_user_id)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, NULLIF($7, '')::uuid)
	`, userID, models.LedgerPaidAdminAdjustment, models.WalletPaid, amount, balanceAfter, reason, actorUserID); err != nil {
		return fmt.Errorf("insert credit ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit credit adjustment: %w", err)
	}
	return nil
}

func (s Service) RefundGeneration(ctx context.Context, tx pgx.Tx, userID string, taskID string, reason string) error {
	var lockedTaskID string
	err := tx.QueryRow(ctx, `
		SELECT id::text
		FROM generation_tasks
		WHERE id = $1::uuid
		FOR UPDATE
	`, taskID).Scan(&lockedTaskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTaskNotFound
	}
	if err != nil {
		return fmt.Errorf("lock generation task for refund: %w", err)
	}

	var debitType, walletType string
	err = tx.QueryRow(ctx, `
		SELECT type, COALESCE(wallet_type, '')
		FROM credit_ledger
		WHERE user_id = $1::uuid
			AND task_id = $2::uuid
			AND type IN ($3, $4, $5)
		ORDER BY CASE WHEN type = $5 THEN 1 ELSE 0 END, created_at DESC
		LIMIT 1
	`, userID, taskID, models.LedgerDailyFreeGenerationDebit, models.LedgerPaidGenerationDebit, models.LedgerGenerationDebit).Scan(&debitType, &walletType)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("original generation debit ledger not found")
	}
	if err != nil {
		return fmt.Errorf("find original generation debit ledger: %w", err)
	}

	var refundType string
	isLegacyDebit := debitType == models.LedgerGenerationDebit
	switch {
	case debitType == models.LedgerDailyFreeGenerationDebit && walletType == models.WalletDailyFree:
		refundType = models.LedgerDailyFreeGenerationRefund
	case debitType == models.LedgerPaidGenerationDebit && walletType == models.WalletPaid:
		refundType = models.LedgerPaidGenerationRefund
	case isLegacyDebit:
		walletType = models.WalletDailyFree
		refundType = models.LedgerDailyFreeGenerationRefund
	default:
		return fmt.Errorf("unsupported generation debit wallet: type=%s wallet_type=%s", debitType, walletType)
	}

	var alreadyRefunded bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM credit_ledger
			WHERE task_id = $1::uuid
				AND (
					(type = $2 AND wallet_type = $3)
					OR ($4 AND type = $5)
				)
		)
	`, taskID, refundType, walletType, isLegacyDebit, models.LedgerGenerationRefund).Scan(&alreadyRefunded); err != nil {
		return fmt.Errorf("check existing generation refund: %w", err)
	}
	if alreadyRefunded {
		return nil
	}

	var balanceAfter int
	switch {
	case refundType == models.LedgerDailyFreeGenerationRefund && walletType == models.WalletDailyFree:
		if err := tx.QueryRow(ctx, `
			UPDATE users
			SET daily_free_credit_balance = daily_free_credit_balance + 1,
				credit_balance = daily_free_credit_balance + 1 + paid_credit_balance,
				updated_at = now()
			WHERE id = $1::uuid
			RETURNING credit_balance
		`, userID).Scan(&balanceAfter); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrUserNotFound
			}
			return fmt.Errorf("refund daily free generation credit: %w", err)
		}
	case refundType == models.LedgerPaidGenerationRefund && walletType == models.WalletPaid:
		if err := tx.QueryRow(ctx, `
			UPDATE users
			SET paid_credit_balance = paid_credit_balance + 1,
				credit_balance = daily_free_credit_balance + paid_credit_balance + 1,
				updated_at = now()
			WHERE id = $1::uuid
			RETURNING credit_balance
		`, userID).Scan(&balanceAfter); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrUserNotFound
			}
			return fmt.Errorf("refund paid generation credit: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, 1, $5, $6)
	`, userID, taskID, refundType, walletType, balanceAfter, reason); err != nil {
		return fmt.Errorf("insert refund ledger: %w", err)
	}

	return nil
}

func (s Service) RefreshDailyFreeCredits(ctx context.Context, userID string) (bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin daily free refresh: %w", err)
	}
	defer tx.Rollback(ctx)

	refreshed, err := s.RefreshDailyFreeCreditsTx(ctx, tx, userID)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit daily free refresh: %w", err)
	}
	return refreshed, nil
}

func (s Service) RefreshDailyFreeCreditsTx(ctx context.Context, tx pgx.Tx, userID string) (bool, error) {
	var freeLimit, oldFreeBalance, paidBalance int
	var stale bool
	err := tx.QueryRow(ctx, `
		SELECT daily_free_credit_limit,
			daily_free_credit_balance,
			paid_credit_balance,
			COALESCE(last_daily_free_credit_refreshed_on < CURRENT_DATE, false)
		FROM users
		WHERE id = $1::uuid
			AND status = $2
		FOR UPDATE
	`, userID, models.UserStatusActive).Scan(&freeLimit, &oldFreeBalance, &paidBalance, &stale)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock user for daily free refresh: %w", err)
	}
	if !stale {
		return false, nil
	}

	refreshAmount := freeLimit - oldFreeBalance
	totalAfter := freeLimit + paidBalance
	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_balance = $2,
			credit_balance = $3,
			last_daily_free_credit_refreshed_on = CURRENT_DATE,
			updated_at = now()
		WHERE id = $1::uuid
	`, userID, freeLimit, totalAfter); err != nil {
		return false, fmt.Errorf("refresh daily free credits: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, type, wallet_type, amount, balance_after, reason, business_date)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, CURRENT_DATE)
		ON CONFLICT DO NOTHING
	`, userID, models.LedgerDailyFreeRefresh, models.WalletDailyFree, refreshAmount, totalAfter, "daily free credits refreshed"); err != nil {
		return false, fmt.Errorf("insert daily free refresh ledger: %w", err)
	}
	return true, nil
}
