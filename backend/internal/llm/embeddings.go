package llm

import (
	"context"
	"fmt"
	"sort"

	"github.com/javdet/nib/internal/embed"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const defaultEmbeddingModel = "text-embedding-3-small"

// EmbeddingsOptions configures the embeddings endpoint.
//
// It is separate from ClientOptions because the two endpoints need not share a
// host: several providers serve chat completions and no /embeddings route at
// all, so an install may well point completions at one and embeddings at
// another.
type EmbeddingsOptions struct {
	APIKey  string
	BaseURL string
	Model   string
	// Dimensions is sent as the request's `dimensions` parameter when
	// positive. Zero omits it.
	Dimensions     int
	TimeoutSeconds int
	HTTPReferer    string
	AppTitle       string
}

// EmbeddingsClient implements Embedder against an OpenAI-compatible
// /embeddings endpoint.
type EmbeddingsClient struct {
	client     openai.Client
	model      string
	dimensions int
}

// NewEmbeddingsClient builds an embeddings client. Model defaults to
// text-embedding-3-small when empty.
func NewEmbeddingsClient(opts EmbeddingsOptions) *EmbeddingsClient {
	model := opts.Model
	if model == "" {
		model = defaultEmbeddingModel
	}
	clientOpts := []option.RequestOption{
		option.WithAPIKey(opts.APIKey),
		option.WithHTTPClient(newHTTPClient(opts.TimeoutSeconds, opts.HTTPReferer, opts.AppTitle)),
	}
	if opts.BaseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(opts.BaseURL))
	}
	return &EmbeddingsClient{
		client:     openai.NewClient(clientOpts...),
		model:      model,
		dimensions: opts.Dimensions,
	}
}

// Embed implements [Embedder] using the OpenAI Embeddings API.
func (c *EmbeddingsClient) Embed(ctx context.Context, texts []string) ([][]float32, int, error) {
	if len(texts) == 0 {
		return nil, 0, nil
	}

	params := openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(c.model),
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	}
	// Omitted rather than defaulted when unset: a provider that does not know
	// the parameter rejects the whole request, and the models nib shipped with
	// are natively the right width.
	if c.dimensions > 0 {
		params.Dimensions = openai.Int(int64(c.dimensions))
	}

	resp, err := c.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, 0, wrapAPIError("openai embeddings", err)
	}
	if len(resp.Data) != len(texts) {
		return nil, 0, fmt.Errorf("openai embeddings: returned %d rows, want %d", len(resp.Data), len(texts))
	}

	data := append([]openai.Embedding(nil), resp.Data...)
	sort.Slice(data, func(i, j int) bool { return data[i].Index < data[j].Index })

	out := make([][]float32, len(texts))
	dim := 0
	for _, item := range data {
		idx := int(item.Index)
		if idx < 0 || idx >= len(texts) {
			return nil, 0, fmt.Errorf("openai embeddings: index %d out of range [0,%d)", idx, len(texts))
		}
		row, d, err := embed.FloatsToFloat32(item.Embedding)
		if err != nil {
			return nil, 0, fmt.Errorf("openai embeddings: row %d: %w", idx, err)
		}
		if dim == 0 {
			dim = d
		} else if d != dim {
			return nil, 0, fmt.Errorf("openai embeddings: row %d has dim %d, want %d", idx, d, dim)
		}
		out[idx] = row
	}

	// A provider that ignores `dimensions` answers 200 at its native width.
	// Those vectors would be rejected row by row on insert, or -- worse, in
	// the tool catalog -- stored and never matched. Fail here, where the width
	// is still attributable to a request. Truncating locally is not an option:
	// a sliced vector from a model not trained for it is meaningless, and
	// afterwards indistinguishable from a good one.
	if c.dimensions > 0 && dim != c.dimensions {
		return nil, 0, fmt.Errorf(
			"openai embeddings: provider returned %d dimensions, want %d (llm.embeddings.dimensions is not honoured by this endpoint or model)",
			dim, c.dimensions)
	}

	return out, dim, nil
}
