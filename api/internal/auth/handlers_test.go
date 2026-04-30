package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterMapsShortPasswordToClientError(t *testing.T) {
	handlers := NewHandlers(Service{}, HandlerOptions{SessionSecret: "test-secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{
		"username": "short-password",
		"password": "12345",
		"invite_code": "invite-code"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handlers.Register(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "新密码至少 6 位" {
		t.Fatalf("error = %q, want %q", body["error"], "新密码至少 6 位")
	}
}
