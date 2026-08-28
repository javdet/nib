package oauth

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
)

const (
	atlassianAuthURL  = "https://auth.atlassian.com/authorize"
	atlassianTokenURL = "https://auth.atlassian.com/oauth/token"
	defaultAtlassianMCPURL = "https://mcp.atlassian.com/v1/mcp"
)

// AtlassianProvider implements the OAuth 2.1 flow for the Atlassian MCP server.
type AtlassianProvider struct {
	cfg         oauth2.Config
	mcpURL      string
}

// AtlassianConfig holds the configuration needed for the Atlassian OAuth provider.
type AtlassianConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	MCPURL       string
	Scopes       []string
}

func NewAtlassianProvider(cfg AtlassianConfig) *AtlassianProvider {
	if cfg.MCPURL == "" {
		cfg.MCPURL = defaultAtlassianMCPURL
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{
			"read:jira-work",
			"write:jira-work",
			"read:confluence-content.all",
			"write:confluence-content",
			"offline_access",
		}
	}

	return &AtlassianProvider{
		cfg: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Scopes:       cfg.Scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:  atlassianAuthURL,
				TokenURL: atlassianTokenURL,
			},
		},
		mcpURL: cfg.MCPURL,
	}
}

func (p *AtlassianProvider) Type() string {
	return "jira"
}

func (p *AtlassianProvider) AuthCodeURL(state, codeChallenge string) string {
	return p.cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", codeChallenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("audience", "api.atlassian.com"),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
}

func (p *AtlassianProvider) Exchange(ctx context.Context, code, codeVerifier string) (*TokenResult, error) {
	tok, err := p.cfg.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", codeVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("atlassian token exchange: %w", err)
	}
	return tokenResultFromOAuth2(tok), nil
}

func (p *AtlassianProvider) Refresh(ctx context.Context, refreshToken string) (*TokenResult, error) {
	src := p.cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	tok, err := src.Token()
	if err != nil {
		return nil, fmt.Errorf("atlassian token refresh: %w", err)
	}
	return tokenResultFromOAuth2(tok), nil
}

func (p *AtlassianProvider) DefaultServerURL() string {
	return p.mcpURL
}
