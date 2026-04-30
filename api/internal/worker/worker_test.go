package worker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/database"
	"imagecreate/api/internal/generations"
	"imagecreate/api/internal/models"
	"imagecreate/api/internal/upstream"
)

func setupWorkerTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	db := database.RequireTestDB(t)

	if err := database.RunMigrations(databaseURL, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	return context.Background(), db
}

func workerGenerationService(db *pgxpool.Pool) generations.Service {
	return generations.Service{
		DB:    db,
		Model: "test-image-model",
		SizePresets: map[string]string{
			"1:1": "1024x1024",
		},
	}
}

func insertWorkerTestUser(t *testing.T, ctx context.Context, db *pgxpool.Pool, username string, credits int) string {
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

func workerCreditBalance(t *testing.T, ctx context.Context, db *pgxpool.Pool, userID string) int {
	t.Helper()

	var balance int
	if err := db.QueryRow(ctx, `SELECT credit_balance FROM users WHERE id = $1`, userID).Scan(&balance); err != nil {
		t.Fatalf("query credit balance: %v", err)
	}
	return balance
}

func workerTaskStatus(t *testing.T, ctx context.Context, db *pgxpool.Pool, taskID string) string {
	t.Helper()

	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM generation_tasks WHERE id = $1`, taskID).Scan(&status); err != nil {
		t.Fatalf("query task status: %v", err)
	}
	return status
}

func workerRefundLedgerRows(t *testing.T, ctx context.Context, db *pgxpool.Pool, userID, taskID string) int {
	t.Helper()

	var rows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1 AND task_id = $2 AND type = $3
	`, userID, taskID, models.LedgerGenerationRefund).Scan(&rows); err != nil {
		t.Fatalf("count refund ledger rows: %v", err)
	}
	return rows
}

type fakeUpstream struct {
	t          *testing.T
	db         *pgxpool.Pool
	taskID     string
	wantPrompt string
	wantSize   string
	result     upstream.Result
	err        error
	called     int
	cancel     context.CancelFunc
}

func (f *fakeUpstream) GenerateImage(ctx context.Context, prompt, size string) (upstream.Result, error) {
	f.called++
	if prompt != f.wantPrompt {
		f.t.Fatalf("upstream prompt = %q, want %q", prompt, f.wantPrompt)
	}
	if size != f.wantSize {
		f.t.Fatalf("upstream size = %q, want %q", size, f.wantSize)
	}
	if got := workerTaskStatus(f.t, ctx, f.db, f.taskID); got != models.TaskRunning {
		f.t.Fatalf("task status during upstream call = %q, want %q", got, models.TaskRunning)
	}
	if f.cancel != nil {
		f.cancel()
	}
	return f.result, f.err
}

type fakeStorage struct {
	t      *testing.T
	path   string
	called int
	taskID string
	data   []byte
	now    time.Time
}

func (s *fakeStorage) Save(ctx context.Context, taskID string, data []byte, now time.Time) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.called++
	s.taskID = taskID
	s.data = append([]byte(nil), data...)
	s.now = now
	return s.path, nil
}

type blockingUpstream struct {
	mu      sync.Mutex
	called  int
	target  int
	entered chan struct{}
}

func newBlockingUpstream(target int) *blockingUpstream {
	return &blockingUpstream{
		target:  target,
		entered: make(chan struct{}),
	}
}

func (u *blockingUpstream) GenerateImage(ctx context.Context, prompt, size string) (upstream.Result, error) {
	u.mu.Lock()
	u.called++
	if u.called == u.target {
		close(u.entered)
	}
	u.mu.Unlock()

	<-ctx.Done()
	return upstream.Result{ErrorCode: "timeout", ErrorMessage: "upstream request timed out"}, ctx.Err()
}

func (u *blockingUpstream) Calls() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.called
}

