package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

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
		r.Post("/password", handlers.ChangeOwnPassword)
		r.Patch("/users/{id}/status", handlers.UpdateUserStatus)
		r.Post("/users/{id}/credits", handlers.AdjustCredits)
		r.Post("/users/{id}/password", handlers.ResetUserPassword)
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
	`, userID, models.LedgerPaidAdminAdjustment, adminID).Scan(&ledgerRows); err != nil {
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

func TestAdjustCreditsUpdatesPaidWalletAndTotal(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUser(t, ctx, db, "admin-paid-adjust", models.RoleAdmin, 0)
	userID := insertAdminTestUser(t, ctx, db, "paid-adjust-target", models.RoleUser, 0)
	_, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 2,
			daily_free_credit_balance = 1,
			paid_credit_balance = 3,
			credit_balance = 4
		WHERE id = $1::uuid
	`, userID)
	if err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/credits", `{"amount":2,"reason":"paid top-up"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}

	var freeBalance, paidBalance, total, ledgerRows int
	if err := db.QueryRow(ctx, `
		SELECT daily_free_credit_balance, paid_credit_balance, credit_balance
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&freeBalance, &paidBalance, &total); err != nil {
		t.Fatalf("query wallets: %v", err)
	}
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM credit_ledger
		WHERE user_id = $1::uuid
			AND type = $2
			AND wallet_type = $3
	`, userID, models.LedgerPaidAdminAdjustment, models.WalletPaid).Scan(&ledgerRows); err != nil {
		t.Fatalf("count paid admin ledger: %v", err)
	}
	if freeBalance != 1 || paidBalance != 5 || total != 6 || ledgerRows != 1 {
		t.Fatalf("free=%d paid=%d total=%d ledgerRows=%d, want 1,5,6,1", freeBalance, paidBalance, total, ledgerRows)
	}
}

