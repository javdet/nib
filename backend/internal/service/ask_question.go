package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
)

const AskQuestionToolName = "ask_question"

var askQuestionParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "questions": {
      "type": "array",
      "description": "List of clarifying questions to ask the user",
      "items": {
        "type": "object",
        "properties": {
          "question": {
            "type": "string",
            "description": "The question text"
          },
          "options": {
            "type": "array",
            "items": { "type": "string" },
            "description": "Suggested answer options (2-3 recommended)"
          }
        },
        "required": ["question"]
      }
    }
  },
  "required": ["questions"]
}`)

// AskQuestionToolDef returns the LLM tool definition for interactive user questions.
func AskQuestionToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        AskQuestionToolName,
		Description: "Ask the user one or more clarifying questions. Each question may include suggested answer options. The user can pick an option or type a free-form answer. Use when you need information from the user before continuing.",
		Parameters:  askQuestionParameters,
	}
}

func parseAskQuestionArgs(args map[string]any) ([]domain.Question, error) {
	raw, ok := args["questions"]
	if !ok {
		return nil, fmt.Errorf("missing required field %q", "questions")
	}

	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("questions must be an array")
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("questions must not be empty")
	}

	out := make([]domain.Question, 0, len(list))
	for i, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("questions[%d] must be an object", i)
		}
		q := strings.TrimSpace(argString(obj["question"]))
		if q == "" {
			return nil, fmt.Errorf("questions[%d].question is required", i)
		}
		question := domain.Question{Question: q}
		if optsRaw, ok := obj["options"]; ok && optsRaw != nil {
			optsList, ok := optsRaw.([]any)
			if !ok {
				return nil, fmt.Errorf("questions[%d].options must be an array", i)
			}
			for j, opt := range optsList {
				s := strings.TrimSpace(argString(opt))
				if s == "" {
					return nil, fmt.Errorf("questions[%d].options[%d] must be a non-empty string", i, j)
				}
				question.Options = append(question.Options, s)
			}
		}
		out = append(out, question)
	}
	return out, nil
}

func parseAskQuestionFromArguments(argumentsJSON string) ([]domain.Question, error) {
	args, err := parseToolArguments(argumentsJSON)
	if err != nil {
		return nil, err
	}
	return parseAskQuestionArgs(args)
}

func formatAskQuestionResult(questions []domain.Question, answers []string) (string, error) {
	if len(questions) != len(answers) {
		return "", fmt.Errorf("answer count %d does not match question count %d", len(answers), len(questions))
	}
	items := make([]map[string]string, len(questions))
	for i, q := range questions {
		items[i] = map[string]string{
			"question": q.Question,
			"answer":   answers[i],
		}
	}
	raw, err := json.Marshal(map[string]any{"answers": items})
	if err != nil {
		return "", fmt.Errorf("marshal ask_question result: %w", err)
	}
	return string(raw), nil
}

func findAskQuestionCall(msgs []domain.DialogMessage, toolCallID string) (llm.ToolCall, error) {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		calls, err := parseStoredToolCalls(m.ToolCalls)
		if err != nil {
			return llm.ToolCall{}, err
		}
		for _, tc := range calls {
			if tc.ID == toolCallID {
				if tc.Name != AskQuestionToolName {
					return llm.ToolCall{}, fmt.Errorf("tool call %q is %q, not %q", toolCallID, tc.Name, AskQuestionToolName)
				}
				return tc, nil
			}
		}
	}
	return llm.ToolCall{}, fmt.Errorf("tool call %q not found in dialog transcript", toolCallID)
}

func toolResultExists(msgs []domain.DialogMessage, toolCallID string) bool {
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID == toolCallID {
			return true
		}
	}
	return false
}
