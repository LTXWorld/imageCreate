package generations

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/database"
	"imagecreate/api/internal/models"
)

func setupGenerationTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	db := database.RequireTestDB(t)

	if err := database.RunMigrations(databaseURL, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return context.Background(), db
}

func testService(db *pgxpool.Pool) Service {
	return Service{
		DB:    db,
		Model: "test-image-model",
		SizePresets: map[string]string{
			"1:1":  "1024x1024",
			"16:9": "1792x1024",
		},
	}
}

func insertGenerationTestUser(t *testing.T, ctx context.Context, db *pgxpool.Pool, username string, credits int) string {
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
	`, username, models.RoleUser, models.UserStatusActive, credits).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	return userID
}

func creditBalance(t *testing.T, ctx context.Context, db *pgxpool.Pool, userID string) int {
	t.Helper()

	var balance int
	if err := db.QueryRow(ctx, `SELECT credit_balance FROM users WHERE id = $1`, userID).Scan(&balance); err != nil {
		t.Fatalf("query credit balance: %v", err)
	}
	return balance
}

func countLedgerRows(t *testing.T, ctx context.Context, db *pgxpool.Pool, userID, taskID, ledgerType string) int {
	t.Helper()

	var rows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1 AND task_id = $2 AND type = $3
	`, userID, taskID, ledgerType).Scan(&rows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	return rows
}

func countUserLedgerRows(t *testing.T, ctx context.Context, db *pgxpool.Pool, userID, ledgerType string) int {
	t.Helper()

	var rows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1 AND type = $2
	`, userID, ledgerType).Scan(&rows); err != nil {
		t.Fatalf("count user ledger rows: %v", err)
	}
	return rows
}

func TestCreateTaskDebitsOneCredit(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "alice", 3)

	task, err := service.CreateTask(ctx, CreateTaskInput{
		UserID: userID,
		Prompt: "draw a quiet mountain lake",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if task.Status != models.TaskQueued {
		t.Fatalf("task status = %q, want %q", task.Status, models.TaskQueued)
	}
	if task.CreatedAt.IsZero() {
		t.Fatal("task created_at is zero")
	}
	if task.CompletedAt.Valid {
		t.Fatalf("task completed_at valid = true, want false for queued task")
	}
	if got := creditBalance(t, ctx, db, userID); got != 2 {
		t.Fatalf("credit_balance = %d, want 2", got)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerDailyFreeGenerationDebit); rows != 1 {
		t.Fatalf("daily_free_generation_debit ledger rows = %d, want 1", rows)
	}
}

func TestCreateTaskDebitsDailyFreeCreditsBeforePaidCredits(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "free-first", 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 2,
			daily_free_credit_balance = 2,
			paid_credit_balance = 4,
			credit_balance = 6
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

	task, err := service.CreateTask(ctx, CreateTaskInput{UserID: userID, Prompt: "a quiet lake", Ratio: "1:1"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	var freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `SELECT daily_free_credit_balance, paid_credit_balance, credit_balance FROM users WHERE id = $1::uuid`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if freeBalance != 1 || paidBalance != 4 || total != 5 {
		t.Fatalf("wallets free=%d paid=%d total=%d, want 1,4,5", freeBalance, paidBalance, total)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerDailyFreeGenerationDebit); rows != 1 {
		t.Fatalf("daily free debit ledger rows = %d, want 1", rows)
	}
}

func TestCreateTaskDebitsPaidCreditsWhenDailyFreeIsEmpty(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "paid-fallback", 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 2,
			daily_free_credit_balance = 0,
			paid_credit_balance = 3,
			credit_balance = 3
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

	task, err := service.CreateTask(ctx, CreateTaskInput{UserID: userID, Prompt: "a bright studio", Ratio: "1:1"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	var freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `SELECT daily_free_credit_balance, paid_credit_balance, credit_balance FROM users WHERE id = $1::uuid`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if freeBalance != 0 || paidBalance != 2 || total != 2 {
		t.Fatalf("wallets free=%d paid=%d total=%d, want 0,2,2", freeBalance, paidBalance, total)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerPaidGenerationDebit); rows != 1 {
		t.Fatalf("paid debit ledger rows = %d, want 1", rows)
	}
}

func TestCreateTaskRefreshesStaleDailyFreeCreditsBeforeDebit(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "refresh-before-debit", 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 2,
			daily_free_credit_balance = 0,
			paid_credit_balance = 0,
			credit_balance = 0,
			last_daily_free_credit_refreshed_on = CURRENT_DATE - 1
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed stale wallets: %v", err)
	}

	task, err := service.CreateTask(ctx, CreateTaskInput{UserID: userID, Prompt: "a fresh sketch", Ratio: "1:1"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	var freeBalance, paidBalance, total int
	if err := db.QueryRow(ctx, `SELECT daily_free_credit_balance, paid_credit_balance, credit_balance FROM users WHERE id = $1::uuid`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if freeBalance != 1 || paidBalance != 0 || total != 1 {
		t.Fatalf("wallets free=%d paid=%d total=%d, want 1,0,1", freeBalance, paidBalance, total)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerDailyFreeGenerationDebit); rows != 1 {
		t.Fatalf("daily free debit ledger rows = %d, want 1", rows)
	}
	if rows := countUserLedgerRows(t, ctx, db, userID, models.LedgerDailyFreeRefresh); rows != 1 {
		t.Fatalf("daily free refresh ledger rows = %d, want 1", rows)
	}
}

func TestCreateTaskRejectsInsufficientCredits(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "bob", 0)

	_, err := service.CreateTask(ctx, CreateTaskInput{
		UserID: userID,
		Prompt: "draw a small red kite",
		Ratio:  "1:1",
	})
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("create task error = %v, want ErrInsufficientCredits", err)
	}

	var taskRows int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM generation_tasks WHERE user_id = $1`, userID).Scan(&taskRows); err != nil {
		t.Fatalf("count task rows: %v", err)
	}
	if taskRows != 0 {
		t.Fatalf("task rows = %d, want 0", taskRows)
	}
}

