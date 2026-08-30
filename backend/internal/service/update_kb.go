package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/kb"
	"github.com/javdet/nib/internal/llm"
)

const UpdateKBToolName = "update_kb"

var updateKBParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "collection": {
      "type": "string",
      "description": "kb_collections.name"
    },
    "content": {
      "type": "string",
      "description": "Full markdown text of the knowledge base document. Replaces the collection entirely."
    }
  },
  "required": ["collection", "content"]
}`)

// UpdateKBToolDef returns the LLM tool definition for the local update_kb handler.
func UpdateKBToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: UpdateKBToolName,
		Description: "Overwrite the knowledge base document for a collection: every existing chunk is deleted " +
			"and replaced with the supplied text, re-embedded into pgvector, and mirrored to disk. This is a full " +
			"overwrite, not an append - read the current document first and send the complete merged text, never a " +
			"fragment. A collection that does not exist yet is created.",
		Parameters: updateKBParameters,
	}
}

// ExecuteUpdateKB re-ingests a collection from the supplied document text.
// Validation and ingest-contract failures are returned as tool output text (not
// Go errors) so the agent loop can correct itself; only infrastructure failures
// return an error.
func (s *KnowledgeService) ExecuteUpdateKB(ctx context.Context, args map[string]any) (string, error) {
	collectionName, _ := args["collection"].(string)
	collectionName = strings.TrimSpace(collectionName)
	// Deliberately no fallback to the default collection: overwriting "default"
	// because the model omitted a name is too destructive.
	if collectionName == "" {
		return "collection is empty", nil
	}

	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return "content is empty", nil
	}

	// The synthetic filename becomes kb_chunks.source_uri, which Status and the
	// UI surface as the document name; {collection}.md is what kbdoc calls the
	// file on disk, so both sides agree.
	result, err := s.UploadDocument(ctx, collectionName, collectionName+".md", []byte(content))
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCollectionName):
			return fmt.Sprintf("invalid collection name %q: must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,63}", collectionName), nil
		case errors.Is(err, ErrEmptyFile), errors.Is(err, ErrNoChunks):
			return "document produced no chunks", nil
		case errors.Is(err, kb.ErrCollectionMismatch):
			return fmt.Sprintf("collection %q was indexed with a different embedding model or dimension, configured model is %q",
				collectionName, s.embeddingModel), nil
		}
		return "", fmt.Errorf("update knowledge base: %w", err)
	}

	out := struct {
		Collection string `json:"collection"`
		ChunkCount int    `json:"chunk_count"`
		SourceURI  string `json:"source_uri"`
	}{
		Collection: result.Collection,
		ChunkCount: result.ChunkCount,
		SourceURI:  result.Filename,
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal update result: %w", err)
	}
	return string(b), nil
}
