package embed

import (
	"testing"
)

func TestNewEmbedder_unknown(t *testing.T) {
	t.Parallel()

	_, err := NewEmbedder("azure", FactoryOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewEmbedder_openrouter(t *testing.T) {
	t.Parallel()

	e, err := NewEmbedder("openrouter", FactoryOptions{
		OpenRouter: OpenRouterOptions{
			BaseURL: "http://example.com",
			Model:   "m",
			APIKey:  "k",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := e.(*OpenRouterEmbedder); !ok {
		t.Fatalf("type %T", e)
	}
}

func TestNewEmbedder_google(t *testing.T) {
	t.Parallel()

	e, err := NewEmbedder("Google", FactoryOptions{
		Google: GoogleOptions{
			BaseURL: "http://example.com/v1beta",
			Model:   "text-embedding-004",
			APIKey:  "k",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := e.(*GoogleEmbedder); !ok {
		t.Fatalf("type %T", e)
	}
}
