package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/javdet/nib/internal/metrics"
)

// metricsRouter mounts the middleware chain the real router uses, in the same
// order, so these tests exercise the composition and not just the middleware.
func metricsRouter(t *testing.T, register func(chi.Router)) (chi.Router, *metrics.Metrics) {
	t.Helper()

	m, err := metrics.Setup(metrics.Config{
		Enabled: true, Host: "127.0.0.1", Port: 9090, Path: "/metrics", RefreshSeconds: 30,
	})
	if err != nil {
		t.Fatalf("metrics.Setup: %v", err)
	}
	prev := metrics.Default()
	t.Cleanup(func() { metrics.SetDefault(prev) })
	metrics.SetDefault(m)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(httpMetrics)
	r.Use(middleware.Recoverer)
	register(r)
	return r, m
}

// TestHTTPMetricsPreservesFlusher is the SSE regression guard.
//
// dialog_events.go type-asserts a flusher straight off the ResponseWriter and,
// when that fails, answers 500 "streaming not supported" with no log line at
// all -- the plan view would just stop updating. A wrapper that hides Flush()
// breaks every event stream silently, so this asserts it does not.
func TestHTTPMetricsPreservesFlusher(t *testing.T) {
	var sawFlusher bool

	r, _ := metricsRouter(t, func(r chi.Router) {
		r.Get("/stream", func(w http.ResponseWriter, _ *http.Request) {
			_, sawFlusher = w.(interface{ Flush() })
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if !sawFlusher {
		t.Fatal("the handler could not assert http.Flusher: SSE would answer 500")
	}
}

// TestHTTPMetricsPreservesResponseController is the write-deadline regression
// guard.
//
// writeDeadline extends the response budget to 30 minutes for the agent routes
// through http.NewResponseController, which walks Unwrap(). It only slog.Warns
// when that fails and then carries on, so a wrapper without Unwrap sends the
// agent routes back to being dropped at the 120s server WriteTimeout -- after
// the agent has already finished and persisted its work.
func TestHTTPMetricsPreservesResponseController(t *testing.T) {
	var deadlineErr error

	r, _ := metricsRouter(t, func(r chi.Router) {
		r.Get("/long", func(w http.ResponseWriter, _ *http.Request) {
			deadlineErr = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Hour))
			w.WriteHeader(http.StatusOK)
		})
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL + "/long")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if deadlineErr != nil {
		t.Fatalf("SetWriteDeadline through the metrics wrapper: %v", deadlineErr)
	}
}

// TestHTTPMetricsComposesWithWriteDeadline runs both middlewares over a handler
// slower than the server WriteTimeout, which is what an agent run looks like.
func TestHTTPMetricsComposesWithWriteDeadline(t *testing.T) {
	m, err := metrics.Setup(metrics.Config{Enabled: true, Port: 9090, Path: "/metrics", RefreshSeconds: 30})
	if err != nil {
		t.Fatalf("metrics.Setup: %v", err)
	}
	prev := metrics.Default()
	t.Cleanup(func() { metrics.SetDefault(prev) })
	metrics.SetDefault(m)

	srv := slowHandlerServer(t, func(next http.Handler) http.Handler {
		return httpMetrics(writeDeadline(time.Minute)(next))
	})

	resp, err := postSlow(t, srv)
	if err != nil {
		t.Fatalf("POST /slow: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	// The slow request must also have been recorded, not lost to the deadline.
	if got := countRequests(t, m, http.MethodPost, "/slow", "200"); got != 1 {
		t.Errorf("slow request recorded = %v, want 1", got)
	}
}

func TestHTTPMetricsRouteLabel(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		wantRoute string
		wantCode  string
	}{
		{
			name:   "a pattern with an id keeps the placeholder",
			method: http.MethodGet,
			path:   "/api/v1/dialogs/2f1c2c1e-0000-4000-8000-000000000000",
			// The concrete UUID must never reach a label: most of this router's
			// paths carry an id, so r.URL.Path would mean one series per dialog.
			wantRoute: "/api/v1/dialogs/{id}",
			wantCode:  "200",
		},
		{
			name:      "a nested pattern comes back whole",
			method:    http.MethodPost,
			path:      "/api/v1/dialogs/2f1c2c1e-0000-4000-8000-000000000000/messages",
			wantRoute: "/api/v1/dialogs/{id}/messages",
			wantCode:  "200",
		},
		{
			name:      "an unrouted path collapses",
			method:    http.MethodGet,
			path:      "/api/v1/nothing-here",
			wantRoute: metrics.RouteUnmatched,
			wantCode:  "404",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			r, m := metricsRouter(t, func(r chi.Router) {
				r.Route("/api/v1/dialogs/{id}", func(r chi.Router) {
					r.Get("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
					r.Post("/messages", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
				})
			})

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(""))
			r.ServeHTTP(httptest.NewRecorder(), req)

			if got := countRequests(t, m, tt.method, tt.wantRoute, tt.wantCode); got != 1 {
				t.Errorf("requests{method=%q,route=%q,code=%q} = %v, want 1",
					tt.method, tt.wantRoute, tt.wantCode, got)
			}
		})
	}
}

// TestHTTPMetricsPreflightRoute covers the one case corsMiddleware creates:
// it answers OPTIONS with 204 before routing, so a preflight never gets a
// pattern and would otherwise pile into the unmatched label.
func TestHTTPMetricsPreflightRoute(t *testing.T) {
	m, err := metrics.Setup(metrics.Config{Enabled: true, Port: 9090, Path: "/metrics", RefreshSeconds: 30})
	if err != nil {
		t.Fatalf("metrics.Setup: %v", err)
	}
	prev := metrics.Default()
	t.Cleanup(func() { metrics.SetDefault(prev) })
	metrics.SetDefault(m)

	r := chi.NewRouter()
	r.Use(httpMetrics)
	r.Use(corsMiddleware([]string{"http://localhost:5173"}))
	r.Get("/api/v1/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if got := countRequests(t, m, http.MethodOptions, metrics.RoutePreflight, "204"); got != 1 {
		t.Errorf("preflight requests = %v, want 1", got)
	}
}

// TestHTTPMetricsCountsPanicAs500 pins down the ordering with Recoverer: our
// deferred record runs while the panic is still unwinding, so the status has to
// come from the writer Recoverer wrote its 500 on.
func TestHTTPMetricsCountsPanicAs500(t *testing.T) {
	r, m := metricsRouter(t, func(r chi.Router) {
		r.Get("/boom", func(http.ResponseWriter, *http.Request) {
			panic("handler exploded")
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if got := countRequests(t, m, http.MethodGet, "/boom", "500"); got != 1 {
		t.Errorf("panicking request recorded as 500 = %v, want 1", got)
	}
	if got := gaugeValue(t, m, "nib_http_requests_in_flight"); got != 0 {
		t.Errorf("in-flight after a panic = %v, want 0", got)
	}
}

func TestHTTPMetricsInFlightReturnsToZero(t *testing.T) {
	r, m := metricsRouter(t, func(r chi.Router) {
		r.Get("/ok", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	})

	for i := 0; i < 3; i++ {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil))
	}

	if got := gaugeValue(t, m, "nib_http_requests_in_flight"); got != 0 {
		t.Errorf("in-flight = %v, want 0", got)
	}
}

// TestHTTPMetricsUnknownMethod pins the method label: net/http will route an
// arbitrary method into the chain.
func TestHTTPMetricsUnknownMethod(t *testing.T) {
	m, err := metrics.Setup(metrics.Config{Enabled: true, Port: 9090, Path: "/metrics", RefreshSeconds: 30})
	if err != nil {
		t.Fatalf("metrics.Setup: %v", err)
	}
	prev := metrics.Default()
	t.Cleanup(func() { metrics.SetDefault(prev) })
	metrics.SetDefault(m)

	r := chi.NewRouter()
	r.Use(httpMetrics)
	r.Get("/ok", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest("FOO", "/ok", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	if got := countRequests(t, m, "other", metrics.RouteUnmatched, "405"); got != 1 {
		t.Errorf("unknown-method requests = %v, want 1", got)
	}
}

func countRequests(t *testing.T, m *metrics.Metrics, method, route, code string) float64 {
	t.Helper()
	return labeledValue(t, m, "nib_http_requests_total", map[string]string{
		"method": method, "route": route, "code": code,
	})
}

func gaugeValue(t *testing.T, m *metrics.Metrics, name string) float64 {
	t.Helper()
	return labeledValue(t, m, name, nil)
}

func labeledValue(t *testing.T, m *metrics.Metrics, name string, labels map[string]string) float64 {
	t.Helper()

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, metric := range f.GetMetric() {
			got := make(map[string]string, len(metric.GetLabel()))
			for _, l := range metric.GetLabel() {
				got[l.GetName()] = l.GetValue()
			}
			match := true
			for k, v := range labels {
				if got[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
			if c := metric.GetCounter(); c != nil {
				return c.GetValue()
			}
			if g := metric.GetGauge(); g != nil {
				return g.GetValue()
			}
		}
		return 0
	}
	return 0
}