func TestAdminAdjustCreditsRejectsOverflow(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUser(t, ctx, db, "overflow-credit-admin", models.RoleAdmin, 0)
	userID := insertAdminTestUser(t, ctx, db, "overflow-credit-user", models.RoleUser, 2147483647)
	if _, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_balance = 1,
			paid_credit_balance = 2147483646,
			credit_balance = 2147483647
		WHERE id = $1::uuid
	`, userID); err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

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
	`, userID, models.LedgerPaidAdminAdjustment).Scan(&ledgerRows); err != nil {
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
	`, userID, models.LedgerPaidAdminAdjustment).Scan(&ledgerRows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if ledgerRows != 0 {
		t.Fatalf("admin_adjustment ledger rows = %d, want 0", ledgerRows)
	}
}

func TestListUsersReturnsWalletFields(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUser(t, ctx, db, "list-wallet-admin", models.RoleAdmin, 0)
	userID := insertAdminTestUser(t, ctx, db, "list-wallet-user", models.RoleUser, 0)
	if _, err := db.Exec(ctx, `
		UPDATE users
		SET daily_free_credit_limit = 4,
			daily_free_credit_balance = 2,
			paid_credit_balance = 7,
			credit_balance = 9
		WHERE id = $1::uuid
	`, userID); err != nil {
		t.Fatalf("seed wallets: %v", err)
	}

	req := authenticatedAdminJSONRequest(t, http.MethodGet, "/api/admin/users", "", adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var resp struct {
		Users []struct {
			ID                     string `json:"id"`
			CreditBalance          int    `json:"credit_balance"`
			DailyFreeCreditLimit   int    `json:"daily_free_credit_limit"`
			DailyFreeCreditBalance int    `json:"daily_free_credit_balance"`
			PaidCreditBalance      int    `json:"paid_credit_balance"`
		} `json:"users"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, user := range resp.Users {
		if user.ID != userID {
			continue
		}
		if user.CreditBalance != 9 || user.DailyFreeCreditLimit != 4 || user.DailyFreeCreditBalance != 2 || user.PaidCreditBalance != 7 {
			t.Fatalf("wallet fields = total:%d limit:%d free:%d paid:%d, want 9,4,2,7",
				user.CreditBalance,
				user.DailyFreeCreditLimit,
				user.DailyFreeCreditBalance,
				user.PaidCreditBalance,
			)
		}
		return
	}
	t.Fatalf("seeded user %s not found in response", userID)
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

func TestAdminCanChangeOwnPassword(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "change-password-admin", models.RoleAdmin, 0, "old-password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/password", `{"current_password":"old-password","new_password":"new-password"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var passwordHash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, adminID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("new-password")); err != nil {
		t.Fatalf("new password does not verify: %v", err)
	}

	var auditRows int
	var metadata string
	if err := db.QueryRow(ctx, `
		SELECT count(*), COALESCE(max(metadata::text), '')
		FROM audit_logs
		WHERE actor_user_id = $1 AND target_user_id = $1 AND action = 'change_own_password'
	`, adminID).Scan(&auditRows, &metadata); err != nil {
		t.Fatalf("query audit rows: %v", err)
	}
	if auditRows != 1 {
		t.Fatalf("change_own_password audit rows = %d, want 1", auditRows)
	}
	if strings.Contains(metadata, "new-password") {
		t.Fatalf("audit metadata contains new password: %s", metadata)
	}
}

func TestAdminChangeOwnPasswordRejectsWrongCurrentPassword(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "wrong-current-admin", models.RoleAdmin, 0, "old-password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/password", `{"current_password":"wrong-password","new_password":"new-password"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	var passwordHash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, adminID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("old-password")); err != nil {
		t.Fatalf("old password no longer verifies: %v", err)
	}
}

func TestAdminChangeOwnPasswordRollsBackWhenAuditFails(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "atomic-change-password-admin", models.RoleAdmin, 0, "old-password")
	failAuditActionForTest(t, ctx, db, "change_own_password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/password", `{"current_password":"old-password","new_password":"new-password"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	var passwordHash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, adminID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("old-password")); err != nil {
		t.Fatalf("old password no longer verifies: %v", err)
	}
}

func TestAdminCanResetUserPassword(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "reset-password-admin", models.RoleAdmin, 0, "admin-password")
	userID := insertAdminTestUserWithPassword(t, ctx, db, "reset-password-user", models.RoleUser, 0, "old-password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/password", `{"new_password":"new-password"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var passwordHash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("new-password")); err != nil {
		t.Fatalf("new password does not verify: %v", err)
	}

	var auditRows int
	var metadata string
	if err := db.QueryRow(ctx, `
		SELECT count(*), COALESCE(max(metadata::text), '')
		FROM audit_logs
		WHERE actor_user_id = $1 AND target_user_id = $2 AND action = 'reset_user_password'
	`, adminID, userID).Scan(&auditRows, &metadata); err != nil {
		t.Fatalf("query audit rows: %v", err)
	}
	if auditRows != 1 {
		t.Fatalf("reset_user_password audit rows = %d, want 1", auditRows)
	}
	if strings.Contains(metadata, "new-password") {
		t.Fatalf("audit metadata contains new password: %s", metadata)
	}
}

func TestAdminResetUserPasswordRollsBackWhenAuditFails(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "atomic-reset-password-admin", models.RoleAdmin, 0, "admin-password")
	userID := insertAdminTestUserWithPassword(t, ctx, db, "atomic-reset-password-user", models.RoleUser, 0, "old-password")
	failAuditActionForTest(t, ctx, db, "reset_user_password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/password", `{"new_password":"new-password"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	var passwordHash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&passwordHash); err != nil {
		t.Fatalf("query password hash: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte("old-password")); err != nil {
		t.Fatalf("old password no longer verifies: %v", err)
	}
}

func TestAdminResetUserPasswordRejectsShortPassword(t *testing.T) {
	ctx, db, handler := setupAdminHandlerTest(t)
	adminID := insertAdminTestUserWithPassword(t, ctx, db, "short-reset-admin", models.RoleAdmin, 0, "admin-password")
	userID := insertAdminTestUserWithPassword(t, ctx, db, "short-reset-user", models.RoleUser, 0, "old-password")

	req := authenticatedAdminJSONRequest(t, http.MethodPost, "/api/admin/users/"+userID+"/password", `{"new_password":"12345"}`, adminID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
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
		INSERT INTO users (username, password_hash, role, status, credit_balance, paid_credit_balance)
		VALUES ($1, 'hash', $2, $3, $4, $4)
		RETURNING id::text
	`, username, role, models.UserStatusActive, credits).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return userID
}

func failAuditActionForTest(t *testing.T, ctx context.Context, db *pgxpool.Pool, action string) {
	t.Helper()

	actionLiteral := strings.ReplaceAll(action, `'`, `''`)
	if _, err := db.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION fail_password_audit_for_test()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF NEW.action = '%s' THEN
				RAISE EXCEPTION 'forced audit failure';
			END IF;
			RETURN NEW;
		END;
		$$;
	`, actionLiteral)); err != nil {
		t.Fatalf("create audit failure function: %v", err)
	}
	if _, err := db.Exec(ctx, `
		CREATE TRIGGER fail_password_audit_for_test
		BEFORE INSERT ON audit_logs
		FOR EACH ROW
		EXECUTE FUNCTION fail_password_audit_for_test();
	`); err != nil {
		t.Fatalf("create audit failure trigger: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DROP TRIGGER IF EXISTS fail_password_audit_for_test ON audit_logs`)
		_, _ = db.Exec(context.Background(), `DROP FUNCTION IF EXISTS fail_password_audit_for_test()`)
	})
}

func insertAdminTestUserWithPassword(t *testing.T, ctx context.Context, db *pgxpool.Pool, username, role string, credits int, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var userID string
	if err := db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, status, credit_balance, paid_credit_balance)
		VALUES ($1, $2, $3, $4, $5, $5)
		RETURNING id::text
	`, username, string(hash), role, models.UserStatusActive, credits).Scan(&userID); err != nil {
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
