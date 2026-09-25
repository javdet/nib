package llm

import (
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/config"
)

// Client is the full LLM surface the application depends on: completions and
// embeddings from a single configured provider.
type Client interface {
	Provider
	Embedder
}

// client joins the two endpoints an install actually has. They are composed
// rather than served by one provider because a host that serves chat
// completions need not serve embeddings, and several do not.
type client struct {
	Provider
	Embedder
}

// NewFromConfig builds an OpenAI-compatible LLM client from application config.
// cfg.API selects the completion endpoint; cfg.Embeddings selects the
// embeddings one, falling back to the completion host when unset.
// cfg is expected to be validated by config.Load; this repeats checks as a safeguard.
func NewFromConfig(cfg config.LLMConfig) (Client, error) {
	// cfg is a copy, so resolving here cannot surprise the caller. It makes
	// the fallback a property of the config rather than of the load path,
	// which keeps a hand-assembled config from silently getting an embeddings
	// client pointed nowhere.
	config.ResolveEmbeddings(&cfg)

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	timeout := cfg.TimeoutSeconds
	if timeout < 1 {
		timeout = 120
	}

	opts := ClientOptions{
		APIKey:          cfg.APIKey,
		Model:           cfg.Model,
		BaseURL:         cfg.BaseURL,
		TimeoutSeconds:  timeout,
		HTTPReferer:     cfg.HTTPReferer,
		AppTitle:        cfg.AppTitle,
		ReasoningEffort: cfg.ReasoningEffort,
	}

	embedder := NewEmbeddingsClient(EmbeddingsOptions{
		APIKey:         cfg.Embeddings.APIKey,
		BaseURL:        cfg.Embeddings.BaseURL,
		Model:          cfg.Embeddings.Model,
		Dimensions:     cfg.Embeddings.Dimensions,
		TimeoutSeconds: timeout,
		HTTPReferer:    cfg.HTTPReferer,
		AppTitle:       cfg.AppTitle,
	})

	var provider Provider
	if strings.EqualFold(strings.TrimSpace(cfg.API), config.APIResponses) {
		provider = NewResponsesProvider(opts)
	} else {
		provider = NewCompatProvider(opts)
	}
	return &client{Provider: provider, Embedder: embedder}, nil
}

func validateConfig(cfg config.LLMConfig) error {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return fmt.Errorf("llm: baseURL is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return fmt.Errorf("llm: model is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("llm: API key is required")
	}
	if strings.TrimSpace(cfg.Embeddings.BaseURL) == "" {
		return fmt.Errorf("llm: embeddings baseURL is required")
	}
	if strings.TrimSpace(cfg.Embeddings.APIKey) == "" {
		return fmt.Errorf("llm: embeddings API key is required")
	}
	return nil
}
