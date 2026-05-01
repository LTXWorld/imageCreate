package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"imagecreate/api/internal/auth"
	"imagecreate/api/internal/credits"
	"imagecreate/api/internal/models"
)

type Handlers struct {
	DB *pgxpool.Pool
}

const (
	minPostgresInteger = -2147483648
	maxPostgresInteger = 2147483647
)

var errCreditBalanceOutOfRange = errors.New("credit balance out of range")

func NewHandlers(db *pgxpool.Pool) Handlers {
	return Handlers{DB: db}
}

func (h Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT id::text,
			username,
			role,
			status,
			credit_balance,
			daily_free_credit_limit,
			daily_free_credit_balance,
			paid_credit_balance,
			created_at,
			updated_at
		FROM users
		ORDER BY created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer rows.Close()

	users := make([]userResponse, 0)
	for rows.Next() {
		var user userResponse
		if err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Role,
			&user.Status,
			&user.CreditBalance,
			&user.DailyFreeCreditLimit,
			&user.DailyFreeCreditBalance,
			&user.PaidCreditBalance,
			&user.CreatedAt,
			&user.UpdatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "服务器错误")
			return
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]userResponse{"users": users})
}

func (h Handlers) ChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer tx.Rollback(r.Context())

	if err := (auth.Service{DB: h.DB}).ChangePasswordTx(r.Context(), tx, actor.ID, req.CurrentPassword, req.NewPassword); err != nil {
		writePasswordError(w, err)
		return
	}
	if err := insertAuditLog(r.Context(), tx, actor.ID, actor.ID, "change_own_password", map[string]any{"username": actor.Username}); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handlers) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}

	userID, ok := validRouteUUID(w, r, "id")
	if !ok {
		return
	}

	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer tx.Rollback(r.Context())

	target, err := findUserSummaryTx(r.Context(), tx, userID)
	if err != nil {
		writePasswordError(w, err)
		return
	}
	if err := (auth.Service{DB: h.DB}).ResetPasswordTx(r.Context(), tx, userID, req.NewPassword); err != nil {
		writePasswordError(w, err)
		return
	}
	if err := insertAuditLog(r.Context(), tx, actor.ID, userID, "reset_user_password", map[string]any{"username": target.Username}); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handlers) UpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}

	userID, ok := validRouteUUID(w, r, "id")
	if !ok {
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if req.Status != models.UserStatusActive && req.Status != models.UserStatusDisabled {
		writeError(w, http.StatusBadRequest, "用户状态无效")
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer tx.Rollback(r.Context())

	user, err := updateUserStatus(r.Context(), tx, userID, req.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "用户不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	if err := insertAuditLog(r.Context(), tx, actor.ID, userID, "update_user_status", map[string]any{"status": req.Status}); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string]userResponse{"user": user})
}

func (h Handlers) AdjustCredits(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}

	userID, ok := validRouteUUID(w, r, "id")
	if !ok {
		return
	}

	var req struct {
		Amount int    `json:"amount"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if validateCreditAdjustmentAmount(req.Amount) != nil || req.Reason == "" {
		writeError(w, http.StatusBadRequest, "积分调整信息无效")
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer tx.Rollback(r.Context())

	user, err := adjustCredits(r.Context(), tx, userID, req.Amount, req.Reason, actor.ID)
	if err != nil {
		writeCreditError(w, err)
		return
	}
	if err := insertAuditLog(r.Context(), tx, actor.ID, userID, "adjust_credits", map[string]any{"amount": req.Amount, "reason": req.Reason}); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]userResponse{"user": user})
}

func (h Handlers) ListInvites(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT id::text, code, initial_credits, status, COALESCE(created_by::text, ''), COALESCE(used_by::text, ''), used_at, created_at
		FROM invites
		ORDER BY created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer rows.Close()

	invites := make([]inviteResponse, 0)
	for rows.Next() {
		var invite inviteResponse
		var usedAt *time.Time
		if err := rows.Scan(&invite.ID, &invite.Code, &invite.InitialCredits, &invite.Status, &invite.CreatedBy, &invite.UsedBy, &usedAt, &invite.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "服务器错误")
			return
		}
		invite.UsedAt = usedAt
		invites = append(invites, invite)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]inviteResponse{"invites": invites})
}

func (h Handlers) CreateInvite(w http.ResponseWriter, r *http.Request) {
	actor, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}

	var req struct {
		Code           string `json:"code"`
		InitialCredits int    `json:"initial_credits"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if validateInviteInitialCredits(req.InitialCredits) != nil {
		writeError(w, http.StatusBadRequest, "初始积分无效")
		return
	}
	if req.Code == "" {
		code, err := generateInviteCode()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "服务器错误")
			return
		}
		req.Code = code
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer tx.Rollback(r.Context())

	var invite inviteResponse
	err = tx.QueryRow(r.Context(), `
		INSERT INTO invites (code, initial_credits, status, created_by)
		VALUES ($1, $2, 'unused', $3::uuid)
		RETURNING id::text, code, initial_credits, status, COALESCE(created_by::text, ''), COALESCE(used_by::text, ''), used_at, created_at
	`, req.Code, req.InitialCredits, actor.ID).Scan(
		&invite.ID,
		&invite.Code,
		&invite.InitialCredits,
		&invite.Status,
		&invite.CreatedBy,
		&invite.UsedBy,
		&invite.UsedAt,
		&invite.CreatedAt,
	)
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "邀请码已存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	if err := insertAuditLog(r.Context(), tx, actor.ID, "", "create_invite", map[string]any{"code": invite.Code, "initial_credits": invite.InitialCredits}); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]inviteResponse{"invite": invite})
}

func (h Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT id::text, COALESCE(actor_user_id::text, ''), COALESCE(target_user_id::text, ''), action, metadata, created_at
		FROM audit_logs
		ORDER BY created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer rows.Close()

	logs := make([]auditLogResponse, 0)
	for rows.Next() {
		var log auditLogResponse
		if err := rows.Scan(&log.ID, &log.ActorUserID, &log.TargetUserID, &log.Action, &log.Metadata, &log.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "服务器错误")
			return
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]auditLogResponse{"audit_logs": logs})
}

