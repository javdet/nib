package mcpconfig

import (
	"encoding/json"
	"testing"
)

func TestStripJSONComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "line comment",
			input: `{
  // "url": "https://old.example.com/mcp",
  "url": "https://new.example.com/mcp"
}`,
			want: `{
  
  "url": "https://new.example.com/mcp"
}`,
		},
		{
			name: "block comment",
			input: `{
  /* disabled server */
  "url": "https://example.com/mcp"
}`,
			want: `{
  
  "url": "https://example.com/mcp"
}`,
		},
		{
			name: "url with slashes preserved",
			input: `{"url": "https://mcp.atlassian.com/v1/mcp"}`,
			want: `{"url": "https://mcp.atlassian.com/v1/mcp"}`,
		},
		{
			name: "slashes inside string preserved",
			input: `{"note": "see // not a comment /* also not */"}`,
			want: `{"note": "see // not a comment /* also not */"}`,
		},
		{
			name: "escaped quote inside string",
			input: `{"note": "value with \" quote"}`,
			want: `{"note": "value with \" quote"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := stripJSONComments(tt.input)
			if got != tt.want {
				t.Fatalf("stripJSONComments() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestParseDocumentWithComments(t *testing.T) {
	t.Parallel()

	content := `{
  "mcpServers": {
    "atlassian": {
      // "url": "https://old.example.com/atlassian/mcp",
      "url": "https://mcp.atlassian.com/v1/mcp",
      "transport": "http"
    }
  }
}`

	doc, err := parseDocument(content)
	if err != nil {
		t.Fatalf("parseDocument() error = %v", err)
	}

	entry, ok := doc.MCPServers["atlassian"]
	if !ok {
		t.Fatal("expected atlassian server entry")
	}
	if entry.URL != "https://mcp.atlassian.com/v1/mcp" {
		t.Fatalf("URL = %q, want %q", entry.URL, "https://mcp.atlassian.com/v1/mcp")
	}
	if entry.Transport != "http" {
		t.Fatalf("Transport = %q, want %q", entry.Transport, "http")
	}
}

func TestStripJSONCommentsProducesValidJSON(t *testing.T) {
	t.Parallel()

	input := `{
  // top-level comment
  "mcpServers": {
    "github": {
      "url": "https://example.com//path",
      /* block */
      "transport": "http"
    }
  }
}`

	stripped := stripJSONComments(input)
	var doc Document
	if err := json.Unmarshal([]byte(stripped), &doc); err != nil {
		t.Fatalf("json.Unmarshal after strip failed: %v\nstripped:\n%s", err, stripped)
	}
}
