package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/javdet/nib/internal/config"
)

// fakeProvider answers each call with the next scripted error, then succeeds.
type fakeProvider struct {
	mu    sync.Mutex
	model string
	errs  []error
	calls int
}

func (f *fakeProvider) next() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.errs) == 0 {
		return nil
	}
	err := f.errs[0]
	f.errs = f.errs[1:]
	return err
}

func (f *fakeProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeProvider) Complete(context.Context, string, string) (string, error) {
	if err := f.next(); err != nil {
		return "", err
	}
	return "answer from " + f.model, nil
}

func (f *fakeProvider) CompleteWithTools(context.Context, []Message, []ToolDef) (AssistantMessage, error) {
	if err := f.next(); err != nil {
		return AssistantMessage{}, err
	}
	return AssistantMessage{Content: "answer from " + f.model}, nil
}

func statusErr(code int) error {
	return &APIError{StatusCode: code, Op: "test"}
}

// timeoutErr is what http.Client returns when llm.timeoutSeconds runs out.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "Client.Timeout exceeded" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// newTestRetrying builds the decorator with sleeping recorded rather than done.
func newTestRetrying(primary, fallback *fakeProvider, policy RetryPolicy) (*retryingProvider, *[]time.Duration) {
	var fb Provider
	fbModel := ""
	if fallback != nil {
		fb, fbModel = fallback, fallback.model
	}
	r := NewRetrying(primary, primary.model, fb, fbModel, policy).(*retryingProvider)
	var slept []time.Duration
	r.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	r.jitter = func(d time.Duration) time.Duration { return d }
	return r, &slept
}