func (h Handlers) ListGenerationTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT generation_tasks.id::text,
			generation_tasks.user_id::text,
			users.username,
			generation_tasks.prompt,
			generation_tasks.size,
			generation_tasks.status,
			COALESCE(generation_tasks.latency_ms, 0),
			COALESCE(generation_tasks.error_code, ''),
			COALESCE(generation_tasks.error_message, ''),
			generation_tasks.created_at,
			generation_tasks.completed_at
		FROM generation_tasks
		JOIN users ON users.id = generation_tasks.user_id
		WHERE generation_tasks.deleted_at IS NULL
		ORDER BY generation_tasks.created_at DESC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	defer rows.Close()

	tasks := make([]generationTaskResponse, 0)
	for rows.Next() {
		var task generationTaskResponse
		if err := rows.Scan(&task.ID, &task.UserID, &task.Username, &task.Prompt, &task.Size, &task.Status, &task.LatencyMS, &task.ErrorCode, &task.ErrorMessage, &task.CreatedAt, &task.CompletedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "服务器错误")
			return
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "服务器错误")
		return
	}

	writeJSON(w, http.StatusOK, map[string][]generationTaskResponse{"tasks": tasks})
}

type userResponse struct {
	ID                     string    `json:"id"`
	Username               string    `json:"username"`
	Role                   string    `json:"role"`
	Status                 string    `json:"status"`
	CreditBalance          int       `json:"credit_balance"`
	DailyFreeCreditLimit   int       `json:"daily_free_credit_limit"`
	DailyFreeCreditBalance int       `json:"daily_free_credit_balance"`
	PaidCreditBalance      int       `json:"paid_credit_balance"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type userSummary struct {
	ID       string
	Username string
}

type inviteResponse struct {
	ID             string     `json:"id"`
	Code           string     `json:"code"`
	InitialCredits int        `json:"initial_credits"`
	Status         string     `json:"status"`
	CreatedBy      string     `json:"created_by,omitempty"`
	UsedBy         string     `json:"used_by,omitempty"`
	UsedAt         *time.Time `json:"used_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type auditLogResponse struct {
	ID           string          `json:"id"`
	ActorUserID  string          `json:"actor_user_id,omitempty"`
	TargetUserID string          `json:"target_user_id,omitempty"`
	Action       string          `json:"action"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
}

type generationTaskResponse struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Username     string     `json:"username"`
	Prompt       string     `json:"prompt"`
	Size         string     `json:"size"`
	Status       string     `json:"status"`
	LatencyMS    int        `json:"latency_ms"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

func updateUserStatus(ctx context.Context, tx pgx.Tx, userID, status string) (userResponse, error) {
	var user userResponse
	err := tx.QueryRow(ctx, `
		UPDATE users
		SET status = $2,
			updated_at = now()
		WHERE id = $1::uuid
		RETURNING id::text,
			username,
			role,
			status,
			credit_balance,
			daily_free_credit_limit,
			daily_free_credit_balance,
			paid_credit_balance,
			created_at,
			updated_at
	`, userID, status).Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
		&user.DailyFreeCreditLimit,
		&user.DailyFreeCreditBalance,
		&user.PaidCreditBalance,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	return user, err
}

func adjustCredits(ctx context.Context, tx pgx.Tx, userID string, amount int, reason string, actorUserID string) (userResponse, error) {
	var user userResponse
	err := tx.QueryRow(ctx, `
		UPDATE users
		SET paid_credit_balance = (paid_credit_balance::bigint + $2::bigint)::integer,
			credit_balance = (daily_free_credit_balance::bigint + paid_credit_balance::bigint + $2::bigint)::integer,
			updated_at = now()
		WHERE id = $1::uuid
			AND paid_credit_balance::bigint + $2::bigint >= 0
			AND daily_free_credit_balance::bigint + paid_credit_balance::bigint + $2::bigint BETWEEN 0 AND 2147483647
		RETURNING id::text,
			username,
			role,
			status,
			credit_balance,
			daily_free_credit_limit,
			daily_free_credit_balance,
			paid_credit_balance,
			created_at,
			updated_at
	`, userID, amount).Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&user.Status,
		&user.CreditBalance,
		&user.DailyFreeCreditLimit,
		&user.DailyFreeCreditBalance,
		&user.PaidCreditBalance,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		var freeBalance, paidBalance int
		if err := tx.QueryRow(ctx, `
			SELECT daily_free_credit_balance, paid_credit_balance
			FROM users
			WHERE id = $1::uuid
		`, userID).Scan(&freeBalance, &paidBalance); errors.Is(err, pgx.ErrNoRows) {
			return userResponse{}, credits.ErrUserNotFound
		} else if err != nil {
			return userResponse{}, err
		}

		finalPaidBalance := int64(paidBalance) + int64(amount)
		if finalPaidBalance < 0 {
			return userResponse{}, credits.ErrInsufficientCredits
		}
		finalBalance := int64(freeBalance) + finalPaidBalance
		if finalBalance < 0 || finalBalance > int64(maxPostgresInteger) {
			return userResponse{}, errCreditBalanceOutOfRange
		}
		return userResponse{}, errCreditBalanceOutOfRange
	}
	if err != nil {
		return userResponse{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO credit_ledger (user_id, type, wallet_type, amount, balance_after, reason, actor_user_id)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::uuid)
	`, userID, models.LedgerPaidAdminAdjustment, models.WalletPaid, amount, user.CreditBalance, reason, actorUserID); err != nil {
		return userResponse{}, err
	}

	return user, nil
}

func findUserSummary(ctx context.Context, db *pgxpool.Pool, userID string) (userSummary, error) {
	return scanUserSummary(ctx, db, userID)
}

func findUserSummaryTx(ctx context.Context, tx pgx.Tx, userID string) (userSummary, error) {
	return scanUserSummary(ctx, tx, userID)
}

type userSummaryQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func scanUserSummary(ctx context.Context, q userSummaryQuerier, userID string) (userSummary, error) {
	var user userSummary
	err := q.QueryRow(ctx, `
		SELECT id::text, username
		FROM users
		WHERE id = $1::uuid
	`, userID).Scan(&user.ID, &user.Username)
	if errors.Is(err, pgx.ErrNoRows) {
		return userSummary{}, auth.ErrUserNotFound
	}
	return user, err
}

func validateInviteInitialCredits(value int) error {
	if value < 0 || !isPostgresInteger(value) {
		return fmt.Errorf("initial_credits out of range")
	}
	return nil
}

func validateCreditAdjustmentAmount(value int) error {
	if value == 0 || !isPostgresInteger(value) {
		return fmt.Errorf("amount out of range")
	}
	return nil
}

func isPostgresInteger(value int) bool {
	return value >= minPostgresInteger && value <= maxPostgresInteger
}

func insertAuditLog(ctx context.Context, tx pgx.Tx, actorUserID, targetUserID, action string, metadata map[string]any) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (actor_user_id, target_user_id, action, metadata)
		VALUES ($1::uuid, NULLIF($2, '')::uuid, $3, $4::jsonb)
	`, actorUserID, targetUserID, action, string(metadataJSON))
	return err
}

