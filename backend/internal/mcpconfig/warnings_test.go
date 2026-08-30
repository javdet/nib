package mcpconfig

import (
	"reflect"
	"testing"
)

func TestWarnings(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "clean document",
			content: `{"mcpServers": {"gw": {"url": "https://gw.example.com/mcp"}}}`,
		},
		{
			name: "entry pasted next to mcpServers",
			content: `{
  "mcpServers": {"knowledge-base": {"url": "http://kb:8081/mcp"}},
  "tool-gw": {"type": "http", "url": "https://gw.example.com/mcp"}
}`,
			want: []string{`"tool-gw" looks like an MCP server but sits outside "mcpServers", so nothing reads it. Move it inside "mcpServers" to use it.`},
		},
		{
			name: "several entries keep file order",
			content: `{
  "mcpServers": {},
  "tool-gw": {"url": "https://gw.example.com/mcp"},
  "github": {"headers": {"Authorization": "Bearer ${GITHUB_TOKEN}"}}
}`,
			want: []string{`"tool-gw" and "github" look like MCP servers but sit outside "mcpServers", so nothing reads them. Move them inside "mcpServers" to use them.`},
		},
		{
			name:    "unrelated top-level keys are left alone",
			content: `{"mcpServers": {}, "inputs": [], "$schema": "https://example.com/schema.json"}`,
		},
		{
			name:    "no mcpServers at all",
			content: `{"inputs": []}`,
			want:    []string{`No "mcpServers" object, so no MCP servers are configured.`},
		},
		{
			name:    "invalid json has nothing to report",
			content: `{"mcpServers":`,
		},
		{
			name: "comments do not hide a stray entry",
			content: `{
  // pasted from another editor
  "mcpServers": {},
  "gw": {"url": "https://gw.example.com/mcp"}
}`,
			want: []string{`"gw" looks like an MCP server but sits outside "mcpServers", so nothing reads it. Move it inside "mcpServers" to use it.`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Warnings(tt.content)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Warnings() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
