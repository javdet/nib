package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/javdet/nib/internal/metrics"
)

// httpMetrics records request counts, latency and response size.
//
// It goes after middleware.Logger and before middleware.Recoverer. After
// Logger, because Logger has already wrapped the writer and reusing that same
// object keeps anything new from coming between a handler and the real
// ResponseWriter -- dialog_events.go type-asserts a flusher straight off w, and
// writeDeadline walks Unwrap() with http.NewResponseController. Before
// Recoverer, because Recoverer writes its 500 on the writer it was handed, so
// ww.Status() reads that, and an http.ErrAbortHandler it re-panics still unwinds
// through the deferred record below.
func httpMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww, ok := w.(middleware.WrapResponseWriter)
		if !ok {
			ww = middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		}

		method := metrics.NormalizeMethod(r.Method)
		isPreflight := r.Method == http.MethodOptions
		start := time.Now()

		metrics.IncHTTPInFlight()
		defer func() {
			metrics.DecHTTPInFlight()

			status := ww.Status()
			if status == 0 {
				// Nothing was written -- an aborted stream, or a handler that
				// just returned. net/http would have sent 200.
				status = http.StatusOK
			}
			metrics.ObserveHTTPRequest(method, routeLabel(r, isPreflight), status,
				time.Since(start), ww.BytesWritten())
		}()

		next.ServeHTTP(ww, r)
	})
}

// routeLabel resolves the chi route pattern for a finished request.
//
// It must be called after the handler has run: chi puts its *chi.Context into
// the request context before the middleware chain and fills the pattern in
// during routing. Using r.URL.Path instead would turn every dialog UUID into its
// own time series -- most of this router's paths carry an id.
func routeLabel(r *http.Request, isPreflight bool) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	if isPreflight {
		// corsMiddleware answers preflights with 204 before routing, so they
		// never get a pattern and would otherwise all read as unmatched.
		return metrics.RoutePreflight
	}
	return metrics.RouteUnmatched
}
