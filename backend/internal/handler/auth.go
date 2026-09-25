package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// AuthOptions guards /api/v1. Built in main.go from config.AuthConfig.
type AuthOptions struct {
	// Token is accepted as "Authorization: Bearer" and exchanged for a
	// session cookie at login.
	Token string
	// Insecure turns authentication off. It only has an effect while Token is
	// empty: a configured token is always enforced.
	Insecure bool
	// AllowedOrigins may make cross-origin writes besides the API's own origin.
	AllowedOrigins []string
	// AllowedHosts, when non-empty, is the only set of Host headers served.
	AllowedHosts []string
}

const (
	sessionCookieName = "nib_session"
	sessionTTL        = 30 * 24 * time.Hour
	// sessionCookiePath covers every API route and nothing the SPA serves.
	sessionCookiePath = "/api/"
)

// apiAuth checks a request's credentials. The session cookie is stateless: an
// expiry signed with a key derived from the token, so rotating NIB_API_TOKEN
// signs every existing session out and the cookie never carries the token.
type apiAuth struct {
	token    []byte
	key      []byte
	insecure bool
	now      func() time.Time
}

func newAPIAuth(opts AuthOptions) *apiAuth {
	token := strings.TrimSpace(opts.Token)
	key := sha256.Sum256([]byte("nib-session-key|" + token))
	return &apiAuth{
		token:    []byte(token),
		key:      key[:],
		insecure: opts.Insecure && token == "",
		now:      time.Now,
	}
}

// required is false only when the operator opted out explicitly. An empty
// token without that opt-out fails closed: nothing authenticates.
func (a *apiAuth) required() bool {
	return !a.insecure
}

func (a *apiAuth) validToken(candidate string) bool {
	if len(a.token) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), a.token) == 1
}

func (a *apiAuth) authenticated(r *http.Request) bool {
	if !a.required() {
		return true
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); auth != "" {
		const prefix = "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			return false
		}
		return a.validToken(strings.TrimSpace(strings.TrimPrefix(auth, prefix)))
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return a.validSession(c.Value)
}

func (a *apiAuth) sign(exp int64) string {
	mac := hmac.New(sha256.New, a.key)
	mac.Write([]byte("nib-session-v1|" + strconv.FormatInt(exp, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *apiAuth) newSession() string {
	exp := a.now().Add(sessionTTL).Unix()
	return strconv.FormatInt(exp, 10) + "." + a.sign(exp)
}

func (a *apiAuth) validSession(value string) bool {
	if len(a.token) == 0 {
		return false
	}
	expRaw, sig, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil || a.now().Unix() >= exp {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(a.sign(exp)))
}

// authMiddleware refuses a request that carries neither a valid bearer token
// nor a valid session cookie.
func authMiddleware(a *apiAuth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !a.authenticated(r) {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// hostCheck serves only the configured Host headers, which is what stops a DNS
// rebinding page from reaching the API as same-origin. An empty list serves
// every host.
func hostCheck(allowed []string) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, h := range allowed {
		set[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		if len(set) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := strings.ToLower(r.Host)
			name := host
			if h, _, err := net.SplitHostPort(host); err == nil {
				name = h
			}
			_, withPort := set[host]
			_, bare := set[name]
			if !withPort && !bare {
				writeError(w, http.StatusMisdirectedRequest, "host not allowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// crossOriginProtection rejects a state-changing request a browser sent from
// another origin, judged by Sec-Fetch-Site or by Origin against Host. A request
// with neither header (curl, the agent-runner) is not a browser's and passes.
// This is what closes CSRF through "simple" requests, which CORS never stops:
// CORS only hides the response, the request has already run.
func crossOriginProtection(trusted []string) func(http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	for _, o := range trusted {
		if err := cop.AddTrustedOrigin(o); err != nil {
			// config validates the list, so this is a programming error.
			slog.Error("ignoring invalid trusted origin", "origin", o, "error", err)
		}
	}
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusForbidden, "cross-origin request refused")
	}))
	return cop.Handler
}

// AuthHandler serves the browser login: the operator pastes the API token once
// and receives an HttpOnly cookie, because EventSource and <img> cannot send
// an Authorization header.
type AuthHandler struct {
	auth *apiAuth
}

func NewAuthHandler(auth *apiAuth) *AuthHandler {
	return &AuthHandler{auth: auth}
}

type sessionResponse struct {
	Required      bool `json:"required"`
	Authenticated bool `json:"authenticated"`
}

type loginRequest struct {
	Token string `json:"token"`
}

// Session reports whether the SPA has to show the login screen.
func (h *AuthHandler) Session() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, sessionResponse{
			Required:      h.auth.required(),
			Authenticated: h.auth.authenticated(r),
		})
	}
}

func (h *AuthHandler) Login() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.auth.required() {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		var req loginRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if !h.auth.validToken(strings.TrimSpace(req.Token)) {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		http.SetCookie(w, sessionCookie(r, h.auth.newSession(), int(sessionTTL.Seconds())))
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *AuthHandler) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, sessionCookie(r, "", -1))
		w.WriteHeader(http.StatusNoContent)
	}
}

// sessionCookie builds the cookie; a negative maxAge deletes it.
func sessionCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     sessionCookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		// Strict, not Lax: no route here is ever reached by following a link
		// from another site, so the cookie never needs to ride a cross-site
		// navigation.
		SameSite: http.SameSiteStrictMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
	}
}
