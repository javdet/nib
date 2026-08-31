package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestCreateTableToolDef(t *testing.T) {
	t.Parallel()
	def := CreateTableToolDef()
	if def.Name != CreateTableToolName {
		t.Fatalf("name = %q, want %q", def.Name, CreateTableToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 2 || schema.Required[0] != "columns" || schema.Required[1] != "rows" {
		t.Fatalf("required = %#v, want [columns rows]", schema.Required)
	}
}

func TestCreateTableHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := &ChatService{}
	handler := svc.createTableHandler(uuid.New())

	t.Run("valid table", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{
			"title":   "Jira board",
			"columns": []any{"To Do", "In Progress", "Done"},
			"rows": []any{
				[]any{"Task A", "", ""},
				[]any{"", "Task B", ""},
			},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "Table rendered to the user." {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("pads short rows", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{
			"columns": []any{"A", "B"},
			"rows":    []any{[]any{"only one"}},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "Table rendered to the user." {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("truncates long rows", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{
			"columns": []any{"A"},
			"rows":    []any{[]any{"x", "y", "z"}},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "Table rendered to the user." {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("empty columns", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{
			"columns": []any{},
			"rows":    []any{},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "columns must not be empty" {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("missing columns", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{
			"rows": []any{},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "columns is required" {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("missing rows", func(t *testing.T) {
		out, err := handler(ctx, map[string]any{
			"columns": []any{"A"},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "rows is required" {
			t.Fatalf("out = %q", out)
		}
	})
}

func TestParseCreateTableArgs(t *testing.T) {
	t.Parallel()

	t.Run("normalizes row width", func(t *testing.T) {
		data, err := parseCreateTableArgs(map[string]any{
			"columns": []any{"A", "B", "C"},
			"rows": []any{
				[]any{"1"},
				[]any{"2", "3", "4", "5"},
			},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(data.Rows) != 2 {
			t.Fatalf("rows len = %d, want 2", len(data.Rows))
		}
		if len(data.Rows[0]) != 3 || data.Rows[0][0] != "1" || data.Rows[0][1] != "" {
			t.Fatalf("row[0] = %#v", data.Rows[0])
		}
		if len(data.Rows[1]) != 3 || data.Rows[1][2] != "4" {
			t.Fatalf("row[1] = %#v", data.Rows[1])
		}
	})
}

func TestAddLocalTools_includesCreateTableForDialog(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{CreateTableToolName: {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[CreateTableToolName]; !ok {
		t.Fatal("expected create_table handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == CreateTableToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected create_table in tool defs")
	}
}

func TestAddLocalTools_excludesCreateTableWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, newToolBinding(uuid.New()))

	if _, ok := catalog.localHandlers[CreateTableToolName]; ok {
		t.Fatal("did not expect create_table handler when not in allow list")
	}
}
