package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mcpconfig"
	"github.com/javdet/nib/internal/service"
	"github.com/javdet/nib/internal/systemprompts"
)

func TestHandleServiceError_tooManyToolFailures(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	handleServiceError(rec, service.ErrTooManyToolFailures)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Error == "" || errors.Is(errors.New(body.Error), service.ErrTooManyToolFailures) {
		t.Fatalf("message = %q, want readable user-facing text", body.Error)
	}
}

// The tools dialog shows the "error" field verbatim, so a failure to reach an
// MCP server has to arrive as its own message rather than a generic 500.
func TestHandleServiceError_mcpDiscoveryFailed(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	handleServiceError(rec, fmt.Errorf("%w: mcp connect: connection refused", mcpconfig.ErrDiscoveryFailed))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !strings.Contains(body.Error, "connection refused") {
		t.Fatalf("message = %q, want the transport cause", body.Error)
	}
}

func TestHandleServiceError_mcpInvalidHeader(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	handleServiceError(rec, fmt.Errorf("%w: %q holds a newline", mcpconfig.ErrInvalidHeader, "Authorization"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleServiceError_promptNotEditable(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	handleServiceError(rec, fmt.Errorf("%w: plan", systemprompts.ErrNotEditable))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestLLMErrorResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "payment required",
			statusCode: http.StatusPaymentRequired,
			wantStatus: http.StatusPaymentRequired,
			wantMsg:    "The AI provider rejected the request: not enough credits on the API key. Top up the provider balance or lower the model's max tokens limit, then retry.",
		},
		{
			name:       "rate limit",
			statusCode: http.StatusTooManyRequests,
			wantStatus: http.StatusTooManyRequests,
			wantMsg:    "The AI provider rate limit was reached. Wait a moment and retry.",
		},
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			wantStatus: http.StatusBadGateway,
			wantMsg:    "The AI provider rejected the API key. Check the provider credentials in the server configuration.",
		},
		{
			name:       "forbidden",
			statusCode: http.StatusForbidden,
			wantStatus: http.StatusBadGateway,
			wantMsg:    "The AI provider rejected the API key. Check the provider credentials in the server configuration.",
		},
		{
			name:       "not found",
			statusCode: http.StatusNotFound,
			wantStatus: http.StatusBadGateway,
			wantMsg:    "The configured AI model is not available at the provider. Check the model name in the server configuration.",
		},
		{
			name:       "bad request",
			statusCode: http.StatusBadRequest,
			wantStatus: http.StatusBadGateway,
			wantMsg:    "The AI provider rejected the request (HTTP 400). Check the model and request settings.",
		},
		{
			name:       "internal server error",
			statusCode: http.StatusInternalServerError,
			wantStatus: http.StatusBadGateway,
			wantMsg:    "The AI provider is temporarily unavailable (HTTP 500). Try again in a moment.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotStatus, gotMsg := llmErrorResponse(&llm.APIError{StatusCode: tt.statusCode})
			if gotStatus != tt.wantStatus {
				t.Errorf("status = %d, want %d", gotStatus, tt.wantStatus)
			}
			if gotMsg != tt.wantMsg {
				t.Errorf("message = %q, want %q", gotMsg, tt.wantMsg)
			}
		})
	}
}
