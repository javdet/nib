// Package metrics exposes the backend's Prometheus instrumentation: an HTTP
// middleware, Go runtime and process collectors, and recorders for the agent,
// LLM, execution and plan machinery.
//
// The repo's Go conventions call for OpenTelemetry. This deliberately uses
// prometheus/client_golang instead: no OTel code, collector or exporter exists
// anywhere in the stack, and a pull-based endpoint is the lower-risk fit for a
// service the chart pins to one replica. Every recording call site goes through
// this package's API rather than a vendor type, so an OTel Prometheus exporter
// can be swapped in underneath later without touching a single caller.
package metrics

import (
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/javdet/nib/internal/version"
)

// Namespace prefixes every metric this package defines.
const Namespace = "nib"

// Config mirrors config.MetricsConfig without importing it, so this package
// stays a leaf that any other package can record into.
type Config struct {
	Enabled        bool
	Host           string
	Port           int
	Path           string
	RefreshSeconds int
}

// Metrics owns a private registry and every collector registered on it.
//
// Callers reach it through the package-level recorder functions, which forward
// to the default set by Setup. That mirrors how slog is already used across this
// codebase: metrics are a cross-cutting concern, and threading a *Metrics
// through constructors that already take twenty positional dependencies would be
// a far larger diff for no behavioural gain. Every recorder is a no-op until
// Setup runs, which is what keeps the existing service tests working untouched.
type Metrics struct {
	enabled bool
	reg     *prometheus.Registry
	host    string
	port    int
	path    string

	http      httpMetrics
	agent     agentMetrics
	llm       llmMetrics
	mcp       mcpMetrics
	execution executionMetrics
	executor  executorMetrics
	sse       sseMetrics
	plans     planMetrics
	db        dbMetrics
}

// def holds the process-wide default. Nil until Setup runs.
var def atomic.Pointer[Metrics]

// Default returns the metrics set installed by Setup, or nil.
func Default() *Metrics { return def.Load() }

// SetDefault installs m as the set the package-level recorders forward to.
func SetDefault(m *Metrics) { def.Store(m) }

// active returns the default when it is usable for recording.
func active() *Metrics {
	m := def.Load()
	if m == nil || !m.enabled {
		return nil
	}
	return m
}

// Setup builds the metric set, registers it and installs it as the default.
//
// It always returns a non-nil *Metrics: a disabled config yields a value whose
// every method is a no-op, so no caller has to guard against nil.
func Setup(cfg Config) (*Metrics, error) {
	m := &Metrics{
		enabled: cfg.Enabled,
		host:    cfg.Host,
		port:    cfg.Port,
		path:    cfg.Path,
	}
	if !cfg.Enabled {
		SetDefault(m)
		return m, nil
	}

	m.reg = prometheus.NewRegistry()
	m.http = newHTTPMetrics()
	m.agent = newAgentMetrics()
	m.llm = newLLMMetrics()
	m.mcp = newMCPMetrics()
	m.execution = newExecutionMetrics()
	m.executor = newExecutorMetrics()
	m.sse = newSSEMetrics()
	m.plans = newPlanMetrics()
	m.db = newDBMetrics()

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "build_info",
		Help:      "Build information for the running backend, always 1.",
	}, []string{"version", "go_version"})
	buildInfo.WithLabelValues(version.Version(), runtime.Version()).Set(1)

	startTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: Namespace,
		Name:      "start_time_seconds",
		Help:      "Unix timestamp of when the backend started.",
	})
	startTime.Set(float64(time.Now().Unix()))

	cs := []prometheus.Collector{
		// MetricsAll adds scheduler-latency and per-size GC histograms on top of
		// the default set. Worth the series count on a single-replica service:
		// an agent that wedges shows up here before it shows up anywhere else.
		collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll)),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
		buildInfo,
		startTime,
	}
	cs = append(cs, m.http.collectors()...)
	cs = append(cs, m.agent.collectors()...)
	cs = append(cs, m.llm.collectors()...)
	cs = append(cs, m.mcp.collectors()...)
	cs = append(cs, m.execution.collectors()...)
	cs = append(cs, m.executor.collectors()...)
	cs = append(cs, m.sse.collectors()...)
	cs = append(cs, m.plans.collectors()...)
	cs = append(cs, m.db.collectors()...)

	// Register rather than MustRegister: a duplicate registration panics, and a
	// second Setup -- a test, or a future reload through config.Manager -- must
	// not take the process down.
	for _, c := range cs {
		if err := m.reg.Register(c); err != nil {
			return nil, fmt.Errorf("metrics: register collector: %w", err)
		}
	}

	m.plans.seed()

	SetDefault(m)
	return m, nil
}

// Enabled reports whether metrics are being collected.
func (m *Metrics) Enabled() bool { return m != nil && m.enabled }

// Addr is the listen address for the metrics server.
func (m *Metrics) Addr() string {
	if m == nil {
		return ""
	}
	return net.JoinHostPort(m.host, strconv.Itoa(m.port))
}

// Path is the URL path the metrics handler is served at.
func (m *Metrics) Path() string {
	if m == nil {
		return ""
	}
	return m.path
}

// Registry exposes the private registry, for tests and custom collectors.
func (m *Metrics) Registry() *prometheus.Registry {
	if m == nil {
		return nil
	}
	return m.reg
}

// Handler serves the registry in Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	if !m.Enabled() {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
}
