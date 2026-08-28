package service

import (
	"context"
	"fmt"

	"github.com/javdet/nib/internal/prompttpl"
	"github.com/javdet/nib/internal/repository"
)

// RenderTemplateVariables substitutes text/template actions in content using
// prompt variables from the database and built-in selection fields.
func RenderTemplateVariables(ctx context.Context, content string, variableRepo repository.VariableRepository, selection *SelectionStore) (string, error) {
	if !prompttpl.HasTemplate(content) {
		return content, nil
	}

	data := make(map[string]map[string]any)
	if variableRepo != nil {
		vars, err := variableRepo.LoadAll(ctx)
		if err != nil {
			return "", fmt.Errorf("load prompt variables: %w", err)
		}
		for scope, names := range vars {
			data[scope] = names
		}
	}

	if selection != nil {
		sel := selection.Get()
		data["builtin"] = map[string]any{
			"Project":     sel.Project,
			"Environment": sel.Environment,
			"Cloud":       sel.Cloud,
			"Location":    sel.Location,
		}
	}

	return prompttpl.Render(content, data)
}
