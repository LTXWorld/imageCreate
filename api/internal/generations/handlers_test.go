package generations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/auth"
	"imagecreate/api/internal/models"
)

const generationHandlerTestSecret = "handler-test-session-secret"

func setupGenerationHandlerTest(t *testing.T) (context.Context, *pgxpool.Pool, ImageStorage, http.Handler) {
	t.Helper()

	ctx, db := setupGenerationTestDB(t)
	storage := ImageStorage{Root: t.TempDir()}
	handlers := NewHandlers(testService(db), storage)

	r := chi.NewRouter()
	r.Use(auth.WithUser(auth.Service{DB: db}, auth.NewSessionCodec(generationHandlerTestSecret)))
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireUser)
		r.Post("/api/generations", handlers.Create)
		r.Get("/api/generations", handlers.List)
		r.Get("/api/generations/{id}", handlers.Get)
		r.Post("/api/generations/{id}/cancel", handlers.Cancel)
		r.Delete("/api/generations/{id}", handlers.Delete)
		r.Get("/api/generations/{id}/image", handlers.Image)
	})

	return ctx, db, storage, r
}

func TestCreateGenerationReturnsQueuedTask(t *testing.T) {
	ctx, db, _, handler := setupGenerationHandlerTest(t)
	userID := insertGenerationTestUser(t, ctx, db, "handler-create", 3)

	req := authenticatedJSONRequest(t, http.MethodPost, "/api/generations", `{"prompt":"一张真实感照片","ratio":"1:1"}`, userID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-test") || strings.Contains(rec.Body.String(), "OPENAI_API_KEY") {
		t.Fatalf("response leaked upstream key material: %s", rec.Body.String())
	}

	var resp struct {
		Task struct {
			ID          string `json:"id"`
			Prompt      string `json:"prompt"`
			Ratio       string `json:"ratio"`
			Size        string `json:"size"`
			Status      string `json:"status"`
			CreatedAt   string `json:"created_at"`
			CompletedAt string `json:"completed_at,omitempty"`
		} `json:"task"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Task.Status != models.TaskQueued {
		t.Fatalf("task status = %q, want %q", resp.Task.Status, models.TaskQueued)
	}
	if resp.Task.Ratio != "1:1" {
		t.Fatalf("task ratio = %q, want 1:1", resp.Task.Ratio)
	}
	if resp.Task.CreatedAt == "" {
		t.Fatal("task created_at is empty")
	}
	if resp.Task.CompletedAt != "" {
		t.Fatalf("task completed_at = %q, want empty for queued task", resp.Task.CompletedAt)
	}
}

func TestCreateGenerationRejectsLongPromptWithClearMessage(t *testing.T) {
	ctx, db, _, handler := setupGenerationHandlerTest(t)
	userID := insertGenerationTestUser(t, ctx, db, "handler-long-prompt", 3)

	body := `{"prompt":"` + strings.Repeat("a", 2001) + `","ratio":"1:1"}`
	req := authenticatedJSONRequest(t, http.MethodPost, "/api/generations", body, userID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var resp struct {
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ErrorCode != "prompt_too_long" {
		t.Fatalf("error_code = %q, want prompt_too_long", resp.ErrorCode)
	}
	const want = "提示词不能超过 2000 个字符"
	if resp.Message != want {
		t.Fatalf("message = %q, want %q", resp.Message, want)
	}
}

func TestImageEndpointRejectsOtherUser(t *testing.T) {
	ctx, db, storage, handler := setupGenerationHandlerTest(t)
	userA := insertGenerationTestUser(t, ctx, db, "image-owner", 1)
	userB := insertGenerationTestUser(t, ctx, db, "image-other", 1)

	imagePath, err := storage.Save(ctx, "owned-task", []byte("secret-file-bytes"), testNow())
	if err != nil {
		t.Fatalf("save image: %v", err)
	}

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model, image_path)
		VALUES ($1, 'prompt', '1024x1024', $2, 'test-image-model', $3)
		RETURNING id::text
	`, userA, models.TaskSucceeded, filepath.ToSlash(imagePath)).Scan(&taskID); err != nil {
		t.Fatalf("insert succeeded task: %v", err)
	}

	req := authenticatedJSONRequest(t, http.MethodGet, "/api/generations/"+taskID+"/image", "", userB)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 404 or 403; body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-file-bytes") {
		t.Fatalf("response included file bytes: %s", rec.Body.String())
	}
}

func TestGenerationFailureMessageIsChinese(t *testing.T) {
	ctx, db, _, handler := setupGenerationHandlerTest(t)
	userID := insertGenerationTestUser(t, ctx, db, "handler-failure", 1)

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model, error_code, error_message)
		VALUES ($1, 'prompt', '1024x1024', $2, 'test-image-model', 'content_rejected', 'upstream rejected content')
		RETURNING id::text
	`, userID, models.TaskFailed).Scan(&taskID); err != nil {
		t.Fatalf("insert failed task: %v", err)
	}

	req := authenticatedJSONRequest(t, http.MethodGet, "/api/generations/"+taskID, "", userID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp struct {
		Task struct {
			ErrorCode string `json:"error_code"`
			Message   string `json:"message"`
		} `json:"task"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Task.ErrorCode != "content_rejected" {
		t.Fatalf("error_code = %q, want content_rejected", resp.Task.ErrorCode)
	}
	const want = "提示词可能包含不支持生成的内容，请调整描述后重试。"
	if resp.Task.Message != want {
		t.Fatalf("message = %q, want %q", resp.Task.Message, want)
	}
}

