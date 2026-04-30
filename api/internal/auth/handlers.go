package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type HandlerOptions struct {
	CookieSecure  bool
	SessionSecret string
}

type Handlers struct {
	Service      Service
	CookieSecure bool
	SessionCodec SessionCodec
}

func NewHandlers(service Service, opts HandlerOptions) Handlers {
	return Handlers{
		Service:      service,
		CookieSecure: opts.CookieSecure,
		SessionCodec: NewSessionCodec(opts.SessionSecret),
	}
}

func (h Handlers) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	user, err := h.Service.Register(r.Context(), req.Username, req.Password, req.InviteCode)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	if err := setSessionCookie(w, user.ID, h.cookieSecure(r), h.SessionCodec); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]User{"user": user})
}

func (h Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "请求格式错误")
		return
	}

	user, err := h.Service.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}

	if err := setSessionCookie(w, user.ID, h.cookieSecure(r), h.SessionCodec); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "服务器错误")
		return
	}
	writeJSON(w, http.StatusOK, map[string]User{"user": user})
}

func (h Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w, h.cookieSecure(r))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h Handlers) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := CurrentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	writeJSON(w, http.StatusOK, map[string]User{"user": user})
}

func (h Handlers) cookieSecure(r *http.Request) bool {
	return h.CookieSecure || r.TLS != nil
}

func setSessionCookie(w http.ResponseWriter, userID string, secure bool, codec SessionCodec) error {
	cookieValue, err := codec.Sign(userID)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    cookieValue,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		writeJSONError(w, http.StatusBadRequest, "请填写完整信息")
	case errors.Is(err, ErrInvalidInvite):
		writeJSONError(w, http.StatusBadRequest, "邀请码无效或已使用")
	case errors.Is(err, ErrDuplicateUsername):
		writeJSONError(w, http.StatusConflict, "用户名已存在")
	case errors.Is(err, ErrPasswordTooShort):
		writeJSONError(w, http.StatusBadRequest, "新密码至少 6 位")
	case errors.Is(err, ErrInvalidCredentials):
		writeJSONError(w, http.StatusUnauthorized, "用户名或密码错误")
	case errors.Is(err, ErrDisabledUser):
		writeJSONError(w, http.StatusForbidden, "账号已被禁用")
	default:
		writeJSONError(w, http.StatusInternalServerError, "服务器错误")
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
