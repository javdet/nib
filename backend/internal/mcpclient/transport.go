package mcpclient

import "net/http"

// AuthScheme controls the Authorization header format.
type AuthScheme string

const (
	AuthSchemeBearer AuthScheme = "bearer"
	AuthSchemeBasic  AuthScheme = "basic"
)

// authRoundTripper injects an Authorization header into every outgoing request.
type authRoundTripper struct {
	scheme   AuthScheme
	token    string
	delegate http.RoundTripper
}

func newAuthRoundTripper(scheme AuthScheme, token string) *authRoundTripper {
	return &authRoundTripper{
		scheme:   scheme,
		token:    token,
		delegate: http.DefaultTransport,
	}
}

func (rt *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	switch rt.scheme {
	case AuthSchemeBasic:
		r.Header.Set("Authorization", "Basic "+rt.token)
	default:
		r.Header.Set("Authorization", "Bearer "+rt.token)
	}
	return rt.delegate.RoundTrip(r)
}
