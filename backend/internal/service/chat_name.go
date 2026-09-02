package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/textutil"
	"github.com/google/uuid"
)

const (
	ChatNameToolName = "chat_name"
	chatNameMaxLen   = 80
)

var chatNameParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "Task or ticket identifier, e.g. DO-236. Omit when no task is associated."
    },
    "chat_name": {
      "type": "string",
      "description": "Short conversation summary (max 80 characters)"
    }
  },
  "required": ["chat_name"]
}`)

// ChatNameToolDef returns the LLM tool definition for the local chat_name handler.
func ChatNameToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        ChatNameToolName,
		Description: "Assign a display name to the current conversation. When task_id is provided, the title is stored as \"<task_id>: <chat_name>\" and task_id is persisted on the dialog.",
		Parameters:  chatNameParameters,
	}
}

// chatNameHandler returns a local tool handler bound to a specific dialog.
func (s *ChatService) chatNameHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		taskID := normalizeChatNameTaskID(argString(args["task_id"]))
		name := strings.TrimSpace(argString(args["chat_name"]))
		if name == "" {
			return "chat_name is required", nil
		}
		name = textutil.TruncateRunes(name, chatNameMaxLen)
		title := name
		if taskID != "" {
			title = taskID + ": " + name
		}
		if s.dialogRepo == nil {
			return "", fmt.Errorf("dialog repository is not configured")
		}
		if err := s.dialogRepo.UpdateTitle(ctx, dialogID, title); err != nil {
			return "", fmt.Errorf("update dialog title: %w", err)
		}
		if taskID != "" {
			taskIDCopy := taskID
			if err := s.dialogRepo.SetDialogTaskID(ctx, dialogID, &taskIDCopy); err != nil {
				return "", fmt.Errorf("update dialog task_id: %w", err)
			}
		}
		return "Conversation named: " + title, nil
	}
}

func normalizeChatNameTaskID(raw string) string {
	taskID := strings.TrimSpace(raw)
	if taskID == "" {
		return ""
	}
	switch strings.ToLower(taskID) {
	case "none", "null":
		return ""
	default:
		return taskID
	}
}

func argString(v any) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	default:
		return fmt.Sprint(v)
	}
}

// argBool reads a boolean tool argument, tolerating the string forms a model
// sometimes emits in place of a JSON boolean.
func argBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(strings.TrimSpace(b), "true")
	default:
		return false
	}
}
