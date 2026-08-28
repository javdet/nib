package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DefaultListToolsTimeout is the HTTP client timeout for one-shot MCP tool discovery.
const DefaultListToolsTimeout = 60 * time.Second

// DefaultUIDiscoveryTimeout is a shorter per-source deadline for UI tool listing
// where unreachable MCP servers must not block the page.
const DefaultUIDiscoveryTimeout = 8 * time.Second

// DefaultUIDiscoveryOverallTimeout caps the entire multi-source UI discovery request.
const DefaultUIDiscoveryOverallTimeout = 15 * time.Second

// DefaultTLSHandshakeTimeout is the TLS handshake deadline used when the caller
// does not supply its own transport. It is larger than net/http's 10s default so
// that slow or cold-starting MCP proxies do not fail discovery with
// "TLS handshake timeout".
const DefaultTLSHandshakeTimeout = 30 * time.Second

// defaultDiscoveryTransport clones http.DefaultTransport and relaxes the TLS
// handshake and response-header deadlines for MCP tool discovery. It is used as
// the base RoundTripper whenever the caller's http.Client has no custom transport.
func defaultDiscoveryTransport() http.RoundTripper {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	clone := t.Clone()
	clone.TLSHandshakeTimeout = DefaultTLSHandshakeTimeout
	clone.ResponseHeaderTimeout = DefaultListToolsTimeout
	return clone
}

// ListToolsFromURL connects to a streamable HTTP MCP server and returns tool metadata.
func ListToolsFromURL(ctx context.Context, serverURL string, headers map[string]string) ([]ToolInfo, error) {
	return ListToolsFromURLWithHTTPClient(ctx, serverURL, headers, &http.Client{Timeout: DefaultListToolsTimeout})
}

// ListToolsFromURLWithHTTPClient is like ListToolsFromURL but uses the given HTTP client
// (for example toolcatalog reindex with a configurable timeout).
func ListToolsFromURLWithHTTPClient(
	ctx context.Context,
	serverURL string,
	headers map[string]string,
	httpClient *http.Client,
) ([]ToolInfo, error) {
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return nil, fmt.Errorf("server url is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultListToolsTimeout}
	}

	base := defaultDiscoveryTransport()
	timeout := httpClient.Timeout
	if httpClient.Transport != nil {
		base = httpClient.Transport
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint: serverURL,
		HTTPClient: &http.Client{
			Timeout:   timeout,
			Transport: newHeadersRoundTripper(headers, base),
		},
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "nib",
		Version: "v0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect: %w", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("mcp list tools: %w", err)
	}

	tools := make([]ToolInfo, 0, len(result.Tools))
	for _, t := range result.Tools {
		schemaBytes, err := json.Marshal(t.InputSchema)
		if err != nil {
			schemaBytes = []byte("{}")
		}
		tools = append(tools, ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schemaBytes,
		})
	}
	return tools, nil
}

// CallToolFromURL connects to a streamable HTTP MCP server, invokes one tool, and closes the session.
func CallToolFromURL(ctx context.Context, serverURL string, headers map[string]string, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	return CallToolFromURLWithHTTPClient(ctx, serverURL, headers, toolName, arguments, &http.Client{Timeout: DefaultListToolsTimeout})
}

// CallToolFromURLWithHTTPClient is like CallToolFromURL but uses the given HTTP client.
func CallToolFromURLWithHTTPClient(
	ctx context.Context,
	serverURL string,
	headers map[string]string,
	toolName string,
	arguments map[string]any,
	httpClient *http.Client,
) (*mcp.CallToolResult, error) {
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return nil, fmt.Errorf("server url is required")
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultListToolsTimeout}
	}

	base := defaultDiscoveryTransport()
	timeout := httpClient.Timeout
	if httpClient.Transport != nil {
		base = httpClient.Transport
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint: serverURL,
		HTTPClient: &http.Client{
			Timeout:   timeout,
			Transport: newHeadersRoundTripper(headers, base),
		},
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "nib",
		Version: "v0.1.0",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect: %w", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      toolName,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp call tool %s: %w", toolName, err)
	}
	return result, nil
}

type headersRoundTripper struct {
	headers  map[string]string
	delegate http.RoundTripper
}

func newHeadersRoundTripper(headers map[string]string, delegate http.RoundTripper) http.RoundTripper {
	if len(headers) == 0 {
		if delegate == nil {
			return http.DefaultTransport
		}
		return delegate
	}
	if delegate == nil {
		delegate = http.DefaultTransport
	}
	cp := make(map[string]string, len(headers))
	for k, v := range headers {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		cp[k] = v
	}
	return &headersRoundTripper{headers: cp, delegate: delegate}
}

func (rt *headersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	for k, v := range rt.headers {
		r.Header.Set(k, v)
	}
	return rt.delegate.RoundTrip(r)
}
