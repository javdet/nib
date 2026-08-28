package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/javdet/nib/internal/llm"
)

const GetSecretsToolName = "get_secrets"

var getSecretsParameters = json.RawMessage(`{"type":"object","properties":{}}`)

// GetSecretsToolDef returns the LLM tool definition for the local get_secrets handler.
func GetSecretsToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: GetSecretsToolName,
		Description: "List existing secrets available in the system. " +
			"Returns a JSON array of {name, description}. Secret values are never returned.",
		Parameters: getSecretsParameters,
	}
}

// ExecuteGetSecrets returns metadata for all secrets as JSON.
func (s *SecretService) ExecuteGetSecrets(ctx context.Context, args map[string]any) (string, error) {
	secrets, err := s.List(ctx)
	if err != nil {
		return "", fmt.Errorf("get secrets: %w", err)
	}

	type row struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	out := make([]row, 0, len(secrets))
	for _, sec := range secrets {
		out = append(out, row{Name: sec.Name, Description: sec.Description})
	}

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal secrets: %w", err)
	}
	return string(b), nil
}
