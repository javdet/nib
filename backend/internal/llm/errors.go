package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

// APIError represents an HTTP error returned by the LLM provider.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Op         string
	// Endpoint, Model and ReasoningEffort are what the failed request was
	// sent with. They are reported back to the operator because a rejected
	// request is most often a config mismatch (a key for one provider sent to
	// another), which is invisible from the provider's message alone.
	Endpoint        string
	Model           string
	ReasoningEffort string
	// RetryAfter is the wait the provider asked for before the next attempt,
	// from Retry-After-Ms or Retry-After; zero when it named none.
	RetryAfter time.Duration
	// ShouldRetry is the provider's explicit x-should-retry verdict: nil when
	// the header is absent, which leaves the decision to the status code.
	ShouldRetry *bool
	err         error
}

// requestSettings is the part of a provider's configuration worth naming in
// an error. The API key is deliberately absent.
type requestSettings struct {
	model           string
	reasoningEffort string
}

// ErrTruncated is returned when the provider stopped at the output cap. The
// partial reply is deliberately not returned with it: a half-written tool-call
// argument object, or a plan cut off mid-stage, is worse than no answer, and
// the agent loops treat a reply with no tool calls as the final answer.
var ErrTruncated = errors.New("llm: response truncated by the output token limit")

// ErrRefused is returned when the model declined outright, either through a
// refusal message or a content filter.
var ErrRefused = errors.New("llm: model refused the request")

// ErrNoOutput is returned when a completion carried neither text nor tool
// calls. It is the shape a silently dropped tool call used to take.
var ErrNoOutput = errors.New("llm: completion carried no content and no tool calls")

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
func wrapAPIError(op string, settings requestSettings, err error) error {
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

	endpoint := ""
	if apiErr.Request != nil {
		endpoint = safeEndpoint(apiErr.Request.URL)
		msg = redactBearer(msg, apiErr.Request.Header.Get("Authorization"))
	}

	var retryAfter time.Duration
	var shouldRetry *bool
	if apiErr.Response != nil {
		retryAfter = parseRetryAfter(apiErr.Response.Header, time.Now())
		shouldRetry = parseShouldRetry(apiErr.Response.Header)
	}

	return &APIError{
		RetryAfter:      retryAfter,
		ShouldRetry:     shouldRetry,
		StatusCode:      apiErr.StatusCode,
		Code:            apiErr.Code,
		Message:         msg,
		Op:              op,
		Endpoint:        endpoint,
		Model:           settings.model,
		ReasoningEffort: settings.reasoningEffort,
		err:             err,
	}
}

// parseRetryAfter reads the wait a provider asked for. Retry-After-Ms is
// preferred when both are present, as the SDK does: it is the more precise of
// the two. Retry-After is either seconds or an HTTP date. A value that does not
// parse, or a date already past, is no instruction at all.
func parseRetryAfter(h http.Header, now time.Time) time.Duration {
	if v := strings.TrimSpace(h.Get("Retry-After-Ms")); v != "" {
		if ms, err := strconv.ParseFloat(v, 64); err == nil && ms > 0 {
			return time.Duration(ms * float64(time.Millisecond))
		}
	}
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs * float64(time.Second))
	}
	if at, err := http.ParseTime(v); err == nil {
		if d := at.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

func parseShouldRetry(h http.Header) *bool {
	switch strings.ToLower(strings.TrimSpace(h.Get("x-should-retry"))) {
	case "true":
		v := true
		return &v
	case "false":
		v := false
		return &v
	}
	return nil
}

// safeEndpoint keeps scheme, host and path only: userinfo and a query string
// are where credentials hide in a base URL, and this string reaches the UI.
func safeEndpoint(u *url.URL) string {
	if u == nil {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path}).String()
}

// redactBearer strips the API key from a provider message that echoes it back.
// Most providers mask it themselves; this one goes to the browser, so nothing
// is assumed.
func redactBearer(msg, authorization string) string {
	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	if len(token) < 8 {
		return msg
	}
	return strings.ReplaceAll(msg, token, "[redacted]")
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
