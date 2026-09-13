package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// ResponsesProvider talks to /v1/responses instead of /v1/chat/completions.
// OpenAI rejects function tools on chat completions for gpt-5.6 unless
// reasoning is disabled, so an agent that needs both tools and reasoning has to
// use this endpoint. Embeddings are unchanged and come from OpenAIProvider.
type ResponsesProvider struct {
	*OpenAIProvider
}

// NewResponsesProvider builds a provider that runs completions through the
// Responses API. Options are shared with the chat completions provider.
func NewResponsesProvider(opts ClientOptions) *ResponsesProvider {
	return &ResponsesProvider{OpenAIProvider: NewCompatProvider(opts)}
}

// requestReasoningEcho asks the API to return reasoning traces in a form that
// can be replayed on the next request. Required because responses are not
// stored server-side (see Store below).
var requestReasoningEcho = []responses.ResponseIncludable{
	responses.ResponseIncludableReasoningEncryptedContent,
}

func (p *ResponsesProvider) baseParams() responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel(p.model),
		// Keep the agent loop stateless: nothing is retained by the provider
		// between rounds, so the full input is replayed every time.
		Store:   openai.Bool(false),
		Include: requestReasoningEcho,
	}
	if p.reasoningEffort != "" {
		params.Reasoning = shared.ReasoningParam{Effort: p.reasoningEffort}
	}
	return params
}

func (p *ResponsesProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	params := p.baseParams()
	params.Instructions = openai.String(systemPrompt)
	params.Input = responses.ResponseNewParamsInputUnion{OfString: openai.String(userPrompt)}

	resp, err := p.client.Responses.New(ctx, params)
	if err != nil {
		return "", wrapAPIError("openai responses", err)
	}
	if err := checkResponseStatus("openai responses", resp); err != nil {
		return "", err
	}
	return resp.OutputText(), nil
}

func (p *ResponsesProvider) CompleteWithTools(ctx context.Context, messages []Message, tools []ToolDef) (AssistantMessage, error) {
	input, err := messagesToResponsesInput(messages)
	if err != nil {
		return AssistantMessage{}, err
	}

	params := p.baseParams()
	params.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: input}
	if len(tools) > 0 {
		params.Tools = toolDefsToResponses(tools)
	}

	const op = "openai responses with tools"

	resp, err := p.client.Responses.New(ctx, params)
	if err != nil {
		return AssistantMessage{}, wrapAPIError(op, err)
	}
	if err := checkResponseStatus(op, resp); err != nil {
		// Usage is carried out even though this is an error. An incomplete
		// response is one that hit the output cap: it was generated, it was
		// billed, and returning a zero message here would make that spend
		// invisible to the statistics that exist to surface it.
		return AssistantMessage{Usage: usageFromResponse(resp)}, err
	}

	asst := assistantFromResponse(resp)
	// A reply with neither text nor a tool call would travel up the agent loop
	// as a successful empty answer. It means the model spent the whole response
	// on reasoning, so report it instead of ending the turn silently.
	if asst.Content == "" && len(asst.ToolCalls) == 0 {
		// Same reasoning: the reasoning tokens this burned were still billed.
		return AssistantMessage{Usage: asst.Usage},
			fmt.Errorf("%s: response carried no output text and no tool calls", op)
	}
	return asst, nil
}

// checkResponseStatus turns a response the provider itself considers unfinished
// into an error. Such a response is delivered with HTTP 200 and an output array
// that is empty or truncated, which is indistinguishable from a deliberate
// short answer for callers that only read the output.
func checkResponseStatus(op string, resp *responses.Response) error {
	switch resp.Status {
	case "", responses.ResponseStatusCompleted:
		return nil
	case responses.ResponseStatusIncomplete:
		reason := resp.IncompleteDetails.Reason
		if reason == "" {
			reason = "unknown reason"
		}
		return fmt.Errorf("%s: response incomplete (%s)", op, reason)
	case responses.ResponseStatusFailed:
		if msg := resp.Error.Message; msg != "" {
			return fmt.Errorf("%s: response failed: %s", op, msg)
		}
		return fmt.Errorf("%s: response failed", op)
	default:
		return fmt.Errorf("%s: unexpected response status %q", op, resp.Status)
	}
}