func TestRetrying_backsOffAndSucceeds(t *testing.T) {
	t.Parallel()

	primary := &fakeProvider{model: "main", errs: []error{statusErr(503), statusErr(429), statusErr(500)}}
	r, slept := newTestRetrying(primary, nil, RetryPolicy{MaxRetries: 5, MaxDelay: time.Minute})

	got, err := r.CompleteWithTools(t.Context(), nil, nil)
	if err != nil {
		t.Fatalf("CompleteWithTools() error = %v", err)
	}
	if got.Content != "answer from main" {
		t.Errorf("Content = %q, want answer from main", got.Content)
	}
	if primary.callCount() != 4 {
		t.Errorf("calls = %d, want 4", primary.callCount())
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	if fmt.Sprint(*slept) != fmt.Sprint(want) {
		t.Errorf("waits = %v, want %v", *slept, want)
	}
}

func TestRetrying_backoffIsCapped(t *testing.T) {
	t.Parallel()

	r, _ := newTestRetrying(&fakeProvider{}, nil, RetryPolicy{MaxRetries: 40, MaxDelay: 2 * time.Minute})
	timeouts := 0
	delay, stop := r.nextDelay(statusErr(503), 35, &timeouts)
	if stop != "" || delay != retryMaxBackoff {
		t.Errorf("nextDelay(attempt 35) = %v, %q; want %v", delay, stop, retryMaxBackoff)
	}

	r.policy.MaxDelay = 10 * time.Second
	delay, _ = r.nextDelay(statusErr(503), 6, &timeouts)
	if delay != 10*time.Second {
		t.Errorf("nextDelay under a 10s cap = %v, want 10s", delay)
	}
}

func TestRetrying_honoursRetryAfter(t *testing.T) {
	t.Parallel()

	limited := &APIError{StatusCode: 429, Op: "test", RetryAfter: 7 * time.Second}
	primary := &fakeProvider{model: "main", errs: []error{limited}}
	r, slept := newTestRetrying(primary, nil, RetryPolicy{MaxRetries: 3, MaxDelay: time.Minute})

	if _, err := r.Complete(t.Context(), "sys", "user"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(*slept) != 1 || (*slept)[0] != 7*time.Second {
		t.Errorf("waits = %v, want [7s]", *slept)
	}
}

func TestRetrying_retryAfterLongerThanCapGivesUp(t *testing.T) {
	t.Parallel()

	limited := &APIError{StatusCode: 429, Op: "test", RetryAfter: time.Hour}
	primary := &fakeProvider{model: "main", errs: []error{limited}}
	r, slept := newTestRetrying(primary, nil, RetryPolicy{MaxRetries: 3, MaxDelay: 2 * time.Minute})

	_, err := r.Complete(t.Context(), "sys", "user")
	if err == nil {
		t.Fatal("Complete() error = nil, want the rate limit")
	}
	if !strings.Contains(err.Error(), "llm.maxRetryDelaySeconds") {
		t.Errorf("error = %q, want it to name llm.maxRetryDelaySeconds", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 {
		t.Errorf("error does not unwrap to the provider's 429: %v", err)
	}
	if primary.callCount() != 1 || len(*slept) != 0 {
		t.Errorf("calls = %d, waits = %v; want one call and no wait", primary.callCount(), *slept)
	}
}

func TestRetrying_finalErrorsAreNotRetried(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "bad request", err: statusErr(400)},
		{name: "unauthorized", err: statusErr(401)},
		{name: "model not found", err: statusErr(404)},
		{name: "truncated", err: fmt.Errorf("op: %w", ErrTruncated)},
		{name: "refused", err: fmt.Errorf("op: %w", ErrRefused)},
		{name: "no output", err: fmt.Errorf("op: %w", ErrNoOutput)},
		{name: "provider said no", err: &APIError{StatusCode: 503, ShouldRetry: new(false)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			primary := &fakeProvider{model: "main", errs: []error{tc.err}}
			fallback := &fakeProvider{model: "spare"}
			r, _ := newTestRetrying(primary, fallback, RetryPolicy{MaxRetries: 5})

			_, err := r.CompleteWithTools(t.Context(), nil, nil)
			if err == nil {
				t.Fatal("CompleteWithTools() error = nil, want the provider's error")
			}
			if primary.callCount() != 1 {
				t.Errorf("primary calls = %d, want 1", primary.callCount())
			}
			if fallback.callCount() != 0 {
				t.Errorf("fallback calls = %d, want 0: a final error is not a reason to switch model", fallback.callCount())
			}
		})
	}
}

func TestRetrying_transportErrorsAreRetried(t *testing.T) {
	t.Parallel()

	refused := &url.Error{Op: "Post", URL: "https://api.example/v1", Err: errors.New("connection refused")}
	primary := &fakeProvider{model: "main", errs: []error{
		fmt.Errorf("op: %w", refused),
		fmt.Errorf("op: %w", io.ErrUnexpectedEOF),
		&APIError{StatusCode: 400, ShouldRetry: new(true)},
	}}
	r, _ := newTestRetrying(primary, nil, RetryPolicy{MaxRetries: 5})

	if _, err := r.Complete(t.Context(), "sys", "user"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if primary.callCount() != 4 {
		t.Errorf("calls = %d, want 4", primary.callCount())
	}
}

func TestRetrying_timeoutIsRetriedOnce(t *testing.T) {
	t.Parallel()

	timeout := fmt.Errorf("op: %w", &url.Error{Op: "Post", URL: "https://api.example", Err: timeoutErr{}})
	primary := &fakeProvider{model: "main", errs: []error{timeout, timeout, timeout}}
	r, _ := newTestRetrying(primary, nil, RetryPolicy{MaxRetries: 8})

	_, err := r.Complete(t.Context(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "timed out again") {
		t.Fatalf("Complete() error = %v, want it to say the request timed out again", err)
	}
	if primary.callCount() != 2 {
		t.Errorf("calls = %d, want 2", primary.callCount())
	}
}

func TestRetrying_zeroRetriesSendsOnce(t *testing.T) {
	t.Parallel()

	primary := &fakeProvider{model: "main", errs: []error{statusErr(503)}}
	r, _ := newTestRetrying(primary, nil, RetryPolicy{MaxRetries: 0})

	_, err := r.Complete(t.Context(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "llm.maxRetries is 0") {
		t.Fatalf("Complete() error = %v, want it to name llm.maxRetries", err)
	}
	if primary.callCount() != 1 {
		t.Errorf("calls = %d, want 1", primary.callCount())
	}
}

func TestRetrying_stopsWhenTheCallerGivesUp(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	primary := &fakeProvider{model: "main", errs: []error{statusErr(503)}}
	fallback := &fakeProvider{model: "spare"}
	r, _ := newTestRetrying(primary, fallback, RetryPolicy{MaxRetries: 5})

	if _, err := r.Complete(ctx, "sys", "user"); err == nil {
		t.Fatal("Complete() error = nil, want the failure")
	}
	if primary.callCount() != 1 || fallback.callCount() != 0 {
		t.Errorf("calls = %d/%d, want 1/0", primary.callCount(), fallback.callCount())
	}
}

func TestRetrying_doesNotSleepPastTheDeadline(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	limited := &APIError{StatusCode: 429, Op: "test", RetryAfter: 30 * time.Second}
	primary := &fakeProvider{model: "main", errs: []error{limited}}
	r, slept := newTestRetrying(primary, nil, RetryPolicy{MaxRetries: 5, MaxDelay: time.Minute})

	_, err := r.Complete(ctx, "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("Complete() error = %v, want it to mention the deadline", err)
	}
	if len(*slept) != 0 {
		t.Errorf("waits = %v, want none", *slept)
	}
}

func TestRetrying_fallsBackAndRemembers(t *testing.T) {
	t.Parallel()

	primary := &fakeProvider{model: "main", errs: []error{statusErr(503), statusErr(503), statusErr(503)}}
	fallback := &fakeProvider{model: "spare"}
	r, _ := newTestRetrying(primary, fallback, RetryPolicy{MaxRetries: 2})
	now := time.Unix(1_700_000_000, 0)
	r.now = func() time.Time { return now }

	got, err := r.CompleteWithTools(t.Context(), nil, nil)
	if err != nil {
		t.Fatalf("CompleteWithTools() error = %v", err)
	}
	if got.Content != "answer from spare" {
		t.Errorf("Content = %q, want the fallback's answer", got.Content)
	}
	if primary.callCount() != 3 || fallback.callCount() != 1 {
		t.Fatalf("calls = %d/%d, want 3/1", primary.callCount(), fallback.callCount())
	}

	// Inside the cooldown the next call goes straight to the fallback rather
	// than paying the primary's retry budget again.
	if _, err := r.CompleteWithTools(t.Context(), nil, nil); err != nil {
		t.Fatalf("second CompleteWithTools() error = %v", err)
	}
	if primary.callCount() != 3 || fallback.callCount() != 2 {
		t.Errorf("calls after second request = %d/%d, want 3/2", primary.callCount(), fallback.callCount())
	}

	// Once it has passed, the primary is tried first again.
	now = now.Add(fallbackCooldown + time.Second)
	if got, err := r.CompleteWithTools(t.Context(), nil, nil); err != nil || got.Content != "answer from main" {
		t.Errorf("after cooldown = %q, %v; want the primary's answer", got.Content, err)
	}
}

func TestRetrying_bothModelsFailing(t *testing.T) {
	t.Parallel()

	primary := &fakeProvider{model: "main", errs: []error{statusErr(503), statusErr(503)}}
	fallback := &fakeProvider{model: "spare", errs: []error{statusErr(502), statusErr(502)}}
	r, _ := newTestRetrying(primary, fallback, RetryPolicy{MaxRetries: 1})

	_, err := r.Complete(t.Context(), "sys", "user")
	if err == nil {
		t.Fatal("Complete() error = nil, want both failures")
	}
	for _, want := range []string{"model spare", "model main failed before it"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 502 {
		t.Errorf("error should unwrap to the fallback's 502, got %v", err)
	}
}

// TestNewFromConfig_retriesAndFallsBack runs the whole stack against a server:
// the SDK's own retries must be off, Retry-After-Ms must be honoured, and the
// fallback must be the same endpoint with the other model.
func TestNewFromConfig_retriesAndFallsBack(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		calls[body.Model]++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if body.Model == "main" {
			w.Header().Set("Retry-After-Ms", "5")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"overloaded"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"c1","object":"chat.completion","created":1,"model":"spare",
			"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"hello"}}]}`)
	}))
	t.Cleanup(srv.Close)

	client, err := NewFromConfig(config.LLMConfig{
		BaseURL:              srv.URL + "/v1",
		Model:                "main",
		FallbackModel:        "spare",
		APIKey:               "test-key",
		EmbeddingModel:       "text-embedding-3-small",
		TimeoutSeconds:       30,
		MaxRetries:           2,
		MaxRetryDelaySeconds: 5,
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}

	got, err := client.Complete(t.Context(), "sys", "user")
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if got != "hello" {
		t.Errorf("Complete() = %q, want hello", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["main"] != 3 {
		t.Errorf("requests for main = %d, want 3 (one plus llm.maxRetries, none from the SDK)", calls["main"])
	}
	if calls["spare"] != 1 {
		t.Errorf("requests for spare = %d, want 1", calls["spare"])
	}
}

func TestNewFromConfig_zeroRetriesMeansOneRequest(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"boom"}}`)
	}))
	t.Cleanup(srv.Close)

	client, err := NewFromConfig(config.LLMConfig{
		BaseURL:        srv.URL + "/v1",
		Model:          "main",
		APIKey:         "test-key",
		EmbeddingModel: "text-embedding-3-small",
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if _, err := client.Complete(t.Context(), "sys", "user"); err == nil {
		t.Fatal("Complete() error = nil, want the 500")
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Errorf("requests = %d, want 1: the SDK must not retry on its own", requests)
	}
}
