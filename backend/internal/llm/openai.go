package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

const defaultEmbeddingModel = "text-embedding-3-small"

// ClientOptions configures an OpenAI-compatible LLM client.
type ClientOptions struct {
	APIKey         string
	Model          string
	BaseURL        string
	EmbeddingModel string
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

// OpenAIProvider implements Provider and Embedder using the OpenAI-compatible API.
type OpenAIProvider struct {
	client          openai.Client
	model           string
	embeddingModel  string
	reasoningEffort shared.ReasoningEffort
}

// NewCompatProvider builds a provider for chat completions and embeddings using ClientOptions.
// EmbeddingModel defaults to text-embedding-3-small when empty.
func NewCompatProvider(opts ClientOptions) *OpenAIProvider {
	embeddingModel := opts.EmbeddingModel
	if embeddingModel == "" {
		embeddingModel = defaultEmbeddingModel
	}
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
		embeddingModel:  embeddingModel,
		reasoningEffort: shared.ReasoningEffort(strings.ToLower(strings.TrimSpace(opts.ReasoningEffort))),
	}
}

// NewOpenAIProvider builds a provider for chat completions and embeddings.
// embeddingModel defaults to text-embedding-3-small when empty.
func NewOpenAIProvider(apiKey, model, baseURL, embeddingModel string) *OpenAIProvider {
	return NewCompatProvider(ClientOptions{
		APIKey:         apiKey,
		Model:          model,
		BaseURL:        baseURL,
		EmbeddingModel: embeddingModel,
	})
}

func (p *OpenAIProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(p.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt),
			openai.UserMessage(userPrompt),
		},
		ReasoningEffort: p.reasoningEffort,
	})
	if err != nil {
		return "", wrapAPIError("openai chat completion", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai chat completion: no choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}

func (p *OpenAIProvider) CompleteWithTools(ctx context.Context, messages []Message, tools []ToolDef) (AssistantMessage, error) {
	openaiMessages, err := messagesToOpenAI(messages)
	if err != nil {
		return AssistantMessage{}, err
	}

	params := openai.ChatCompletionNewParams{
		Model:           openai.ChatModel(p.model),
		Messages:        openaiMessages,
		ReasoningEffort: p.reasoningEffort,
	}
	if len(tools) > 0 {
		params.Tools = toolDefsToOpenAI(tools)
	}

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return AssistantMessage{}, wrapAPIError("openai chat completion with tools", err)
	}
	if len(resp.Choices) == 0 {
		return AssistantMessage{}, fmt.Errorf("openai chat completion with tools: no choices returned")
	}

	msg := resp.Choices[0].Message
	return AssistantMessage{
		Content:   msg.Content,
		ToolCalls: toolCallsFromOpenAI(msg.ToolCalls),
	}, nil
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
			if m.Content != "" {
				asst.Content.OfString = openai.String(m.Content)
			}
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

func functionParametersFromRaw(raw json.RawMessage) shared.FunctionParameters {
	if len(raw) == 0 {
		return shared.FunctionParameters{"type": "object"}
	}
	var params shared.FunctionParameters
	if err := json.Unmarshal(raw, &params); err != nil || len(params) == 0 {
		return shared.FunctionParameters{"type": "object"}
	}
	return params
}

func toolCallsFromOpenAI(calls []openai.ChatCompletionMessageToolCallUnion) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(calls))
	for _, tc := range calls {
		fn, ok := tc.AsAny().(openai.ChatCompletionMessageFunctionToolCall)
		if !ok {
			continue
		}
		out = append(out, ToolCall{
			ID:        fn.ID,
			Name:      fn.Function.Name,
			Arguments: fn.Function.Arguments,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
