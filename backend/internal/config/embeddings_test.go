package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes a YAML file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// loadWithEnv clears the LLM environment, applies env, and loads path.
func loadWithEnv(t *testing.T, path string, env map[string]string) (Config, error) {
	t.Helper()
	for _, key := range []string{
		"LLM_API_KEY", "OPENAI_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "OPENAI_MODEL",
		"LLM_API", "LLM_REASONING_EFFORT", "OPENAI_EMBEDDING_MODEL",
		"LLM_EMBEDDINGS_BASE_URL", "LLM_EMBEDDINGS_API_KEY",
		"LLM_EMBEDDINGS_MODEL", "LLM_EMBEDDINGS_DIMENSIONS",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("LLM_API_KEY", "test-key")
	for k, v := range env {
		t.Setenv(k, v)
	}
	return Load(path)
}

const baseLLM = `llm:
  baseURL: https://completions.example/v1
  model: some-model
  embeddingModel: text-embedding-3-small
`

// TestEmbeddingsFallBackToLLM keeps every existing single-endpoint install
// behaving exactly as it did before the block existed.
func TestEmbeddingsFallBackToLLM(t *testing.T) {
	cfg, err := loadWithEnv(t, writeConfig(t, baseLLM), nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	e := cfg.LLM.Embeddings
	if e.BaseURL != "https://completions.example/v1" {
		t.Errorf("BaseURL = %q, want the llm.baseURL fallback", e.BaseURL)
	}
	if e.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want the llm API key fallback", e.APIKey)
	}
	if e.Model != "text-embedding-3-small" {
		t.Errorf("Model = %q, want the llm.embeddingModel fallback", e.Model)
	}
	if e.Dimensions != 0 {
		t.Errorf("Dimensions = %d, want 0 so the parameter is omitted", e.Dimensions)
	}
}

// TestEmbeddingsOverrideFromYAML covers the split that makes DeepSeek, Grok
// and Kimi usable: completions on a host with no /embeddings route, embeddings
// somewhere else.
func TestEmbeddingsOverrideFromYAML(t *testing.T) {
	cfg, err := loadWithEnv(t, writeConfig(t, baseLLM+`  embeddings:
    baseURL: https://embeddings.example/v1
    model: gemini-embedding-001
    dimensions: 1536
`), nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	e := cfg.LLM.Embeddings
	if e.BaseURL != "https://embeddings.example/v1" {
		t.Errorf("BaseURL = %q, want the override", e.BaseURL)
	}
	if e.Dimensions != 1536 {
		t.Errorf("Dimensions = %d, want 1536", e.Dimensions)
	}
	// The API key still falls back: sharing one key is the common case.
	if e.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want the fallback", e.APIKey)
	}
}

// TestEmbeddingsModelIsWrittenBack is the subtle one. Knowledge search compares
// LLM.EmbeddingModel against kb_collections.embedding_model as an exact string,
// so an override that only moved forward would return no results and no error.
func TestEmbeddingsModelIsWrittenBack(t *testing.T) {
	cfg, err := loadWithEnv(t, writeConfig(t, baseLLM+`  embeddings:
    model: gemini-embedding-001
`), nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LLM.EmbeddingModel != "gemini-embedding-001" {
		t.Errorf("LLM.EmbeddingModel = %q, want it overwritten by llm.embeddings.model",
			cfg.LLM.EmbeddingModel)
	}
}

func TestEmbeddingsFromEnv(t *testing.T) {
	cfg, err := loadWithEnv(t, writeConfig(t, baseLLM), map[string]string{
		"LLM_EMBEDDINGS_BASE_URL":   "https://env.example/v1",
		"LLM_EMBEDDINGS_API_KEY":    "embed-key",
		"LLM_EMBEDDINGS_MODEL":      "text-embedding-v4",
		"LLM_EMBEDDINGS_DIMENSIONS": "1536",
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	e := cfg.LLM.Embeddings
	if e.BaseURL != "https://env.example/v1" || e.APIKey != "embed-key" ||
		e.Model != "text-embedding-v4" || e.Dimensions != 1536 {
		t.Errorf("Embeddings = %+v, want the environment values", e)
	}
}

// TestReasoningEffortCanBeClearedByYAML is the fix that lets a preset turn the
// parameter off. It is sent on every request whatever the model is, and Grok-4
// and Kimi reject it outright.
func TestReasoningEffortCanBeClearedByYAML(t *testing.T) {
	t.Run("explicit empty clears the environment", func(t *testing.T) {
		cfg, err := loadWithEnv(t, writeConfig(t, baseLLM+`  reasoningEffort: ""
`), map[string]string{"LLM_REASONING_EFFORT": "high"})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.LLM.ReasoningEffort != "" {
			t.Errorf("ReasoningEffort = %q, want it cleared", cfg.LLM.ReasoningEffort)
		}
	})

	t.Run("absent key defers to the environment", func(t *testing.T) {
		cfg, err := loadWithEnv(t, writeConfig(t, baseLLM),
			map[string]string{"LLM_REASONING_EFFORT": "high"})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.LLM.ReasoningEffort != "high" {
			t.Errorf("ReasoningEffort = %q, want %q from the environment",
				cfg.LLM.ReasoningEffort, "high")
		}
	})

	t.Run("non-empty value overrides the environment", func(t *testing.T) {
		cfg, err := loadWithEnv(t, writeConfig(t, baseLLM+`  reasoningEffort: low
`), map[string]string{"LLM_REASONING_EFFORT": "high"})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.LLM.ReasoningEffort != "low" {
			t.Errorf("ReasoningEffort = %q, want %q", cfg.LLM.ReasoningEffort, "low")
		}
	})
}

func TestEmbeddingsDimensionsValidation(t *testing.T) {
	_, err := loadWithEnv(t, writeConfig(t, baseLLM+`  embeddings:
    dimensions: 99999
`), nil)
	if err == nil {
		t.Fatal("Load() error = nil, want a range error")
	}
	if !strings.Contains(err.Error(), "llm.embeddings.dimensions") {
		t.Errorf("error = %v, want it to name llm.embeddings.dimensions", err)
	}
}

// TestResolveEmbeddingsIsIdempotent matters because both Load and
// llm.NewFromConfig call it.
func TestResolveEmbeddingsIsIdempotent(t *testing.T) {
	t.Parallel()

	llm := LLMConfig{
		APIKey: "k", BaseURL: "https://a/v1", EmbeddingModel: "m",
		Embeddings: EmbeddingsConfig{Model: "override"},
	}
	ResolveEmbeddings(&llm)
	once := llm
	ResolveEmbeddings(&llm)
	if llm != once {
		t.Errorf("ResolveEmbeddings is not idempotent:\nonce  = %+v\ntwice = %+v", once, llm)
	}
}
