package llm

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
)

// APIError represents an HTTP error returned by the LLM provider.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Op         string
	err        error
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Op, e.Message)
	}
	return fmt.Sprintf("%s: provider returned HTTP %d", e.Op, e.StatusCode)
}

func (e *APIError) Unwrap() error {
	return e.err
}

// wrapAPIError converts an OpenAI-compatible provider error into *APIError.
// Non-API errors are wrapped with op and returned unchanged.
func wrapAPIError(op string, err error) error {
	if err == nil {
		return nil
	}

	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%s: %w", op, err)
	}

	msg := apiErr.Message
	if msg == "" {
		msg = extractProviderMessage(apiErr.RawJSON())
	}

	return &APIError{
		StatusCode: apiErr.StatusCode,
		Code:       apiErr.Code,
		Message:    msg,
		Op:         op,
		err:        err,
	}
}

func extractProviderMessage(raw string) string {
	if raw == "" {
		return ""
	}

	var root struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return ""
	}
	if root.Message != "" {
		return root.Message
	}
	return root.Error.Message
}
