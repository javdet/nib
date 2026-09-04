package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/llm"
)

// UpdateToolCategoryToolName is the OpenAI function name for editing the
// patterns of a tool category.
const UpdateToolCategoryToolName = "update_tool_category"

// discussDialogMode is the only mode that may edit tool categories. Categories
// decide which MCP tools a plan inherits, so a planning or executing agent must
// not reshape the tool surface under the plan it is already working from; the
// discuss conversation is the one an operator drives directly to curate them.
const discussDialogMode = "discuss"

// Pattern edit actions. add is the default because the agent cannot see a
// category's current patterns before its first call, and a silent replace would
// drop patterns nothing in the conversation ever mentioned.
const (
	toolCategoryActionAdd     = "add"
	toolCategoryActionRemove  = "remove"
	toolCategoryActionReplace = "replace"
)

// UpdateToolCategoryToolDef returns the LLM tool definition for editing one
// category's patterns. When categoryNames is non-empty the category argument is
// constrained to those names via enum: the category list itself comes from the
// global toolCategories variable and cannot be extended from here.
func UpdateToolCategoryToolDef(categoryNames []string) llm.ToolDef {
	categorySchema := map[string]any{
		"type":        "string",
		"description": "Name of an existing tool category (e.g. logging, monitoring, kubernetes).",
	}
	if len(categoryNames) > 0 {
		categorySchema["enum"] = categoryNames
	}

	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"category": categorySchema,
			"patterns": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
				"description": "Tool name patterns, one per item: either an exact tool name " +
					"(grafana_query_loki_logs) or a prefix ending in a single trailing wildcard " +
					"(grafana_*). Patterns must not contain whitespace. A newline- or " +
					"comma-separated string is accepted too.",
			},
			"action": map[string]any{
				"type": "string",
				"enum": []string{toolCategoryActionAdd, toolCategoryActionRemove, toolCategoryActionReplace},
				"description": "add (default) keeps the category's existing patterns, remove deletes " +
					"the given ones, replace makes the given patterns the whole category.",
			},
		},
		"required": []string{"category", "patterns"},
	}
	paramsJSON, _ := json.Marshal(params)

	return llm.ToolDef{
		Name: UpdateToolCategoryToolName,
		Description: "Edit the tool name patterns of an existing tool category. Patterns decide which " +
			"MCP tools belong to the category, and a category's tools are handed to the planning stage " +
			"of every plan that selects it, so an edit changes what future plans can call. The result " +
			"lists the category's full pattern set after the edit and how many catalog tools it matches.",
		Parameters: json.RawMessage(paramsJSON),
	}
}

// updateToolCategoryHandler edits one category's patterns and recomputes the
// tool assignments derived from them.
func (s *ChatService) updateToolCategoryHandler() localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		if s.toolCategorySvc == nil {
			return "", fmt.Errorf("tool category service is not configured")
		}

		name := strings.TrimSpace(argString(args["category"]))
		if name == "" {
			return "category is required", nil
		}

		raw, ok := args["patterns"]
		if !ok || raw == nil {
			return "patterns is required", nil
		}
		given, err := normalizePatterns(argPatternSlice(raw))
		if err != nil {
			return err.Error(), nil
		}
		if len(given) == 0 {
			return "patterns must contain at least one non-empty pattern", nil
		}

		action := strings.ToLower(strings.TrimSpace(argString(args["action"])))
		if action == "" {
			action = toolCategoryActionAdd
		}
		switch action {
		case toolCategoryActionAdd, toolCategoryActionRemove, toolCategoryActionReplace:
		default:
			return fmt.Sprintf(
				"unknown action %q (valid: %s, %s, %s)",
				action, toolCategoryActionAdd, toolCategoryActionRemove, toolCategoryActionReplace,
			), nil
		}

		// Read straight from the service rather than through the cached name
		// list: the current patterns decide the outcome of add and remove, and
		// a minute-old copy of them would silently resurrect or drop entries.
		cats, err := s.toolCategorySvc.List(ctx)
		if err != nil {
			return "", fmt.Errorf("list tool categories: %w", err)
		}

		var current []string
		stored := ""
		known := make([]string, 0, len(cats))
		for _, cat := range cats {
			known = append(known, cat.Name)
			if strings.EqualFold(cat.Name, name) {
				stored = cat.Name
				current = cat.Patterns
			}
		}
		if stored == "" {
			return fmt.Sprintf(
				"unknown category %q (valid: %s). Categories come from the global toolCategories variable and cannot be created here.",
				name, strings.Join(known, ", "),
			), nil
		}

		next := applyPatternAction(action, current, given)

		updated, err := s.toolCategorySvc.SetPatterns(ctx, stored, next)
		if err != nil {
			return "", fmt.Errorf("update category %q patterns: %w", stored, err)
		}

		if len(updated.Patterns) == 0 {
			return fmt.Sprintf("Category %s now has no patterns and matches no tools.", stored), nil
		}
		return fmt.Sprintf(
			"Category %s updated (%s). Patterns: %s. Tools matched: %d.",
			stored, action, strings.Join(updated.Patterns, ", "), updated.ToolCount,
		), nil
	}
}

// applyPatternAction folds the given patterns into the current ones. Order is
// preserved so a call that changes nothing writes the same list back.
func applyPatternAction(action string, current, given []string) []string {
	if action == toolCategoryActionReplace {
		return given
	}

	if action == toolCategoryActionRemove {
		drop := make(map[string]struct{}, len(given))
		for _, p := range given {
			drop[p] = struct{}{}
		}
		out := make([]string, 0, len(current))
		for _, p := range current {
			if _, ok := drop[p]; ok {
				continue
			}
			out = append(out, p)
		}
		return out
	}

	have := make(map[string]struct{}, len(current))
	out := make([]string, 0, len(current)+len(given))
	for _, p := range current {
		have[p] = struct{}{}
		out = append(out, p)
	}
	for _, p := range given {
		if _, ok := have[p]; ok {
			continue
		}
		have[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// argPatternSlice reads a pattern list, accepting the plain text a model
// sometimes sends instead of an array: newline-, comma- or space-separated.
func argPatternSlice(v any) []string {
	if items := argStringSlice(v); len(items) > 0 {
		return splitPatternText(items...)
	}
	if text, ok := v.(string); ok {
		return splitPatternText(text)
	}
	return nil
}

func splitPatternText(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, field := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			field = strings.TrimSpace(field)
			if field != "" {
				out = append(out, field)
			}
		}
	}
	return out
}