func TestCancelGenerationReturnsCanceledTask(t *testing.T) {
	ctx, db, _, handler := setupGenerationHandlerTest(t)
	userID := insertGenerationTestUser(t, ctx, db, "handler-cancel", 1)

	task, err := testService(db).CreateTask(ctx, CreateTaskInput{
		UserID: userID,
		Prompt: "wrong prompt",
		Ratio:  "1:1",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	req := authenticatedJSONRequest(t, http.MethodPost, "/api/generations/"+task.ID+"/cancel", "", userID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp struct {
		Task struct {
			Status      string `json:"status"`
			CompletedAt string `json:"completed_at"`
		} `json:"task"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Task.Status != models.TaskCanceled {
		t.Fatalf("status = %q, want %q", resp.Task.Status, models.TaskCanceled)
	}
	if resp.Task.CompletedAt == "" {
		t.Fatal("completed_at is empty")
	}
}

func TestCancelRunningGenerationReturnsStartedError(t *testing.T) {
	ctx, db, _, handler := setupGenerationHandlerTest(t)
	userID := insertGenerationTestUser(t, ctx, db, "handler-cancel-running", 1)

	task, err := testService(db).CreateTask(ctx, CreateTaskInput{
		UserID: userID,
		Prompt: "already started prompt",
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

	req := authenticatedJSONRequest(t, http.MethodPost, "/api/generations/"+task.ID+"/cancel", "", userID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	var resp struct {
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ErrorCode != "task_already_started" || resp.Message != "已开始生成，无法取消" {
		t.Fatalf("error response = %+v, want task_already_started", resp)
	}
}

func TestGenerationInvalidIDReturnsStableError(t *testing.T) {
	ctx, db, _, handler := setupGenerationHandlerTest(t)
	userID := insertGenerationTestUser(t, ctx, db, "handler-invalid-id", 1)

	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "get", method: http.MethodGet, path: "/api/generations/not-a-uuid"},
		{name: "cancel", method: http.MethodPost, path: "/api/generations/not-a-uuid/cancel"},
		{name: "delete", method: http.MethodDelete, path: "/api/generations/not-a-uuid"},
		{name: "image", method: http.MethodGet, path: "/api/generations/not-a-uuid/image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := authenticatedJSONRequest(t, tc.method, tc.path, "", userID)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			var resp struct {
				ErrorCode string `json:"error_code"`
				Message   string `json:"message"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.ErrorCode != "invalid_task_id" || resp.Message == "" {
				t.Fatalf("error response = %+v, want invalid_task_id with message", resp)
			}
		})
	}
}

func TestGenerationFailureMessagesUseRefundWording(t *testing.T) {
	for _, tc := range []struct {
		code string
		want string
	}{
		{code: "rate_limited", want: "当前生成服务繁忙，请稍后再试。"},
		{code: "timeout", want: "生成超时，本次额度已退回，请稍后重试。"},
		{code: "upstream_error", want: "生成失败，本次额度已退回。"},
		{code: "", want: "生成失败，本次额度已退回。"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			resp := newTaskResponse(Task{Status: models.TaskFailed, ErrorCode: tc.code})
			if resp.Message != tc.want {
				t.Fatalf("message = %q, want %q", resp.Message, tc.want)
			}
		})
	}
}

func TestImageEndpointReturnsSameUserImage(t *testing.T) {
	ctx, db, storage, handler := setupGenerationHandlerTest(t)
	userID := insertGenerationTestUser(t, ctx, db, "image-same-user", 1)
	const imageBytes = "same-user-image-bytes"

	imagePath, err := storage.Save(ctx, "same-user-task", []byte(imageBytes), testNow())
	if err != nil {
		t.Fatalf("save image: %v", err)
	}

	var taskID string
	if err := db.QueryRow(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model, image_path)
		VALUES ($1, 'prompt', '1024x1024', $2, 'test-image-model', $3)
		RETURNING id::text
	`, userID, models.TaskSucceeded, filepath.ToSlash(imagePath)).Scan(&taskID); err != nil {
		t.Fatalf("insert succeeded task: %v", err)
	}

	req := authenticatedJSONRequest(t, http.MethodGet, "/api/generations/"+taskID+"/image", "", userID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content-type = %q, want image/png", got)
	}
	if rec.Body.String() != imageBytes {
		t.Fatalf("body = %q, want %q", rec.Body.String(), imageBytes)
	}
}

func authenticatedJSONRequest(t *testing.T, method, target, body, userID string) *http.Request {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set("Content-Type", "application/json")

	cookieValue, err := auth.NewSessionCodec(generationHandlerTestSecret).Sign(userID)
	if err != nil {
		t.Fatalf("sign session cookie: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookieValue})
	return req
}

func testNow() time.Time {
	return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
}
