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

const CreateDAGToolName = "create_dag"

var createDAGParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "mermaid": {
      "type": "string",
      "description": "Mermaid flowchart TD diagram describing the plan stages and dependencies. May include or omit mermaid code fences."
    }
  },
  "required": ["mermaid"]
}`)

// CreateDAGToolDef returns the LLM tool definition for persisting a DAG diagram.
func CreateDAGToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        CreateDAGToolName,
		Description: "Save a mermaid flowchart TD diagram as a markdown file for the current conversation. The diagram is stored for rendering in the web interface.",
		Parameters:  createDAGParameters,
	}
}

// createDAGHandler returns a local tool handler bound to a specific dialog.
func (s *ChatService) createDAGHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw := strings.TrimSpace(argString(args["mermaid"]))
		if raw == "" {
			return "mermaid is required", nil
		}

		content := formatDAGMarkdown(raw)
		relPath := filepath.Join("dags", dialogID.String()+".md")

		if err := os.MkdirAll(s.dagsDir, 0o755); err != nil {
			return "", fmt.Errorf("create dags directory: %w", err)
		}

		dest := filepath.Join(s.dagsDir, dialogID.String()+".md")
		if err := writeDAGFile(dest, content); err != nil {
			return "", err
		}

		return "DAG saved to " + relPath, nil
	}
}

// formatDAGMarkdown normalizes mermaid input into a single fenced markdown block.
func formatDAGMarkdown(raw string) string {
	body := strings.TrimSpace(raw)
	body = strings.TrimPrefix(body, "```mermaid")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)

	return "```mermaid\n" + body + "\n```"
}

func writeDAGFile(path, content string) error {
	return atomicfile.WriteString(path, content)
}
