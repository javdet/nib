package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/javdet/nib/internal/metrics"
)

const (
	// retryBaseDelay is the first backoff step; each retry doubles it.
	retryBaseDelay = time.Second
	// retryMaxBackoff caps the computed backoff. It is separate from
	// RetryPolicy.MaxDelay, which bounds a wait the provider asks for: an
	// exponential series left to grow to two minutes would spend most of an
	// action's budget asleep on the guess alone.
	retryMaxBackoff = time.Minute
	// maxTimeoutRetries is how many times a request that hit llm.timeoutSeconds
	// is sent again. One more chance covers a stalled connection; a model that
	// takes longer than the timeout to answer will take longer again, and
	// eight more five-minute waits would outlast the action that is waiting.
	maxTimeoutRetries = 1
	// fallbackCooldown is how long calls go straight to the fallback model
	// after the primary used up its retries. Without it every round of an
	// agent loop would spend the primary's whole retry budget again before
	// switching, and an outage would cost minutes per round.
	fallbackCooldown = 5 * time.Minute
)

// RetryPolicy says how a completion that failed is sent again.
type RetryPolicy struct {
	// MaxRetries is the number of attempts after the first. Zero never retries.
	MaxRetries int
	// MaxDelay caps one wait. A Retry-After longer than this gives the model
	// up rather than sleeping on it.
	MaxDelay time.Duration
}

// retryingProvider retries failed completions and, when one is configured,
// moves on to a fallback model once the primary has used up its retries.
//
// It owns retrying outright: the providers under it are built with the SDK's
// own retries off. The SDK's loop backs off for at most eight seconds, honours
// a Retry-After of any length, and is silent -- none of which suits an action
// that runs for half an hour and whose operator wants to know why it stalled.
type retryingProvider struct {
	primary       Provider
	primaryModel  string
	fallback      Provider
	fallbackModel string
	policy        RetryPolicy

	// sleep and jitter are fields so tests run without waiting.
	sleep  func(ctx context.Context, d time.Duration) error
	jitter func(d time.Duration) time.Duration
	now    func() time.Time

	mu               sync.Mutex
	primaryDownUntil time.Time
}

// NewRetrying wraps primary in the retry policy. fallback may be nil.
func NewRetrying(primary Provider, primaryModel string, fallback Provider, fallbackModel string, policy RetryPolicy) Provider {
	if policy.MaxRetries < 0 {
		policy.MaxRetries = 0
	}
	return &retryingProvider{
		primary:       primary,
		primaryModel:  primaryModel,
		fallback:      fallback,
		fallbackModel: fallbackModel,
		policy:        policy,
		sleep:         sleepContext,
		jitter:        backoffJitter,
		now:           time.Now,
	}
}

func (r *retryingProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return withFallback(ctx, r, func(p Provider) (string, error) {
		return p.Complete(ctx, systemPrompt, userPrompt)
	})
}

func (r *retryingProvider) CompleteWithTools(ctx context.Context, messages []Message, tools []ToolDef) (AssistantMessage, error) {
	return withFallback(ctx, r, func(p Provider) (AssistantMessage, error) {
		return p.CompleteWithTools(ctx, messages, tools)
	})
}

type retryTarget struct {
	provider Provider
	model    string
	primary  bool
}

// targets is the order to try the models in. While the primary is cooling down
// the fallback goes first, but the primary stays in the list: if the fallback
// is failing too, the primary may well have recovered.
func (r *retryingProvider) targets() []retryTarget {
	primary := retryTarget{provider: r.primary, model: r.primaryModel, primary: true}
	if r.fallback == nil {
		return []retryTarget{primary}
	}
	fallback := retryTarget{provider: r.fallback, model: r.fallbackModel}

	r.mu.Lock()
	cooling := r.now().Before(r.primaryDownUntil)
	r.mu.Unlock()
	if cooling {
		return []retryTarget{fallback, primary}
	}
	return []retryTarget{primary, fallback}
}

func (r *retryingProvider) setPrimaryDown(down bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if down {
		r.primaryDownUntil = r.now().Add(fallbackCooldown)
	} else {
		r.primaryDownUntil = time.Time{}
	}
}

