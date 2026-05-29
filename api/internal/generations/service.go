package generations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/credits"
	"imagecreate/api/internal/models"
)

var (
	ErrInvalidPrompt       = errors.New("invalid prompt")
	ErrPromptTooLong       = errors.New("prompt too long")
	ErrUnsupportedRatio    = errors.New("unsupported ratio")
	ErrInsufficientCredits = credits.ErrInsufficientCredits
	ErrActiveTaskExists    = errors.New("active task exists")
	ErrNotFound            = errors.New("not found")
	ErrDisabledUser        = errors.New("disabled user")
	ErrTaskNotActive       = errors.New("task is not active")
	ErrTaskActive          = errors.New("task is active")
	ErrTaskAlreadyStarted  = errors.New("task already started")
	ErrInvalidTitle        = errors.New("invalid title")
	ErrTitleTooLong        = errors.New("title too long")
)

type Service struct {
	DB          *pgxpool.Pool
	Model       string
	SizePresets map[string]string
}

type CreateTaskInput struct {
	UserID             string
	Prompt             string
	Ratio              string
	ReferenceImagePath string
}

type Task struct {
	ID                 string
	UserID             string
	Prompt             string
	Size               string
	Status             string
	ImagePath          string
	ReferenceImagePath string
	ErrorCode          string
	ErrorMessage       string
	IsFavorite         bool
	Title              string
	CreatedAt          time.Time
	CompletedAt        sql.NullTime
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

	creditService := credits.Service{DB: s.DB}
	if _, err := creditService.RefreshDailyFreeCreditsTx(ctx, tx, input.UserID); err != nil {
		return Task{}, err
	}

	creditCost := generationCreditCost(input.ReferenceImagePath)
	debits, err := debitGenerationCredit(ctx, tx, input.UserID, creditCost)
	if err != nil {
		return Task{}, err
	}

	task, err := insertTask(ctx, tx, input.UserID, prompt, size, s.Model, input.ReferenceImagePath)
	if err != nil {
		return Task{}, err
	}

	for _, debit := range debits {
		if _, err := tx.Exec(ctx, `
			INSERT INTO credit_ledger (user_id, task_id, type, wallet_type, amount, balance_after, reason)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
		`, input.UserID, task.ID, debit.ledgerType, debit.walletType, -debit.amount, debit.balanceAfter, "generation task created"); err != nil {
			return Task{}, fmt.Errorf("insert debit ledger: %w", err)
		}
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
			AND created_at >= now() - interval '30 days'
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

func (s Service) UpdateTaskFavoriteForUser(ctx context.Context, userID, taskID string, favorite bool) (Task, error) {
	task, err := scanTask(s.DB.QueryRow(ctx, `
		UPDATE generation_tasks
		SET is_favorite = $3
		WHERE user_id = $1::uuid
			AND id = $2::uuid
			AND deleted_at IS NULL
		RETURNING id::text,
			user_id::text,
			prompt,
			size,
			status,
			COALESCE(image_path, ''),
			COALESCE(reference_image_path, ''),
			COALESCE(error_code, ''),
			COALESCE(error_message, ''),
			is_favorite,
			COALESCE(title, ''),
			created_at,
			completed_at
	`, userID, taskID, favorite))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("update task favorite: %w", err)
	}
	return task, nil
}

func (s Service) UpdateTaskTitleForUser(ctx context.Context, userID, taskID, title string) (Task, error) {
	normalizedTitle, err := validateTitle(title)
	if err != nil {
		return Task{}, err
	}

	task, err := scanTask(s.DB.QueryRow(ctx, `
		UPDATE generation_tasks
		SET title = NULLIF($3, '')
		WHERE user_id = $1::uuid
			AND id = $2::uuid
			AND deleted_at IS NULL
		RETURNING id::text,
			user_id::text,
			prompt,
			size,
			status,
			COALESCE(image_path, ''),
			COALESCE(reference_image_path, ''),
			COALESCE(error_code, ''),
			COALESCE(error_message, ''),
			is_favorite,
			COALESCE(title, ''),
			created_at,
			completed_at
	`, userID, taskID, normalizedTitle))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("update task title: %w", err)
	}
	return task, nil
}

