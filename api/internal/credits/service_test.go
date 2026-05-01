package credits

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/database"
	"imagecreate/api/internal/models"
)

func setupCreditTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	lockTestDatabase(t, databaseURL)
	db := database.RequireTestDB(t)

	if err := database.RunMigrations(databaseURL, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return context.Background(), db
}

func lockTestDatabase(t *testing.T, databaseURL string) {
	t.Helper()
	if databaseURL == "" {
		return
	}

	ctx := context.Background()
	lockPool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database lock: %v", err)
	}
	if _, err := lockPool.Exec(ctx, `SELECT pg_advisory_lock(20260501)`); err != nil {
		lockPool.Close()
		t.Fatalf("lock test database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = lockPool.Exec(context.Background(), `SELECT pg_advisory_unlock(20260501)`)
		lockPool.Close()
	})
}

func insertCreditTestUser(t *testing.T, ctx context.Context, db *pgxpool.Pool, username, role string, credits int) string {
	t.Helper()

	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (
			username,
			password_hash,
			role,
			status,
			credit_balance,
			daily_free_credit_limit,
			daily_free_credit_balance
		)
		VALUES ($1, 'hash', $2, $3, $4, $4, $4)
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

func waitForBlockedApplication(t *testing.T, ctx context.Context, db *pgxpool.Pool, applicationName string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE application_name = $1
					AND wait_event_type = 'Lock'
			)
		`, applicationName).Scan(&blocked); err != nil {
			t.Fatalf("check blocked refresh session: %v", err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("refresh session %q did not block waiting for row lock", applicationName)
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
	if _, err := db.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, -1, 1, 'generation task created')
	`, userID, taskID, models.LedgerDailyFreeGenerationDebit, models.WalletDailyFree); err != nil {
		t.Fatalf("insert debit ledger: %v", err)
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
			AND wallet_type = $4
	`, userID, taskID, models.LedgerDailyFreeGenerationRefund, models.WalletDailyFree).Scan(&amount, &balanceAfter, &reason)
	if errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("daily_free_generation_refund ledger row not found")
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
	if _, err := db.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, -1, 1, 'generation task created')
	`, userID, taskID, models.LedgerDailyFreeGenerationDebit, models.WalletDailyFree); err != nil {
		t.Fatalf("insert debit ledger: %v", err)
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
	`, userID, taskID, models.LedgerDailyFreeGenerationRefund).Scan(&rows); err != nil {
		t.Fatalf("count refund rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("daily_free_generation_refund ledger rows = %d, want 1", rows)
	}
}

func TestRefundGenerationReturnsCreditToOriginalPaidWallet(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	service := Service{DB: db}
	userID := insertCreditTestUser(t, ctx, db, "paid-refund", models.RoleUser, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 0,
			daily_free_credit_balance = 0,
			paid_credit_balance = 1,
			credit_balance = 1
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed paid wallet: %v", err)
	}

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model)
		VALUES ($1, 'prompt', '1024x1024', $2, 'test-model')
		RETURNING id::text
	`, userID, models.TaskFailed).Scan(&taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
		VALUES ($1::uuid, $2::uuid, $3, $4, -1, 0, 'generation task created')
	`, userID, taskID, models.LedgerPaidGenerationDebit, models.WalletPaid); err != nil {
		t.Fatalf("insert paid debit ledger: %v", err)
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

	var freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if freeBalance != 0 || paidBalance != 2 || total != 2 {
		t.Fatalf("wallets free=%d paid=%d total=%d, want 0,2,2", freeBalance, paidBalance, total)
	}

	var rows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1::uuid
			AND task_id = $2::uuid
			AND type = $3
			AND wallet_type = $4
	`, userID, taskID, models.LedgerPaidGenerationRefund, models.WalletPaid).Scan(&rows); err != nil {
		t.Fatalf("count paid refund rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("paid refund ledger rows = %d, want 1", rows)
	}
}

