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

// isMermaidNodeOpener reports whether b starts a node label. Mermaid closes each
// shape with its own mirror (`]]`, `)]`, `}}`), but only the label matters here,
// so the scanner stops at the first closing byte instead of matching pairs.
func isMermaidNodeOpener(b byte) bool {
	return b == '[' || b == '(' || b == '{'
}

// isMermaidIDByte reports whether b may appear in a node id. `-` is deliberately
// excluded: it would make the scanner read `-->` as the id `--` followed by the
// `>` shape opener and swallow the rest of the edge as a label.
func isMermaidIDByte(b byte) bool {
	return b == '_' ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

// dagStageTitles returns the node labels of a stored mermaid diagram in
// declaration order. A label is what the action plan calls a stage title, so
// this is the list of stages a plan for that DAG may contain. Nodes drawn
// without a label (`a --> b`) carry no stage name and are skipped.
func dagStageTitles(markdown string) []string {
	body := strings.TrimSpace(markdown)
	body = strings.TrimPrefix(body, "```mermaid")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")

	var titles []string
	seen := make(map[string]struct{})
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		for _, label := range mermaidNodeLabels(line) {
			key := normalizeStageTitle(label)
			if key == "" {
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			titles = append(titles, label)
		}
	}
	return titles
}

// mermaidNodeLabels extracts every labelled node on one diagram line, in order.
func mermaidNodeLabels(line string) []string {
	var labels []string

	for i := 0; i < len(line); {
		if !isMermaidIDByte(line[i]) {
			i++
			continue
		}
		for i < len(line) && isMermaidIDByte(line[i]) {
			i++
		}
		if i >= len(line) {
			break
		}
		// `>` opens the asymmetric shape `id>label]`; every other shape starts
		// with one of the bracket bytes, possibly doubled (`[[`, `((`, `{{`).
		if line[i] != '>' && !isMermaidNodeOpener(line[i]) {
			continue
		}
		i++
		for i < len(line) && isMermaidNodeOpener(line[i]) {
			i++
		}

		if i < len(line) && line[i] == '"' {
			i++
			end := strings.IndexByte(line[i:], '"')
			if end < 0 {
				break
			}
			labels = append(labels, strings.TrimSpace(line[i:i+end]))
			i += end + 1
			continue
		}

		end := strings.IndexAny(line[i:], "]})")
		if end < 0 {
			break
		}
		labels = append(labels, strings.TrimSpace(line[i:i+end]))
		i += end
	}

	return labels
}

// normalizeStageTitle folds a stage title to the form stage names are compared
// in: the model rarely reproduces a DAG label byte for byte, so case and runs of
// whitespace are ignored.
func normalizeStageTitle(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(title), " "))
}

// matchDAGStage returns the DAG label matching name, so a stage is always stored
// under the spelling the diagram uses.
func matchDAGStage(titles []string, name string) (string, bool) {
	key := normalizeStageTitle(name)
	if key == "" {
		return "", false
	}
	for _, title := range titles {
		if normalizeStageTitle(title) == key {
			return title, true
		}
	}
	return "", false
}
