package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/skills"
)

const GetSkillToolName = "get_skill"

var getSkillParameters = json.RawMessage(`{
	"type": "object",
	"properties": {
		"name": {
			"type": "string",
			"description": "Skill name (filename without .md extension)"
		}
	},
	"required": ["name"]
}`)

// GetSkillToolDef returns the LLM tool definition for loading a skill by name.
func GetSkillToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: GetSkillToolName,
		Description: "Load the full instructions of a skill by name. " +
			"Skills are listed in the system prompt.",
		Parameters: getSkillParameters,
	}
}

// ExecuteGetSkill loads and renders a skill file by name.
func (s *ChatService) ExecuteGetSkill(ctx context.Context, args map[string]any) (string, error) {
	if s.skillsSvc == nil {
		return "", fmt.Errorf("get_skill: skills service not configured")
	}

	rawName, ok := args["name"].(string)
	if !ok {
		return "", fmt.Errorf("get_skill: name is required")
	}
	name := strings.TrimSpace(rawName)
	if name == "" {
		return "", fmt.Errorf("get_skill: name is required")
	}

	skill, err := s.skillsSvc.GetAny(name)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", fmt.Errorf("skill %q not found", name)
		}
		return "", fmt.Errorf("get skill: %w", err)
	}

	// A system skill is shipped documentation, not operator text: it quotes
	// template syntax like {{ .global.CompanyName }} as an example, and
	// prompttpl renders with missingkey=error, so putting it through the
	// renderer would fail the tool call on its own documentation.
	if skills.IsSystem(name) {
		return skill.Content, nil
	}

	rendered, err := RenderTemplateVariables(ctx, skill.Content, s.variableRepo, s.selection)
	if err != nil {
		return "", fmt.Errorf("render skill: %w", err)
	}
	return rendered, nil
}

// buildSkillsSection renders the catalog of every skill the agent can load --
// the operator's own plus the ones the image owns -- for the tail of the system
// prompt.
func (s *ChatService) buildSkillsSection() (string, error) {
	if s.skillsSvc == nil {
		return "", nil
	}

	list, err := s.skillsSvc.ListAll()
	if err != nil {
		return "", err
	}
	if len(list.Skills) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("## Skills\n")
	sb.WriteString("The following skills are available. To use one, call get_skill with its name to load the full instructions, then follow them.\n")
	for _, m := range list.Skills {
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
