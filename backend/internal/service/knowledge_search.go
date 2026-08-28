package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/jackc/pgx/v5"
)

const (
	KnowledgeSearchToolName = "knowledge_search"

	knowledgeSearchDefaultLimit = 5
	knowledgeSearchMaxLimit     = 100
)

var knowledgeSearchParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Search query text"
    },
    "collection": {
      "type": "string",
      "description": "kb_collections.name"
    },
    "limit": {
      "type": "number",
      "description": "Maximum hits (default 5, max 100)"
    }
  },
  "required": ["query", "collection"]
}`)

// KnowledgeSearchToolDef returns the LLM tool definition for the local knowledge_search handler.
func KnowledgeSearchToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        KnowledgeSearchToolName,
		Description: "Embed a natural-language query and return the closest kb_chunks rows from the named collection (cosine similarity via pgvector).",
		Parameters:  knowledgeSearchParameters,
	}
}

// ExecuteKnowledgeSearch runs vector search against the knowledge base.
// Validation and lookup failures are returned as tool output text (not Go errors)
// so the agent loop can continue.
func (s *KnowledgeService) ExecuteKnowledgeSearch(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return "query is empty", nil
	}

	collectionName, _ := args["collection"].(string)
	collectionName = strings.TrimSpace(collectionName)
	if collectionName == "" {
		return "collection is empty", nil
	}

	limit := knowledgeSearchDefaultLimit
	if raw, ok := args["limit"]; ok && raw != nil {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = knowledgeSearchDefaultLimit
	}
	if limit > knowledgeSearchMaxLimit {
		limit = knowledgeSearchMaxLimit
	}

	coll, err := s.store.GetCollectionByName(ctx, collectionName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Sprintf("unknown collection %q", collectionName), nil
		}
		return "", fmt.Errorf("get collection: %w", err)
	}

	if coll.EmbeddingModel != s.embeddingModel {
		return fmt.Sprintf("collection %q uses embedding_model %q, configured model is %q",
			collectionName, coll.EmbeddingModel, s.embeddingModel), nil
	}

	vecs, dim, err := s.embedder.Embed(ctx, []string{query})
	if err != nil {
		return "", fmt.Errorf("embed query: %w", err)
	}
	if len(vecs) != 1 {
		return "", fmt.Errorf("embedder returned %d vectors, want 1", len(vecs))
	}
	if dim != coll.Dimensions {
		return fmt.Sprintf("embedding dimension mismatch: got %d, collection has %d", dim, coll.Dimensions), nil
	}

	hits, err := s.store.SearchCosine(ctx, coll.ID, vecs[0], limit)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}

	type row struct {
		Content        string          `json:"content"`
		SourceURI      string          `json:"source_uri"`
		Metadata       json.RawMessage `json:"metadata"`
		Score          float64         `json:"score"`
		Collection     string          `json:"collection"`
		EmbeddingModel string          `json:"embedding_model"`
	}
	out := make([]row, 0, len(hits))
	for _, h := range hits {
		meta := json.RawMessage(h.Metadata)
		if len(meta) == 0 {
			meta = json.RawMessage(`{}`)
		}
		out = append(out, row{
			Content:        h.Content,
			SourceURI:      h.SourceURI,
			Metadata:       meta,
			Score:          h.Score,
			Collection:     coll.Name,
			EmbeddingModel: coll.EmbeddingModel,
		})
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal search results: %w", err)
	}
	return string(b), nil
}
