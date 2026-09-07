package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mode"
)

// UpdateIncludedToolsToolName is the OpenAI function name for narrowing a mode's
// included MCP tools.
const UpdateIncludedToolsToolName = "update_included_tools"

// UpdateIncludedToolsToolDef returns the LLM tool definition for removing MCP
// tools from one mode's included list. The mode argument is constrained to the
// fixed mode list: a mode cannot be created from here.
func UpdateIncludedToolsToolDef() llm.ToolDef {
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{
				"type":        "string",
				"enum":        mode.Modes,
				"description": "Chat mode whose included tools are narrowed.",
			},
			"tools": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
				"description": "Exact MCP tool names to take out of the mode, one per item. " +
					"Names must not contain whitespace. A newline- or comma-separated " +
					"string is accepted too.",
			},
		},
		"required": []string{"mode", "tools"},
	}
	paramsJSON, _ := json.Marshal(params)

	return llm.ToolDef{
		Name: UpdateIncludedToolsToolName,
		Description: "Remove MCP tools from one mode's included list. Included tools are handed to " +
			"every turn that runs in the mode; a tool taken out stays reachable through tool_search. " +
			"This tool only removes -- putting a tool back is done in the interface. The result " +
			"reports how many names were removed, how many were not in the list, and how many remain.",
		Parameters: json.RawMessage(paramsJSON),
	}
}

// updateIncludedToolsHandler subtracts the given tool names from one mode's
// included list.
//
// Removal is the only direction on purpose. SyncCatalog can drop a name only
// once the catalog has carried it, so a name the model invented would otherwise
// sit in mcp-included.json forever; subtracting makes a misspelling a no-op the
// result reports back instead.
func (s *ChatService) updateIncludedToolsHandler() localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		if s.includedToolsSvc == nil {
			return "", fmt.Errorf("included tools service is not configured")
		}

		modeName := strings.TrimSpace(argString(args["mode"]))
		if modeName == "" {
			return "mode is required", nil
		}
		if !mode.IsValid(modeName) {
			return fmt.Sprintf(
				"unknown mode %q (valid: %s)",
				modeName, strings.Join(mode.Modes, ", "),
			), nil
		}

		raw, ok := args["tools"]
		if !ok || raw == nil {
			return "tools is required", nil
		}
		given := dedupeToolNames(argPatternSlice(raw))
		if len(given) == 0 {
			return "tools must contain at least one non-empty tool name", nil
		}

		current, err := s.includedToolsSvc.Get(modeName)
		if err != nil {
			return "", fmt.Errorf("read included tools for mode %q: %w", modeName, err)
		}

		included := make(map[string]struct{}, len(current))
		for _, name := range current {
			included[name] = struct{}{}
		}

		var removed, unknown []string
		for _, name := range given {
			if _, ok := included[name]; !ok {
				unknown = append(unknown, name)
				continue
			}
			delete(included, name)
			removed = append(removed, name)
		}

		if len(removed) > 0 {
			next := make([]string, 0, len(included))
			for name := range included {
				next = append(next, name)
			}
			sort.Strings(next)
			if err := s.includedToolsSvc.Set(modeName, next); err != nil {
				return "", fmt.Errorf("update included tools for mode %q: %w", modeName, err)
			}
		}

		msg := fmt.Sprintf(
			"Mode %s: removed %d tool(s), %d remain included.",
			modeName, len(removed), len(included),
		)
		if len(unknown) > 0 {
			// Naming them is the misspelling signal: a name that was never in the
			// list is either already excluded or simply wrong.
			msg += fmt.Sprintf(
				" %d name(s) were not in the list: %s.",
				len(unknown), strings.Join(unknown, ", "),
			)
		}
		return msg, nil
	}
}

// dedupeToolNames drops blanks and repeats while preserving the order given, so
// a name listed twice is not counted as removed twice.
func dedupeToolNames(names []string) []string {
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
