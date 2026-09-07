package service

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/includedtools"
)

// newIncludedToolsChatService builds a ChatService over a throwaway included
// tools file seeded with one mode's list.
func newIncludedToolsChatService(t *testing.T, modeName string, tools []string) (*ChatService, *includedtools.Service) {
	t.Helper()

	svc := includedtools.NewService(t.TempDir())
	if err := svc.Set(modeName, tools); err != nil {
		t.Fatalf("seed included tools: %v", err)
	}
	return &ChatService{includedToolsSvc: svc}, svc
}

func TestUpdateIncludedToolsHandler(t *testing.T) {
	const seedMode = "execute"
	seed := []string{"github_create_pull_request", "github_get_file", "k8s_get_pod", "k8s_delete_pod"}

	tests := []struct {
		name         string
		args         map[string]any
		wantContains []string
		wantRemain   []string
	}{
		{
			name: "removes the named tools",
			args: map[string]any{
				"mode":  seedMode,
				"tools": []any{"github_create_pull_request", "k8s_delete_pod"},
			},
			wantContains: []string{"Mode execute: removed 2 tool(s), 2 remain included."},
			wantRemain:   []string{"github_get_file", "k8s_get_pod"},
		},
		{
			name: "accepts tools as separated text",
			args: map[string]any{
				"mode":  seedMode,
				"tools": "github_create_pull_request, k8s_delete_pod\nk8s_get_pod",
			},
			wantContains: []string{"removed 3 tool(s), 1 remain included."},
			wantRemain:   []string{"github_get_file"},
		},
		{
			name: "counts a name that is not in the list without failing",
			args: map[string]any{
				"mode":  seedMode,
				"tools": []any{"k8s_get_pod", "nope_delete_everything"},
			},
			wantContains: []string{
				"removed 1 tool(s), 3 remain included.",
				"1 name(s) were not in the list: nope_delete_everything.",
			},
			wantRemain: []string{"github_create_pull_request", "github_get_file", "k8s_delete_pod"},
		},
		{
			name: "counts a repeated name once",
			args: map[string]any{
				"mode":  seedMode,
				"tools": []any{"k8s_get_pod", "k8s_get_pod"},
			},
			wantContains: []string{"removed 1 tool(s), 3 remain included."},
			wantRemain:   []string{"github_create_pull_request", "github_get_file", "k8s_delete_pod"},
		},
		{
			name:         "rejects a missing mode",
			args:         map[string]any{"tools": []any{"k8s_get_pod"}},
			wantContains: []string{"mode is required"},
			wantRemain:   seed,
		},
		{
			name:         "rejects an unknown mode",
			args:         map[string]any{"mode": "audit", "tools": []any{"k8s_get_pod"}},
			wantContains: []string{`unknown mode "audit" (valid: main, decompose, plan, execute, discuss, incident)`},
			wantRemain:   seed,
		},
		{
			name:         "rejects missing tools",
			args:         map[string]any{"mode": seedMode},
			wantContains: []string{"tools is required"},
			wantRemain:   seed,
		},
		{
			name:         "rejects empty tools",
			args:         map[string]any{"mode": seedMode, "tools": []any{"   "}},
			wantContains: []string{"at least one non-empty tool name"},
			wantRemain:   seed,
		},
		{
			name:         "a list of unknown names leaves the file alone",
			args:         map[string]any{"mode": seedMode, "tools": []any{"nope_one", "nope_two"}},
			wantContains: []string{"removed 0 tool(s), 4 remain included."},
			wantRemain:   seed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store := newIncludedToolsChatService(t, seedMode, seed)

			out, err := svc.updateIncludedToolsHandler()(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(out, want) {
					t.Fatalf("output %q does not contain %q", out, want)
				}
			}

			got, err := store.Get(seedMode)
			if err != nil {
				t.Fatalf("read back included tools: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(sortedCopy(tt.wantRemain), ",") {
				t.Fatalf("included tools %v, want %v", got, tt.wantRemain)
			}
		})
	}
}

