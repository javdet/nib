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

// NewFromConfig builds an OpenAI-compatible LLM provider from application config.
// cfg.API selects the completion endpoint; embeddings always use /v1/embeddings.
// cfg is expected to be validated by config.Load; this repeats checks as a safeguard.
func NewFromConfig(cfg config.LLMConfig) (Client, error) {
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
		EmbeddingModel:  cfg.EmbeddingModel,
		TimeoutSeconds:  timeout,
		HTTPReferer:     cfg.HTTPReferer,
		AppTitle:        cfg.AppTitle,
		ReasoningEffort: cfg.ReasoningEffort,
	}

	if strings.EqualFold(strings.TrimSpace(cfg.API), config.APIResponses) {
		return NewResponsesProvider(opts), nil
	}
	return NewCompatProvider(opts), nil
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
	return nil
}