func withFallback[T any](ctx context.Context, r *retryingProvider, call func(Provider) (T, error)) (T, error) {
	targets := r.targets()
	var firstErr error
	var firstModel string
	for i, t := range targets {
		out, err := withRetry(ctx, r, t.model, func() (T, error) { return call(t.provider) })
		if err == nil {
			if t.primary && r.fallback != nil {
				r.setPrimaryDown(false)
			}
			return out, nil
		}
		last := i == len(targets)-1
		// Only a failure worth repeating moves on to the other model. A request
		// the provider rejected outright -- a bad key, a schema it refuses -- is
		// a config error, and answering it from another model would hide it.
		if last || ctx.Err() != nil || !isRetryableLLMError(err) {
			if firstErr != nil {
				return out, fmt.Errorf("%w (model %s failed before it: %v)", err, firstModel, firstErr)
			}
			return out, err
		}
		if t.primary {
			r.setPrimaryDown(true)
		}
		next := targets[i+1]
		slog.Warn("llm: switching model after repeated failures",
			"from_model", t.model, "to_model", next.model, "error", err)
		metrics.RecordLLMFallback(t.model, next.model)
		firstErr, firstModel = err, t.model
	}
	var zero T
	return zero, errors.New("llm: no model to call")
}

// withRetry sends one model the same request until it succeeds, fails in a way
// not worth repeating, or the policy says stop. The error it gives up with is
// annotated with why, since "HTTP 503" alone reads as a single unlucky request.
func withRetry[T any](ctx context.Context, r *retryingProvider, model string, call func() (T, error)) (T, error) {
	timeouts := 0
	for attempt := 0; ; attempt++ {
		out, err := call()
		if err == nil {
			return out, nil
		}
		// The caller stopped waiting: a force stop, an action timeout, a
		// cancelled turn. Nothing is retried past that.
		if ctx.Err() != nil || !isRetryableLLMError(err) {
			return out, err
		}

		delay, stop := r.nextDelay(err, attempt, &timeouts)
		if stop == "" {
			if deadline, ok := ctx.Deadline(); ok && !r.now().Add(delay).Before(deadline) {
				stop = fmt.Sprintf("the next attempt would start after the caller's deadline (in %s)",
					deadline.Sub(r.now()).Round(time.Second))
			}
		}
		if stop != "" {
			return out, fmt.Errorf("%w (model %s, %d attempt(s), not retried further: %s)", err, model, attempt+1, stop)
		}

		slog.Warn("llm: request failed, retrying",
			"model", model,
			"attempt", attempt+1,
			"max_retries", r.policy.MaxRetries,
			"delay", delay.Round(time.Millisecond),
			"error", err,
		)
		metrics.RecordLLMRetry(model, OutcomeFor(err))
		if serr := r.sleep(ctx, delay); serr != nil {
			return out, err
		}
	}
}

// nextDelay is the wait before the attempt after attempt (zero-based), or the
// reason there is none.
func (r *retryingProvider) nextDelay(err error, attempt int, timeouts *int) (time.Duration, string) {
	if attempt >= r.policy.MaxRetries {
		if r.policy.MaxRetries == 0 {
			return 0, "llm.maxRetries is 0"
		}
		return 0, fmt.Sprintf("llm.maxRetries (%d) used up", r.policy.MaxRetries)
	}
	if isTimeoutError(err) {
		*timeouts++
		if *timeouts > maxTimeoutRetries {
			return 0, "the request timed out again (llm.timeoutSeconds)"
		}
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		if r.policy.MaxDelay > 0 && apiErr.RetryAfter > r.policy.MaxDelay {
			return 0, fmt.Sprintf("the provider asked to wait %s, longer than llm.maxRetryDelaySeconds (%s)",
				apiErr.RetryAfter.Round(time.Second), r.policy.MaxDelay)
		}
		return apiErr.RetryAfter, ""
	}

	limit := retryMaxBackoff
	if r.policy.MaxDelay > 0 && r.policy.MaxDelay < limit {
		limit = r.policy.MaxDelay
	}
	delay := limit
	// Shifting past ~30 overflows; by then the cap has long applied.
	if attempt < 30 {
		if d := retryBaseDelay << attempt; d < limit {
			delay = d
		}
	}
	return r.jitter(delay), ""
}

// isRetryableLLMError reports whether sending the same request again can be
// expected to go differently: the statuses the SDK itself retries, unless the
// provider said otherwise with x-should-retry, and a connection that failed or
// dropped before a whole reply arrived. Everything this package returns on a
// reply it did receive -- truncation, refusal, an empty answer -- is final.
func isRetryableLLMError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.ShouldRetry != nil {
			return *apiErr.ShouldRetry
		}
		switch s := apiErr.StatusCode; {
		case s == http.StatusRequestTimeout, s == http.StatusConflict, s == http.StatusTooManyRequests:
			return true
		case s >= http.StatusInternalServerError:
			return true
		}
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// backoffJitter takes up to a quarter off, so callers that failed together do
// not all come back in the same instant.
func backoffJitter(d time.Duration) time.Duration {
	if q := int64(d / 4); q > 0 {
		return d - time.Duration(rand.Int64N(q))
	}
	return d
}
