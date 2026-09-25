package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/javdet/nib/internal/config"
)

// embeddingsBody renders a float embeddings response of the given width.
func embeddingsBody(rows, width int) string {
	var b strings.Builder
	b.WriteString(`{"object":"list","data":[`)
	for i := 0; i < rows; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"object":"embedding","index":%d,"embedding":[`, i)
		for d := 0; d < width; d++ {
			if d > 0 {
				b.WriteByte(',')
			}
			b.WriteString("0.1")
		}
		b.WriteString(`]}`)
	}
	b.WriteString(`]}`)
	return b.String()
}

// TestCompletionsAndEmbeddingsUseSeparateEndpoints is the headline of the
// split. DeepSeek, Grok and Kimi serve chat completions and no /embeddings
// route at all, so the two must be independently addressable.
func TestCompletionsAndEmbeddingsUseSeparateEndpoints(t *testing.T) {
	t.Parallel()

	var chatHits, embedHits atomic.Int32
	var embedPath string

	chatSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","model":"m","choices":[{"index":0,"finish_reason":"stop",` +
			`"message":{"role":"assistant","content":"hi"}}]}`))
	}))
	t.Cleanup(chatSrv.Close)

	embedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embedHits.Add(1)
		embedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(embeddingsBody(1, 4)))
	}))
	t.Cleanup(embedSrv.Close)

	client, err := NewFromConfig(config.LLMConfig{
		APIKey:         "chat-key",
		Model:          "chat-model",
		BaseURL:        chatSrv.URL,
		EmbeddingModel: "ignored-fallback",
		TimeoutSeconds: 5,
		Embeddings: config.EmbeddingsConfig{
			BaseURL: embedSrv.URL,
			APIKey:  "embed-key",
			Model:   "embed-model",
		},
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}

	if _, err := client.Complete(context.Background(), "sys", "user"); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if _, _, err := client.Embed(context.Background(), []string{"text"}); err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if got := chatHits.Load(); got != 1 {
		t.Errorf("completions host got %d requests, want 1", got)
	}
	if got := embedHits.Load(); got != 1 {
		t.Errorf("embeddings host got %d requests, want 1", got)
	}
	if !strings.HasSuffix(embedPath, "/embeddings") {
		t.Errorf("embeddings path = %q, want it to end in /embeddings", embedPath)
	}
}

// TestEmbeddingsFallBackToCompletionHost keeps the single-endpoint install --
// every existing one -- behaving exactly as before.
func TestEmbeddingsFallBackToCompletionHost(t *testing.T) {
	t.Parallel()

	var embedModel string
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		embedModel, _ = body["model"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(embeddingsBody(1, 4)))
	}))
	t.Cleanup(srv.Close)

	client, err := NewFromConfig(config.LLMConfig{
		APIKey:         "k",
		Model:          "m",
		BaseURL:        srv.URL,
		EmbeddingModel: "text-embedding-3-small",
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if _, _, err := client.Embed(context.Background(), []string{"text"}); err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("host got %d requests, want 1", hits.Load())
	}
	if embedModel != "text-embedding-3-small" {
		t.Errorf("model = %q, want the llm.embeddingModel fallback", embedModel)
	}
}

// TestEmbeddingsDimensionsParameter covers both halves of narrowing a model to
// the width kb_chunks.embedding is declared at.
func TestEmbeddingsDimensionsParameter(t *testing.T) {
	t.Parallel()

	t.Run("sent when configured", func(t *testing.T) {
		t.Parallel()
		var got map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(embeddingsBody(1, 8)))
		}))
		t.Cleanup(srv.Close)

		c := NewEmbeddingsClient(EmbeddingsOptions{
			APIKey: "k", BaseURL: srv.URL, Model: "m", Dimensions: 8,
		})
		if _, _, err := c.Embed(context.Background(), []string{"t"}); err != nil {
			t.Fatalf("Embed() error = %v", err)
		}
		if got["dimensions"] != float64(8) {
			t.Errorf("dimensions = %v, want 8", got["dimensions"])
		}
	})

	t.Run("omitted when zero", func(t *testing.T) {
		t.Parallel()
		var got map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(embeddingsBody(1, 4)))
		}))
		t.Cleanup(srv.Close)

		c := NewEmbeddingsClient(EmbeddingsOptions{APIKey: "k", BaseURL: srv.URL, Model: "m"})
		if _, _, err := c.Embed(context.Background(), []string{"t"}); err != nil {
			t.Fatalf("Embed() error = %v", err)
		}
		// A provider that does not know the parameter rejects the whole
		// request, so it must be absent rather than zero.
		if _, has := got["dimensions"]; has {
			t.Error("dimensions was sent although none is configured")
		}
	})
}

// TestEmbeddingsWidthMismatchFails guards the silent half: a provider that
// ignores `dimensions` answers 200 at its native width, and those vectors
// would be stored in the tool catalog and never match anything.
func TestEmbeddingsWidthMismatchFails(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(embeddingsBody(1, 1024)))
	}))
	t.Cleanup(srv.Close)

	c := NewEmbeddingsClient(EmbeddingsOptions{
		APIKey: "k", BaseURL: srv.URL, Model: "m", Dimensions: 1536,
	})
	_, _, err := c.Embed(context.Background(), []string{"t"})
	if err == nil {
		t.Fatal("Embed() error = nil, want a width mismatch")
	}
	for _, want := range []string{"1024", "1536"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to name %s", err, want)
		}
	}
}

// TestEmbeddingsReordersByIndex keeps the row-order contract that came with
// the code this client was moved from.
func TestEmbeddingsReordersByIndex(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Deliberately out of order.
		_, _ = w.Write([]byte(`{"data":[` +
			`{"index":1,"embedding":[2,2]},` +
			`{"index":0,"embedding":[1,1]}]}`))
	}))
	t.Cleanup(srv.Close)

	c := NewEmbeddingsClient(EmbeddingsOptions{APIKey: "k", BaseURL: srv.URL, Model: "m"})
	vecs, dim, err := c.Embed(context.Background(), []string{"first", "second"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if dim != 2 {
		t.Errorf("dim = %d, want 2", dim)
	}
	if vecs[0][0] != 1 || vecs[1][0] != 2 {
		t.Errorf("rows were not reordered by index: %v", vecs)
	}
}
