package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/config"
)

func TestNewFromConfig_validationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     config.LLMConfig
		wantSub string
	}{
		{
			name: "missing baseURL",
			cfg: config.LLMConfig{
				Model:          "gpt-test",
				APIKey:         "key",
				TimeoutSeconds: 120,
			},
			wantSub: "baseURL is required",
		},
		{
			name: "missing model",
			cfg: config.LLMConfig{
				BaseURL:        "https://api.example/v1",
				APIKey:         "key",
				TimeoutSeconds: 120,
			},
			wantSub: "model is required",
		},
		{
			name: "missing api key",
			cfg: config.LLMConfig{
				BaseURL:        "https://api.example/v1",
				Model:          "gpt-test",
				TimeoutSeconds: 120,
			},
			wantSub: "API key is required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewFromConfig(tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("NewFromConfig() = %v, want error containing %q", err, tc.wantSub)
			}
		})
	}
}

func TestNewFromConfig_attributionHeadersOnRequest(t *testing.T) {
	t.Parallel()

	var gotReferer, gotTitle string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReferer = r.Header.Get("HTTP-Referer")
		gotTitle = r.Header.Get("X-Title")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{
				{
					"object":    "embedding",
					"index":     0,
					"embedding": []float64{0.1, 0.2, 0.3},
				},
			},
			"model": "text-embedding-3-small",
			"usage": map[string]int{"prompt_tokens": 1, "total_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)

	provider, err := NewFromConfig(config.LLMConfig{
		BaseURL:        srv.URL + "/v1",
		Model:          "gpt-test",
		APIKey:         "test-key",
		EmbeddingModel: "text-embedding-3-small",
		TimeoutSeconds: 30,
		HTTPReferer:    "https://github.com/javdet/nib",
		AppTitle:       "nib",
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}

	_, _, err = provider.Embed(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if gotReferer != "https://github.com/javdet/nib" {
		t.Errorf("HTTP-Referer = %q, want https://github.com/javdet/nib", gotReferer)
	}
	if gotTitle != "nib" {
		t.Errorf("X-Title = %q, want nib", gotTitle)
	}
}
