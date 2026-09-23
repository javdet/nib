package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
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

// ExecuteGetSkill loads and renders a skill file by name, outside any dialog.
// With no transcript to find a slash command in, an explicit-only skill is
// refused here; the chat path is getSkillHandler.
func (s *ChatService) ExecuteGetSkill(ctx context.Context, args map[string]any) (string, error) {
	return s.getSkill(ctx, uuid.Nil, args)
}

// getSkillHandler binds get_skill to the plan's root dialog, whose operator
// messages are what an explicit-only skill has to be invoked from.
func (s *ChatService) getSkillHandler(b toolBinding) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		return s.getSkill(ctx, b.planID, args)
	}
}

func (s *ChatService) getSkill(ctx context.Context, rootID uuid.UUID, args map[string]any) (string, error) {
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

	// The catalog leaving a skill out is only a hint; this is the gate. Every
	// mode carries get_skill, so a sub-agent that guesses a name must meet the
	// same refusal the orchestrator would.
	switch skills.ParseAccess(skill.Content) {
	case skills.AccessDisabled:
		return "", fmt.Errorf("skill %q is disabled", name)
	case skills.AccessExplicit:
		invoked, err := s.operatorInvokedSkill(ctx, rootID, name)
		if err != nil {
			return "", fmt.Errorf("get skill: %w", err)
		}
		if !invoked {
			return "", fmt.Errorf("skill %q runs only when the operator invokes it with /%s", name, name)
		}
	}

	rendered, err := RenderTemplateVariables(ctx, skill.Content, s.variableRepo, s.selection)
	if err != nil {
		return "", fmt.Errorf("render skill: %w", err)
	}
	return rendered, nil
}

// operatorInvokedSkill reports whether an operator message in the root dialog
// carries the slash command for name. The whole transcript counts, not only the
// latest turn: a plan invoked with /name keeps working under it across turns and
// in every sub-agent, all of which share the root.
func (s *ChatService) operatorInvokedSkill(ctx context.Context, rootID uuid.UUID, name string) (bool, error) {
	if rootID == uuid.Nil || s.dialogRepo == nil {
		return false, nil
	}
	msgs, err := s.dialogRepo.ListMessages(ctx, rootID)
	if err != nil {
		return false, fmt.Errorf("list messages: %w", err)
	}
	for _, msg := range msgs {
		// A named user row is nib's own writing, never the operator's.
		if msg.Role != "user" || msg.Name != "" {
			continue
		}
		if invokesSkill(msg.Content, name) {
			return true, nil
		}
	}
	return false, nil
}

// invokesSkill reports whether text holds `/name` as a command: the slash must
// open a word, so a path or URL ending in the name does not count, and the name
// must end there, so /name-two does not invoke name.
func invokesSkill(text, name string) bool {
	token := "/" + name
	for offset := 0; ; {
		i := strings.Index(text[offset:], token)
		if i < 0 {
			return false
		}
		start := offset + i
		end := start + len(token)
		if (start == 0 || !isSlashCommandPrefix(text[start-1])) &&
			(end == len(text) || !isSkillNameByte(text[end])) {
			return true
		}
		offset = start + 1
	}
}

// isSlashCommandPrefix reports whether b glues a following slash to a word,
// the way a path segment or a URL does.
func isSlashCommandPrefix(b byte) bool {
	return isSkillNameByte(b) || strings.IndexByte("./:~\\", b) >= 0
}

// isSkillNameByte matches filestore.ValidateBasename's character set.
func isSkillNameByte(b byte) bool {
	return b == '_' || b == '-' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// buildSkillsSection renders the catalog of every skill the agent may pick on
// its own -- the operator's enabled ones plus the ones the image owns -- for the
// tail of the system prompt. Explicit-only skills stay out of it: the operator
// names them with a slash command, and the agent learns only how to honour one.
func (s *ChatService) buildSkillsSection() (string, error) {
	if s.skillsSvc == nil {
		return "", nil
	}

	list, err := s.skillsSvc.ListAll()
	if err != nil {
		return "", err
	}

	var listed []skills.Meta
	hasExplicit := false
	for _, m := range list.Skills {
		switch m.Access {
		case skills.AccessDisabled:
		case skills.AccessExplicit:
			hasExplicit = true
		default:
			listed = append(listed, m)
		}
	}
	if len(listed) == 0 && !hasExplicit {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("## Skills\n")
	if len(listed) > 0 {
		sb.WriteString("The following skills are available. To use one, call get_skill with its name to load the full instructions, then follow them.\n")
		for _, m := range listed {
			sb.WriteString("- ")
			sb.WriteString(m.Name)
			if m.Description != "" {
				sb.WriteString(": ")
				sb.WriteString(m.Description)
			}
			sb.WriteByte('\n')
		}
	}
	if hasExplicit {
		if len(listed) > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString("Other skills run only on the operator's command. When an operator message contains a slash command such as `/get-jira-task`, call get_skill with that name (without the slash) and follow the instructions it returns. Never load a skill that is not listed above unless the operator has invoked it this way.\n")
	}
	return sb.String(), nil
}
