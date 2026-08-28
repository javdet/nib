package llm

import (
	"context"
	"encoding/json"
)

// Message is a single chat message in the OpenAI Chat Completions schema.
// Reasoning is only populated for assistant messages produced by the Responses
// API and is ignored by providers that cannot consume it.
type Message struct {
	Role       string
	Content    string
	Images     []ImageContent
	ToolCalls  []ToolCall
	ToolCallID string
	Reasoning  []ReasoningItem
}

// ReasoningItem is one reasoning trace emitted alongside an assistant reply.
// Feeding it back with the next request lets a reasoning model continue from
// its own thinking instead of restarting it after every tool call. The content
// is encrypted by the provider, so it is opaque to nib and must be passed
// through unmodified.
type ReasoningItem struct {
	ID               string
	EncryptedContent string
	Summary          []string
}

// ImageContent is a base64 data URL or remote image URL for multimodal user messages.
type ImageContent struct {
	DataURL string
}

// ToolDef describes a function tool exposed to the model.
type ToolDef struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// ToolCall is one element from an assistant message tool_calls array.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// AssistantMessage holds the assistant reply from a tool-capable completion.
// Callers running an agent loop must copy Reasoning into the assistant Message
// they append to the history; dropping it loses the model's thinking between
// tool-call rounds.
type AssistantMessage struct {
	Content   string
	ToolCalls []ToolCall
	Reasoning []ReasoningItem
}

// Provider abstracts LLM text completion so implementations
// (OpenAI, Anthropic, etc.) can be swapped without changing callers.
type Provider interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
	CompleteWithTools(ctx context.Context, messages []Message, tools []ToolDef) (AssistantMessage, error)
}
