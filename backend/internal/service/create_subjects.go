package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

const CreateSubjectsToolName = "create_subjects"

var createSubjectsParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "subjects": {
      "type": "array",
      "items": {
        "type": "string"
      },
      "description": "List of one-word infrastructure subjects (e.g. postgres, nginx, kubernetes) used to attach matching rules during planning."
    }
  },
  "required": ["subjects"]
}`)

// CreateSubjectsToolDef returns the LLM tool definition for persisting dialog subjects.
func CreateSubjectsToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        CreateSubjectsToolName,
		Description: "Save the list of infrastructure subjects for the current conversation. Subjects are used to attach matching rules during detailed planning.",
		Parameters:  createSubjectsParameters,
	}
}

// createSubjectsHandler returns a local tool handler bound to a specific dialog.
func (s *ChatService) createSubjectsHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw, ok := args["subjects"]
		if !ok || raw == nil {
			return "subjects is required", nil
		}

		subjects := normalizeSubjects(argStringSlice(raw))
		if len(subjects) == 0 {
			return "subjects must contain at least one non-empty word", nil
		}

		if s.dialogRepo == nil {
			return "", fmt.Errorf("dialog repository is not configured")
		}
		if err := s.dialogRepo.SetDialogSubjects(ctx, dialogID, subjects); err != nil {
			return "", fmt.Errorf("save dialog subjects: %w", err)
		}

		return fmt.Sprintf("Subjects saved: %s", strings.Join(subjects, ", ")), nil
	}
}

func argStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch items := v.(type) {
	case []string:
		return items
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			s := strings.TrimSpace(argString(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func normalizeSubjects(subjects []string) []string {
	if len(subjects) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(subjects))
	out := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		normalized := strings.ToLower(strings.TrimSpace(subject))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}
