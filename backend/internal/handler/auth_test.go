package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/javdet/nib/internal/service"
)

const testToken = "0123456789abcdef0123456789abcdef"

func TestAPIAuthAuthenticated(t *testing.T) {
	t.Parallel()

	a := newAPIAuth(AuthOptions{Token: testToken})
	valid := a.newSession()

	expired := newAPIAuth(AuthOptions{Token: testToken})
	expired.now = func() time.Time { return time.Now().Add(-sessionTTL - time.Minute) }

	rotated := newAPIAuth(AuthOptions{Token: strings.Repeat("z", 32)})

	tests := []struct {
		name   string
		header string
		cookie string
		want   bool
	}{
		{name: "bearer", header: "Bearer " + testToken, want: true},
		{name: "wrong bearer", header: "Bearer " + strings.Repeat("x", 32)},
		{name: "basic scheme", header: "Basic " + testToken},
		{name: "no credentials"},
		{name: "session cookie", cookie: valid, want: true},
		{name: "expired cookie", cookie: expired.newSession()},
		{name: "tampered expiry", cookie: "9999999999" + valid[strings.Index(valid, "."):]},
		{name: "garbage cookie", cookie: "not-a-session"},
		{name: "cookie signed by a rotated token", cookie: rotated.newSession()},
		// A wrong header is not rescued by a good cookie: the caller said
		// which credential it meant.
		{name: "bad bearer beside good cookie", header: "Bearer nope", cookie: valid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/api/v1/rules", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tt.cookie})
			}
			if got := a.authenticated(r); got != tt.want {
				t.Fatalf("authenticated = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIAuthRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts AuthOptions
		want bool
	}{
		{name: "token", opts: AuthOptions{Token: testToken}, want: true},
		{name: "explicit opt-out", opts: AuthOptions{Insecure: true}, want: false},
		{name: "token wins over opt-out", opts: AuthOptions{Token: testToken, Insecure: true}, want: true},
		{name: "nothing configured fails closed", opts: AuthOptions{}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := newAPIAuth(tt.opts).required(); got != tt.want {
				t.Fatalf("required = %v, want %v", got, tt.want)
			}
		})
	}

	// With neither a token nor the opt-out nothing may authenticate, not even
	// an empty bearer that would match the empty token.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer ")
	if newAPIAuth(AuthOptions{}).authenticated(r) {
		t.Fatal("authenticated = true with no token configured, want false")
	}
}

func newTestRouter(opts AuthOptions, webhookToken string) http.Handler {
	return NewRouter(Deps{
		Auth:              opts,
		Selection:         service.NewSelectionStore(),
		AgentWebhookToken: webhookToken,
	})
}

func serve(h http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRouterAuth(t *testing.T) {
	t.Parallel()

	h := newTestRouter(AuthOptions{Token: testToken, AllowedOrigins: []string{"http://localhost:5173"}}, "")
	bearer := "Bearer " + testToken
	jsonCT := "application/json"

	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		headers map[string]string
		want    int
	}{
		{name: "health is public", method: http.MethodGet, path: "/api/v1/health", want: http.StatusOK},
		{name: "version is public", method: http.MethodGet, path: "/api/v1/version", want: http.StatusOK},
		{name: "session status is public", method: http.MethodGet, path: "/api/v1/auth/session", want: http.StatusOK},
		{name: "protected without credentials", method: http.MethodGet, path: "/api/v1/selection", want: http.StatusUnauthorized},
		{name: "modes is protected", method: http.MethodGet, path: "/api/v1/modes", want: http.StatusUnauthorized},
		{name: "protected with bearer", method: http.MethodGet, path: "/api/v1/selection",
			headers: map[string]string{"Authorization": bearer}, want: http.StatusOK},
		{name: "json write with bearer", method: http.MethodPut, path: "/api/v1/selection", body: `{}`,
			headers: map[string]string{"Authorization": bearer, "Content-Type": jsonCT}, want: http.StatusOK},
		{name: "json with charset", method: http.MethodPut, path: "/api/v1/selection", body: `{}`,
			headers: map[string]string{"Authorization": bearer, "Content-Type": "application/json; charset=utf-8"}, want: http.StatusOK},
		{name: "text/plain body is refused", method: http.MethodPut, path: "/api/v1/selection", body: `{}`,
			headers: map[string]string{"Authorization": bearer, "Content-Type": "text/plain"}, want: http.StatusUnsupportedMediaType},
		{name: "missing content type is refused", method: http.MethodPut, path: "/api/v1/selection", body: `{}`,
			headers: map[string]string{"Authorization": bearer}, want: http.StatusUnsupportedMediaType},
		{name: "cross-site write", method: http.MethodPut, path: "/api/v1/selection", body: `{}`,
			headers: map[string]string{"Authorization": bearer, "Content-Type": jsonCT, "Sec-Fetch-Site": "cross-site"},
			want:    http.StatusForbidden},
		{name: "foreign origin write", method: http.MethodPut, path: "/api/v1/selection", body: `{}`,
			headers: map[string]string{"Authorization": bearer, "Content-Type": jsonCT, "Origin": "https://evil.example"},
			want:    http.StatusForbidden},
		{name: "cross-site login", method: http.MethodPost, path: "/api/v1/auth/session", body: `{"token":"` + testToken + `"}`,
			headers: map[string]string{"Content-Type": jsonCT, "Sec-Fetch-Site": "cross-site"}, want: http.StatusForbidden},
		{name: "trusted origin write", method: http.MethodPut, path: "/api/v1/selection", body: `{}`,
			headers: map[string]string{"Authorization": bearer, "Content-Type": jsonCT, "Origin": "http://localhost:5173"},
			want:    http.StatusOK},
		{name: "same-origin write", method: http.MethodPut, path: "/api/v1/selection", body: `{}`,
			headers: map[string]string{"Authorization": bearer, "Content-Type": jsonCT, "Sec-Fetch-Site": "same-origin"},
			want:    http.StatusOK},
		{name: "cross-site read is not a write", method: http.MethodGet, path: "/api/v1/selection",
			headers: map[string]string{"Authorization": bearer, "Sec-Fetch-Site": "cross-site"}, want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := serve(h, tt.method, tt.path, tt.body, tt.headers)
			if w.Code != tt.want {
				t.Fatalf("%s %s = %d, want %d (body %s)", tt.method, tt.path, w.Code, tt.want, w.Body.String())
			}
		})
	}
}

