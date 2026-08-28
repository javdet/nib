package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCreateDAGToolDef(t *testing.T) {
	t.Parallel()
	def := CreateDAGToolDef()
	if def.Name != CreateDAGToolName {
		t.Fatalf("name = %q, want %q", def.Name, CreateDAGToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "mermaid" {
		t.Fatalf("required = %#v, want [mermaid]", schema.Required)
	}
}

func TestFormatDAGMarkdown(t *testing.T) {
	t.Parallel()
	raw := "flowchart TD\na --> b"
	want := "```mermaid\nflowchart TD\na --> b\n```"
	if got := formatDAGMarkdown(raw); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	fenced := "```mermaid\nflowchart TD\na --> b\n```"
	if got := formatDAGMarkdown(fenced); got != want {
		t.Fatalf("fenced: got %q, want %q", got, want)
	}
}

func TestCreateDAGHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dialogID := uuid.New()
	dir := t.TempDir()

	svc := &ChatService{dagsDir: dir}
	handler := svc.createDAGHandler(dialogID)

	t.Run("writes fenced markdown", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{
			"mermaid": "flowchart TD\nstepA[Step A] --> stepB[Step B]",
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantRel := filepath.Join("dags", dialogID.String()+".md")
		if out != "DAG saved to "+wantRel {
			t.Fatalf("out = %q", out)
		}

		data, err := os.ReadFile(filepath.Join(dir, dialogID.String()+".md"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		content := string(data)
		if !strings.HasPrefix(content, "```mermaid\n") {
			t.Fatalf("content missing mermaid fence: %q", content)
		}
		if !strings.HasSuffix(strings.TrimSpace(content), "```") {
			t.Fatalf("content missing closing fence: %q", content)
		}
		if !strings.Contains(content, "stepA[Step A] --> stepB[Step B]") {
			t.Fatalf("content = %q", content)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		first, err := handler(ctx, map[string]any{
			"mermaid": "flowchart TD\nold --> end",
		})
		if err != nil {
			t.Fatalf("first call: %v", err)
		}
		if first == "" {
			t.Fatal("expected non-empty output")
		}

		out, err := handler(ctx, map[string]any{
			"mermaid": "flowchart TD\nnew --> end",
		})
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
		if out != "DAG saved to "+filepath.Join("dags", dialogID.String()+".md") {
			t.Fatalf("out = %q", out)
		}

		data, err := os.ReadFile(filepath.Join(dir, dialogID.String()+".md"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if strings.Contains(string(data), "old --> end") {
			t.Fatalf("file was not overwritten: %q", string(data))
		}
		if !strings.Contains(string(data), "new --> end") {
			t.Fatalf("file missing new content: %q", string(data))
		}
	})

	t.Run("missing mermaid", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "mermaid is required" {
			t.Fatalf("out = %q", out)
		}
	})
}

func TestAddLocalTools_includesCreateDAGForDialog(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{CreateDAGToolName: {}}

	svc.addLocalTools(&catalog, allow, uuid.New())

	if _, ok := catalog.localHandlers[CreateDAGToolName]; !ok {
		t.Fatal("expected create_dag handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == CreateDAGToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected create_dag in tool defs")
	}
}

func TestAddLocalTools_excludesCreateDAGWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, uuid.New())

	if _, ok := catalog.localHandlers[CreateDAGToolName]; ok {
		t.Fatal("did not expect create_dag handler when not in allow list")
	}
}