func TestCreateTaskRejectsSecondActiveTask(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "carol", 3)

	if _, err := db.Exec(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model)
		VALUES ($1, 'existing active task', '1024x1024', $2, 'test-image-model')
	`, userID, models.TaskRunning); err != nil {
		t.Fatalf("insert running task: %v", err)
	}

	_, err := service.CreateTask(ctx, CreateTaskInput{
		UserID: userID,
		Prompt: "draw a second task",
		Ratio:  "1:1",
	})
	if !errors.Is(err, ErrActiveTaskExists) {
		t.Fatalf("create task error = %v, want ErrActiveTaskExists", err)
	}
	if got := creditBalance(t, ctx, db, userID); got != 3 {
		t.Fatalf("credit_balance = %d, want 3", got)
	}
}

func TestListTasksForUserReturnsOnlyRecentTasks(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "recent-history", 3)

	var recentID, oldID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model, created_at)
		VALUES ($1, 'recent task', '1024x1024', $2, 'test-image-model', now() - interval '7 days')
		RETURNING id::text
	`, userID, models.TaskSucceeded).Scan(&recentID); err != nil {
		t.Fatalf("insert recent task: %v", err)
	}
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model, created_at)
		VALUES ($1, 'old task', '1024x1024', $2, 'test-image-model', now() - interval '31 days')
		RETURNING id::text
	`, userID, models.TaskSucceeded).Scan(&oldID); err != nil {
		t.Fatalf("insert old task: %v", err)
	}

	tasks, err := service.ListTasksForUser(ctx, userID)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("task count = %d, want 1; tasks=%+v", len(tasks), tasks)
	}
	if tasks[0].ID != recentID {
		t.Fatalf("task id = %q, want recent id %q; old id was %q", tasks[0].ID, recentID, oldID)
	}
}

func TestFailTaskRefundsCredit(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "dana", 3)

	task, err := service.CreateTask(ctx, CreateTaskInput{
		UserID: userID,
		Prompt: "draw a cedar forest",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := service.MarkFailedAndRefund(ctx, task.ID, "upstream_error", "provider returned an error", 321); err != nil {
		t.Fatalf("mark failed and refund: %v", err)
	}

	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM generation_tasks WHERE id = $1`, task.ID).Scan(&status); err != nil {
		t.Fatalf("query task status: %v", err)
	}
	if status != models.TaskFailed {
		t.Fatalf("task status = %q, want %q", status, models.TaskFailed)
	}
	if got := creditBalance(t, ctx, db, userID); got != 3 {
		t.Fatalf("credit_balance = %d, want 3", got)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerDailyFreeGenerationRefund); rows != 1 {
		t.Fatalf("daily_free_generation_refund ledger rows = %d, want 1", rows)
	}
}

