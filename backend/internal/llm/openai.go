package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/javdet/nib/internal/toolschema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// ClientOptions configures an OpenAI-compatible LLM client.
type ClientOptions struct {
	APIKey         string
	Model          string
	BaseURL        string
	TimeoutSeconds int
	HTTPReferer    string
	AppTitle       string
	// ReasoningEffort is sent as reasoning_effort when non-empty. OpenAI rejects
	// function tools on /v1/chat/completions for gpt-5.6 unless this is "none".
	ReasoningEffort string
}

// attributionTransport injects OpenRouter attribution headers on every request.
type attributionTransport struct {
	base        http.RoundTripper
	httpReferer string
	appTitle    string
}

func (t *attributionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.httpReferer != "" {
		req.Header.Set("HTTP-Referer", t.httpReferer)
	}
	if t.appTitle != "" {
		req.Header.Set("X-Title", t.appTitle)
	}
	return t.base.RoundTrip(req)
}

func newHTTPClient(timeoutSeconds int, httpReferer, appTitle string) *http.Client {
	base := http.DefaultTransport
	if httpReferer != "" || appTitle != "" {
		base = &attributionTransport{
			base:        http.DefaultTransport,
			httpReferer: httpReferer,
			appTitle:    appTitle,
		}
	}
	client := &http.Client{Transport: base}
	if timeoutSeconds > 0 {
		client.Timeout = time.Duration(timeoutSeconds) * time.Second
	}
	return client
}

// OpenAIProvider implements Provider using the OpenAI-compatible chat
// completions API. Embeddings are [EmbeddingsClient]'s job, on an endpoint of
// their own.
type OpenAIProvider struct {
	client          openai.Client
	model           string
	reasoningEffort shared.ReasoningEffort
}

// NewCompatProvider builds a chat-completions provider using ClientOptions.
func NewCompatProvider(opts ClientOptions) *OpenAIProvider {
	clientOpts := []option.RequestOption{
		option.WithAPIKey(opts.APIKey),
		option.WithHTTPClient(newHTTPClient(opts.TimeoutSeconds, opts.HTTPReferer, opts.AppTitle)),
	}
	if opts.BaseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(opts.BaseURL))
	}
	client := openai.NewClient(clientOpts...)
	return &OpenAIProvider{
		client:          client,
		model:           opts.Model,
		reasoningEffort: shared.ReasoningEffort(strings.ToLower(strings.TrimSpace(opts.ReasoningEffort))),
	}
}

func (p *OpenAIProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	const op = "openai chat completion"

	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModel(p.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
	}
	p.applyReasoningEffort(&params)

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return "", p.wrapCompletionError(op, err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("%s: no choices returned", op)
	}
	choice := resp.Choices[0]
	// Checked here too, not only on the tool path: this call backs chat naming
	// and summaries, where a truncated answer is silently wrong rather than
	// obviously missing.
	if err := completionStopError(op, finishReason(choice.FinishReason), choice.Message.Refusal); err != nil {
		return "", err
	}
	return choice.Message.Content, nil
}

// applyReasoningEffort sets reasoning_effort only when one is configured.
//
// The SDK's omitzero tag already drops an empty value; the explicit guard
// keeps the intent local and survives a tag change. It matters because the
// parameter is sent on every request whatever the model is, and several models
// reject it outright.
func (p *OpenAIProvider) applyReasoningEffort(params *openai.ChatCompletionNewParams) {
	if p.reasoningEffort != "" {
		params.ReasoningEffort = p.reasoningEffort
	}
}

// wrapCompletionError adds a hint when a request carrying reasoning_effort is
// rejected outright, which is how a model that does not accept the parameter
// presents. Deliberately not a retry without it: that doubles spend on every
// genuinely bad request and hides a config error behind a latency spike.
func (p *OpenAIProvider) wrapCompletionError(op string, err error) error {
	wrapped := wrapAPIError(op, err)
	if p.reasoningEffort == "" {
		return wrapped
	}
	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		return wrapped
	}
	return fmt.Errorf("%w (reasoning_effort=%q is sent on every request; clear llm.reasoningEffort if this model rejects it)",
		wrapped, string(p.reasoningEffort))
}

