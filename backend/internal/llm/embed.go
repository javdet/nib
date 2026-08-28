package llm

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/openai/openai-go/v3"
)

// Embed implements [Embedder] using the OpenAI Embeddings API.
func (p *OpenAIProvider) Embed(ctx context.Context, texts []string) ([][]float32, int, error) {
	if len(texts) == 0 {
		return nil, 0, nil
	}

	resp, err := p.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(p.embeddingModel),
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: texts,
		},
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	})
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
		row, d, err := floatsToFloat32(item.Embedding)
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
	return out, dim, nil
}

func floatsToFloat32(xs []float64) ([]float32, int, error) {
	if len(xs) == 0 {
		return nil, 0, errors.New("empty embedding vector")
	}
	out := make([]float32, len(xs))
	for i, v := range xs {
		out[i] = float32(v)
	}
	return out, len(out), nil
}
