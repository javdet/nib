package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/javdet/nib/internal/llm"
)

const GetKBDocumentToolName = "get_kb_document"

var getKBDocumentParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "collection": {
      "type": "string",
      "description": "kb_collections.name"
    }
  },
  "required": ["collection"]
}`)

// GetKBDocumentToolDef returns the LLM tool definition for the local get_kb_document handler.
func GetKBDocumentToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: GetKBDocumentToolName,
		Description: "Return the full knowledge base source document for a collection, not the ~500-character " +
			"chunks that knowledge_search returns. This is the read half of update_kb's full overwrite: read here, " +
			"merge, then send the complete text back. \"source\": \"uploaded\" means the text is the document last " +
			"uploaded to the collection; \"source\": \"template\" means nothing has been uploaded yet and the text " +
			"is the current empty skeleton to fill in. Always read it at the moment you need it - the skeleton " +
			"shipped by the image changes between releases.",
		Parameters: getKBDocumentParameters,
	}
}

// ExecuteGetKBDocument returns the effective source document for a collection.
// A bad collection name comes back as tool output text (not a Go error) so the
// agent loop can correct itself; only infrastructure failures return an error.
func (s *KnowledgeService) ExecuteGetKBDocument(ctx context.Context, args map[string]any) (string, error) {
	if s.docs == nil {
		return "", fmt.Errorf("get_kb_document: knowledge document store not configured")
	}

	collectionName, _ := args["collection"].(string)

	doc, err := s.GetDocument(ctx, collectionName)
	if err != nil {
		if errors.Is(err, ErrInvalidCollectionName) {
			return fmt.Sprintf("invalid collection name %q: must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,63}", collectionName), nil
		}
		return "", fmt.Errorf("get knowledge document: %w", err)
	}

	out := struct {
		Collection string `json:"collection"`
		Source     string `json:"source"`
		UpdatedAt  string `json:"updated_at,omitempty"`
		Content    string `json:"content"`
	}{
		Collection: doc.Collection,
		Source:     string(doc.Source),
		Content:    doc.Content,
	}
	// Zero for the template, and for an uploaded document whose mtime was
	// unreadable; both are cases where there is no timestamp worth showing.
	if !doc.UpdatedAt.IsZero() {
		out.UpdatedAt = doc.UpdatedAt.UTC().Format(time.RFC3339)
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal knowledge document: %w", err)
	}
	return string(b), nil
}
