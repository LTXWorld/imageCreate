package generations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"imagecreate/api/internal/auth"
	"imagecreate/api/internal/models"
)

const contentRejectedMessage = "提示词可能包含不支持生成的内容，请调整描述后重试。"

type TaskService interface {
	CreateTask(ctx context.Context, input CreateTaskInput) (Task, error)
	GetTaskForUser(ctx context.Context, userID, taskID string) (Task, error)
	ListTasksForUser(ctx context.Context, userID string) ([]Task, error)
	DeleteTaskForUser(ctx context.Context, userID, taskID string) error
}

type Handlers struct {
	Service TaskService
	Storage ImageStorage
}

func NewHandlers(service TaskService, storage ImageStorage) Handlers {
	return Handlers{Service: service, Storage: storage}
}

func (h Handlers) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}

	var req struct {
		Prompt string `json:"prompt"`
		Ratio  string `json:"ratio"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "请求格式错误")
		return
	}

	task, err := h.Service.CreateTask(r.Context(), CreateTaskInput{
		UserID: user.ID,
		Prompt: req.Prompt,
		Ratio:  req.Ratio,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]taskResponse{"task": newTaskResponse(task)})
}

func (h Handlers) List(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}

	tasks, err := h.Service.ListTasksForUser(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "服务器错误")
		return
	}

	resp := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		resp = append(resp, newTaskResponse(task))
	}
	writeJSON(w, http.StatusOK, map[string][]taskResponse{"tasks": resp})
}

func (h Handlers) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}

	taskID, ok := validTaskID(w, r)
	if !ok {
		return
	}

	task, err := h.Service.GetTaskForUser(r.Context(), user.ID, taskID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]taskResponse{"task": newTaskResponse(task)})
}

func (h Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}

	taskID, ok := validTaskID(w, r)
	if !ok {
		return
	}

	if err := h.Service.DeleteTaskForUser(r.Context(), user.ID, taskID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handlers) Image(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.CurrentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}

	taskID, ok := validTaskID(w, r)
	if !ok {
		return
	}

	task, err := h.Service.GetTaskForUser(r.Context(), user.ID, taskID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if task.Status != models.TaskSucceeded || task.ImagePath == "" {
		writeError(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}

	file, err := h.Storage.Open(r.Context(), task.ImagePath)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "图片不存在")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "image/png")
	http.ServeContent(w, r, "image.png", time.Time{}, file)
}

type taskResponse struct {
	ID        string `json:"id"`
	Prompt    string `json:"prompt"`
	Size      string `json:"size"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	Message   string `json:"message,omitempty"`
}

func newTaskResponse(task Task) taskResponse {
	resp := taskResponse{
		ID:     task.ID,
		Prompt: task.Prompt,
		Size:   task.Size,
		Status: task.Status,
	}
	if task.Status == models.TaskFailed {
		resp.ErrorCode = stableErrorCode(task.ErrorCode)
		resp.Message = userFacingFailureMessage(resp.ErrorCode)
	}
	return resp
}

func stableErrorCode(code string) string {
	if code == "" {
		return "upstream_error"
	}
	return code
}

func userFacingFailureMessage(code string) string {
	switch code {
	case "content_rejected":
		return contentRejectedMessage
	case "rate_limited":
		return "当前生成服务繁忙，请稍后再试。"
	case "timeout":
		return "生成超时，本次额度已退回，请稍后重试。"
	default:
		return "生成失败，本次额度已退回。"
	}
}

func validTaskID(w http.ResponseWriter, r *http.Request) (string, bool) {
	taskID := chi.URLParam(r, "id")
	if !isCanonicalUUID(taskID) {
		writeError(w, http.StatusBadRequest, "invalid_task_id", "任务编号格式错误")
		return "", false
	}
	return taskID, true
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

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidPrompt):
		writeError(w, http.StatusBadRequest, "invalid_prompt", "请填写提示词")
	case errors.Is(err, ErrUnsupportedRatio):
		writeError(w, http.StatusBadRequest, "unsupported_ratio", "不支持的图片比例")
	case errors.Is(err, ErrInsufficientCredits):
		writeError(w, http.StatusPaymentRequired, "insufficient_credits", "积分不足")
	case errors.Is(err, ErrActiveTaskExists):
		writeError(w, http.StatusConflict, "active_task_exists", "已有生成任务正在进行")
	case errors.Is(err, ErrTaskActive):
		writeError(w, http.StatusConflict, "task_active", "任务正在进行中")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "任务不存在")
	case errors.Is(err, ErrDisabledUser):
		writeError(w, http.StatusForbidden, "disabled_user", "账号已被禁用")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "服务器错误")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{
		"error_code": code,
		"message":    message,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
