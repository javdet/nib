package oauth

import (
	"context"
	"time"

	"golang.org/x/oauth2"
)

// TokenResult holds the tokens returned after a successful OAuth exchange or refresh.
type TokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    *time.Time
}

// Provider abstracts the OAuth 2.1 flow for a specific MCP server provider.
type Provider interface {
	// Type returns the provider identifier (e.g. "atlassian").
	Type() string

	// AuthCodeURL builds the authorization URL that the user must visit.
	// The state and PKCE code verifier are managed by the caller via StateStore.
	AuthCodeURL(state, codeChallenge string) string

	// Exchange trades an authorization code + PKCE verifier for tokens.
	Exchange(ctx context.Context, code, codeVerifier string) (*TokenResult, error)

	// Refresh obtains a new access token using a refresh token.
	Refresh(ctx context.Context, refreshToken string) (*TokenResult, error)

	// DefaultServerURL returns the default MCP server endpoint for this provider.
	DefaultServerURL() string
}

func tokenResultFromOAuth2(tok *oauth2.Token) *TokenResult {
	tr := &TokenResult{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
	}
	if !tok.Expiry.IsZero() {
		t := tok.Expiry
		tr.ExpiresAt = &t
	}
	return tr
}
