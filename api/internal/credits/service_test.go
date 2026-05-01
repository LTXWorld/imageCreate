package credits

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/database"
	"imagecreate/api/internal/models"
)

func setupCreditTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	db := database.RequireTestDB(t)

	if err := database.RunMigrations(databaseURL, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return context.Background(), db
}

func insertCreditTestUser(t *testing.T, ctx context.Context, db *pgxpool.Pool, username, role string, credits int) string {
	t.Helper()

	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ($1, 'hash', $2, $3, $4)
		RETURNING id::text
	`, username, role, models.UserStatusActive, credits).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	return userID
}

func creditTestBalance(t *testing.T, ctx context.Context, db *pgxpool.Pool, userID string) int {
	t.Helper()

	var balance int
	if err := db.QueryRow(ctx, `SELECT credit_balance FROM users WHERE id = $1`, userID).Scan(&balance); err != nil {
		t.Fatalf("query balance: %v", err)
	}
	return balance
}

func TestAdjustIncreasesAndDecreasesBalanceAndWritesLedger(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	service := Service{DB: db}
	userID := insertCreditTestUser(t, ctx, db, "alice", models.RoleUser, 5)
	actorID := insertCreditTestUser(t, ctx, db, "root", models.RoleAdmin, 0)

	if err := service.Adjust(ctx, userID, 3, "bonus credits", actorID); err != nil {
		t.Fatalf("increase adjustment: %v", err)
	}
	if got := creditTestBalance(t, ctx, db, userID); got != 8 {
		t.Fatalf("credit_balance after increase = %d, want 8", got)
	}

	if err := service.Adjust(ctx, userID, -2, "manual correction", actorID); err != nil {
		t.Fatalf("decrease adjustment: %v", err)
	}
	if got := creditTestBalance(t, ctx, db, userID); got != 6 {
		t.Fatalf("credit_balance after decrease = %d, want 6", got)
	}

	var rows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1
			AND actor_user_id = $2
			AND type = $3
			AND ((amount = 3 AND balance_after = 8 AND reason = 'bonus credits')
				OR (amount = -2 AND balance_after = 6 AND reason = 'manual correction'))
	`, userID, actorID, models.LedgerAdminAdjustment).Scan(&rows); err != nil {
		t.Fatalf("count adjustment ledger rows: %v", err)
	}
	if rows != 2 {
		t.Fatalf("admin_adjustment ledger rows = %d, want 2", rows)
	}
}

func TestAdjustRejectsNegativeFinalBalanceAndLeavesStateUnchanged(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	service := Service{DB: db}
	userID := insertCreditTestUser(t, ctx, db, "bob", models.RoleUser, 2)
	actorID := insertCreditTestUser(t, ctx, db, "root", models.RoleAdmin, 0)

	err := service.Adjust(ctx, userID, -3, "too much", actorID)
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("adjust error = %v, want ErrInsufficientCredits", err)
	}
	if got := creditTestBalance(t, ctx, db, userID); got != 2 {
		t.Fatalf("credit_balance = %d, want 2", got)
	}

	var rows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1
	`, userID).Scan(&rows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if rows != 0 {
		t.Fatalf("ledger rows = %d, want 0", rows)
	}
}

func TestRefundGenerationIncrementsBalanceAndWritesTaskLedger(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	service := Service{DB: db}
	userID := insertCreditTestUser(t, ctx, db, "carol", models.RoleUser, 1)

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model)
		VALUES ($1, 'prompt', '1024x1024', $2, 'test-model')
		RETURNING id::text
	`, userID, models.TaskFailed).Scan(&taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	if err := service.RefundGeneration(ctx, tx, userID, taskID, "provider failed"); err != nil {
		t.Fatalf("refund generation: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	if got := creditTestBalance(t, ctx, db, userID); got != 2 {
		t.Fatalf("credit_balance = %d, want 2", got)
	}

	var amount, balanceAfter int
	var reason string
	err = db.QueryRow(ctx, `
		SELECT amount, balance_after, reason
		FROM credit_ledger
		WHERE user_id = $1
			AND task_id = $2
			AND type = $3
	`, userID, taskID, models.LedgerGenerationRefund).Scan(&amount, &balanceAfter, &reason)
	if errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("generation_refund ledger row not found")
	}
	if err != nil {
		t.Fatalf("query refund ledger: %v", err)
	}
	if amount != 1 || balanceAfter != 2 || reason != "provider failed" {
		t.Fatalf("refund ledger amount=%d balance_after=%d reason=%q, want amount=1 balance_after=2 reason=%q", amount, balanceAfter, reason, "provider failed")
	}
}

func TestRefundGenerationDoesNotDoubleCreditTask(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	service := Service{DB: db}
	userID := insertCreditTestUser(t, ctx, db, "dana", models.RoleUser, 1)

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model)
		VALUES ($1, 'prompt', '1024x1024', $2, 'test-model')
		RETURNING id::text
	`, userID, models.TaskFailed).Scan(&taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	for i := 0; i < 2; i++ {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx %d: %v", i, err)
		}

		if err := service.RefundGeneration(ctx, tx, userID, taskID, "provider failed"); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("refund generation %d: %v", i, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit tx %d: %v", i, err)
		}
	}

	if got := creditTestBalance(t, ctx, db, userID); got != 2 {
		t.Fatalf("credit_balance = %d, want 2", got)
	}

	var rows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1
			AND task_id = $2
			AND type = $3
	`, userID, taskID, models.LedgerGenerationRefund).Scan(&rows); err != nil {
		t.Fatalf("count refund rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("generation_refund ledger rows = %d, want 1", rows)
	}
}

func TestRefreshDailyFreeCreditsRestoresFreeBalanceOnlyOnce(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	userID := insertCreditTestUser(t, ctx, db, "refresh-alice", models.RoleUser, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 5,
			daily_free_credit_balance = 1,
			paid_credit_balance = 9,
			credit_balance = 10,
			last_daily_free_credit_refreshed_on = CURRENT_DATE - 1
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed stale wallet: %v", err)
	}

	service := Service{DB: db}
	refreshed, err := service.RefreshDailyFreeCredits(ctx, userID)
	if err != nil {
		t.Fatalf("refresh daily free credits: %v", err)
	}
	if !refreshed {
		t.Fatal("refreshed = false, want true")
	}

	refreshed, err = service.RefreshDailyFreeCredits(ctx, userID)
	if err != nil {
		t.Fatalf("second refresh daily free credits: %v", err)
	}
	if refreshed {
		t.Fatal("second refreshed = true, want false")
	}

	var freeBalance, paidBalance, total, ledgerRows int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallet balances: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1::uuid
			AND type = $2
			AND wallet_type = $3
			AND business_date = CURRENT_DATE
	`, userID, models.LedgerDailyFreeRefresh, models.WalletDailyFree).Scan(&ledgerRows); err != nil {
		t.Fatalf("count refresh ledger: %v", err)
	}
	if freeBalance != 5 || paidBalance != 9 || total != 14 || ledgerRows != 1 {
		t.Fatalf("free=%d paid=%d total=%d ledgerRows=%d, want 5,9,14,1", freeBalance, paidBalance, total, ledgerRows)
	}
}