func TestWorkerProcessesQueuedTaskSuccessfully(t *testing.T) {
	ctx, db := setupWorkerTestDB(t)
	service := workerGenerationService(db)
	userID := insertWorkerTestUser(t, ctx, db, "worker-success", 1)

	task, err := service.CreateTask(ctx, generations.CreateTaskInput{
		UserID: userID,
		Prompt: "draw a tiny bright moon",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if got := workerTaskStatus(t, ctx, db, task.ID); got != models.TaskQueued {
		t.Fatalf("initial task status = %q, want %q", got, models.TaskQueued)
	}

	imageBytes := []byte{0x89, 'P', 'N', 'G'}
	storage := &fakeStorage{t: t, path: "generated/task.png"}
	upstreamClient := &fakeUpstream{
		t:          t,
		db:         db,
		taskID:     task.ID,
		wantPrompt: task.Prompt,
		wantSize:   task.Size,
		result: upstream.Result{
			RequestID:  "req-success",
			ImageBytes: imageBytes,
		},
	}
	worker := Worker{
		DB:          db,
		Generations: service,
		Upstream:    upstreamClient,
		Storage:     storage,
	}

	processed, err := worker.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if upstreamClient.called != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamClient.called)
	}
	if storage.called != 1 {
		t.Fatalf("storage calls = %d, want 1", storage.called)
	}
	if storage.taskID != task.ID {
		t.Fatalf("storage taskID = %q, want %q", storage.taskID, task.ID)
	}
	if string(storage.data) != string(imageBytes) {
		t.Fatalf("storage data = %v, want %v", storage.data, imageBytes)
	}

	var status, imagePath, requestID string
	if err := db.QueryRow(ctx, `
		SELECT status, image_path, upstream_request_id
		FROM generation_tasks
		WHERE id = $1
	`, task.ID).Scan(&status, &imagePath, &requestID); err != nil {
		t.Fatalf("query completed task: %v", err)
	}
	if status != models.TaskSucceeded {
		t.Fatalf("final task status = %q, want %q", status, models.TaskSucceeded)
	}
	if imagePath != storage.path {
		t.Fatalf("image_path = %q, want %q", imagePath, storage.path)
	}
	if requestID != "req-success" {
		t.Fatalf("upstream_request_id = %q, want req-success", requestID)
	}
	if rows := workerRefundLedgerRows(t, ctx, db, userID, task.ID); rows != 0 {
		t.Fatalf("refund ledger rows = %d, want 0", rows)
	}
}

