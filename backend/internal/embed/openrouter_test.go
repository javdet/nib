package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenRouterEmbedder_Embed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Fatalf("auth: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("HTTP-Referer") != "https://example.com" {
			t.Fatalf("referer: %q", r.Header.Get("HTTP-Referer"))
		}
		if r.Header.Get("X-Title") != "toolchain" {
			t.Fatalf("title: %q", r.Header.Get("X-Title"))
		}

		var body openRouterEmbeddingsRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "m" {
			t.Fatalf("model: %q", body.Model)
		}
		if len(body.Input) != 2 || body.Input[0] != "a" || body.Input[1] != "b" {
			t.Fatalf("input: %#v", body.Input)
		}

		_ = json.NewEncoder(w).Encode(openRouterEmbeddingsResponse{
			Data: []struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float64 `json:"embedding"`
			}{
				{Object: "embedding", Index: 1, Embedding: []float64{0, 1, 2}},
				{Object: "embedding", Index: 0, Embedding: []float64{3, 4, 5}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c, err := NewOpenRouterEmbedder(OpenRouterOptions{
		BaseURL:     srv.URL,
		Model:       "m",
		APIKey:      "k",
		HTTPReferer: "https://example.com",
		AppTitle:    "toolchain",
	})
	if err != nil {
		t.Fatal(err)
	}

	vecs, dim, err := c.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if dim != 3 {
		t.Fatalf("dim: %d", dim)
	}
	if len(vecs) != 2 {
		t.Fatalf("len: %d", len(vecs))
	}
	if vecs[0][0] != 3 || vecs[0][1] != 4 || vecs[0][2] != 5 {
		t.Fatalf("vec0: %v", vecs[0])
	}
	if vecs[1][0] != 0 || vecs[1][1] != 1 || vecs[1][2] != 2 {
		t.Fatalf("vec1: %v", vecs[1])
	}
}

func TestOpenRouterEmbedder_Embed_empty(t *testing.T) {
	t.Parallel()

	c, err := NewOpenRouterEmbedder(OpenRouterOptions{
		BaseURL: "http://example.com",
		Model:   "m",
		APIKey:  "k",
	})
	if err != nil {
		t.Fatal(err)
	}
	vecs, dim, err := c.Embed(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if dim != 0 || vecs != nil {
		t.Fatalf("got dim=%d vecs=%v", dim, vecs)
	}
}
