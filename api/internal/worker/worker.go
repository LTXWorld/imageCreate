package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/generations"
	"imagecreate/api/internal/models"
	"imagecreate/api/internal/upstream"
)

const (
	defaultPollInterval  = time.Second
	fallbackErrorCode    = "upstream_error"
	fallbackErrorMessage = "upstream image generation failed"
	storageErrorCode     = "internal_error"
	storageErrorMessage  = "failed to save generated image"
)

type Upstream interface {
	GenerateImage(ctx context.Context, prompt, size string) (upstream.Result, error)
}

type Storage interface {
	Save(ctx context.Context, taskID string, data []byte, now time.Time) (string, error)
}

type Worker struct {
	DB           *pgxpool.Pool
	Generations  generations.Service
	Upstream     Upstream
	Storage      Storage
	PollInterval time.Duration
}

type claimedTask struct {
	id     string
	prompt string
	size   string
}

func (w Worker) Run(ctx context.Context) {
	interval := w.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		processed, err := w.ProcessOne(ctx)
		if err != nil || !processed {
			timer.Reset(interval)
			continue
		}
		timer.Reset(0)
	}
}

func (w Worker) ProcessOne(ctx context.Context) (bool, error) {
	task, ok, err := w.claimOne(ctx)
	if err != nil || !ok {
		return ok, err
	}

	started := time.Now()
	result, err := w.Upstream.GenerateImage(ctx, task.prompt, task.size)
	latencyMS := elapsedMilliseconds(started)
	if err != nil {
		code, message := upstreamFailure(result)
		if markErr := w.Generations.MarkFailedAndRefund(ctx, task.id, code, message, latencyMS); markErr != nil {
			return true, markErr
		}
		return true, nil
	}

	imagePath, err := w.Storage.Save(ctx, task.id, result.ImageBytes, time.Now())
	latencyMS = elapsedMilliseconds(started)
	if err != nil {
		if markErr := w.Generations.MarkFailedAndRefund(ctx, task.id, storageErrorCode, storageErrorMessage, latencyMS); markErr != nil {
			return true, fmt.Errorf("save image: %v; mark failed: %w", err, markErr)
		}
		return true, nil
	}

	if err := w.Generations.MarkSucceeded(ctx, task.id, result.RequestID, imagePath, latencyMS); err != nil {
		return true, err
	}
	return true, nil
}

func (w Worker) claimOne(ctx context.Context) (claimedTask, bool, error) {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return claimedTask{}, false, fmt.Errorf("begin claim task: %w", err)
	}
	defer tx.Rollback(ctx)

	var task claimedTask
	err = tx.QueryRow(ctx, `
		SELECT id::text, prompt, size
		FROM generation_tasks
		WHERE status = $1
			AND deleted_at IS NULL
		ORDER BY created_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, models.TaskQueued).Scan(&task.id, &task.prompt, &task.size)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimedTask{}, false, nil
	}
	if err != nil {
		return claimedTask{}, false, fmt.Errorf("select queued task: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE generation_tasks
		SET status = $2,
			started_at = now()
		WHERE id = $1::uuid
	`, task.id, models.TaskRunning); err != nil {
		return claimedTask{}, false, fmt.Errorf("mark task running: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return claimedTask{}, false, fmt.Errorf("commit claim task: %w", err)
	}
	return task, true, nil
}

func upstreamFailure(result upstream.Result) (string, string) {
	code := result.ErrorCode
	if code == "" {
		code = fallbackErrorCode
	}
	message := result.ErrorMessage
	if message == "" {
		message = fallbackErrorMessage
	}
	return code, message
}

func elapsedMilliseconds(started time.Time) int {
	return int(time.Since(started).Milliseconds())
}
