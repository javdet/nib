package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/repository"
)

const GetRuleToolName = "get_rule"

var getRuleParameters = json.RawMessage(`{
	"type": "object",
	"properties": {
		"name": {
			"type": "string",
			"description": "Rule name (filename without .md extension)"
		}
	},
	"required": ["name"]
}`)

// GetRuleToolDef returns the LLM tool definition for loading a rule by name.
func GetRuleToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: GetRuleToolName,
		Description: "Load the full text of a rule by name. " +
			"Rules are listed under \"Rule library\" in the system prompt.",
		Parameters: getRuleParameters,
	}
}

// ExecuteGetRule loads and renders a rule file by name.
func (s *ChatService) ExecuteGetRule(ctx context.Context, args map[string]any) (string, error) {
	if s.rulesSvc == nil {
		return "", fmt.Errorf("get_rule: rules service not configured")
	}

	rawName, ok := args["name"].(string)
	if !ok {
		return "", fmt.Errorf("get_rule: name is required")
	}
	name := strings.TrimSpace(rawName)
	if name == "" {
		return "", fmt.Errorf("get_rule: name is required")
	}

	rule, err := s.rulesSvc.Get(name)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", fmt.Errorf("rule %q not found", name)
		}
		return "", fmt.Errorf("get rule: %w", err)
	}

	rendered, err := RenderTemplateVariables(ctx, rule.Content, s.variableRepo, s.selection)
	if err != nil {
		return "", fmt.Errorf("render rule: %w", err)
	}
	return rendered, nil
}

// buildRulesSection renders the catalog of every rule on disk for the tail of
// the system prompt.
func (s *ChatService) buildRulesSection() (string, error) {
	if s.rulesSvc == nil {
		return "", nil
	}

	list, err := s.rulesSvc.List()
	if err != nil {
		return "", err
	}
	if len(list.Rules) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("## Rule library\n")
	sb.WriteString("The following rules are available. Each one holds the guardrails for a specific component or product. Before you plan work that touches a component a rule covers, call get_rule with its name to load the full text, then follow it.\n")
	for _, m := range list.Rules {
		if m.Description == "" {
			sb.WriteString("- ")
			sb.WriteString(m.Name)
			sb.WriteByte('\n')
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(m.Name)
		sb.WriteString(": ")
		sb.WriteString(m.Description)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}
