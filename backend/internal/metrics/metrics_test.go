package metrics

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// withDefault installs m for the duration of the test, mirroring how
// logging/setup_test.go saves and restores slog.Default().
func withDefault(t *testing.T, m *Metrics) {
	t.Helper()
	prev := Default()
	t.Cleanup(func() { SetDefault(prev) })
	SetDefault(m)
}

func newTestMetrics(t *testing.T) *Metrics {
	t.Helper()
	m, err := Setup(Config{Enabled: true, Host: "127.0.0.1", Port: 9090, Path: "/metrics", RefreshSeconds: 30})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(func() { SetDefault(nil) })
	return m
}

func TestSetup_disabledRecordersAreNoops(t *testing.T) {
	m, err := Setup(Config{Enabled: false})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	withDefault(t, m)

	if m == nil {
		t.Fatal("Setup returned nil for a disabled config")
	}
	if m.Enabled() {
		t.Fatal("Enabled() is true for a disabled config")
	}
	if m.Registry() != nil {
		t.Fatal("a disabled config should register nothing")
	}

	// Every recorder must survive a disabled setup: these are called from the
	// agent loop and the request path, which have no idea metrics are off.
	ObserveHTTPRequest("GET", "/x", 200, time.Second, 10)
	IncHTTPInFlight()
	DecHTTPInFlight()
	RecordAgentTurn("main", OutcomeSuccess, 3, time.Second)
	RecordToolCall(ToolSourceLocal, "create_action_plan", nil, time.Second)
	RecordLLMRequest(LLMOpComplete, "gpt", OutcomeSuccess, time.Second)
	RecordLeaseAcquired("subagent")
	RecordLeaseReleased("subagent", time.Second)
	RecordSSEEvent("turn_end")
	m.SetPlansByStatus(map[string]int{"draft": 1})
	if err := m.RegisterPool(func() PoolStats { return PoolStats{} }); err != nil {
		t.Fatalf("RegisterPool on a disabled set: %v", err)
	}
}

func TestSetup_recordersAreNoopsWithNoDefault(t *testing.T) {
	withDefault(t, nil)

	// This is what keeps every existing service test working: they construct
	// services directly and never call Setup.
	ObserveHTTPRequest("GET", "/x", 200, time.Second, 10)
	RecordAgentTurn("main", OutcomeSuccess, 1, time.Second)
	RecordToolCall(ToolSourceMCP, "", nil, time.Second)
	RecordLeaseRejected("container")
	AddStuckRunsReconciled(1, 1)
	RecordPlanStatusTransition("draft", "done")
}

func TestSetup_registersRuntimeAndBuildCollectors(t *testing.T) {
	m := newTestMetrics(t)

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	got := make(map[string]bool, len(families))
	for _, f := range families {
		got[f.GetName()] = true
	}

	// process_* is deliberately not asserted: NewProcessCollector needs procfs
	// and yields nothing off Linux.
	for _, want := range []string{"go_goroutines", "go_build_info", "nib_build_info", "nib_start_time_seconds"} {
		if !got[want] {
			t.Errorf("registry is missing %q", want)
		}
	}
}

func TestSetup_twiceDoesNotPanic(t *testing.T) {
	first := newTestMetrics(t)
	second := newTestMetrics(t)

	// Each Setup gets its own registry, so a duplicate registration -- which
	// would panic under MustRegister -- cannot happen.
	if first.Registry() == second.Registry() {
		t.Fatal("two Setup calls shared a registry")
	}
	if Default() != second {
		t.Fatal("the second Setup did not become the default")
	}
}

func TestHandler_disabledIs404(t *testing.T) {
	m, err := Setup(Config{Enabled: false})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if m.Handler() == nil {
		t.Fatal("Handler returned nil")
	}
	if m.Path() != "" {
		t.Errorf("Path() = %q, want empty for a disabled set", m.Path())
	}
}

func TestAddr(t *testing.T) {
	m := newTestMetrics(t)
	if got, want := m.Addr(), "127.0.0.1:9090"; got != want {
		t.Errorf("Addr() = %q, want %q", got, want)
	}
}

func TestNormalizeMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		want   string
	}{
		{"get", "GET", "GET"},
		{"delete", "DELETE", "DELETE"},
		{"options", "OPTIONS", "OPTIONS"},
		// net/http will route an arbitrary method into the chain, so it must
		// not become a label of its own.
		{"invented", "FOO", "other"},
		{"lowercase is not a known method", "get", "other"},
		{"empty", "", "other"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeMethod(tt.method); got != tt.want {
				t.Errorf("NormalizeMethod(%q) = %q, want %q", tt.method, got, tt.want)
			}
		})
	}
}

func TestLLMOutcomeForStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		want   string
	}{
		{"no status", 0, OutcomeError},
		{"request timeout", 408, "timeout"},
		{"gateway timeout", 504, "timeout"},
		{"rate limited", 429, "rate_limited"},
		{"bad request", 400, "client_error"},
		{"unauthorized", 401, "client_error"},
		{"server error", 500, "server_error"},
		{"bad gateway", 502, "server_error"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := LLMOutcomeForStatus(tt.status); got != tt.want {
				t.Errorf("LLMOutcomeForStatus(%d) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestNormalizeAgentRunnerStatus(t *testing.T) {
	t.Parallel()

	// The status arrives from a container over the network, so anything not on
	// the known list has to collapse rather than become a series.
	for _, known := range []string{"success", "failed", "timeout", "error"} {
		if got := NormalizeAgentRunnerStatus(known); got != known {
			t.Errorf("NormalizeAgentRunnerStatus(%q) = %q, want %q", known, got, known)
		}
	}
	for _, unknown := range []string{"", "weird", strings.Repeat("x", 200)} {
		if got := NormalizeAgentRunnerStatus(unknown); got != "other" {
			t.Errorf("NormalizeAgentRunnerStatus(%q) = %q, want %q", unknown, got, "other")
		}
	}
}

func TestObserveHTTPRequest(t *testing.T) {
	m := newTestMetrics(t)
	withDefault(t, m)

	ObserveHTTPRequest("GET", "/api/v1/dialogs", 200, 250*time.Millisecond, 512)
	ObserveHTTPRequest("GET", "/api/v1/dialogs", 500, time.Second, 64)
	ObserveHTTPRequest("POST", "/api/v1/chat", 200, 20*time.Minute, 1024)

	if got := testutil.ToFloat64(m.http.requests.WithLabelValues("GET", "/api/v1/dialogs", "200")); got != 1 {
		t.Errorf("GET /dialogs 200 count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.http.requests.WithLabelValues("GET", "/api/v1/dialogs", "500")); got != 1 {
		t.Errorf("GET /dialogs 500 count = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.http.respBytes.WithLabelValues("GET", "/api/v1/dialogs")); got != 576 {
		t.Errorf("response bytes = %v, want 576", got)
	}

	// Buckets are cumulative, so the 20-minute request showing up at 1200s but
	// not at 600s is what proves it landed in a finite bucket rather than +Inf.
	// With DefBuckets, which stop at 10s, every agent request would be
	// unmeasurable.
	if got := histogramCount(t, m, "nib_http_request_duration_seconds", 600); got != 2 {
		t.Errorf("requests at or under 600s = %v, want 2", got)
	}
	if got := histogramCount(t, m, "nib_http_request_duration_seconds", 1200); got != 3 {
		t.Errorf("requests at or under 1200s = %v, want 3", got)
	}
}

func TestRecordToolCall_onlyNamesLocalTools(t *testing.T) {
	m := newTestMetrics(t)
	withDefault(t, m)

	RecordToolCall(ToolSourceLocal, "create_action_plan", nil, time.Second)
	RecordToolCall(ToolSourceMCP, "some-server__list_issues", nil, time.Second)
	RecordToolCall(ToolSourceUnknown, "tool_the_model_invented", errNotFound{}, time.Second)

	if got := testutil.ToFloat64(m.agent.toolCalls.WithLabelValues(ToolSourceLocal, OutcomeSuccess)); got != 1 {
		t.Errorf("local tool calls = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.agent.toolCalls.WithLabelValues(ToolSourceUnknown, OutcomeError)); got != 1 {
		t.Errorf("unknown tool call errors = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.agent.localToolCalls.WithLabelValues("create_action_plan", OutcomeSuccess)); got != 1 {
		t.Errorf("named local tool = %v, want 1", got)
	}
	// An MCP name comes from operator config and an invented name from the
	// model; neither may create a series.
	if got := testutil.CollectAndCount(m.agent.localToolCalls); got != 1 {
		t.Errorf("named tool series = %d, want 1 (only the local tool)", got)
	}
}

func TestRecordAgentRunnerResult_rejectsOutOfRangeValues(t *testing.T) {
	m := newTestMetrics(t)
	withDefault(t, m)

	// Counter.Add panics on a negative and a NaN poisons the series, and every
	// one of these arrives from a container over the network.
	RecordAgentRunnerResult("success", "true", -5, -1, -3.5)
	RecordAgentRunnerResult("success", "true", 0, 0, 0)

	if got := testutil.ToFloat64(m.executor.agentCost); got != 0 {
		t.Errorf("cost total = %v, want 0", got)
	}
	if got := testutil.ToFloat64(m.executor.results.WithLabelValues("success", "true")); got != 2 {
		t.Errorf("results = %v, want 2", got)
	}
}

func TestRecordLeaseHeldGaugeReturnsToZero(t *testing.T) {
	m := newTestMetrics(t)
	withDefault(t, m)

	RecordLeaseAcquired("subagent")
	if got := testutil.ToFloat64(m.execution.leaseHeld); got != 1 {
		t.Fatalf("lease held = %v, want 1", got)
	}
	RecordLeaseReleased("subagent", 30*time.Second)
	if got := testutil.ToFloat64(m.execution.leaseHeld); got != 0 {
		t.Fatalf("lease held after release = %v, want 0", got)
	}

	RecordLeaseRejected("container")
	if got := testutil.ToFloat64(m.execution.leaseRejections.WithLabelValues("container")); got != 1 {
		t.Errorf("lease rejections = %v, want 1", got)
	}
}

func TestRecordSSESubscribersGauge(t *testing.T) {
	m := newTestMetrics(t)
	withDefault(t, m)

	IncSSESubscribers()
	IncSSESubscribers()
	DecSSESubscribers()
	if got := testutil.ToFloat64(m.sse.subscribers); got != 1 {
		t.Errorf("subscribers = %v, want 1", got)
	}

	RecordSSEEventDropped("plan_stage_done")
	if got := testutil.ToFloat64(m.sse.dropped.WithLabelValues("plan_stage_done")); got != 1 {
		t.Errorf("dropped = %v, want 1", got)
	}
}

func TestRecordPlanStatusTransition_ignoresSelfTransitions(t *testing.T) {
	m := newTestMetrics(t)
	withDefault(t, m)

	RecordPlanStatusTransition("draft", "draft")
	RecordPlanStatusTransition("draft", "in_progress")

	if got := testutil.CollectAndCount(m.execution.planTransitions); got != 1 {
		t.Errorf("transition series = %d, want 1", got)
	}
	if got := testutil.ToFloat64(m.execution.planTransitions.WithLabelValues("draft", "in_progress")); got != 1 {
		t.Errorf("draft->in_progress = %v, want 1", got)
	}
}

// TestRecordersAreRaceFree has no assertion of its own: it exists for -race.
// It is deliberately not parallel, because it swaps the package default.
func TestRecordersAreRaceFree(t *testing.T) {
	m := newTestMetrics(t)
	withDefault(t, m)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				ObserveHTTPRequest("GET", "/api/v1/health", 200, time.Millisecond, 16)
				RecordToolCall(ToolSourceLocal, "tool_search", nil, time.Millisecond)
				IncSSESubscribers()
				DecSSESubscribers()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 50; j++ {
			SetDefault(m)
		}
	}()
	wg.Wait()
}

type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }

// histogramCount returns the cumulative count of the named histogram at or
// below bound, summed across every label combination.
func histogramCount(t *testing.T, m *Metrics, name string, bound float64) float64 {
	t.Helper()

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		var total float64
		for _, metric := range f.GetMetric() {
			for _, b := range metric.GetHistogram().GetBucket() {
				if b.GetUpperBound() == bound {
					total += float64(b.GetCumulativeCount())
				}
			}
		}
		return total
	}
	t.Fatalf("histogram %q not found", name)
	return 0
}
