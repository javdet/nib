package llm

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestWrapAPIErrorProviderError(t *testing.T) {
	t.Parallel()

	providerErr := &openai.Error{
		StatusCode: http.StatusPaymentRequired,
		Code:       "402",
		Message:    "This request requires more credits, or fewer max_tokens.",
	}

	wrapped := wrapAPIError("openai chat completion with tools", providerErr)
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
	wrapped := wrapAPIError("openai chat completion with tools", providerErr)
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
	wrapped := wrapAPIError("openai chat completion with tools", base)

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

	wrapped := wrapAPIError("openai chat completion with tools", &providerErr)
	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatal("errors.As() = false, want true")
	}
	if apiErr.Message != "requires more credits" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "requires more credits")
	}
}