func TestRunPoolProcessesTasksConcurrently(t *testing.T) {
	ctx, db := setupWorkerTestDB(t)
	service := workerGenerationService(db)

	firstUserID := insertWorkerTestUser(t, ctx, db, "worker-pool-one", 1)
	secondUserID := insertWorkerTestUser(t, ctx, db, "worker-pool-two", 1)

	if _, err := service.CreateTask(ctx, generations.CreateTaskInput{
		UserID: firstUserID,
		Prompt: "draw first queued task",
		Ratio:  "1:1",
	}); err != nil {
		t.Fatalf("create first task: %v", err)
	}
	if _, err := service.CreateTask(ctx, generations.CreateTaskInput{
		UserID: secondUserID,
		Prompt: "draw second queued task",
		Ratio:  "1:1",
	}); err != nil {
		t.Fatalf("create second task: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	upstreamClient := newBlockingUpstream(2)
	storage := &fakeStorage{t: t, path: "unused.png"}
	done := RunPool(runCtx, Worker{
		DB:          db,
		Generations: service,
		Upstream:    upstreamClient,
		Storage:     storage,
	}, 2)

	select {
	case <-upstreamClient.entered:
	case <-time.After(2 * time.Second):
		t.Fatalf("upstream calls = %d, want 2 concurrent calls", upstreamClient.Calls())
	}

	cancelRun()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker pool did not stop after cancellation")
	}
}

func TestWorkerRefundsOnUpstreamFailure(t *testing.T) {
	ctx, db := setupWorkerTestDB(t)
	service := workerGenerationService(db)
	userID := insertWorkerTestUser(t, ctx, db, "worker-failure", 1)

	task, err := service.CreateTask(ctx, generations.CreateTaskInput{
		UserID: userID,
		Prompt: "draw a policy rejected image",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if got := workerCreditBalance(t, ctx, db, userID); got != 0 {
		t.Fatalf("credit_balance after create = %d, want 0", got)
	}
	if got := workerTaskStatus(t, ctx, db, task.ID); got != models.TaskQueued {
		t.Fatalf("initial task status = %q, want %q", got, models.TaskQueued)
	}

	storage := &fakeStorage{t: t, path: "should-not-save.png"}
	upstreamClient := &fakeUpstream{
		t:          t,
		db:         db,
		taskID:     task.ID,
		wantPrompt: task.Prompt,
		wantSize:   task.Size,
		result: upstream.Result{
			ErrorCode:    "content_rejected",
			ErrorMessage: "upstream rejected the requested content",
		},
		err: errors.New("content rejected"),
	}
	worker := Worker{
		DB:          db,
		Generations: service,
		Upstream:    upstreamClient,
		Storage:     storage,
	}

	processed, err := worker.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if storage.called != 0 {
		t.Fatalf("storage calls = %d, want 0", storage.called)
	}

	var status, errorCode, errorMessage string
	if err := db.QueryRow(ctx, `
		SELECT status, error_code, error_message
		FROM generation_tasks
		WHERE id = $1
	`, task.ID).Scan(&status, &errorCode, &errorMessage); err != nil {
		t.Fatalf("query failed task: %v", err)
	}
	if status != models.TaskFailed {
		t.Fatalf("final task status = %q, want %q", status, models.TaskFailed)
	}
	if errorCode != "content_rejected" {
		t.Fatalf("error_code = %q, want content_rejected", errorCode)
	}
	if errorMessage != "upstream rejected the requested content" {
		t.Fatalf("error_message = %q, want upstream rejected the requested content", errorMessage)
	}
	if got := workerCreditBalance(t, ctx, db, userID); got != 1 {
		t.Fatalf("credit_balance after failure = %d, want 1", got)
	}
	if rows := workerRefundLedgerRows(t, ctx, db, userID, task.ID); rows != 1 {
		t.Fatalf("refund ledger rows = %d, want 1", rows)
	}
}

func TestWorkerSkipsWhenNoQueuedTask(t *testing.T) {
	ctx, db := setupWorkerTestDB(t)
	service := workerGenerationService(db)
	upstreamClient := &fakeUpstream{t: t}
	storage := &fakeStorage{t: t}
	worker := Worker{
		DB:          db,
		Generations: service,
		Upstream:    upstreamClient,
		Storage:     storage,
	}

	processed, err := worker.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("process one: %v", err)
	}
	if processed {
		t.Fatal("processed = true, want false")
	}
	if upstreamClient.called != 0 {
		t.Fatalf("upstream calls = %d, want 0", upstreamClient.called)
	}
	if storage.called != 0 {
		t.Fatalf("storage calls = %d, want 0", storage.called)
	}
}

func TestWorkerRecoversStaleRunningTask(t *testing.T) {
	ctx, db := setupWorkerTestDB(t)
	service := workerGenerationService(db)
	userID := insertWorkerTestUser(t, ctx, db, "worker-stale", 1)

	task, err := service.CreateTask(ctx, generations.CreateTaskInput{
		UserID: userID,
		Prompt: "draw a stalled task",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := db.Exec(ctx, `
		UPDATE generation_tasks
		SET status = $2,
			started_at = now() - interval '2 hours'
		WHERE id = $1
	`, task.ID, models.TaskRunning); err != nil {
		t.Fatalf("mark task stale running: %v", err)
	}

	upstreamClient := &fakeUpstream{t: t}
	storage := &fakeStorage{t: t}
	worker := Worker{
		DB:             db,
		Generations:    service,
		Upstream:       upstreamClient,
		Storage:        storage,
		RunningTimeout: time.Minute,
	}

	processed, err := worker.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if upstreamClient.called != 0 {
		t.Fatalf("upstream calls = %d, want 0", upstreamClient.called)
	}
	if storage.called != 0 {
		t.Fatalf("storage calls = %d, want 0", storage.called)
	}

	var status, errorCode, errorMessage string
	if err := db.QueryRow(ctx, `
		SELECT status, error_code, error_message
		FROM generation_tasks
		WHERE id = $1
	`, task.ID).Scan(&status, &errorCode, &errorMessage); err != nil {
		t.Fatalf("query recovered task: %v", err)
	}
	if status != models.TaskFailed {
		t.Fatalf("status = %q, want %q", status, models.TaskFailed)
	}
	if errorCode != staleRunningErrorCode {
		t.Fatalf("error_code = %q, want %q", errorCode, staleRunningErrorCode)
	}
	if errorMessage != staleRunningErrorMessage {
		t.Fatalf("error_message = %q, want %q", errorMessage, staleRunningErrorMessage)
	}
	if got := workerCreditBalance(t, ctx, db, userID); got != 1 {
		t.Fatalf("credit_balance after recovery = %d, want 1", got)
	}
	if rows := workerRefundLedgerRows(t, ctx, db, userID, task.ID); rows != 1 {
		t.Fatalf("refund ledger rows = %d, want 1", rows)
	}
}

func TestWorkerFinalizesFailureAfterRequestCancellation(t *testing.T) {
	ctx, db := setupWorkerTestDB(t)
	service := workerGenerationService(db)
	userID := insertWorkerTestUser(t, ctx, db, "worker-canceled", 1)

	task, err := service.CreateTask(ctx, generations.CreateTaskInput{
		UserID: userID,
		Prompt: "draw a canceled task",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	requestCtx, cancelRequest := context.WithCancel(ctx)
	storage := &fakeStorage{t: t, path: "should-not-save.png"}
	upstreamClient := &fakeUpstream{
		t:          t,
		db:         db,
		taskID:     task.ID,
		wantPrompt: task.Prompt,
		wantSize:   task.Size,
		result: upstream.Result{
			ErrorCode:    "timeout",
			ErrorMessage: "upstream request timed out",
		},
		err:    context.Canceled,
		cancel: cancelRequest,
	}
	worker := Worker{
		DB:          db,
		Generations: service,
		Upstream:    upstreamClient,
		Storage:     storage,
	}

	processed, err := worker.ProcessOne(requestCtx)
	if err != nil {
		t.Fatalf("process one: %v", err)
	}
	if !processed {
		t.Fatal("processed = false, want true")
	}
	if err := requestCtx.Err(); err == nil {
		t.Fatal("request context is not canceled")
	}

	var status, errorCode string
	if err := db.QueryRow(ctx, `
		SELECT status, error_code
		FROM generation_tasks
		WHERE id = $1
	`, task.ID).Scan(&status, &errorCode); err != nil {
		t.Fatalf("query finalized task: %v", err)
	}
	if status != models.TaskFailed {
		t.Fatalf("status = %q, want %q", status, models.TaskFailed)
	}
	if errorCode != "timeout" {
		t.Fatalf("error_code = %q, want timeout", errorCode)
	}
	if got := workerCreditBalance(t, ctx, db, userID); got != 1 {
		t.Fatalf("credit_balance after cancellation = %d, want 1", got)
	}
	if rows := workerRefundLedgerRows(t, ctx, db, userID, task.ID); rows != 1 {
		t.Fatalf("refund ledger rows = %d, want 1", rows)
	}
}

func TestFinalizationContextIgnoresCanceledRequestContext(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	cancelRequest()

	ctx, cancel := finalizationContext()
	defer cancel()

	if err := requestCtx.Err(); err == nil {
		t.Fatal("request context is not canceled")
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("finalization context error = %v, want nil", err)
	}
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("finalization context has no deadline")
	}
}