func messagesToResponsesInput(messages []Message) ([]responses.ResponseInputItemUnionParam, error) {
	out := make([]responses.ResponseInputItemUnionParam, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "system":
			out = append(out, responses.ResponseInputItemParamOfMessage(m.Content, responses.EasyInputMessageRoleSystem))
		case "user":
			out = append(out, userInputItem(m))
		case "assistant":
			// Reasoning has to arrive before the function calls it produced,
			// otherwise the API rejects the item ordering.
			for _, r := range m.Reasoning {
				out = append(out, reasoningInputItem(r))
			}
			if m.Content != "" {
				out = append(out, responses.ResponseInputItemParamOfMessage(m.Content, responses.EasyInputMessageRoleAssistant))
			}
			for _, tc := range m.ToolCalls {
				out = append(out, responses.ResponseInputItemParamOfFunctionCall(tc.Arguments, tc.ID, tc.Name))
			}
		case "tool":
			out = append(out, responses.ResponseInputItemParamOfFunctionCallOutput(m.ToolCallID, m.Content))
		default:
			return nil, fmt.Errorf("openai responses: unsupported message role %q", m.Role)
		}
	}
	return out, nil
}

func userInputItem(m Message) responses.ResponseInputItemUnionParam {
	if len(m.Images) == 0 {
		return responses.ResponseInputItemParamOfMessage(m.Content, responses.EasyInputMessageRoleUser)
	}

	content := make(responses.ResponseInputMessageContentListParam, 0, 1+len(m.Images))
	if m.Content != "" {
		content = append(content, responses.ResponseInputContentParamOfInputText(m.Content))
	}
	for _, img := range m.Images {
		part := responses.ResponseInputContentParamOfInputImage(responses.ResponseInputImageDetailAuto)
		part.OfInputImage.ImageURL = openai.String(img.DataURL)
		content = append(content, part)
	}
	return responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser)
}

func reasoningInputItem(r ReasoningItem) responses.ResponseInputItemUnionParam {
	summary := make([]responses.ResponseReasoningItemSummaryParam, 0, len(r.Summary))
	for _, s := range r.Summary {
		summary = append(summary, responses.ResponseReasoningItemSummaryParam{Text: s})
	}
	item := responses.ResponseReasoningItemParam{ID: r.ID, Summary: summary}
	if r.EncryptedContent != "" {
		item.EncryptedContent = openai.String(r.EncryptedContent)
	}
	return responses.ResponseInputItemUnionParam{OfReasoning: &item}
}

func toolDefsToResponses(tools []ToolDef) []responses.ToolUnionParam {
	out := make([]responses.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		// Strict mode defaults to true and would reject the loosely typed
		// schemas that MCP servers publish.
		tool := responses.ToolParamOfFunction(t.Name, responsesParametersFromRaw(t.Parameters), false)
		if t.Description != "" {
			tool.OfFunction.Description = openai.String(t.Description)
		}
		out = append(out, tool)
	}
	return out
}

func responsesParametersFromRaw(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{"type": "object"}
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil || len(params) == 0 {
		return map[string]any{"type": "object"}
	}
	return params
}

func assistantFromResponse(resp *responses.Response) AssistantMessage {
	var out AssistantMessage
	var text strings.Builder

	out.Usage = usageFromResponse(resp)

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" {
					text.WriteString(c.Text)
				}
			}
		case "function_call":
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:        item.CallID,
				Name:      item.Name,
				Arguments: item.Arguments.OfString,
			})
		case "reasoning":
			r := ReasoningItem{ID: item.ID, EncryptedContent: item.EncryptedContent}
			for _, s := range item.Summary {
				r.Summary = append(r.Summary, s.Text)
			}
			out.Reasoning = append(out.Reasoning, r)
		}
	}

	out.Content = text.String()
	return out
}

// usageFromResponse lifts the usage block the Responses API already returns.
// The model comes off the response rather than from config: a gateway may route
// to a variant, and statistics are only worth keeping if they name what ran.
func usageFromResponse(resp *responses.Response) Usage {
	if resp == nil {
		return Usage{}
	}
	u := resp.Usage
	return Usage{
		Model:              string(resp.Model),
		PromptTokens:       int(u.InputTokens),
		CachedPromptTokens: int(u.InputTokensDetails.CachedTokens),
		CompletionTokens:   int(u.OutputTokens),
		ReasoningTokens:    int(u.OutputTokensDetails.ReasoningTokens),
		TotalTokens:        int(u.TotalTokens),
		CostUSD:            costFromUsageJSON(u.RawJSON()),
	}
}
