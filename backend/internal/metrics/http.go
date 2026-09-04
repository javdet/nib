package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RouteUnmatched is the route label for a request chi never matched. Requests
// that miss the route table are the one unbounded input here, so they all
// collapse onto this constant instead of turning every scanned URL into a series.
const RouteUnmatched = "unmatched"

// RoutePreflight is the route label for a CORS preflight. corsMiddleware answers
// OPTIONS with 204 before routing happens, so those requests never get a pattern
// and would otherwise all pile into RouteUnmatched.
const RoutePreflight = "preflight"

type httpMetrics struct {
	requests  *prometheus.CounterVec
	duration  *prometheus.HistogramVec
	respBytes *prometheus.CounterVec
	inFlight  prometheus.Gauge
}

func newHTTPMetrics() httpMetrics {
	return httpMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "HTTP requests by method, chi route pattern and status code.",
		}, []string{"method", "route", "code"}),
		// No code label here: it would multiply the bucket count by the number
		// of status codes for no question anyone asks of latency.
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: Namespace,
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request latency by method and chi route pattern.",
			Buckets:   httpBuckets,
		}, []string{"method", "route"}),
		respBytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: Namespace,
			Subsystem: "http",
			Name:      "response_bytes_total",
			Help:      "Response bytes written by method and chi route pattern.",
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: Namespace,
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "HTTP requests currently being served.",
		}),
	}
}

func (h httpMetrics) collectors() []prometheus.Collector {
	return []prometheus.Collector{h.requests, h.duration, h.respBytes, h.inFlight}
}

// NormalizeMethod maps a request method onto the known set. r.Method is
// client-controlled and net/http will happily route "FOO /api/v1/health" into
// the middleware chain, so an unbounded value must never reach a label.
func NormalizeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodHead, http.MethodOptions:
		return method
	default:
		return "other"
	}
}

// ObserveHTTPRequest records one finished request. route must be a chi route
// pattern, RouteUnmatched or RoutePreflight -- never a concrete URL path.
func ObserveHTTPRequest(method, route string, code int, d time.Duration, respBytes int) {
	m := active()
	if m == nil {
		return
	}
	m.http.requests.WithLabelValues(method, route, strconv.Itoa(code)).Inc()
	m.http.duration.WithLabelValues(method, route).Observe(d.Seconds())
	if respBytes > 0 {
		m.http.respBytes.WithLabelValues(method, route).Add(float64(respBytes))
	}
}

// IncHTTPInFlight marks a request as started.
func IncHTTPInFlight() {
	if m := active(); m != nil {
		m.http.inFlight.Inc()
	}
}

// DecHTTPInFlight marks a request as finished.
func DecHTTPInFlight() {
	if m := active(); m != nil {
		m.http.inFlight.Dec()
	}
}
