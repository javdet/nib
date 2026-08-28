package embed

import (
	"fmt"
	"strings"
	"time"
)

// Provider selects which embedding backend to construct in [NewEmbedder].
type Provider string

const (
	ProviderOpenRouter Provider = "openrouter"
	ProviderGoogle     Provider = "google"
)

// FactoryOptions groups settings for [NewEmbedder]. Only the fields for the
// selected provider are used.
type FactoryOptions struct {
	Provider Provider

	OpenRouter OpenRouterOptions
	Google     GoogleOptions

	// Timeout applies to whichever client is built when the nested option's
	// Timeout is zero (OpenRouterOptions.Timeout / GoogleOptions.Timeout).
	Timeout time.Duration
}

// NewEmbedder returns an [Embedder] for the given provider name (e.g. from a flag).
// Recognized values: "openrouter", "google" (case-insensitive). Extra whitespace
// is trimmed.
func NewEmbedder(provider string, opts FactoryOptions) (Embedder, error) {
	p := Provider(strings.ToLower(strings.TrimSpace(provider)))
	if p == "" {
		p = ProviderOpenRouter
	}

	timeout := opts.Timeout
	switch p {
	case ProviderOpenRouter:
		o := opts.OpenRouter
		if o.Timeout == 0 {
			o.Timeout = timeout
		}
		return NewOpenRouterEmbedder(o)
	case ProviderGoogle:
		g := opts.Google
		if g.Timeout == 0 {
			g.Timeout = timeout
		}
		return NewGoogleEmbedder(g)
	default:
		return nil, fmt.Errorf("embed: unknown provider %q (want %q or %q)", provider, ProviderOpenRouter, ProviderGoogle)
	}
}