// finishReason normalises the provider's stop reason. Gemini's compatibility
// layer has been seen answering with an upper-case "STOP".
func finishReason(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// completionStopError turns a stop reason that did not produce a usable answer
// into an error. An unrecognised or absent reason passes.
func completionStopError(op, reason, refusal string) error {
	switch reason {
	case "length":
		return fmt.Errorf("%s: %w", op, ErrTruncated)
	case "content_filter":
		return fmt.Errorf("%s: %w (content filter)", op, ErrRefused)
	}
	if strings.TrimSpace(refusal) != "" {
		return fmt.Errorf("%s: %w: %s", op, ErrRefused, strings.TrimSpace(refusal))
	}
	return nil
}

func (p *OpenAIProvider) CompleteWithTools(ctx context.Context, messages []Message, tools []ToolDef) (AssistantMessage, error) {
	const op = "openai chat completion with tools"

	openaiMessages, err := messagesToOpenAI(messages)
	if err != nil {
		return AssistantMessage{}, err
	}

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(p.model),
		Messages: openaiMessages,
	}
	p.applyReasoningEffort(&params)
	if len(tools) > 0 {
		params.Tools = toolDefsToOpenAI(tools)
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return AssistantMessage{}, p.wrapCompletionError(op, err)
	}
	if len(resp.Choices) == 0 {
		return AssistantMessage{}, fmt.Errorf("%s: no choices returned", op)
	}

	choice := resp.Choices[0]
	msg := choice.Message
	out := AssistantMessage{
		Content:      msg.Content,
		ToolCalls:    toolCallsFromOpenAI(msg.ToolCalls),
		Reasoning:    reasoningFromMessage(msg),
		FinishReason: finishReason(choice.FinishReason),
		Refusal:      strings.TrimSpace(msg.Refusal),
		Usage:        usageFromCompletion(resp),
	}

	// Usage is carried out alongside the error, as the responses endpoint
	// already does for an incomplete response: those tokens were generated and
	// billed whether or not the answer was usable.
	if err := completionStopError(op, out.FinishReason, out.Refusal); err != nil {
		return AssistantMessage{Usage: out.Usage}, err
	}
	// A round with no text and no tool calls cannot move the agent forward, and
	// both loops would read it as the turn's final answer. The responses
	// endpoint has had this guard all along.
	if out.Content == "" && len(out.ToolCalls) == 0 {
		return AssistantMessage{Usage: out.Usage}, fmt.Errorf("%s: %w", op, ErrNoOutput)
	}
	return out, nil
}

// usageFromCompletion lifts the usage block the provider already returns. The
// model is read off the response rather than from config here: a gateway may
// route to a variant, and the statistics are only worth keeping if they name
// what actually ran.
func usageFromCompletion(resp *openai.ChatCompletion) Usage {
	if resp == nil {
		return Usage{}
	}
	u := resp.Usage
	return Usage{
		Model:              resp.Model,
		PromptTokens:       int(u.PromptTokens),
		CachedPromptTokens: int(u.PromptTokensDetails.CachedTokens),
		CompletionTokens:   int(u.CompletionTokens),
		ReasoningTokens:    int(u.CompletionTokensDetails.ReasoningTokens),
		TotalTokens:        int(u.TotalTokens),
		CostUSD:            costFromUsageJSON(u.RawJSON()),
	}
}

func messagesToOpenAI(messages []Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "system":
			out = append(out, openai.SystemMessage(m.Content))
		case "user":
			if len(m.Images) > 0 {
				parts := make([]openai.ChatCompletionContentPartUnionParam, 0, 1+len(m.Images))
				if m.Content != "" {
					parts = append(parts, openai.TextContentPart(m.Content))
				}
				for _, img := range m.Images {
					parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
						URL: img.DataURL,
					}))
				}
				out = append(out, openai.UserMessage(parts))
			} else {
				out = append(out, openai.UserMessage(m.Content))
			}
		case "assistant":
			asst := openai.ChatCompletionAssistantMessageParam{}
			// Set unconditionally, including when empty: the union omits the
			// key at its zero value, and several gateways reject an assistant
			// message that carries tool_calls and no content key at all.
			asst.Content.OfString = openai.String(m.Content)
			// m.Reasoning is deliberately not replayed. On this endpoint it is
			// the provider's plaintext chain of thought, and DeepSeek rejects a
			// request that sends reasoning_content back in an assistant
			// message. Only the responses endpoint replays reasoning, and only
			// the encrypted form it issued itself.
			for _, tc := range m.ToolCalls {
				asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
					OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					},
				})
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{OfAssistant: &asst})
		case "tool":
			out = append(out, openai.ToolMessage(m.Content, m.ToolCallID))
		default:
			return nil, fmt.Errorf("openai chat completion: unsupported message role %q", m.Role)
		}
	}
	return out, nil
}

func toolDefsToOpenAI(tools []ToolDef) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		fn := shared.FunctionDefinitionParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  functionParametersFromRaw(t.Parameters),
		}
		out = append(out, openai.ChatCompletionFunctionTool(fn))
	}
	return out
}

// functionParametersFromRaw is one of the two places a published schema is
// rewritten before it is sent; responsesParametersFromRaw is the other.
// Between them they see every tool on both endpoints, which is why the
// sanitizer sits here rather than beside the MCP catalog: local tools publish
// the same awkward keywords and would otherwise bypass it.
func functionParametersFromRaw(raw json.RawMessage) shared.FunctionParameters {
	return shared.FunctionParameters(toolschema.Sanitize(raw))
}
