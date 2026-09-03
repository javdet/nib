package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/google/uuid"
)

const SetCategoryToolName = "set_category"

// SetCategoryToolDef returns the LLM tool definition for persisting dialog tool categories.
// When categoryNames is non-empty, each item is constrained to those names via enum.
func SetCategoryToolDef(categoryNames []string) llm.ToolDef {
	itemsSchema := map[string]any{
		"type":        "string",
		"description": "Tool category name (e.g. logging, monitoring, kubernetes).",
	}
	if len(categoryNames) > 0 {
		itemsSchema["enum"] = categoryNames
	}

	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"categories": map[string]any{
				"type":        "array",
				"items":       itemsSchema,
				"description": "List of tool category names that describe which MCP tool groups the task will need during planning.",
			},
		},
		"required": []string{"categories"},
	}
	paramsJSON, _ := json.Marshal(params)

	return llm.ToolDef{
		Name:        SetCategoryToolName,
		Description: "Save the list of tool categories for the current conversation. Categories determine which MCP tools are available to the planning stage.",
		Parameters:  json.RawMessage(paramsJSON),
	}
}

// setCategoryHandler returns a local tool handler bound to a specific dialog.
func (s *ChatService) setCategoryHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		raw, ok := args["categories"]
		if !ok || raw == nil {
			return "categories is required", nil
		}

		categories := normalizeCategories(argStringSlice(raw))
		if len(categories) == 0 {
			return "categories must contain at least one non-empty name", nil
		}

		validNames := s.cachedToolCategoryNames(ctx)
		if len(validNames) > 0 {
			validSet := make(map[string]struct{}, len(validNames))
			for _, name := range validNames {
				validSet[name] = struct{}{}
			}
			var unknown []string
			for _, cat := range categories {
				if _, ok := validSet[cat]; !ok {
					unknown = append(unknown, cat)
				}
			}
			if len(unknown) > 0 {
				return fmt.Sprintf(
					"unknown categories: %s (valid: %s)",
					strings.Join(unknown, ", "),
					strings.Join(validNames, ", "),
				), nil
			}
		}

		if s.dialogRepo == nil {
			return "", fmt.Errorf("dialog repository is not configured")
		}
		if err := s.dialogRepo.SetDialogCategories(ctx, dialogID, categories); err != nil {
			return "", fmt.Errorf("save dialog categories: %w", err)
		}

		return fmt.Sprintf("Categories saved: %s", strings.Join(categories, ", ")), nil
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

func normalizeCategories(categories []string) []string {
	if len(categories) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(categories))
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		normalized := strings.ToLower(strings.TrimSpace(category))
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
