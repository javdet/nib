package mcpclient

import "net/http"

// AuthScheme controls the Authorization header format.
type AuthScheme string

const (
	AuthSchemeBearer AuthScheme = "bearer"
	AuthSchemeBasic  AuthScheme = "basic"
)

// authRoundTripper injects an Authorization header into every outgoing request
// addressed to the connection's own origin.
type authRoundTripper struct {
	scheme   AuthScheme
	token    string
	origin   origin
	originOK bool
	delegate http.RoundTripper
}

func newAuthRoundTripper(endpoint string, scheme AuthScheme, token string) *authRoundTripper {
	o, ok := parseOrigin(endpoint)
	return &authRoundTripper{
		scheme:   scheme,
		token:    token,
		origin:   o,
		originOK: ok,
		delegate: http.DefaultTransport,
	}
}

func (rt *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !forOrigin(req, rt.origin, rt.originOK) {
		return rt.delegate.RoundTrip(req)
	}
	r := req.Clone(req.Context())
	switch rt.scheme {
	case AuthSchemeBasic:
		r.Header.Set("Authorization", "Basic "+rt.token)
	default:
		r.Header.Set("Authorization", "Bearer "+rt.token)
	}
	return rt.delegate.RoundTrip(r)
}
