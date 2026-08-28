package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

const CreateSummaryToolName = "create_summary"

var createSummaryParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "summary": {
      "type": "string",
      "description": "1-3 sentence plan summary describing the proposed solution."
    }
  },
  "required": ["summary"]
}`)

// CreateSummaryToolDef returns the LLM tool definition for persisting a plan summary.
func CreateSummaryToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        CreateSummaryToolName,
		Description: "Save a plan summary for the current conversation. The text is stored for rendering in the web interface.",
		Parameters:  createSummaryParameters,
	}
}

// WriteSummary persists a plan summary for the given dialog.
func (s *ChatService) WriteSummary(dialogID uuid.UUID, content string) error {
	if err := os.MkdirAll(s.summariesDir, 0o755); err != nil {
		return fmt.Errorf("create summaries directory: %w", err)
	}

	dest := filepath.Join(s.summariesDir, dialogID.String()+".txt")
	return writeSummaryFile(dest, content)
}

// createSummaryHandler returns a local tool handler bound to a specific dialog.
func (s *ChatService) createSummaryHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw := strings.TrimSpace(argString(args["summary"]))
		if raw == "" {
			return "summary is required", nil
		}

		relPath := filepath.Join("summaries", dialogID.String()+".txt")

		if err := s.WriteSummary(dialogID, raw); err != nil {
			return "", err
		}

		return "Summary saved to " + relPath, nil
	}
}

func writeSummaryFile(path, content string) error {
	return atomicfile.WriteString(path, content)
}
