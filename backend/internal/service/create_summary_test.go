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

func TestCreateSummaryToolDef(t *testing.T) {
	t.Parallel()
	def := CreateSummaryToolDef()
	if def.Name != CreateSummaryToolName {
		t.Fatalf("name = %q, want %q", def.Name, CreateSummaryToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "summary" {
		t.Fatalf("required = %#v, want [summary]", schema.Required)
	}
}

func TestCreateSummaryHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dialogID := uuid.New()
	dir := t.TempDir()

	svc := &ChatService{summariesDir: dir}
	handler := svc.createSummaryHandler(dialogID)

	t.Run("writes summary text", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{
			"summary": "Deploy Envoy Gateway to both clusters.",
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		wantRel := filepath.Join("summaries", dialogID.String()+".txt")
		if out != "Summary saved to "+wantRel {
			t.Fatalf("out = %q", out)
		}

		data, err := os.ReadFile(filepath.Join(dir, dialogID.String()+".txt"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if string(data) != "Deploy Envoy Gateway to both clusters." {
			t.Fatalf("content = %q", string(data))
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		first, err := handler(ctx, map[string]any{
			"summary": "Old summary.",
		})
		if err != nil {
			t.Fatalf("first call: %v", err)
		}
		if first == "" {
			t.Fatal("expected non-empty output")
		}

		out, err := handler(ctx, map[string]any{
			"summary": "New summary.",
		})
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
		if out != "Summary saved to "+filepath.Join("summaries", dialogID.String()+".txt") {
			t.Fatalf("out = %q", out)
		}

		data, err := os.ReadFile(filepath.Join(dir, dialogID.String()+".txt"))
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if strings.Contains(string(data), "Old summary") {
			t.Fatalf("file was not overwritten: %q", string(data))
		}
		if string(data) != "New summary." {
			t.Fatalf("file missing new content: %q", string(data))
		}
	})

	t.Run("missing summary", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "summary is required" {
			t.Fatalf("out = %q", out)
		}
	})
}

func TestAddLocalTools_includesCreateSummaryForDialog(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{CreateSummaryToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[CreateSummaryToolName]; !ok {
		t.Fatal("expected create_summary handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == CreateSummaryToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected create_summary in tool defs")
	}
}

func TestAddLocalTools_excludesCreateSummaryWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[CreateSummaryToolName]; ok {
		t.Fatal("did not expect create_summary handler when not in allow list")
	}
}