func TestCancelTaskRefundsCreditAndReleasesActiveSlot(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "cancel-active", 2)

	task, err := service.CreateTask(ctx, CreateTaskInput{
		UserID: userID,
		Prompt: "draw the wrong prompt",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	canceled, err := service.CancelTaskForUser(ctx, userID, task.ID)
	if err != nil {
		t.Fatalf("cancel task: %v", err)
	}
	if canceled.Status != models.TaskCanceled {
		t.Fatalf("canceled task status = %q, want %q", canceled.Status, models.TaskCanceled)
	}
	if !canceled.CompletedAt.Valid {
		t.Fatal("canceled task completed_at valid = false, want true")
	}
	if got := creditBalance(t, ctx, db, userID); got != 2 {
		t.Fatalf("credit_balance = %d, want 2", got)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerDailyFreeGenerationRefund); rows != 1 {
		t.Fatalf("daily_free_generation_refund ledger rows = %d, want 1", rows)
	}

	if _, err := service.CreateTask(ctx, CreateTaskInput{UserID: userID, Prompt: "draw the corrected prompt", Ratio: "1:1"}); err != nil {
		t.Fatalf("create task after cancellation: %v", err)
	}
}

func TestCancelTaskRejectsTerminalTask(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "cancel-terminal", 2)

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model)
		VALUES ($1, 'completed task', '1024x1024', $2, 'test-image-model')
		RETURNING id::text
	`, userID, models.TaskSucceeded).Scan(&taskID); err != nil {
		t.Fatalf("insert succeeded task: %v", err)
	}

	_, err := service.CancelTaskForUser(ctx, userID, taskID)
	if !errors.Is(err, ErrTaskNotActive) {
		t.Fatalf("cancel task error = %v, want ErrTaskNotActive", err)
	}
}

func TestCancelTaskRejectsRunningTaskWithoutRefund(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "cancel-running", 2)

	task, err := service.CreateTask(ctx, CreateTaskInput{
		UserID: userID,
		Prompt: "draw a prompt already sent upstream",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE generation_tasks
		SET status = $2,
			started_at = now()
		WHERE id = $1::uuid
	`, task.ID, models.TaskRunning); err != nil {
		t.Fatalf("mark task running: %v", err)
	}

	_, err = service.CancelTaskForUser(ctx, userID, task.ID)
	if !errors.Is(err, ErrTaskAlreadyStarted) {
		t.Fatalf("cancel task error = %v, want ErrTaskAlreadyStarted", err)
	}
	if got := creditBalance(t, ctx, db, userID); got != 1 {
		t.Fatalf("credit_balance = %d, want 1", got)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerDailyFreeGenerationRefund); rows != 0 {
		t.Fatalf("daily_free_generation_refund ledger rows = %d, want 0", rows)
	}
}

func TestSucceedTaskDoesNotRefundCredit(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)
	userID := insertGenerationTestUser(t, ctx, db, "erin", 3)

	task, err := service.CreateTask(ctx, CreateTaskInput{
		UserID: userID,
		Prompt: "draw a glass teapot",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	const imagePath = "generated/erin/task.png"
	if err := service.MarkSucceeded(ctx, task.ID, "req-123", imagePath, 456); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}

	var status, storedImagePath string
	if err := db.QueryRow(ctx, `
		SELECT status, image_path
		FROM generation_tasks
		WHERE id = $1
	`, task.ID).Scan(&status, &storedImagePath); err != nil {
		t.Fatalf("query task: %v", err)
	}
	if status != models.TaskSucceeded {
		t.Fatalf("task status = %q, want %q", status, models.TaskSucceeded)
	}
	if storedImagePath != imagePath {
		t.Fatalf("image_path = %q, want %q", storedImagePath, imagePath)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerDailyFreeGenerationRefund); rows != 0 {
		t.Fatalf("daily_free_generation_refund ledger rows = %d, want 0", rows)
	}
}

func TestDeleteTaskRejectsActiveTask(t *testing.T) {
	ctx, db := setupGenerationTestDB(t)
	service := testService(db)

	for _, status := range []string{models.TaskQueued, models.TaskRunning} {
		userID := insertGenerationTestUser(t, ctx, db, "delete-"+status, 3)

		var taskID string
		if err := db.QueryRow(ctx, `
			INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model)
			VALUES ($1, 'active task', '1024x1024', $2, 'test-image-model')
			RETURNING id::text
		`, userID, status).Scan(&taskID); err != nil {
			t.Fatalf("insert %s task: %v", status, err)
		}

		err := service.DeleteTaskForUser(ctx, userID, taskID)
		if !errors.Is(err, ErrTaskActive) {
			t.Fatalf("delete %s task error = %v, want ErrTaskActive", status, err)
		}

		var deleted bool
		if err := db.QueryRow(ctx, `
			SELECT deleted_at IS NOT NULL
			FROM generation_tasks
			WHERE id = $1
		`, taskID).Scan(&deleted); err != nil {
			t.Fatalf("query %s task deleted_at: %v", status, err)
		}
		if deleted {
			t.Fatalf("%s task was soft-deleted", status)
		}
	}
}