// TestUpdateIncludedToolsHandler_OtherModesUntouched guards the blast radius: a
// call names one mode, and the shared mcp-included.json holds every other.
func TestUpdateIncludedToolsHandler_OtherModesUntouched(t *testing.T) {
	svc, store := newIncludedToolsChatService(t, "execute", []string{"k8s_get_pod", "k8s_delete_pod"})
	if err := store.Set("discuss", []string{"k8s_get_pod", "k8s_delete_pod"}); err != nil {
		t.Fatalf("seed discuss: %v", err)
	}

	if _, err := svc.updateIncludedToolsHandler()(context.Background(), map[string]any{
		"mode":  "execute",
		"tools": []any{"k8s_delete_pod"},
	}); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	discuss, err := store.Get("discuss")
	if err != nil {
		t.Fatalf("read discuss: %v", err)
	}
	if len(discuss) != 2 {
		t.Fatalf("discuss included tools %v, want both kept", discuss)
	}
}

func TestUpdateIncludedToolsHandler_NoServiceFails(t *testing.T) {
	svc := &ChatService{}

	if _, err := svc.updateIncludedToolsHandler()(context.Background(), map[string]any{
		"mode":  "execute",
		"tools": []any{"k8s_get_pod"},
	}); err == nil {
		t.Fatal("expected an error when the included tools service is missing")
	}
}

// The tool narrows the tool surface of whichever mode it names, so a plan or
// execute agent must not reach it even when that mode's allow list carries the
// name -- SeedAllowLists never takes a name back out of a list on disk.
func TestUpdateIncludedToolsTool_OnlyRegisteredForDiscuss(t *testing.T) {
	allow := map[string]struct{}{UpdateIncludedToolsToolName: {}}
	svc := &ChatService{includedToolsSvc: includedtools.NewService(t.TempDir())}

	for _, modeName := range []string{"discuss", "plan", "execute", "main", "decompose", "incident"} {
		t.Run(modeName, func(t *testing.T) {
			catalog := newToolCatalog()
			b := newToolBinding(uuid.New())
			b.mode = modeName
			svc.addLocalTools(catalog, allow, b)

			_, ok := catalog.localHandlers[UpdateIncludedToolsToolName]
			if want := modeName == "discuss"; ok != want {
				t.Fatalf("mode %q: registered=%v, want %v", modeName, ok, want)
			}
		})
	}
}

func TestEnforceModeToolLimits_WithdrawsUpdateIncludedTools(t *testing.T) {
	for _, modeName := range []string{"main", "decompose", "plan", "execute", "incident"} {
		allow := map[string]struct{}{UpdateIncludedToolsToolName: {}, "tool_search": {}}
		enforceModeToolLimits(modeName, allow)
		if _, ok := allow[UpdateIncludedToolsToolName]; ok {
			t.Fatalf("mode %q kept %s", modeName, UpdateIncludedToolsToolName)
		}
		if _, ok := allow["tool_search"]; !ok {
			t.Fatalf("mode %q lost an unrelated tool", modeName)
		}
	}

	allow := map[string]struct{}{UpdateIncludedToolsToolName: {}}
	enforceModeToolLimits("discuss", allow)
	if _, ok := allow[UpdateIncludedToolsToolName]; !ok {
		t.Fatalf("discuss lost %s", UpdateIncludedToolsToolName)
	}
}

// TestSystemToolDefs_UpdateIncludedToolsIsDiscussOnly is the in-process form of
// the developer catalog check: with the real allow lists seeded out of the
// binary, the tool must report discuss and nothing else.
func TestSystemToolDefs_UpdateIncludedToolsIsDiscussOnly(t *testing.T) {
	svc := &ChatService{
		includedToolsSvc: includedtools.NewService(t.TempDir()),
		allowToolsDir:    systemToolsTestDataDir(t),
	}

	defs, err := svc.SystemToolDefs()
	if err != nil {
		t.Fatalf("SystemToolDefs() err = %v", err)
	}

	for _, def := range defs {
		if def.Name != UpdateIncludedToolsToolName {
			continue
		}
		if strings.Join(def.Modes, ",") != "discuss" {
			t.Fatalf("modes = %v, want [discuss]", def.Modes)
		}
		return
	}
	t.Fatalf("%s is missing from the developer catalog", UpdateIncludedToolsToolName)
}

func sortedCopy(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}
