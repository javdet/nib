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

func TestDAGStageTitles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		markdown string
		want     []string
	}{
		{
			name:     "quoted labels on one edge",
			markdown: "```mermaid\nflowchart TD\n  jmx[\"Configure Cassandra JMX\"] --> reaper[\"Setup and connect Cassandra Reaper\"]\n```",
			want:     []string{"Configure Cassandra JMX", "Setup and connect Cassandra Reaper"},
		},
		{
			name:     "unquoted labels",
			markdown: "flowchart TD\n  a[Step A] --> b[Step B]",
			want:     []string{"Step A", "Step B"},
		},
		{
			name:     "parentheses inside a quoted label",
			markdown: "flowchart TD\n  one[\"Deploy Centrifugo (prod)\"] --> two{\"Traffic healthy?\"}",
			want:     []string{"Deploy Centrifugo (prod)", "Traffic healthy?"},
		},
		{
			name:     "edge labels and repeated nodes are not stages",
			markdown: "flowchart TD\n  a[\"Build\"] -->|ok| b[\"Ship\"]\n  b[\"Ship\"] --> c\n  %% a[\"Commented\"]",
			want:     []string{"Build", "Ship"},
		},
		{
			name:     "declaration order is kept across lines",
			markdown: "flowchart TD\n  a([\"Round\"])\n  b[[\"Subroutine\"]]\n  c{{\"Hexagon\"}}\n  a --> b --> c",
			want:     []string{"Round", "Subroutine", "Hexagon"},
		},
		{
			name:     "diagram without labels has no stages",
			markdown: "flowchart TD\n  a --> b",
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := dagStageTitles(tt.markdown)
			if len(got) != len(tt.want) {
				t.Fatalf("titles = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("titles = %#v, want %#v", got, tt.want)
				}
			}
		})
	}
}

func TestMatchDAGStage(t *testing.T) {
	t.Parallel()

	titles := []string{"Configure Cassandra JMX", "Setup and connect Cassandra Reaper"}

	t.Run("returns the DAG spelling for a fuzzy match", func(t *testing.T) {
		t.Parallel()
		got, ok := matchDAGStage(titles, "  configure   cassandra jmx ")
		if !ok {
			t.Fatal("expected a match")
		}
		if got != "Configure Cassandra JMX" {
			t.Fatalf("title = %q, want %q", got, "Configure Cassandra JMX")
		}
	})

	t.Run("rejects a stage the DAG does not name", func(t *testing.T) {
		t.Parallel()
		if _, ok := matchDAGStage(titles, "Install Prometheus"); ok {
			t.Fatal("expected no match")
		}
	})

	t.Run("rejects a blank name", func(t *testing.T) {
		t.Parallel()
		if _, ok := matchDAGStage(titles, "   "); ok {
			t.Fatal("expected no match")
		}
	})
}