func generateInviteCode() (string, error) {
	var data [12]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return "invite-" + hex.EncodeToString(data[:]), nil
}

func validRouteUUID(w http.ResponseWriter, r *http.Request, param string) (string, bool) {
	value := chi.URLParam(r, param)
	if !isCanonicalUUID(value) {
		writeError(w, http.StatusBadRequest, "编号格式错误")
		return "", false
	}
	return value, true
}

func isCanonicalUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, r := range value {
		switch i {
		case 8, 13, 18, 23:
			if r != '-' {
				return false
			}
		default:
			if !isHex(r) {
				return false
			}
		}
	}
	return true
}

func isHex(r rune) bool {
	return (r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'f') ||
		(r >= 'A' && r <= 'F')
}

func writeCreditError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, credits.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "用户不存在")
	case errors.Is(err, errCreditBalanceOutOfRange):
		writeError(w, http.StatusBadRequest, "积分余额超出范围")
	case errors.Is(err, credits.ErrInsufficientCredits):
		writeError(w, http.StatusPaymentRequired, "积分不足")
	default:
		writeError(w, http.StatusInternalServerError, "服务器错误")
	}
}

func writePasswordError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		writeError(w, http.StatusBadRequest, "新密码至少 6 位")
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, "当前密码错误")
	case errors.Is(err, auth.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "用户不存在")
	default:
		writeError(w, http.StatusInternalServerError, "服务器错误")
	}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
