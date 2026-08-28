package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleEmbedder_Embed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/text-embedding-004:batchEmbedContents" {
			t.Fatalf("path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "secret" {
			t.Fatalf("key: %q", r.URL.Query().Get("key"))
		}

		var body googleBatchEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Requests) != 2 {
			t.Fatalf("requests: %d", len(body.Requests))
		}
		if body.Requests[0].Model != "models/text-embedding-004" {
			t.Fatalf("model0: %q", body.Requests[0].Model)
		}
		if body.Requests[0].Content.Parts[0].Text != "hello" {
			t.Fatalf("text0: %q", body.Requests[0].Content.Parts[0].Text)
		}
		if body.Requests[1].Content.Parts[0].Text != "world" {
			t.Fatalf("text1: %q", body.Requests[1].Content.Parts[0].Text)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": []map[string]any{
				{"values": []float64{1, 2, 3, 4}},
				{"values": []float64{5, 6, 7, 8}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c, err := NewGoogleEmbedder(GoogleOptions{
		BaseURL: srv.URL + "/v1beta",
		Model:   "text-embedding-004",
		APIKey:  "secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	vecs, dim, err := c.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if dim != 4 {
		t.Fatalf("dim: %d", dim)
	}
	if len(vecs) != 2 || len(vecs[0]) != 4 || len(vecs[1]) != 4 {
		t.Fatalf("vecs: %v", vecs)
	}
	if vecs[0][0] != 1 || vecs[1][3] != 8 {
		t.Fatalf("values: %v %v", vecs[0], vecs[1])
	}
}

func TestGoogleEmbedder_Embed_empty(t *testing.T) {
	t.Parallel()

	c, err := NewGoogleEmbedder(GoogleOptions{
		BaseURL: "http://example.com/v1beta",
		Model:   "text-embedding-004",
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