func (s Service) CancelTaskForUser(ctx context.Context, userID, taskID string) (Task, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Task{}, fmt.Errorf("begin cancel task: %w", err)
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM generation_tasks
		WHERE user_id = $1::uuid
			AND id = $2::uuid
			AND deleted_at IS NULL
		FOR UPDATE
	`, userID, taskID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("lock task for cancel: %w", err)
	}
	if status == models.TaskRunning {
		return Task{}, ErrTaskAlreadyStarted
	}
	if status != models.TaskQueued {
		return Task{}, ErrTaskNotActive
	}

	task, err := scanTask(tx.QueryRow(ctx, `
		UPDATE generation_tasks
		SET status = $3,
			error_code = 'user_canceled',
			error_message = 'generation canceled by user',
			completed_at = now()
		WHERE user_id = $1::uuid
			AND id = $2::uuid
		RETURNING id::text,
			user_id::text,
			prompt,
			size,
			status,
			COALESCE(image_path, ''),
			COALESCE(reference_image_path, ''),
			COALESCE(error_code, ''),
			COALESCE(error_message, ''),
			is_favorite,
			COALESCE(title, ''),
			created_at,
			completed_at
	`, userID, taskID, models.TaskCanceled))
	if err != nil {
		return Task{}, fmt.Errorf("mark task canceled: %w", err)
	}

	creditService := credits.Service{DB: s.DB}
	if err := creditService.RefundGeneration(ctx, tx, userID, taskID, "generation task canceled"); err != nil {
		return Task{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Task{}, fmt.Errorf("commit cancel task: %w", err)
	}
	return task, nil
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
		COALESCE(reference_image_path, ''),
		COALESCE(error_code, ''),
		COALESCE(error_message, ''),
		is_favorite,
		COALESCE(title, ''),
		created_at,
		completed_at
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
		&task.ReferenceImagePath,
		&task.ErrorCode,
		&task.ErrorMessage,
		&task.IsFavorite,
		&task.Title,
		&task.CreatedAt,
		&task.CompletedAt,
	)
	if err != nil {
		return Task{}, err
	}
	return task, nil
}

func validatePrompt(prompt string) (string, error) {
	trimmed := strings.TrimSpace(prompt)
	runeCount := utf8.RuneCountInString(trimmed)
	if runeCount < 1 {
		return "", ErrInvalidPrompt
	}
	if runeCount > 2000 {
		return "", ErrPromptTooLong
	}
	return trimmed, nil
}

func validateTitle(title string) (string, error) {
	trimmed := strings.TrimSpace(title)
	if utf8.RuneCountInString(trimmed) > 80 {
		return "", ErrTitleTooLong
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return "", ErrInvalidTitle
	}
	return trimmed, nil
}

func generationCreditCost(referenceImagePath string) int {
	if referenceImagePath != "" {
		return 2
	}
	return 1
}

type generationCreditDebit struct {
	amount       int
	balanceAfter int
	walletType   string
	ledgerType   string
}

func debitGenerationCredit(ctx context.Context, tx pgx.Tx, userID string, amount int) ([]generationCreditDebit, error) {
	var freeBalance, paidBalance, totalBalance int
	err := tx.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
			AND status = $2
		FOR UPDATE
	`, userID, models.UserStatusActive).Scan(&freeBalance, &paidBalance, &totalBalance)
	if errors.Is(err, pgx.ErrNoRows) {
		var status string
		err = tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1::uuid`, userID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("inspect user credit state: %w", err)
		}
		if status != models.UserStatusActive {
			return nil, ErrDisabledUser
		}
		return nil, ErrInsufficientCredits
	}
	if err != nil {
		return nil, fmt.Errorf("lock user for generation credit debit: %w", err)
	}
	if totalBalance < amount || freeBalance+paidBalance < amount {
		return nil, ErrInsufficientCredits
	}

	freeDebit := minInt(freeBalance, amount)
	paidDebit := amount - freeDebit
	updatedFreeBalance := freeBalance - freeDebit
	updatedPaidBalance := paidBalance - paidDebit
	updatedTotalBalance := updatedFreeBalance + updatedPaidBalance

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_balance = $2,
			paid_credit_balance = $3,
			credit_balance = $4,
			updated_at = now()
		WHERE id = $1::uuid
	`, userID, updatedFreeBalance, updatedPaidBalance, updatedTotalBalance); err != nil {
		return nil, fmt.Errorf("debit generation credits: %w", err)
	}

	debits := make([]generationCreditDebit, 0, 2)
	if freeDebit > 0 {
		debits = append(debits, generationCreditDebit{
			amount:       freeDebit,
			balanceAfter: totalBalance - freeDebit,
			walletType:   models.WalletDailyFree,
			ledgerType:   models.LedgerDailyFreeGenerationDebit,
		})
	}
	if paidDebit > 0 {
		debits = append(debits, generationCreditDebit{
			amount:       paidDebit,
			balanceAfter: updatedTotalBalance,
			walletType:   models.WalletPaid,
			ledgerType:   models.LedgerPaidGenerationDebit,
		})
	}
	return debits, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func insertTask(ctx context.Context, tx pgx.Tx, userID, prompt, size, model, referenceImagePath string) (Task, error) {
	var task Task
	err := tx.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model, reference_image_path)
		VALUES ($1::uuid, $2, $3, $4, $5, NULLIF($6, ''))
		RETURNING id::text,
			user_id::text,
			prompt,
			size,
			status,
			COALESCE(image_path, ''),
			COALESCE(reference_image_path, ''),
			COALESCE(error_code, ''),
			COALESCE(error_message, ''),
			is_favorite,
			COALESCE(title, ''),
			created_at,
			completed_at
	`, userID, prompt, size, models.TaskQueued, model, referenceImagePath).Scan(
		&task.ID,
		&task.UserID,
		&task.Prompt,
		&task.Size,
		&task.Status,
		&task.ImagePath,
		&task.ReferenceImagePath,
		&task.ErrorCode,
		&task.ErrorMessage,
		&task.IsFavorite,
		&task.Title,
		&task.CreatedAt,
		&task.CompletedAt,
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
