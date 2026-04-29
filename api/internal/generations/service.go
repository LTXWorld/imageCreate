package generations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/credits"
	"imagecreate/api/internal/models"
)

var (
	ErrInvalidPrompt       = errors.New("invalid prompt")
	ErrUnsupportedRatio    = errors.New("unsupported ratio")
	ErrInsufficientCredits = credits.ErrInsufficientCredits
	ErrActiveTaskExists    = errors.New("active task exists")
	ErrNotFound            = errors.New("not found")
	ErrDisabledUser        = errors.New("disabled user")
	ErrTaskNotActive       = errors.New("task is not active")
	ErrTaskActive          = errors.New("task is active")
)

type Service struct {
	DB          *pgxpool.Pool
	Model       string
	SizePresets map[string]string
}

type CreateTaskInput struct {
	UserID string
	Prompt string
	Ratio  string
}

type Task struct {
	ID           string
	UserID       string
	Prompt       string
	Size         string
	Status       string
	ImagePath    string
	ErrorCode    string
	ErrorMessage string
}

func (s Service) CreateTask(ctx context.Context, input CreateTaskInput) (Task, error) {
	prompt, err := validatePrompt(input.Prompt)
	if err != nil {
		return Task{}, err
	}

	size, ok := s.SizePresets[input.Ratio]
	if !ok || size == "" {
		return Task{}, ErrUnsupportedRatio
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("begin create task: %w", err)
	}
	defer tx.Rollback(ctx)

	balanceAfter, err := debitGenerationCredit(ctx, tx, input.UserID)
	if err != nil {
		return Task{}, err
	}

	task, err := insertTask(ctx, tx, input.UserID, prompt, size, s.Model)
	if err != nil {
		return Task{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, task_id, type, amount, balance_after, reason)
		VALUES ($1::uuid, $2::uuid, $3, -1, $4, $5)
	`, input.UserID, task.ID, models.LedgerGenerationDebit, balanceAfter, "generation task created"); err != nil {
		return Task{}, fmt.Errorf("insert debit ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Task{}, fmt.Errorf("commit create task: %w", err)
	}
	return task, nil
}

func (s Service) GetTaskForUser(ctx context.Context, userID, taskID string) (Task, error) {
	task, err := scanTask(s.DB.QueryRow(ctx, taskSelectSQL+`
		WHERE user_id = $1::uuid
			AND id = $2::uuid
			AND deleted_at IS NULL
	`, userID, taskID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("get task: %w", err)
	}
	return task, nil
}

func (s Service) ListTasksForUser(ctx context.Context, userID string) ([]Task, error) {
	rows, err := s.DB.Query(ctx, taskSelectSQL+`
		WHERE user_id = $1::uuid
			AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}
	return tasks, nil
}

func (s Service) DeleteTaskForUser(ctx context.Context, userID, taskID string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin delete task: %w", err)
	}
	defer tx.Rollback(ctx)

	var status string
	var deleted bool
	err = tx.QueryRow(ctx, `
		SELECT status, deleted_at IS NOT NULL
		FROM generation_tasks
		WHERE user_id = $1::uuid
			AND id = $2::uuid
		FOR UPDATE
	`, userID, taskID).Scan(&status, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock task for delete: %w", err)
	}
	if deleted {
		return ErrNotFound
	}
	if isActiveTaskStatus(status) {
		return ErrTaskActive
	}

	if _, err := tx.Exec(ctx, `
		UPDATE generation_tasks
		SET deleted_at = now()
		WHERE user_id = $1::uuid
			AND id = $2::uuid
	`, userID, taskID); err != nil {
		return fmt.Errorf("delete task: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete task: %w", err)
	}
	return nil
}

