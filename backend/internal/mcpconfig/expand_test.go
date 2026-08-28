package mcpconfig

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func staticLookup(values map[string]string) Lookup {
	return func(_ context.Context, name string) (string, bool, error) {
		v, ok := values[name]
		return v, ok, nil
	}
}

func TestExpandString(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"MCP_GITHUB_TOKEN": "ghp_secret",
		"HOST":             "mcp.example.com",
		"my-token":         "dashed",
		"EMPTY":            "",
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no reference",
			input: "https://mcp.example.com/mcp",
			want:  "https://mcp.example.com/mcp",
		},
		{
			name:  "single reference in header",
			input: "Bearer ${MCP_GITHUB_TOKEN}",
			want:  "Bearer ghp_secret",
		},
		{
			name:  "reference is the whole value",
			input: "${MCP_GITHUB_TOKEN}",
			want:  "ghp_secret",
		},
		{
			name:  "multiple distinct references",
			input: "https://${HOST}/mcp?t=${MCP_GITHUB_TOKEN}",
			want:  "https://mcp.example.com/mcp?t=ghp_secret",
		},
		{
			name:  "repeated reference",
			input: "${HOST}/${HOST}",
			want:  "mcp.example.com/mcp.example.com",
		},
		{
			name:  "dashed name",
			input: "Bearer ${my-token}",
			want:  "Bearer dashed",
		},
		{
			name:  "empty value expands to empty",
			input: "prefix-${EMPTY}-suffix",
			want:  "prefix--suffix",
		},
		{
			name:  "escaped dollar brace stays literal",
			input: "$${MCP_GITHUB_TOKEN}",
			want:  "${MCP_GITHUB_TOKEN}",
		},
		{
			name:  "bare dollar is not expanded",
			input: "p$$w0rd$HOST",
			want:  "p$$w0rd$HOST",
		},
		{
			name:  "unterminated reference stays literal",
			input: "Bearer ${MCP_GITHUB_TOKEN",
			want:  "Bearer ${MCP_GITHUB_TOKEN",
		},
		{
			name:  "invalid name stays literal",
			input: "${not a name} and ${}",
			want:  "${not a name} and ${}",
		},
		{
			name:  "invalid name does not stop later expansion",
			input: "${bad name} ${HOST}",
			want:  "${bad name} mcp.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := expandString(context.Background(), tt.input, staticLookup(values))
			if err != nil {
				t.Fatalf("expandString(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("expandString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExpandStringMissingVariable(t *testing.T) {
	t.Parallel()

	_, err := expandString(context.Background(), "Bearer ${MCP_GITHUB_TOKEN}", staticLookup(nil))
	if !errors.Is(err, ErrUnresolvedVariable) {
		t.Fatalf("error = %v, want ErrUnresolvedVariable", err)
	}
	if want := "${MCP_GITHUB_TOKEN}"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not name %q", err.Error(), want)
	}
}

func TestExpandStringLookupError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	lookup := func(_ context.Context, _ string) (string, bool, error) {
		return "", false, sentinel
	}

	if _, err := expandString(context.Background(), "${HOST}", lookup); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
}

func TestRedactValues(t *testing.T) {
	t.Parallel()

	got := RedactValues(`Get "https://x/mcp?t=ghp_secret": 401`, []string{"ghp_secret", ""})
	want := `Get "https://x/mcp?t=***": 401`
	if got != want {
		t.Fatalf("RedactValues() = %q, want %q", got, want)
	}
}
