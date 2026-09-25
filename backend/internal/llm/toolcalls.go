package llm

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/openai/openai-go/v3"
)

// toolCallsFromOpenAI reads the tool calls off an assistant message.
//
// It deliberately does not go through ChatCompletionMessageToolCallUnion.AsAny:
// that switches on "type", which several OpenAI-compatible gateways omit
// entirely, and the nil variant it returns for an unrecognised type used to
// drop every call silently -- turning a tool-calling round into what looked
// like an ordinary empty answer. The union's own ID and Function fields are
// filled from the body whatever "type" says, so they are read directly.
func toolCallsFromOpenAI(calls []openai.ChatCompletionMessageToolCallUnion) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(calls))
	for i, tc := range calls {
		if tc.Type == "custom" {
			continue // nib publishes no custom tools
		}
		call := ToolCall{
			ID:        strings.TrimSpace(tc.ID),
			Name:      strings.TrimSpace(tc.Function.Name),
			Arguments: tc.Function.Arguments,
		}
		if call.Name == "" || strings.TrimSpace(call.Arguments) == "" {
			// Typed decoding leaves a field at its zero value rather than
			// failing when the wire shape differs, so a provider that sends
			// `arguments` as an object instead of a string yields "" with no
			// error anywhere. Re-read the raw body before giving up.
			if raw, ok := toolCallFromRawJSON(tc.RawJSON()); ok {
				if call.ID == "" {
					call.ID = raw.ID
				}
				if call.Name == "" {
					call.Name = raw.Name
				}
				if strings.TrimSpace(call.Arguments) == "" {
					call.Arguments = raw.Arguments
				}
			}
		}
		if call.Name == "" {
			slog.Warn("llm: dropping tool call with no function name", "raw", tc.RawJSON())
			continue
		}
		if call.ID == "" {
			// Every later step pairs the result back to the call by id, and a
			// tool message with an empty tool_call_id is rejected by most
			// gateways. Providers that omit it send one call at a time, so the
			// index is a stable enough stand-in.
			call.ID = fmt.Sprintf("call_%d", i)
		}
		if strings.TrimSpace(call.Arguments) == "" {
			call.Arguments = "{}"
		}
		out = append(out, call)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// toolCallFromRawJSON re-reads a tool call from the raw body, tolerating
// `arguments` sent as a JSON object rather than a string.
func toolCallFromRawJSON(raw string) (ToolCall, bool) {
	if strings.TrimSpace(raw) == "" {
		return ToolCall{}, false
	}
	var probe struct {
		ID       string `json:"id"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return ToolCall{}, false
	}
	args := strings.TrimSpace(string(probe.Function.Arguments))
	if strings.HasPrefix(args, `"`) {
		var unquoted string
		if err := json.Unmarshal(probe.Function.Arguments, &unquoted); err == nil {
			args = unquoted
		}
	}
	if args == "null" {
		args = ""
	}
	return ToolCall{
		ID:        strings.TrimSpace(probe.ID),
		Name:      strings.TrimSpace(probe.Function.Name),
		Arguments: args,
	}, true
}
