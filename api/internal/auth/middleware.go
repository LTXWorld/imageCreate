package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"imagecreate/api/internal/models"
)

const SessionCookieName = "imagecreate_session"

var ErrSessionSecretRequired = errors.New("session secret required")

type SessionCodec struct {
	secret []byte
}

func NewSessionCodec(secret string) SessionCodec {
	return SessionCodec{secret: []byte(secret)}
}

func (c SessionCodec) Sign(userID string) (string, error) {
	if userID == "" {
		return "", ErrInvalidInput
	}
	if len(c.secret) == 0 {
		return "", ErrSessionSecretRequired
	}

	encodedUserID := base64.RawURLEncoding.EncodeToString([]byte(userID))
	signature := c.signature(encodedUserID)
	return encodedUserID + "." + signature, nil
}

func (c SessionCodec) Verify(cookieValue string) (string, bool) {
	if len(c.secret) == 0 {
		return "", false
	}

	encodedUserID, signature, ok := strings.Cut(cookieValue, ".")
	if !ok || encodedUserID == "" || signature == "" {
		return "", false
	}

	if !hmac.Equal([]byte(signature), []byte(c.signature(encodedUserID))) {
		return "", false
	}

	userID, err := base64.RawURLEncoding.DecodeString(encodedUserID)
	if err != nil || len(userID) == 0 {
		return "", false
	}

	return string(userID), true
}

func (c SessionCodec) signature(encodedUserID string) string {
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write([]byte(encodedUserID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type userContextKey struct{}

func WithUser(service Service, codec SessionCodec) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}

			userID, ok := codec.Verify(cookie.Value)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			user, err := service.userByID(r.Context(), userID)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), userContextKey{}, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := CurrentUser(r); !ok {
			writeMiddlewareError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := CurrentUser(r)
		if !ok {
			writeMiddlewareError(w, http.StatusUnauthorized, "请先登录")
			return
		}
		if user.Role != models.RoleAdmin {
			writeMiddlewareError(w, http.StatusForbidden, "需要管理员权限")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func CurrentUser(r *http.Request) (User, bool) {
	user, ok := r.Context().Value(userContextKey{}).(User)
	return user, ok
}

func writeMiddlewareError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
