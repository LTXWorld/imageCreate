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
		INSERT INTO users (username, password_hash, role, status, credit_balance)
		VALUES ($1, 'hash', $2, $3, $4)
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
	if got := creditBalance(t, ctx, db, userID); got != 2 {
		t.Fatalf("credit_balance = %d, want 2", got)
	}
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerGenerationDebit); rows != 1 {
		t.Fatalf("generation_debit ledger rows = %d, want 1", rows)
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
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerGenerationRefund); rows != 1 {
		t.Fatalf("generation_refund ledger rows = %d, want 1", rows)
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
	if rows := countLedgerRows(t, ctx, db, userID, task.ID, models.LedgerGenerationRefund); rows != 0 {
		t.Fatalf("generation_refund ledger rows = %d, want 0", rows)
	}
}