func TestRefundGenerationIgnoresWrongWalletRefundLedger(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	service := Service{DB: db}
	userID := insertCreditTestUser(t, ctx, db, "wrong-wallet-refund", models.RoleUser, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 0,
			daily_free_credit_balance = 0,
			paid_credit_balance = 1,
			credit_balance = 1
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed paid wallet: %v", err)
	}

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model)
		VALUES ($1, 'prompt', '1024x1024', $2, 'test-model')
		RETURNING id::text
	`, userID, models.TaskFailed).Scan(&taskID); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
		VALUES
			($1::uuid, $2::uuid, $3, $4, -1, 0, 'generation task created'),
			($1::uuid, $2::uuid, $5, $6, 1, 1, 'wrong wallet refund')
	`, userID, taskID, models.LedgerPaidGenerationDebit, models.WalletPaid, models.LedgerDailyFreeGenerationRefund, models.WalletDailyFree); err != nil {
		t.Fatalf("insert ledger rows: %v", err)
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

	var freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if freeBalance != 0 || paidBalance != 2 || total != 2 {
		t.Fatalf("wallets free=%d paid=%d total=%d, want 0,2,2", freeBalance, paidBalance, total)
	}

	var rows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1::uuid
			AND task_id = $2::uuid
			AND type = $3
			AND wallet_type = $4
	`, userID, taskID, models.LedgerPaidGenerationRefund, models.WalletPaid).Scan(&rows); err != nil {
		t.Fatalf("count paid refund rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("paid refund ledger rows = %d, want 1", rows)
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

	var freeBalance, paidBalance, total, ledgerRows, ledgerAmount int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallet balances: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*), COALESCE(MAX(amount), 0)
		FROM credit_ledger
		WHERE user_id = $1::uuid
			AND type = $2
			AND wallet_type = $3
			AND business_date = CURRENT_DATE
	`, userID, models.LedgerDailyFreeRefresh, models.WalletDailyFree).Scan(&ledgerRows, &ledgerAmount); err != nil {
		t.Fatalf("count refresh ledger: %v", err)
	}
	if freeBalance != 5 || paidBalance != 9 || total != 14 || ledgerRows != 1 || ledgerAmount != 4 {
		t.Fatalf("free=%d paid=%d total=%d ledgerRows=%d ledgerAmount=%d, want 5,9,14,1,4", freeBalance, paidBalance, total, ledgerRows, ledgerAmount)
	}
}

func TestRefreshDailyFreeCreditsDoesNotOverwriteConcurrentRefreshAndDebit(t *testing.T) {
	ctx, db := setupCreditTestDB(t)
	userID := insertCreditTestUser(t, ctx, db, "refresh-concurrent", models.RoleUser, 0)
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

	blockerTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocker tx: %v", err)
	}
	defer blockerTx.Rollback(ctx)
	if _, err := blockerTx.Exec(ctx, `
		SELECT 1
		FROM users
		WHERE id = $1::uuid
		FOR UPDATE
	`, userID); err != nil {
		t.Fatalf("lock user row: %v", err)
	}

	waiterTx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin waiter tx: %v", err)
	}
	applicationName := "credit-refresh-waiter-" + userID
	if _, err := waiterTx.Exec(ctx, `SELECT set_config('application_name', $1, true)`, applicationName); err != nil {
		_ = waiterTx.Rollback(ctx)
		t.Fatalf("set waiter application name: %v", err)
	}

	service := Service{DB: db}
	type refreshResult struct {
		refreshed bool
		err       error
	}
	resultCh := make(chan refreshResult, 1)
	go func() {
		refreshed, err := service.RefreshDailyFreeCreditsTx(ctx, waiterTx, userID)
		if err != nil {
			_ = waiterTx.Rollback(ctx)
			resultCh <- refreshResult{err: err}
			return
		}
		if err := waiterTx.Commit(ctx); err != nil {
			resultCh <- refreshResult{err: err}
			return
		}
		resultCh <- refreshResult{refreshed: refreshed}
	}()

	waitForBlockedApplication(t, ctx, db, applicationName)

	if _, err := blockerTx.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_balance = 4,
			paid_credit_balance = 9,
			credit_balance = 13,
			last_daily_free_credit_refreshed_on = CURRENT_DATE,
			updated_at = now()
		WHERE id = $1::uuid
	`, userID); err != nil {
		t.Fatalf("simulate concurrent refresh and debit: %v", err)
	}
	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatalf("commit blocker tx: %v", err)
	}

	var result refreshResult
	select {
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh transaction did not finish after blocker committed")
	}
	if result.err != nil {
		t.Fatalf("refresh daily free credits: %v", result.err)
	}
	if result.refreshed {
		t.Fatal("refreshed = true after concurrent refresh, want false")
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
	if freeBalance != 4 || paidBalance != 9 || total != 13 || ledgerRows != 0 {
		t.Fatalf("free=%d paid=%d total=%d ledgerRows=%d, want 4,9,13,0", freeBalance, paidBalance, total, ledgerRows)
	}
}
