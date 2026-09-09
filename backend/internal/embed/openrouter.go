package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OpenRouterOptions configures the OpenAI-compatible /embeddings client (OpenRouter,
// OpenAI, or any compatible endpoint).
type OpenRouterOptions struct {
	// BaseURL is the API root, e.g. https://openrouter.ai/api/v1
	BaseURL string
	// Model is the embedding model id (request body).
	Model string
	// APIKey is sent as Authorization: Bearer <APIKey>
	APIKey string
	// HTTPReferer and AppTitle are optional OpenRouter attribution headers.
	HTTPReferer string
	AppTitle    string
	// Timeout bounds the entire HTTP request. Zero means no client timeout.
	Timeout time.Duration
}

// OpenRouterEmbedder calls POST {BaseURL}/embeddings with an OpenAI-shaped body.
type OpenRouterEmbedder struct {
	httpClient  *http.Client
	baseURL     string
	model       string
	apiKey      string
	httpReferer string
	appTitle    string
}

// NewOpenRouterEmbedder validates options and returns an Embedder.
func NewOpenRouterEmbedder(opts OpenRouterOptions) (*OpenRouterEmbedder, error) {
	base := strings.TrimSpace(opts.BaseURL)
	if base == "" {
		base = "https://openrouter.ai/api/v1"
	}
	base = strings.TrimRight(base, "/")
	if strings.TrimSpace(opts.Model) == "" {
		return nil, errors.New("embed: OpenRouter Model is required")
	}
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("embed: OpenRouter APIKey is required")
	}
	return &OpenRouterEmbedder{
		httpClient:  &http.Client{Timeout: opts.Timeout},
		baseURL:     base,
		model:       opts.Model,
		apiKey:      opts.APIKey,
		httpReferer: opts.HTTPReferer,
		appTitle:    opts.AppTitle,
	}, nil
}

// Embed implements [Embedder].
func (c *OpenRouterEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, int, error) {
	if len(texts) == 0 {
		return nil, 0, nil
	}

	payload, err := json.Marshal(openRouterEmbeddingsRequest{Model: c.model, Input: texts})
	if err != nil {
		return nil, 0, fmt.Errorf("embed: marshal openrouter request: %w", err)
	}

	url := c.baseURL + "/embeddings"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("embed: build openrouter request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	if c.httpReferer != "" {
		httpReq.Header.Set("HTTP-Referer", c.httpReferer)
	}
	if c.appTitle != "" {
		httpReq.Header.Set("X-Title", c.appTitle)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("embed: openrouter request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("embed: read openrouter body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("embed: openrouter %s: %s", resp.Status, truncateForErr(body))
	}

	var parsed openRouterEmbeddingsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, 0, fmt.Errorf("embed: decode openrouter response: %w", err)
	}
	if len(parsed.Data) != len(texts) {
		return nil, 0, fmt.Errorf("embed: openrouter returned %d rows, want %d", len(parsed.Data), len(texts))
	}

	sort.Slice(parsed.Data, func(i, j int) bool { return parsed.Data[i].Index < parsed.Data[j].Index })

	out := make([][]float32, len(texts))
	dim := 0
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, 0, fmt.Errorf("embed: openrouter index %d out of range [0,%d)", item.Index, len(texts))
		}
		row, d, err := FloatsToFloat32(item.Embedding)
		if err != nil {
			return nil, 0, fmt.Errorf("embed: openrouter row %d: %w", item.Index, err)
		}
		if dim == 0 {
			dim = d
		} else if d != dim {
			return nil, 0, fmt.Errorf("embed: openrouter row %d has dim %d, want %d", item.Index, d, dim)
		}
		out[item.Index] = row
	}
	return out, dim, nil
}

type openRouterEmbeddingsRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openRouterEmbeddingsResponse struct {
	Data []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}
