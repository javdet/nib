package llm

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
)

func TestWrapAPIErrorProviderError(t *testing.T) {
	t.Parallel()

	providerErr := &openai.Error{
		StatusCode: http.StatusPaymentRequired,
		Code:       "402",
		Message:    "This request requires more credits, or fewer max_tokens.",
	}

	wrapped := wrapAPIError("openai chat completion with tools", requestSettings{}, providerErr)
	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatalf("errors.As() = false, want *APIError")
	}
	if apiErr.StatusCode != http.StatusPaymentRequired {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusPaymentRequired)
	}
	if apiErr.Code != "402" {
		t.Errorf("Code = %q, want %q", apiErr.Code, "402")
	}
	if apiErr.Message != providerErr.Message {
		t.Errorf("Message = %q, want %q", apiErr.Message, providerErr.Message)
	}
	if apiErr.Op != "openai chat completion with tools" {
		t.Errorf("Op = %q, want %q", apiErr.Op, "openai chat completion with tools")
	}
}

func TestWrapAPIErrorThroughWrappingChain(t *testing.T) {
	t.Parallel()

	providerErr := &openai.Error{
		StatusCode: http.StatusTooManyRequests,
		Code:       "429",
		Message:    "rate limit exceeded",
	}
	wrapped := wrapAPIError("openai chat completion with tools", requestSettings{}, providerErr)
	outer := fmt.Errorf("completion round %d: %w", 1, wrapped)
	inner := fmt.Errorf("chat send in dialog: %w", outer)

	var apiErr *APIError
	if !errors.As(inner, &apiErr) {
		t.Fatal("errors.As() = false through wrapping chain, want true")
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
}

func TestWrapAPIErrorNonProviderError(t *testing.T) {
	t.Parallel()

	base := errors.New("connection reset")
	wrapped := wrapAPIError("openai chat completion with tools", requestSettings{}, base)

	var apiErr *APIError
	if errors.As(wrapped, &apiErr) {
		t.Fatal("errors.As() = true for non-provider error, want false")
	}
	if !errors.Is(wrapped, base) {
		t.Error("wrapped error should preserve base error in chain")
	}
}

func TestExtractProviderMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "openrouter root message",
			raw:  `{"message":"requires more credits","code":402}`,
			want: "requires more credits",
		},
		{
			name: "openai nested error message",
			raw:  `{"error":{"message":"invalid api key","type":"invalid_request_error"}}`,
			want: "invalid api key",
		},
		{
			name: "empty",
			raw:  "",
			want: "",
		},
		{
			name: "invalid json",
			raw:  "not json",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractProviderMessage(tt.raw)
			if got != tt.want {
				t.Errorf("extractProviderMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWrapAPIErrorExtractsMessageFromRawJSON(t *testing.T) {
	t.Parallel()

	var providerErr openai.Error
	if err := providerErr.UnmarshalJSON([]byte(`{"message":"requires more credits","code":"402"}`)); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	providerErr.StatusCode = http.StatusPaymentRequired

	wrapped := wrapAPIError("openai chat completion with tools", requestSettings{}, &providerErr)
	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatal("errors.As() = false, want true")
	}
	if apiErr.Message != "requires more credits" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "requires more credits")
	}
}

func TestWrapAPIErrorReportsSafeSettings(t *testing.T) {
	t.Parallel()

	const key = "sk-proj-secretsecretsecret"
	req, err := http.NewRequest(http.MethodPost, "https://user:pass@gw.example/api/v1/chat/completions?key=abc", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	providerErr := &openai.Error{
		StatusCode: http.StatusUnauthorized,
		Message:    "Incorrect API key provided: " + key,
		Request:    req,
	}

	wrapped := wrapAPIError("openai chat completion with tools", requestSettings{model: "gpt-x", reasoningEffort: "low"}, providerErr)
	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatal("errors.As() = false, want true")
	}
	if want := "https://gw.example/api/v1/chat/completions"; apiErr.Endpoint != want {
		t.Errorf("Endpoint = %q, want %q", apiErr.Endpoint, want)
	}
	if apiErr.Model != "gpt-x" || apiErr.ReasoningEffort != "low" {
		t.Errorf("Model, ReasoningEffort = %q, %q, want %q, %q", apiErr.Model, apiErr.ReasoningEffort, "gpt-x", "low")
	}
	if want := "Incorrect API key provided: [redacted]"; apiErr.Message != want {
		t.Errorf("Message = %q, want %q", apiErr.Message, want)
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		header map[string]string
		want   time.Duration
	}{
		{name: "absent", want: 0},
		{name: "seconds", header: map[string]string{"Retry-After": "20"}, want: 20 * time.Second},
		{name: "fractional seconds", header: map[string]string{"Retry-After": "1.5"}, want: 1500 * time.Millisecond},
		{name: "milliseconds win", header: map[string]string{"Retry-After": "20", "Retry-After-Ms": "250"}, want: 250 * time.Millisecond},
		{name: "http date", header: map[string]string{"Retry-After": now.Add(90 * time.Second).Format(http.TimeFormat)}, want: 90 * time.Second},
		{name: "date in the past", header: map[string]string{"Retry-After": now.Add(-time.Minute).Format(http.TimeFormat)}, want: 0},
		{name: "negative", header: map[string]string{"Retry-After": "-5"}, want: 0},
		{name: "garbage", header: map[string]string{"Retry-After": "soon"}, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := http.Header{}
			for k, v := range tc.header {
				h.Set(k, v)
			}
			if got := parseRetryAfter(h, now); got != tc.want {
				t.Errorf("parseRetryAfter() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWrapAPIErrorCarriesRetryHints(t *testing.T) {
	t.Parallel()

	resp := &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{}}
	resp.Header.Set("Retry-After", "12")
	resp.Header.Set("x-should-retry", "true")
	providerErr := &openai.Error{StatusCode: http.StatusTooManyRequests, Response: resp}

	var apiErr *APIError
	if !errors.As(wrapAPIError("op", requestSettings{}, providerErr), &apiErr) {
		t.Fatal("errors.As() = false, want *APIError")
	}
	if apiErr.RetryAfter != 12*time.Second {
		t.Errorf("RetryAfter = %v, want 12s", apiErr.RetryAfter)
	}
	if apiErr.ShouldRetry == nil || !*apiErr.ShouldRetry {
		t.Errorf("ShouldRetry = %v, want true", apiErr.ShouldRetry)
	}
}
