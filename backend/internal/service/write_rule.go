package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/filestore"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/rules"
)

// WriteRuleToolName is the OpenAI function name for creating or replacing a rule.
const WriteRuleToolName = "write_rule"

var writeRuleParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "Rule name, also the filename without .md. Must match [a-zA-Z0-9][a-zA-Z0-9_-]* - no dots, no slashes, no spaces."
    },
    "description": {
      "type": "string",
      "description": "One line saying what the rule covers. This is the only thing the planner sees in the rule library, so it has to be specific enough to decide whether to load the rule."
    },
    "body": {
      "type": "string",
      "description": "Full markdown text of the rule, without the frontmatter block. Replaces the previous body entirely."
    }
  },
  "required": ["name", "description", "body"]
}`)

// WriteRuleToolDef returns the LLM tool definition for writing a rule file.
func WriteRuleToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: WriteRuleToolName,
		Description: "Create a rule, or replace one that already exists, under the rules directory. " +
			"Rules are the guardrails Plan mode reads: it lists every rule with its description and loads " +
			"the ones it needs with get_rule. This is a full overwrite, not an append - for an existing " +
			"rule read it first with get_rule and pass the complete merged text, never a fragment.",
		Parameters: writeRuleParameters,
	}
}

// writeRuleHandler writes {name}.md into the rules directory, composing the
// frontmatter from name and description so the rule is shaped exactly like one
// saved from the Rules page.
//
// Argument problems come back as tool output rather than Go errors, so the
// agent can correct a bad name or an empty body in the next round instead of
// losing the turn.
func (s *ChatService) writeRuleHandler() localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		if s.rulesSvc == nil {
			return "", fmt.Errorf("rules service is not configured")
		}

		name := strings.TrimSpace(argString(args["name"]))
		if name == "" {
			return "name is required", nil
		}
		if err := rules.ValidateName(name); err != nil {
			return fmt.Sprintf("invalid rule name %q: must match [a-zA-Z0-9][a-zA-Z0-9_-]*", name), nil
		}

		description := strings.TrimSpace(argString(args["description"]))
		if description == "" {
			return "description is required: without it the rule library lists a name the planner cannot judge", nil
		}
		// The description is one frontmatter line; a pasted multi-line value
		// would put everything after the first newline outside the block.
		description = strings.Join(strings.Fields(description), " ")

		body := strings.TrimSpace(argString(args["body"]))
		if body == "" {
			return "body is empty", nil
		}

		existed := true
		if _, err := s.rulesSvc.Get(name); err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				return "", fmt.Errorf("read rule %q: %w", name, err)
			}
			existed = false
		}

		content := filestore.BuildDocument(filestore.Meta{Name: name, Description: description}, body)
		if err := s.rulesSvc.Put(name, content); err != nil {
			return "", fmt.Errorf("write rule %q: %w", name, err)
		}

		if existed {
			return fmt.Sprintf("Rule %s replaced.", name), nil
		}
		return fmt.Sprintf("Rule %s created.", name), nil
	}
}
