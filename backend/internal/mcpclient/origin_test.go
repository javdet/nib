package mcpclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSameOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want bool
	}{
		{"https://api.example.com/mcp", "https://api.example.com/other?x=1", true},
		{"https://api.example.com/mcp", "https://API.example.com:443/mcp", true},
		{"http://kb:80/mcp", "http://kb/mcp", true},
		{"https://api.example.com/mcp", "http://api.example.com/mcp", false},
		{"https://api.example.com/mcp", "https://api.example.com:8443/mcp", false},
		{"https://api.example.com/mcp", "https://evil.test/mcp", false},
		{"https://api.example.com/mcp", "https://api.example.com.evil.test/mcp", false},
		{"", "https://api.example.com/mcp", false},
		{"not a url", "not a url", false},
	}
	for _, tt := range tests {
		if got := SameOrigin(tt.a, tt.b); got != tt.want {
			t.Errorf("SameOrigin(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// A redirecting server used to get its credentials forwarded: net/http strips
// Authorization before a cross-domain hop, and the round tripper put it back.
func TestHTTPClientDoesNotForwardCredentialsAcrossOrigins(t *testing.T) {
	t.Parallel()

	var otherHits atomic.Int32
	var leaked atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		leaked.Store(r.Header.Get("X-Api-Key") + "|" + r.Header.Get("Authorization"))
	}))
	defer other.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	clients := map[string]*http.Client{
		"headers": newHTTPClient(time.Second, newHeadersRoundTripper(origin.URL+"/mcp",
			map[string]string{"X-Api-Key": "secret"}, http.DefaultTransport)),
		"auth": newHTTPClient(time.Second, newAuthRoundTripper(origin.URL+"/mcp", AuthSchemeBearer, "secret")),
	}
	for name, client := range clients {
		t.Run(name, func(t *testing.T) {
			resp, err := client.Get(origin.URL + "/mcp")
			if err == nil {
				resp.Body.Close()
			}
			if !errors.Is(err, ErrCrossOriginRedirect) {
				t.Fatalf("error = %v, want ErrCrossOriginRedirect", err)
			}
			if n := otherHits.Load(); n != 0 {
				t.Fatalf("redirect target hit %d times (headers %v), want 0", n, leaked.Load())
			}
		})
	}
}

func TestRoundTrippersAddCredentialsOnlyForTheirOrigin(t *testing.T) {
	t.Parallel()

	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("X-Api-Key") + "|" + r.Header.Get("Authorization"))
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{"own origin", srv.URL + "/mcp", "secret|Bearer secret"},
		{"other origin", "https://api.example.com/mcp", "|"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newHeadersRoundTripper(tt.endpoint, map[string]string{"X-Api-Key": "secret"},
				newAuthRoundTripper(tt.endpoint, AuthSchemeBearer, "secret"))
			resp, err := newHTTPClient(time.Second, rt).Get(srv.URL + "/mcp")
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			resp.Body.Close()
			if g := got.Load(); g != tt.want {
				t.Fatalf("headers seen = %q, want %q", g, tt.want)
			}
		})
	}
}
