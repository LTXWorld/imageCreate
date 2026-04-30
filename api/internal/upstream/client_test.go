package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerateImageSendsExpectedRequest(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	var gotBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("X-Request-Id", "req-test-123")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"gen-test-456","data":[{"b64_json":"aW1hZ2UtYnl0ZXM="}]}`))
	}))
	defer server.Close()

	client := Client{
		BaseURL:    server.URL,
		APIKey:     "test-api-key",
		Model:      "gpt-image-2",
		HTTPClient: server.Client(),
	}

	result, err := client.GenerateImage(context.Background(), "draw a moonlit garden", "1024x1024")
	if err != nil {
		t.Fatalf("generate image: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("path = %q, want /v1/images/generations", gotPath)
	}
	if gotAuth != "Bearer test-api-key" {
		t.Fatalf("authorization = %q, want bearer API key", gotAuth)
	}

	want := map[string]any{
		"model":         "gpt-image-2",
		"prompt":        "draw a moonlit garden",
		"n":             float64(1),
		"size":          "1024x1024",
		"quality":       "auto",
		"output_format": "png",
		"background":    "auto",
	}
	for key, wantValue := range want {
		if gotBody[key] != wantValue {
			t.Fatalf("body[%q] = %#v, want %#v", key, gotBody[key], wantValue)
		}
	}
	if string(result.ImageBytes) != "image-bytes" {
		t.Fatalf("image bytes = %q, want decoded response bytes", string(result.ImageBytes))
	}
	if result.RequestID != "req-test-123" {
		t.Fatalf("request ID = %q, want req-test-123", result.RequestID)
	}
}

func TestGenerateImageDoesNotDuplicateV1WhenBaseURLIncludesVersion(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1hZ2UtYnl0ZXM="}]}`))
	}))
	defer server.Close()

	client := Client{
		BaseURL:    server.URL + "/v1",
		APIKey:     "test-api-key",
		Model:      "gpt-image-2",
		HTTPClient: server.Client(),
	}

	if _, err := client.GenerateImage(context.Background(), "draw a moonlit garden", "1024x1024"); err != nil {
		t.Fatalf("generate image: %v", err)
	}
	if gotPath != "/v1/images/generations" {
		t.Fatalf("path = %q, want /v1/images/generations", gotPath)
	}
}

func TestGenerateImageMapsContentRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"content_policy_violation","message":"prompt violates policy"}}`, http.StatusBadRequest)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, APIKey: "test-api-key", Model: "gpt-image-2", HTTPClient: server.Client()}
	result, err := client.GenerateImage(context.Background(), "blocked prompt", "1024x1024")
	if err == nil {
		t.Fatal("generate image error = nil, want content rejection error")
	}
	if result.ErrorCode != "content_rejected" {
		t.Fatalf("error code = %q, want content_rejected", result.ErrorCode)
	}
	if !errors.Is(err, ErrContentRejected) {
		t.Fatalf("error = %v, want ErrContentRejected", err)
	}
}

func TestGenerateImageLogsUpstreamHTTPFailureMetadata(t *testing.T) {
	const apiKey = "sk-test-secret-key"
	const prompt = "draw a private prompt"
	var logs bytes.Buffer

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-failed-123")
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":{"message":"provider exploded"}}`, http.StatusBadGateway)
	}))
	defer server.Close()

	client := Client{
		BaseURL:    server.URL,
		APIKey:     apiKey,
		Model:      "gpt-image-2",
		HTTPClient: server.Client(),
		Logger:     log.New(&logs, "", 0),
	}

	result, err := client.GenerateImage(context.Background(), prompt, "1024x1024")
	if err == nil {
		t.Fatal("generate image error = nil, want upstream error")
	}
	if result.ErrorCode != "upstream_error" {
		t.Fatalf("error code = %q, want upstream_error", result.ErrorCode)
	}

	output := logs.String()
	for _, want := range []string{
		"upstream_request_start",
		"upstream_request_finished",
		"endpoint=" + server.URL + "/v1/images/generations",
		"status=502",
		"request_id=req-failed-123",
		"error_code=upstream_error",
		"elapsed_ms=",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("logs %q missing %q", output, want)
		}
	}
	if strings.Contains(output, apiKey) {
		t.Fatalf("logs leaked API key: %q", output)
	}
	if strings.Contains(output, prompt) {
		t.Fatalf("logs leaked prompt: %q", output)
	}
}

