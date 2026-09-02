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

// repairOrphanToolCalls rebuilds the transcript so every assistant tool_call is
// answered exactly once, by a tool row sitting immediately after it.
//
// Results are matched by tool_call_id across the whole transcript rather than by
// adjacency. A round writes its assistant row and its tool rows as separate
// inserts, so anything appended in between — an action sub-agent posting its
// result, a fan-out posting its questions — leaves the pair separated on disk.
// Scanning only forward from the assistant row would then miss the real results,
// synthesise replacements for calls that did complete, and replay the real rows
// afterwards as well: two results per id, which the completion API rejects.
//
// The repair is in-memory only; the stored transcript keeps whatever order it
// was written in.
func repairOrphanToolCalls(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}

	// First result wins: a duplicate id can only be a repair of an earlier one.
	resultByID := make(map[string]int)
	for i, m := range msgs {
		if m.Role != "tool" || m.ToolCallID == "" {
			continue
		}
		if _, dup := resultByID[m.ToolCallID]; !dup {
			resultByID[m.ToolCallID] = i
		}
	}

	out := make([]llm.Message, 0, len(msgs))
	emitted := make(map[string]struct{}, len(resultByID))

	for _, m := range msgs {
		if m.Role == "tool" {
			// Tool rows are emitted beside the call they answer, so the only
			// ones left here are already placed, or answer nothing at all and
			// would be rejected wherever they went.
			continue
		}

		out = append(out, m)
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}

		for _, tc := range m.ToolCalls {
			if _, done := emitted[tc.ID]; done {
				continue
			}
			emitted[tc.ID] = struct{}{}

			if idx, ok := resultByID[tc.ID]; ok {
				out = append(out, msgs[idx])
				continue
			}
			out = append(out, llm.Message{
				Role:       "tool",
				Content:    missingToolResultPayload(tc.ID),
				ToolCallID: tc.ID,
			})
		}
	}
	return out
}
