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

func setupGenerationHandlerTest(t *testing.T) (context.Context, *pgxpool.Pool, http.Handler) {
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
		r.Delete("/api/generations/{id}", handlers.Delete)
		r.Get("/api/generations/{id}/image", handlers.Image)
	})

	return ctx, db, r
}

func TestCreateGenerationReturnsQueuedTask(t *testing.T) {
	ctx, db, handler := setupGenerationHandlerTest(t)
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
		Task Task `json:"task"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Task.Status != models.TaskQueued {
		t.Fatalf("task status = %q, want %q", resp.Task.Status, models.TaskQueued)
	}
}

func TestImageEndpointRejectsOtherUser(t *testing.T) {
	ctx, db, handler := setupGenerationHandlerTest(t)
	userA := insertGenerationTestUser(t, ctx, db, "image-owner", 1)
	userB := insertGenerationTestUser(t, ctx, db, "image-other", 1)

	storage := ImageStorage{Root: t.TempDir()}
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
	ctx, db, handler := setupGenerationHandlerTest(t)
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