func TestGenerateImageDoesNotMapMalformedContentRequestToRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"invalid_request_error","message":"content must be a string"}}`, http.StatusBadRequest)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, APIKey: "test-api-key", Model: "gpt-image-2", HTTPClient: server.Client()}
	result, err := client.GenerateImage(context.Background(), "draw a comet", "1024x1024")
	if err == nil {
		t.Fatal("generate image error = nil, want upstream error")
	}
	if result.ErrorCode != "upstream_error" {
		t.Fatalf("error code = %q, want upstream_error", result.ErrorCode)
	}
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("error = %v, want ErrUpstream", err)
	}
}

func TestGenerateImageSanitizesClassifiedUpstreamErrors(t *testing.T) {
	const apiKey = "sk-test-secret-key"

	tests := []struct {
		name        string
		status      int
		body        string
		wantCode    string
		wantMessage string
	}{
		{
			name:        "bad request policy",
			status:      http.StatusBadRequest,
			body:        `{"error":{"code":"content_policy_violation","message":"policy rejected request with sk-test-secret-key"}}`,
			wantCode:    "content_rejected",
			wantMessage: "upstream rejected the requested content",
		},
		{
			name:        "forbidden policy",
			status:      http.StatusForbidden,
			body:        `{"error":{"code":"content_policy_violation","message":"policy rejected request with sk-test-secret-key"}}`,
			wantCode:    "content_rejected",
			wantMessage: "upstream rejected the requested content",
		},
		{
			name:        "rate limited",
			status:      http.StatusTooManyRequests,
			body:        `{"error":{"code":"rate_limit_exceeded","message":"too many requests for sk-test-secret-key"}}`,
			wantCode:    "rate_limited",
			wantMessage: "upstream rate limited the request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, tt.body, tt.status)
			}))
			defer server.Close()

			client := Client{BaseURL: server.URL, APIKey: apiKey, Model: "gpt-image-2", HTTPClient: server.Client()}
			result, err := client.GenerateImage(context.Background(), "draw a comet", "1024x1024")
			if err == nil {
				t.Fatal("generate image error = nil, want classified upstream error")
			}
			if result.ErrorCode != tt.wantCode {
				t.Fatalf("error code = %q, want %q", result.ErrorCode, tt.wantCode)
			}
			if result.ErrorMessage != tt.wantMessage {
				t.Fatalf("error message = %q, want %q", result.ErrorMessage, tt.wantMessage)
			}
			if strings.Contains(err.Error(), apiKey) {
				t.Fatalf("error %q contains API key", err.Error())
			}
			if strings.Contains(result.ErrorMessage, apiKey) {
				t.Fatalf("result error message %q contains API key", result.ErrorMessage)
			}
		})
	}
}

func TestGenerateImageMapsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	client := Client{BaseURL: server.URL, APIKey: "test-api-key", Model: "gpt-image-2", HTTPClient: server.Client()}
	result, err := client.GenerateImage(ctx, "draw slowly", "1024x1024")
	if err == nil {
		t.Fatal("generate image error = nil, want timeout error")
	}
	if result.ErrorCode != "timeout" {
		t.Fatalf("error code = %q, want timeout", result.ErrorCode)
	}
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v, want ErrTimeout", err)
	}
}

func TestGenerateImageDoesNotExposeAPIKeyInErrors(t *testing.T) {
	const apiKey = "sk-test-secret-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "provider exploded", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, APIKey: apiKey, Model: "gpt-image-2", HTTPClient: server.Client()}
	result, err := client.GenerateImage(context.Background(), "draw a comet", "1024x1024")
	if err == nil {
		t.Fatal("generate image error = nil, want upstream error")
	}
	if result.ErrorCode != "upstream_error" {
		t.Fatalf("error code = %q, want upstream_error", result.ErrorCode)
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatalf("error %q contains API key", err.Error())
	}
	if strings.Contains(result.ErrorMessage, apiKey) {
		t.Fatalf("result error message %q contains API key", result.ErrorMessage)
	}
}
