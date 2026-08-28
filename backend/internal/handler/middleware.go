package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// agentWriteTimeout is the response budget for endpoints that block on an LLM
// agent run. Such runs routinely outlast the server-wide WriteTimeout, and once
// that deadline passes the final write fails and the connection is dropped even
// though the agent already finished and persisted its work.
const agentWriteTimeout = 30 * time.Minute

// writeDeadline replaces the server WriteTimeout with d for the wrapped routes.
func writeDeadline(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc := http.NewResponseController(w)
			if err := rc.SetWriteDeadline(time.Now().Add(d)); err != nil {
				slog.Warn("cannot extend write deadline, slow responses may be dropped",
					"path", r.URL.Path,
					"error", err,
				)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[strings.TrimRight(o, "/")] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if _, ok := originSet[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
