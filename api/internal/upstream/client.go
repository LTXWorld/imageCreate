package upstream

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

var (
	ErrContentRejected = errors.New("content_rejected")
	ErrRateLimited     = errors.New("rate_limited")
	ErrTimeout         = errors.New("timeout")
	ErrUpstream        = errors.New("upstream_error")
)

type Client struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
	Logger     *log.Logger
}

type Result struct {
	RequestID    string
	ImageBytes   []byte
	ErrorCode    string
	ErrorMessage string
}

type generateRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	N            int    `json:"n"`
	Size         string `json:"size"`
	Quality      string `json:"quality"`
	OutputFormat string `json:"output_format"`
	Background   string `json:"background"`
}

type generateResponse struct {
	ID   string `json:"id"`
	Data []struct {
		B64JSON string `json:"b64_json"`
	} `json:"data"`
}

type upstreamErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

func (c Client) GenerateImage(ctx context.Context, prompt, size string) (Result, error) {
	if err := ctx.Err(); err != nil {
		return errorResult("timeout", sanitizedMessage("timeout")), ErrTimeout
	}

	body, err := json.Marshal(generateRequest{
		Model:        c.Model,
		Prompt:       prompt,
		N:            1,
		Size:         size,
		Quality:      "auto",
		OutputFormat: "png",
		Background:   "auto",
	})
	if err != nil {
		return errorResult("upstream_error", "encode upstream request"), fmt.Errorf("%w: encode request", ErrUpstream)
	}

	endpoint := imageGenerationEndpoint(c.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errorResult("upstream_error", "create upstream request"), fmt.Errorf("%w: create request", ErrUpstream)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	started := time.Now()
	logf(c.Logger, "upstream_request_start endpoint=%s model=%s size=%s", endpoint, c.Model, size)

	resp, err := httpClient.Do(req)
	elapsedMS := elapsedMilliseconds(started)
	if err != nil {
		if isContextTimeout(ctx, err) {
			logf(c.Logger, "upstream_request_error endpoint=%s model=%s size=%s elapsed_ms=%d error_code=timeout ctx_err=%v", endpoint, c.Model, size, elapsedMS, ctx.Err())
			return errorResult("timeout", sanitizedMessage("timeout")), ErrTimeout
		}
		logf(c.Logger, "upstream_request_error endpoint=%s model=%s size=%s elapsed_ms=%d error_code=upstream_error err=%q ctx_err=%v", endpoint, c.Model, size, elapsedMS, err.Error(), ctx.Err())
		return errorResult("upstream_error", sanitizedMessage("upstream_error")), fmt.Errorf("%w: %s", ErrUpstream, sanitizedMessage("upstream_error"))
	}
	defer resp.Body.Close()

	requestID := requestIDFromResponse(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result, handleErr := c.handleErrorResponse(resp, requestID)
		logf(c.Logger, "upstream_request_finished endpoint=%s model=%s size=%s status=%d request_id=%s elapsed_ms=%d error_code=%s", endpoint, c.Model, size, resp.StatusCode, requestID, elapsedMS, result.ErrorCode)
		return result, handleErr
	}
	var decoded generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		logf(c.Logger, "upstream_response_decode_failed endpoint=%s model=%s size=%s status=%d request_id=%s elapsed_ms=%d error_code=upstream_error", endpoint, c.Model, size, resp.StatusCode, requestID, elapsedMS)
		return Result{RequestID: requestID, ErrorCode: "upstream_error", ErrorMessage: "decode upstream response"}, fmt.Errorf("%w: decode response", ErrUpstream)
	}
	if requestID == "" && decoded.ID != "" {
		requestID = decoded.ID
	}
	if len(decoded.Data) == 0 || decoded.Data[0].B64JSON == "" {
		logf(c.Logger, "upstream_response_missing_image endpoint=%s model=%s size=%s status=%d request_id=%s elapsed_ms=%d error_code=upstream_error", endpoint, c.Model, size, resp.StatusCode, requestID, elapsedMS)
		return Result{RequestID: requestID, ErrorCode: "upstream_error", ErrorMessage: "upstream response missing image"}, fmt.Errorf("%w: missing image", ErrUpstream)
	}

	imageBytes, err := base64.StdEncoding.DecodeString(decoded.Data[0].B64JSON)
	if err != nil {
		logf(c.Logger, "upstream_image_decode_failed endpoint=%s model=%s size=%s status=%d request_id=%s elapsed_ms=%d error_code=upstream_error", endpoint, c.Model, size, resp.StatusCode, requestID, elapsedMS)
		return Result{RequestID: requestID, ErrorCode: "upstream_error", ErrorMessage: "decode upstream image"}, fmt.Errorf("%w: decode image", ErrUpstream)
	}

	logf(c.Logger, "upstream_request_finished endpoint=%s model=%s size=%s status=%d request_id=%s elapsed_ms=%d error_code=", endpoint, c.Model, size, resp.StatusCode, requestID, elapsedMS)
	return Result{RequestID: requestID, ImageBytes: imageBytes}, nil
}

func imageGenerationEndpoint(baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/images/generations"
	}
	return base + "/v1/images/generations"
}

func (c Client) handleErrorResponse(resp *http.Response, requestID string) (Result, error) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	code := "upstream_error"
	sentinel := ErrUpstream
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		code = "rate_limited"
		sentinel = ErrRateLimited
	case (resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusForbidden) && isPolicyError(body):
		code = "content_rejected"
		sentinel = ErrContentRejected
	}

	message := sanitizedMessage(code)
	return Result{RequestID: requestID, ErrorCode: code, ErrorMessage: message}, fmt.Errorf("%w: %s", sentinel, message)
}

func requestIDFromResponse(resp *http.Response) string {
	for _, header := range []string{"X-Request-Id", "X-Request-ID", "OpenAI-Request-ID"} {
		if value := resp.Header.Get(header); value != "" {
			return value
		}
	}
	return ""
}

func isPolicyError(body []byte) bool {
	var decoded upstreamErrorBody
	if err := json.Unmarshal(body, &decoded); err == nil {
		codeAndType := strings.ToLower(decoded.Error.Code + " " + decoded.Error.Type)
		message := strings.ToLower(decoded.Error.Message)
		return containsPolicySignal(codeAndType) || containsPolicyPhrase(message)
	}
	lowerBody := strings.ToLower(string(body))
	return containsPolicyPhrase(lowerBody)
}

func containsPolicySignal(value string) bool {
	return strings.Contains(value, "policy") ||
		strings.Contains(value, "safety") ||
		strings.Contains(value, "moderation")
}

func containsPolicyPhrase(value string) bool {
	policyPhrases := []string{
		"content policy",
		"safety policy",
		"violates policy",
		"violate policy",
		"policy violation",
		"rejected by policy",
		"blocked by policy",
		"flagged by safety",
		"safety violation",
		"moderation",
	}
	for _, phrase := range policyPhrases {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

func sanitizedMessage(code string) string {
	switch code {
	case "content_rejected":
		return "upstream rejected the requested content"
	case "rate_limited":
		return "upstream rate limited the request"
	case "timeout":
		return "upstream request timed out"
	default:
		return "upstream image generation failed"
	}
}

func errorResult(code, message string) Result {
	return Result{ErrorCode: code, ErrorMessage: message}
}

func logf(logger *log.Logger, format string, args ...any) {
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf(format, args...)
}

func elapsedMilliseconds(started time.Time) int {
	return int(time.Since(started).Milliseconds())
}

func isContextTimeout(ctx context.Context, err error) bool {
	return errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		errors.Is(ctx.Err(), context.Canceled)
}
