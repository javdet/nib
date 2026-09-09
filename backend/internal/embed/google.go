package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GoogleOptions configures the Generative Language API embedContent-style client
// (batchEmbedContents for multiple strings).
type GoogleOptions struct {
	// BaseURL is the API root, e.g. https://generativelanguage.googleapis.com/v1beta
	BaseURL string
	// Model is the model id for the URL and request bodies, e.g. text-embedding-004
	Model string
	// APIKey is sent as the key= query parameter (Gemini API key).
	APIKey string
	// Timeout bounds the entire HTTP request. Zero means no client timeout.
	Timeout time.Duration
}

// GoogleEmbedder calls POST .../models/{model}:batchEmbedContents?key=...
type GoogleEmbedder struct {
	httpClient *http.Client
	baseURL    string
	model      string
	modelRef   string // e.g. models/text-embedding-004
	apiKey     string
}

// NewGoogleEmbedder validates options and returns an Embedder.
func NewGoogleEmbedder(opts GoogleOptions) (*GoogleEmbedder, error) {
	base := strings.TrimSpace(opts.BaseURL)
	if base == "" {
		base = "https://generativelanguage.googleapis.com/v1beta"
	}
	base = strings.TrimRight(base, "/")
	if strings.TrimSpace(opts.Model) == "" {
		return nil, errors.New("embed: Google Model is required")
	}
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, errors.New("embed: Google APIKey is required")
	}
	modelID := strings.TrimSpace(opts.Model)
	modelID = strings.TrimPrefix(modelID, "models/")
	modelRef := "models/" + modelID
	return &GoogleEmbedder{
		httpClient: &http.Client{Timeout: opts.Timeout},
		baseURL:    base,
		model:      modelID,
		modelRef:   modelRef,
		apiKey:     opts.APIKey,
	}, nil
}

// Embed implements [Embedder].
func (c *GoogleEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, int, error) {
	if len(texts) == 0 {
		return nil, 0, nil
	}

	reqBody := googleBatchEmbedRequest{Requests: make([]googleEmbedRequestItem, len(texts))}
	for i, t := range texts {
		reqBody.Requests[i] = googleEmbedRequestItem{
			Model: c.modelRef,
			Content: googleContent{
				Parts: []googlePart{{Text: t}},
			},
		}
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("embed: marshal google request: %w", err)
	}

	u, err := url.Parse(c.baseURL + "/models/" + url.PathEscape(c.model) + ":batchEmbedContents")
	if err != nil {
		return nil, 0, fmt.Errorf("embed: parse google URL: %w", err)
	}
	q := u.Query()
	q.Set("key", c.apiKey)
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("embed: build google request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("embed: google request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("embed: read google body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, 0, fmt.Errorf("embed: google %s: %s", resp.Status, truncateForErr(body))
	}

	var parsed googleBatchEmbedResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, 0, fmt.Errorf("embed: decode google response: %w", err)
	}
	if len(parsed.Embeddings) != len(texts) {
		return nil, 0, fmt.Errorf("embed: google returned %d embeddings, want %d", len(parsed.Embeddings), len(texts))
	}

	out := make([][]float32, len(texts))
	dim := 0
	for i := range parsed.Embeddings {
		row, d, err := FloatsToFloat32(parsed.Embeddings[i].Values)
		if err != nil {
			return nil, 0, fmt.Errorf("embed: google row %d: %w", i, err)
		}
		if dim == 0 {
			dim = d
		} else if d != dim {
			return nil, 0, fmt.Errorf("embed: google row %d has dim %d, want %d", i, d, dim)
		}
		out[i] = row
	}
	return out, dim, nil
}

type googleBatchEmbedRequest struct {
	Requests []googleEmbedRequestItem `json:"requests"`
}

type googleEmbedRequestItem struct {
	Model   string        `json:"model"`
	Content googleContent `json:"content"`
}

type googleContent struct {
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	Text string `json:"text"`
}

type googleBatchEmbedResponse struct {
	Embeddings []struct {
		Values []float64 `json:"values"`
	} `json:"embeddings"`
}
