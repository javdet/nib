package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// slowHandlerServer serves a handler that answers after the server WriteTimeout
// has already elapsed, which is what an LLM agent run looks like to net/http.
func slowHandlerServer(t *testing.T, wrap func(http.Handler) http.Handler) *httptest.Server {
	t.Helper()

	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	if wrap != nil {
		r.With(wrap).Post("/slow", slow)
	} else {
		r.Post("/slow", slow)
	}

	srv := httptest.NewUnstartedServer(r)
	srv.Config.WriteTimeout = 100 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func postSlow(t *testing.T, srv *httptest.Server) (*http.Response, error) {
	t.Helper()
	return srv.Client().Post(srv.URL+"/slow", "application/json", strings.NewReader("{}"))
}

func TestWriteDeadlineKeepsSlowResponseAlive(t *testing.T) {
	srv := slowHandlerServer(t, writeDeadline(time.Minute))

	resp, err := postSlow(t, srv)
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestWithoutWriteDeadlineSlowResponseIsDropped(t *testing.T) {
	srv := slowHandlerServer(t, nil)

	resp, err := postSlow(t, srv)
	if err == nil {
		resp.Body.Close()
		t.Fatalf("status = %d, want the connection to be dropped", resp.StatusCode)
	}
}
