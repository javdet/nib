package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/javdet/nib/internal/llm"
)

const maxToolFailuresPerTurn = 8

// ErrTooManyToolFailures is returned when a single turn exceeds the allowed
// number of recoverable tool failures.
var ErrTooManyToolFailures = errors.New("too many failed tool calls in one turn")

func toolErrorPayload(name string, err error) string {
	text := fmt.Sprintf("tool %q failed: %s", name, err.Error())
	payload := map[string]any{
		"isError": true,
		"content": []map[string]string{
			{"type": "text", "text": text},
		},
	}
	b, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return fmt.Sprintf(`{"isError":true,"content":[{"type":"text","text":%q}]}`, text)
	}
	return string(b)
}

func missingToolResultPayload(toolCallID string) string {
	text := fmt.Sprintf("tool result missing for call %q: the previous turn was interrupted", toolCallID)
	payload := map[string]any{
		"isError": true,
		"content": []map[string]string{
			{"type": "text", "text": text},
		},
	}
	b, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return fmt.Sprintf(`{"isError":true,"content":[{"type":"text","text":%q}]}`, text)
	}
	return string(b)
}

func isTurnFatalToolError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx.Err() != nil {
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// repairOrphanToolCalls inserts synthetic tool results for assistant tool_calls
// that have no matching tool message in the transcript. The repair is in-memory
// only so existing dialogs with interrupted turns can be replayed to the LLM.
func repairOrphanToolCalls(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}

	out := make([]llm.Message, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		out = append(out, m)
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}

		seen := make(map[string]struct{})
		j := i + 1
		for j < len(msgs) && msgs[j].Role == "tool" {
			if msgs[j].ToolCallID != "" {
				seen[msgs[j].ToolCallID] = struct{}{}
			}
			out = append(out, msgs[j])
			j++
		}

		for _, tc := range m.ToolCalls {
			if _, ok := seen[tc.ID]; ok {
				continue
			}
			out = append(out, llm.Message{
				Role:       "tool",
				Content:    missingToolResultPayload(tc.ID),
				ToolCallID: tc.ID,
			})
		}
		i = j - 1
	}
	return out
}
