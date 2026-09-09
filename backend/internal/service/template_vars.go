package service

import (
	"context"
	"fmt"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/prompttpl"
	"github.com/javdet/nib/internal/repository"
)

// RenderTemplateVariables substitutes text/template actions in content using
// prompt variables from the database and built-in selection fields.
func RenderTemplateVariables(ctx context.Context, content string, variableRepo repository.VariableRepository, selection *SelectionStore) (string, error) {
	if !prompttpl.HasTemplate(content) {
		return content, nil
	}

	var sel domain.Selection
	if selection != nil {
		sel = selection.Get()
	}

	data := make(map[string]map[string]any)
	if variableRepo != nil {
		vars, err := variableRepo.LoadAll(ctx, activeScopeNames(sel))
		if err != nil {
			return "", fmt.Errorf("load prompt variables: %w", err)
		}
		for scope, names := range vars {
			data[scope] = names
		}
	}

	if selection != nil {
		data["builtin"] = map[string]any{
			"Project":     sel.Project,
			"Environment": sel.Environment,
			"Cloud":       sel.Cloud,
			"Location":    sel.Location,
		}
	}

	return prompttpl.Render(content, data)
}

// activeScopeNames maps each entity-level scope to the entity currently
// selected, so a project-scoped variable resolves to the selected project's
// value rather than to whichever project's row the database returned first.
// "any" means nothing is selected and is left out, which leaves the ordered
// fallback in repository.VariableRepository.LoadAll in charge.
func activeScopeNames(sel domain.Selection) map[string]string {
	names := make(map[string]string, 4)
	for scope, name := range map[string]string{
		VariableScopeProject:     sel.Project,
		VariableScopeEnvironment: sel.Environment,
		VariableScopeCloud:       sel.Cloud,
		VariableScopeLocation:    sel.Location,
	} {
		if name == "" || name == selectionAny {
			continue
		}
		names[scope] = name
	}
	return names
}
