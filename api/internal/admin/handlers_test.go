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

func TestValidateInviteInitialCreditsRange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value int
		valid bool
	}{
		{name: "zero", value: 0, valid: true},
		{name: "max int32", value: 2147483647, valid: true},
		{name: "negative", value: -1, valid: false},
		{name: "over int32", value: 2147483648, valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateInviteInitialCredits(tc.value)
			if tc.valid && err != nil {
				t.Fatalf("validateInviteInitialCredits(%d) error = %v, want nil", tc.value, err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("validateInviteInitialCredits(%d) error = nil, want error", tc.value)
			}
		})
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

	var auditRows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE actor_user_id = $1 AND target_user_id = $2 AND action = 'adjust_credits'
	`, adminID, userID).Scan(&auditRows); err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	if auditRows != 1 {
		t.Fatalf("adjust_credits audit rows = %d, want 1", auditRows)
	}
}

func TestAdminAdjustCreditsRejectsOverflow(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUser(t, ctx, db, "overflow-credit-admin", models.RoleAdmin, 0)
	userID := insertAdminTestUser(t, ctx, db, "overflow-credit-user", models.RoleUser, 2147483647)

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/credits", `{"amount":1,"reason":"too much"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	var balance int
	if err := db.QueryRow(ctx, `SELECT credit_balance FROM users WHERE id = $1`, userID).Scan(&balance); err != nil {
		t.Fatalf("query balance: %v", err)
	}
	if balance != 2147483647 {
		t.Fatalf("credit_balance = %d, want unchanged 2147483647", balance)
	}

	var ledgerRows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1 AND type = $2
	`, userID, models.LedgerAdminAdjustment).Scan(&ledgerRows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if ledgerRows != 0 {
		t.Fatalf("admin_adjustment ledger rows = %d, want 0", ledgerRows)
	}
}

func TestValidateCreditAdjustmentAmountRange(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value int
		valid bool
	}{
		{name: "positive", value: 3, valid: true},
		{name: "negative", value: -3, valid: true},
		{name: "max int32", value: 2147483647, valid: true},
		{name: "min int32", value: -2147483648, valid: true},
		{name: "zero", value: 0, valid: false},
		{name: "over int32", value: 2147483648, valid: false},
		{name: "under int32", value: -2147483649, valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreditAdjustmentAmount(tc.value)
			if tc.valid && err != nil {
				t.Fatalf("validateCreditAdjustmentAmount(%d) error = %v, want nil", tc.value, err)
			}
			if !tc.valid && err == nil {
				t.Fatalf("validateCreditAdjustmentAmount(%d) error = nil, want error", tc.value)
			}
		})
	}
}

func TestAdminAdjustCreditsRollsBackWhenAuditFails(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUser(t, ctx, db, "atomic-credit-admin", models.RoleAdmin, 0)
	userID := insertAdminTestUser(t, ctx, db, "atomic-credit-user", models.RoleUser, 4)

	if _, err := db.Exec(ctx, `
		CREATE OR REPLACE FUNCTION fail_adjust_credits_audit_for_test()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.action = 'adjust_credits' THEN
				RAISE EXCEPTION 'forced audit failure';
			END IF;
			RETURN NEW;
		END;
		$$;
	`); err != nil {
		t.Fatalf("create audit failure function: %v", err)
	}
	if _, err := db.Exec(ctx, `
		CREATE TRIGGER fail_adjust_credits_audit_for_test
		BEFORE INSERT ON audit_logs
		FOR EACH ROW
		EXECUTE FUNCTION fail_adjust_credits_audit_for_test();
	`); err != nil {
		t.Fatalf("create audit failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DROP TRIGGER IF EXISTS fail_adjust_credits_audit_for_test ON audit_logs`)
		_, _ = db.Exec(context.Background(), `DROP FUNCTION IF EXISTS fail_adjust_credits_audit_for_test()`)
	})

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/credits", `{"amount":3,"reason":"manual top-up"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	var balance int
	if err := db.QueryRow(ctx, `SELECT credit_balance FROM users WHERE id = $1`, userID).Scan(&balance); err != nil {
		t.Fatalf("query balance: %v", err)
	}
	if balance != 4 {
		t.Fatalf("credit_balance = %d, want unchanged 4", balance)
	}

	var ledgerRows int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1 AND type = $2 AND amount = 3
	`, userID, models.LedgerAdminAdjustment).Scan(&ledgerRows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if ledgerRows != 0 {
		t.Fatalf("admin_adjustment ledger rows = %d, want 0", ledgerRows)
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
