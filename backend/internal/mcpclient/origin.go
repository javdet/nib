package mcpclient

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrCrossOriginRedirect is returned when an MCP server redirects to another
// origin. The request is not followed at all: its body is a JSON-RPC payload
// carrying tool arguments, which are no business of the redirect's target.
var ErrCrossOriginRedirect = errors.New("mcp server redirected to another origin")

// maxRedirects matches net/http's own default policy, which a custom
// CheckRedirect replaces.
const maxRedirects = 10

// origin is the scheme, host and port credentials are bound to. Ports are
// spelled out, so https://host and https://host:443 compare equal.
type origin struct {
	scheme, host, port string
}

func originOf(u *url.URL) (origin, bool) {
	if u == nil {
		return origin{}, false
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if scheme == "" || host == "" {
		return origin{}, false
	}
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return origin{scheme: scheme, host: host, port: port}, true
}

func parseOrigin(raw string) (origin, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return origin{}, false
	}
	return originOf(u)
}

// SameOrigin reports whether two URLs share scheme, host and port. A URL that
// does not parse matches nothing, so a caller asking "may the credential
// follow?" gets no.
func SameOrigin(a, b string) bool {
	oa, ok := parseOrigin(a)
	if !ok {
		return false
	}
	ob, ok := parseOrigin(b)
	return ok && oa == ob
}

// newHTTPClient builds the client an MCP transport talks through. Credentials
// are added by rt, which runs again on every redirect hop — net/http strips
// Authorization before a cross-domain hop, but a RoundTripper puts it straight
// back. So a redirect leaving the endpoint's origin is refused here, and rt
// itself only adds credentials for that origin.
func newHTTPClient(timeout time.Duration, rt http.RoundTripper) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     rt,
		CheckRedirect: refuseCrossOriginRedirect,
	}
}

func refuseCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	from, okFrom := originOf(via[0].URL)
	to, okTo := originOf(req.URL)
	if !okFrom || !okTo || from != to {
		// Only the host is named: the Location may echo a query string back.
		return fmt.Errorf("%w: %s", ErrCrossOriginRedirect, req.URL.Host)
	}
	return nil
}

// forOrigin reports whether req is addressed to want. A zero want, from an
// endpoint that did not parse, matches nothing.
func forOrigin(req *http.Request, want origin, wantOK bool) bool {
	if !wantOK {
		return false
	}
	got, ok := originOf(req.URL)
	return ok && got == want
}