func (s Service) MarkSucceeded(ctx context.Context, taskID, requestID, imagePath string, latencyMS int) error {
	tag, err := s.DB.Exec(ctx, `
		UPDATE generation_tasks
		SET status = $2,
			upstream_request_id = $3,
			image_path = $4,
			latency_ms = $5,
			completed_at = now()
		WHERE id = $1::uuid
			AND status IN ($6, $7)
			AND deleted_at IS NULL
	`, taskID, models.TaskSucceeded, requestID, imagePath, latencyMS, models.TaskQueued, models.TaskRunning)
	if err != nil {
		return fmt.Errorf("mark task succeeded: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return s.inactiveTaskError(ctx, taskID)
	}
	return nil
}

func (s Service) MarkFailedAndRefund(ctx context.Context, taskID, code, message string, latencyMS int) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin fail task: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID, status string
	err = tx.QueryRow(ctx, `
		SELECT user_id::text, status
		FROM generation_tasks
		WHERE id = $1::uuid
			AND deleted_at IS NULL
		FOR UPDATE
	`, taskID).Scan(&userID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock task: %w", err)
	}
	if !isActiveTaskStatus(status) {
		return ErrTaskNotActive
	}

	if _, err := tx.Exec(ctx, `
		UPDATE generation_tasks
		SET status = $2,
			error_code = $3,
			error_message = $4,
			latency_ms = $5,
			completed_at = now()
		WHERE id = $1::uuid
	`, taskID, models.TaskFailed, code, message, latencyMS); err != nil {
		return fmt.Errorf("mark task failed: %w", err)
	}

	creditService := credits.Service{DB: s.DB}
	if err := creditService.RefundGeneration(ctx, tx, userID, taskID, "generation task failed"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit fail task: %w", err)
	}
	return nil
}

const taskSelectSQL = `
	SELECT id::text,
		user_id::text,
		prompt,
		size,
		status,
		COALESCE(image_path, ''),
		COALESCE(error_code, ''),
		COALESCE(error_message, '')
	FROM generation_tasks
`

type taskScanner interface {
	Scan(dest ...any) error
}

func scanTask(scanner taskScanner) (Task, error) {
	var task Task
	err := scanner.Scan(
		&task.ID,
		&task.UserID,
		&task.Prompt,
		&task.Size,
		&task.Status,
		&task.ImagePath,
		&task.ErrorCode,
		&task.ErrorMessage,
	)
	if err != nil {
		return Task{}, err
	}
	return task, nil
}

func validatePrompt(prompt string) (string, error) {
	trimmed := strings.TrimSpace(prompt)
	runeCount := utf8.RuneCountInString(trimmed)
	if runeCount < 1 || runeCount > 2000 {
		return "", ErrInvalidPrompt
	}
	return trimmed, nil
}

func debitGenerationCredit(ctx context.Context, tx pgx.Tx, userID string) (int, error) {
	var balanceAfter int
	err := tx.QueryRow(ctx, `
		UPDATE users
		SET credit_balance = credit_balance - 1,
			updated_at = now()
		WHERE id = $1::uuid
			AND status = $2
			AND credit_balance >= 1
		RETURNING credit_balance
	`, userID, models.UserStatusActive).Scan(&balanceAfter)
	if err == nil {
		return balanceAfter, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("debit generation credit: %w", err)
	}

	var status string
	var balance int
	err = tx.QueryRow(ctx, `SELECT status, credit_balance FROM users WHERE id = $1::uuid`, userID).Scan(&status, &balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("inspect user credit state: %w", err)
	}
	if status != models.UserStatusActive {
		return 0, ErrDisabledUser
	}
	return 0, ErrInsufficientCredits
}

func insertTask(ctx context.Context, tx pgx.Tx, userID, prompt, size, model string) (Task, error) {
	var task Task
	err := tx.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model)
		VALUES ($1::uuid, $2, $3, $4, $5)
		RETURNING id::text,
			user_id::text,
			prompt,
			size,
			status,
			COALESCE(image_path, ''),
			COALESCE(error_code, ''),
			COALESCE(error_message, '')
	`, userID, prompt, size, models.TaskQueued, model).Scan(
		&task.ID,
		&task.UserID,
		&task.Prompt,
		&task.Size,
		&task.Status,
		&task.ImagePath,
		&task.ErrorCode,
		&task.ErrorMessage,
	)
	if err != nil {
		if isActiveTaskUniqueViolation(err) {
			return Task{}, ErrActiveTaskExists
		}
		return Task{}, fmt.Errorf("insert task: %w", err)
	}
	return task, nil
}

func isActiveTaskUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "generation_tasks_one_active_per_user"
}

func isActiveTaskStatus(status string) bool {
	return status == models.TaskQueued || status == models.TaskRunning
}

func (s Service) inactiveTaskError(ctx context.Context, taskID string) error {
	var status string
	var deleted bool
	err := s.DB.QueryRow(ctx, `
		SELECT status, deleted_at IS NOT NULL
		FROM generation_tasks
		WHERE id = $1::uuid
	`, taskID).Scan(&status, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("inspect task status: %w", err)
	}
	if deleted {
		return ErrNotFound
	}
	return ErrTaskNotActive
}
