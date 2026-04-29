package admin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/auth"
	"imagecreate/api/internal/database"
	"imagecreate/api/internal/models"
)

const adminHandlerTestSecret = "admin-handler-test-session-secret"

func setupAdminHandlerTest(t *testing.T) (context.Context, *pgxpool.Pool, http.Handler) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	db := database.RequireTestDB(t)

	if err := database.RunMigrations(databaseURL, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	handlers := NewHandlers(db)
	r := chi.NewRouter()
	r.Use(auth.WithUser(auth.Service{DB: db}, auth.NewSessionCodec(adminHandlerTestSecret)))
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Get("/users", handlers.ListUsers)
		r.Patch("/users/{id}/status", handlers.UpdateUserStatus)
		r.Post("/users/{id}/credits", handlers.AdjustCredits)
		r.Get("/invites", handlers.ListInvites)
		r.Post("/invites", handlers.CreateInvite)
		r.Get("/audit-logs", handlers.ListAuditLogs)
		r.Get("/generation-tasks", handlers.ListGenerationTasks)
	})

	return context.Background(), db, r
}

func TestAdminCanCreateInvite(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUser(t, ctx, db, "invite-admin", models.RoleAdmin, 0)

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/invites", `{"code":"welcome-code","initial_credits":7}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var resp struct {
		Invite struct {
			Code           string `json:"code"`
			Status         string `json:"status"`
			InitialCredits int    `json:"initial_credits"`
		} `json:"invite"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Invite.Code != "welcome-code" {
		t.Fatalf("invite code = %q, want welcome-code", resp.Invite.Code)
	}
	if resp.Invite.Status != "unused" {
		t.Fatalf("invite status = %q, want unused", resp.Invite.Status)
	}
	if resp.Invite.InitialCredits != 7 {
		t.Fatalf("initial_credits = %d, want 7", resp.Invite.InitialCredits)
	}
}

func TestNonAdminCannotCreateInvite(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	userID := insertAdminTestUser(t, ctx, db, "invite-user", models.RoleUser, 0)

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/invites", `{"code":"blocked","initial_credits":1}`, userID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestAdminCanAdjustCredits(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUser(t, ctx, db, "credit-admin", models.RoleAdmin, 0)
	userID := insertAdminTestUser(t, ctx, db, "credit-user", models.RoleUser, 4)

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/credits", `{"amount":3,"reason":"manual top-up"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var balance int
	if err := db.QueryRow(ctx, `SELECT credit_balance FROM users WHERE id = $1`, userID).Scan(&balance); err != nil {
		t.Fatalf("query balance: %v", err)
	}
	if balance != 7 {
		t.Fatalf("credit_balance = %d, want 7", balance)
	}

	var ledgerRows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1 AND type = $2 AND amount = 3 AND balance_after = 7 AND actor_user_id = $3
	`, userID, models.LedgerAdminAdjustment, adminID).Scan(&ledgerRows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if ledgerRows != 1 {
		t.Fatalf("admin_adjustment ledger rows = %d, want 1", ledgerRows)
	}
}

func TestAdminGenerationListDoesNotReturnImageURL(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUser(t, ctx, db, "task-admin", models.RoleAdmin, 0)
	userID := insertAdminTestUser(t, ctx, db, "task-user", models.RoleUser, 1)

	if _, err := db.Exec(ctx, `
		INSERT INTO generation_tasks (user_id, prompt, size, status, upstream_model, image_path, latency_ms)
		VALUES ($1, 'secret waterfall prompt', '1024x1024', $2, 'test-image-model', 'private/task.png', 123)
	`, userID, models.TaskSucceeded); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	req := authenticatedAdminJSONRequest(t, http.MethodGet, "/api/admin/generation-tasks", "", adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "secret waterfall prompt") {
		t.Fatalf("response body missing prompt: %s", body)
	}
	if !strings.Contains(body, models.TaskSucceeded) {
		t.Fatalf("response body missing status: %s", body)
	}
	for _, forbidden := range []string{"image_path", "imageUrl", "/api/generations/"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response contains forbidden %q: %s", forbidden, body)
		}
	}
}

func TestAdminCanDisableUser(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUser(t, ctx, db, "status-admin", models.RoleAdmin, 0)
	userID := insertAdminTestUser(t, ctx, db, "status-user", models.RoleUser, 0)

	req := authenticatedAdminJSONRequest(t, http.MethodPatch, "/api/admin/users/"+userID+"/status", `{"status":"disabled"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM users WHERE id = $1`, userID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != models.UserStatusDisabled {
		t.Fatalf("status = %q, want %q", status, models.UserStatusDisabled)
	}
}

func insertAdminTestUser(t *testing.T, ctx context.Context, db *pgxpool.Pool, username, role string, credits int) string {
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

func authenticatedAdminJSONRequest(t *testing.T, method, target, body, userID string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	cookieValue, err := auth.NewSessionCodec(adminHandlerTestSecret).Sign(userID)
	if err != nil {
		t.Fatalf("sign session cookie: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: cookieValue})
	return req
}
