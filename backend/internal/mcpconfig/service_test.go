package mcpconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/repository"
)

func seededService(t *testing.T, content string) *Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("seed mcp.json: %v", err)
	}
	return NewServiceAtPath(path)
}

func readFile(t *testing.T, svc *Service) string {
	t.Helper()
	data, err := os.ReadFile(svc.Path())
	if err != nil {
		t.Fatalf("read mcp.json: %v", err)
	}
	return string(data)
}

// A save must keep everything the user wrote, including the parts this package
// does not model — that silent rewrite is what made an edit look like a no-op.
func TestSetRawStoresContentVerbatim(t *testing.T) {
	svc := seededService(t, "{\n  \"mcpServers\": {}\n}\n")

	content := `{
  // gateway is fronted by the shared token
  "mcpServers": {
    "mcp-gw": {
      "type": "http",
      "url": "https://gw.example.com/mcp",
      "headers": { "Authorization": "Bearer ${MCP_GW_TOKEN}" }
    }
  },
  "inputs": []
}
`
	if err := svc.SetRaw(content); err != nil {
		t.Fatalf("SetRaw: %v", err)
	}

	if got := readFile(t, svc); got != content {
		t.Errorf("file content = %q, want %q", got, content)
	}
	got, err := svc.GetRaw()
	if err != nil {
		t.Fatalf("GetRaw: %v", err)
	}
	if got != content {
		t.Errorf("GetRaw = %q, want %q", got, content)
	}
}

// A ${NAME} reference is resolved when the server is contacted, so an unknown
// secret must not stand in the way of saving.
func TestSetRawAcceptsUnresolvedSecretReference(t *testing.T) {
	svc := seededService(t, "{\n  \"mcpServers\": {}\n}\n")

	content := `{"mcpServers": {"gw": {"url": "https://gw.example.com/mcp", "headers": {"Authorization": "Bearer ${MCP_GW_TOKEN}"}}}}`
	if err := svc.SetRaw(content); err != nil {
		t.Fatalf("SetRaw: %v", err)
	}

	servers, err := svc.ListServers()
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 1 || servers[0].Headers["Authorization"] != "Bearer ${MCP_GW_TOKEN}" {
		t.Fatalf("servers = %+v, want the reference stored unexpanded", servers)
	}
}

func TestSetRawAppendsMissingTrailingNewline(t *testing.T) {
	svc := seededService(t, "{\n  \"mcpServers\": {}\n}\n")

	if err := svc.SetRaw(`{"mcpServers": {}}`); err != nil {
		t.Fatalf("SetRaw: %v", err)
	}
	if got := readFile(t, svc); got != "{\"mcpServers\": {}}\n" {
		t.Errorf("file content = %q, want a trailing newline", got)
	}
}

func TestSetRawRejectsBrokenContentWithoutWriting(t *testing.T) {
	original := "{\n  \"mcpServers\": {\"kb\": {\"url\": \"http://kb:8081/mcp\"}}\n}\n"

	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{"invalid json", `{"mcpServers": {`, ErrInvalidJSON},
		{"entry is not an object", `{"mcpServers": {"gw": "https://gw.example.com/mcp"}}`, ErrInvalidJSON},
		{"invalid server name", `{"mcpServers": {"../gw": {"url": "https://gw.example.com/mcp"}}}`, ErrInvalidName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := seededService(t, original)
			err := svc.SetRaw(tt.content)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("SetRaw error = %v, want %v", err, tt.wantErr)
			}
			if got := readFile(t, svc); got != original {
				t.Errorf("file content = %q, want it left untouched", got)
			}
		})
	}
}

// Editing one server through the dialog must not strip fields another MCP
// client wrote, nor other top-level keys.
func TestStructuredEditsPreserveUnmodelledContent(t *testing.T) {
	original := `{
  "mcpServers": {
    "gw": {
      "type": "http",
      "disabled": false,
      "url": "https://gw.example.com/mcp"
    }
  },
  "inputs": [{"id": "token"}]
}
`
	t.Run("update", func(t *testing.T) {
		svc := seededService(t, original)
		if err := svc.UpdateServer("gw", "gw", ServerEntry{
			URL:     "https://gw.example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer ${MCP_GW_TOKEN}"},
		}); err != nil {
			t.Fatalf("UpdateServer: %v", err)
		}

		got := readFile(t, svc)
		for _, want := range []string{`"type": "http"`, `"disabled": false`, `"inputs"`, `"Bearer ${MCP_GW_TOKEN}"`} {
			if !strings.Contains(got, want) {
				t.Errorf("file content %s\nmissing %s", got, want)
			}
		}
	})

	t.Run("rename", func(t *testing.T) {
		svc := seededService(t, original)
		if err := svc.UpdateServer("gw", "gateway", ServerEntry{URL: "https://gw.example.com/mcp"}); err != nil {
			t.Fatalf("UpdateServer: %v", err)
		}

		got := readFile(t, svc)
		if strings.Contains(got, `"gw"`) {
			t.Errorf("file content %s\nstill holds the old name", got)
		}
		if !strings.Contains(got, `"type": "http"`) {
			t.Errorf("file content %s\nlost the renamed entry's extra fields", got)
		}
	})

	t.Run("add and delete", func(t *testing.T) {
		svc := seededService(t, original)
		if err := svc.AddServer("kb", ServerEntry{URL: "http://kb:8081/mcp"}); err != nil {
			t.Fatalf("AddServer: %v", err)
		}
		if err := svc.DeleteServer("kb"); err != nil {
			t.Fatalf("DeleteServer: %v", err)
		}

		got := readFile(t, svc)
		for _, want := range []string{`"type": "http"`, `"disabled": false`, `"inputs"`} {
			if !strings.Contains(got, want) {
				t.Errorf("file content %s\nmissing %s", got, want)
			}
		}
		if strings.Contains(got, `"kb"`) {
			t.Errorf("file content %s\nstill holds the deleted server", got)
		}
	})

	t.Run("clearing a field removes it", func(t *testing.T) {
		svc := seededService(t, original)
		if err := svc.UpdateServer("gw", "gw", ServerEntry{Command: "npx", Args: []string{"-y", "gw"}}); err != nil {
			t.Fatalf("UpdateServer: %v", err)
		}

		server, err := svc.GetServer("gw")
		if err != nil {
			t.Fatalf("GetServer: %v", err)
		}
		if server.URL != "" {
			t.Errorf("URL = %q, want it cleared", server.URL)
		}
	})

	t.Run("missing server", func(t *testing.T) {
		svc := seededService(t, original)
		if err := svc.DeleteServer("nope"); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("DeleteServer error = %v, want ErrNotFound", err)
		}
	})
}
