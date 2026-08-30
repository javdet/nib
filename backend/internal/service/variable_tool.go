package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/javdet/nib/internal/llm"
)

const ListVariablesToolName = "list_variables"

var listVariablesParameters = json.RawMessage(`{"type":"object","properties":{}}`)

// ListVariablesToolDef returns the LLM tool definition for listing prompt variables.
func ListVariablesToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: ListVariablesToolName,
		Description: "List prompt template variables configured in the system. " +
			"Returns a JSON array of {scope, name, description, value, kind}. " +
			"Variables are referenced in system prompts and skills as {{ .scope.Name }} " +
			"(e.g. {{ .global.CompanyName }}). " +
			"When kind is \"list\", value is a JSON-encoded array of strings.",
		Parameters: listVariablesParameters,
	}
}

type listVariablesRow struct {
	Scope       string `json:"scope"`
	ScopeName   string `json:"scopeName,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       string `json:"value"`
	Kind        string `json:"kind"`
}

// ExecuteListVariables returns all prompt variables as a JSON array.
func (s *ChatService) ExecuteListVariables(ctx context.Context, args map[string]any) (string, error) {
	if s.variableRepo == nil {
		return "", fmt.Errorf("list_variables: variable repository not configured")
	}

	vars, err := s.variableRepo.List(ctx)
	if err != nil {
		return "", fmt.Errorf("list variables: %w", err)
	}

	out := make([]listVariablesRow, 0, len(vars))
	for _, v := range vars {
		out = append(out, listVariablesRow{
			Scope:       v.Scope,
			ScopeName:   v.ScopeName,
			Name:        v.Name,
			Description: v.Description,
			Value:       v.Value,
			Kind:        v.Kind,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		return out[i].Name < out[j].Name
	})

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal variables: %w", err)
	}
	return string(b), nil
}