func TestRouterLoginFlow(t *testing.T) {
	t.Parallel()

	h := newTestRouter(AuthOptions{Token: testToken}, "")
	jsonCT := map[string]string{"Content-Type": "application/json"}

	if w := serve(h, http.MethodPost, "/api/v1/auth/session", `{"token":"wrong"}`, jsonCT); w.Code != http.StatusUnauthorized {
		t.Fatalf("login with wrong token = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	w := serve(h, http.MethodPost, "/api/v1/auth/session", `{"token":"`+testToken+`"}`, jsonCT)
	if w.Code != http.StatusNoContent {
		t.Fatalf("login = %d, want %d (body %s)", w.Code, http.StatusNoContent, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login set %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != sessionCookieName || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != sessionCookiePath {
		t.Fatalf("cookie = %+v, want HttpOnly SameSite=Strict %s on %s", c, sessionCookieName, sessionCookiePath)
	}
	if strings.Contains(c.Value, testToken) {
		t.Fatal("session cookie carries the API token")
	}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/selection", nil)
	r.AddCookie(c)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET with session cookie = %d, want %d", rec.Code, http.StatusOK)
	}

	r = httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	r.AddCookie(c)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if got, want := strings.TrimSpace(rec.Body.String()), `{"required":true,"authenticated":true}`; got != want {
		t.Fatalf("session status = %s, want %s", got, want)
	}

	w = serve(h, http.MethodDelete, "/api/v1/auth/session", "", nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want %d", w.Code, http.StatusNoContent)
	}
	if out := w.Result().Cookies(); len(out) != 1 || out[0].MaxAge >= 0 {
		t.Fatalf("logout cookies = %+v, want one expiring cookie", out)
	}

	secure := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", strings.NewReader(`{"token":"`+testToken+`"}`))
	secure.Header.Set("Content-Type", "application/json")
	secure.Header.Set("X-Forwarded-Proto", "https")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, secure)
	if out := rec.Result().Cookies(); len(out) != 1 || !out[0].Secure {
		t.Fatalf("cookies behind https = %+v, want one Secure cookie", out)
	}
}

func TestRouterInsecureMode(t *testing.T) {
	t.Parallel()

	h := newTestRouter(AuthOptions{Insecure: true}, "")

	if w := serve(h, http.MethodGet, "/api/v1/selection", "", nil); w.Code != http.StatusOK {
		t.Fatalf("GET without credentials in insecure mode = %d, want %d", w.Code, http.StatusOK)
	}
	w := serve(h, http.MethodGet, "/api/v1/auth/session", "", nil)
	if got, want := strings.TrimSpace(w.Body.String()), `{"required":false,"authenticated":true}`; got != want {
		t.Fatalf("session status = %s, want %s", got, want)
	}
	// Opting out of auth does not opt out of CSRF protection.
	w = serve(h, http.MethodPut, "/api/v1/selection", `{}`, map[string]string{"Content-Type": "text/plain"})
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain write in insecure mode = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestRouterHostCheck(t *testing.T) {
	t.Parallel()

	h := newTestRouter(AuthOptions{Insecure: true, AllowedHosts: []string{"nib.example.com", "localhost:8080"}}, "")

	tests := []struct {
		name string
		host string
		path string
		want int
	}{
		{name: "allowed bare name", host: "nib.example.com", path: "/api/v1/selection", want: http.StatusOK},
		{name: "allowed bare name with port", host: "nib.example.com:443", path: "/api/v1/selection", want: http.StatusOK},
		{name: "allowed host:port", host: "localhost:8080", path: "/api/v1/selection", want: http.StatusOK},
		{name: "same name other port", host: "localhost:9999", path: "/api/v1/selection", want: http.StatusMisdirectedRequest},
		{name: "rebound name", host: "attacker.example", path: "/api/v1/selection", want: http.StatusMisdirectedRequest},
		// Probes arrive with the pod IP as Host.
		{name: "health ignores host", host: "10.0.0.7:8080", path: "/api/v1/health", want: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			r.Host = tt.host
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.want {
				t.Fatalf("GET %s with Host %q = %d, want %d", tt.path, tt.host, w.Code, tt.want)
			}
		})
	}
}

func TestAgentWebhookAuthorize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		insecure bool
		header   string
		want     bool
	}{
		{name: "matching token", token: "hook", header: "Bearer hook", want: true},
		{name: "wrong token", token: "hook", header: "Bearer nope"},
		{name: "empty token fails closed", header: "Bearer anything"},
		{name: "empty token without header fails closed"},
		{name: "empty token in insecure mode", insecure: true, want: true},
		{name: "insecure mode does not bypass a configured token", token: "hook", insecure: true, header: "Bearer nope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := NewAgentWebhookHandler(nil, tt.token, tt.insecure)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/agent-runner/webhook", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			if got := h.authorize(r); got != tt.want {
				t.Fatalf("authorize = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRouterWebhookOutsideAPIAuth(t *testing.T) {
	t.Parallel()

	h := newTestRouter(AuthOptions{Token: testToken}, "hook")

	// The webhook answers to its own token, not the API's.
	w := serve(h, http.MethodPost, "/api/v1/agent-runner/webhook", `{}`,
		map[string]string{"Authorization": "Bearer hook", "Content-Type": "application/json"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("webhook with its token = %d, want %d (past auth, missing chat_id)", w.Code, http.StatusBadRequest)
	}
	w = serve(h, http.MethodPost, "/api/v1/agent-runner/webhook", `{}`,
		map[string]string{"Authorization": "Bearer " + testToken, "Content-Type": "application/json"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("webhook with the API token = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
