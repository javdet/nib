package llm

import (
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3"
)

// reasoningFromMessage lifts a plaintext chain of thought off the raw message.
//
// The openai-go schema has no field for it, so it would otherwise be dropped
// into JSON.ExtraFields and lost: DeepSeek and Qwen put it in
// "reasoning_content", OpenRouter in "reasoning".
func reasoningFromMessage(msg openai.ChatCompletionMessage) []ReasoningItem {
	raw := strings.TrimSpace(msg.RawJSON())
	if raw == "" {
		return nil
	}
	var probe struct {
		ReasoningContent string `json:"reasoning_content"`
		Reasoning        string `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil
	}
	text := probe.ReasoningContent
	if strings.TrimSpace(text) == "" {
		text = probe.Reasoning
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	// Summary, never EncryptedContent: that field is provider-encrypted and is
	// replayed verbatim by the responses endpoint, where plaintext would be
	// rejected. This text is for the operator and the logs, not for replay.
	return []ReasoningItem{{Summary: []string{text}}}
}
