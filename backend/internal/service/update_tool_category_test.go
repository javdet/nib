package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/toolcatalog"
)

func TestUpdateToolCategoryHandler(t *testing.T) {
	tests := []struct {
		name         string
		categories   []toolcatalog.CategoryWithPatterns
		args         map[string]any
		wantContains string
		wantName     string
		wantPatterns []string
		wantNoWrite  bool
	}{
		{
			name:       "adds to existing patterns by default",
			categories: []toolcatalog.CategoryWithPatterns{{Name: "monitoring", Patterns: []string{"grafana_*"}}},
			args: map[string]any{
				"category": "monitoring",
				"patterns": []any{"prometheus_query", "grafana_*"},
			},
			wantContains: "Category monitoring updated (add)",
			wantName:     "monitoring",
			wantPatterns: []string{"grafana_*", "prometheus_query"},
		},
		{
			name:       "replace makes the given patterns the whole set",
			categories: []toolcatalog.CategoryWithPatterns{{Name: "logging", Patterns: []string{"loki_*", "elastic_*"}}},
			args: map[string]any{
				"category": "logging",
				"patterns": []any{"loki_*"},
				"action":   "replace",
			},
			wantContains: "Category logging updated (replace)",
			wantName:     "logging",
			wantPatterns: []string{"loki_*"},
		},
		{
			name:       "remove drops the given patterns",
			categories: []toolcatalog.CategoryWithPatterns{{Name: "logging", Patterns: []string{"loki_*", "elastic_*"}}},
			args: map[string]any{
				"category": "logging",
				"patterns": []any{"elastic_*"},
				"action":   "remove",
			},
			wantContains: "Category logging updated (remove)",
			wantName:     "logging",
			wantPatterns: []string{"loki_*"},
		},
		{
			name:       "remove of every pattern empties the category",
			categories: []toolcatalog.CategoryWithPatterns{{Name: "logging", Patterns: []string{"loki_*"}}},
			args: map[string]any{
				"category": "logging",
				"patterns": []any{"loki_*"},
				"action":   "remove",
			},
			wantContains: "Category logging now has no patterns",
			wantName:     "logging",
			wantPatterns: []string{},
		},
		{
			name:       "accepts patterns as separated text",
			categories: []toolcatalog.CategoryWithPatterns{{Name: "kubernetes"}},
			args: map[string]any{
				"category": "kubernetes",
				"patterns": "kubectl_*, helm_*\nargocd_get_application",
			},
			wantContains: "Category kubernetes updated (add)",
			wantName:     "kubernetes",
			wantPatterns: []string{"kubectl_*", "helm_*", "argocd_get_application"},
		},
		{
			name:       "matches the stored category name case-insensitively",
			categories: []toolcatalog.CategoryWithPatterns{{Name: "monitoring"}},
			args: map[string]any{
				"category": "Monitoring",
				"patterns": []any{"grafana_*"},
			},
			wantContains: "Category monitoring updated",
			wantName:     "monitoring",
			wantPatterns: []string{"grafana_*"},
		},
		{
			name:         "rejects an unknown category",
			categories:   []toolcatalog.CategoryWithPatterns{{Name: "monitoring"}},
			args:         map[string]any{"category": "networking", "patterns": []any{"cilium_*"}},
			wantContains: `unknown category "networking" (valid: monitoring)`,
			wantNoWrite:  true,
		},
		{
			name:         "rejects a missing category",
			categories:   []toolcatalog.CategoryWithPatterns{{Name: "monitoring"}},
			args:         map[string]any{"patterns": []any{"grafana_*"}},
			wantContains: "category is required",
			wantNoWrite:  true,
		},
		{
			name:         "rejects missing patterns",
			categories:   []toolcatalog.CategoryWithPatterns{{Name: "monitoring"}},
			args:         map[string]any{"category": "monitoring"},
			wantContains: "patterns is required",
			wantNoWrite:  true,
		},
		{
			name:         "rejects empty patterns",
			categories:   []toolcatalog.CategoryWithPatterns{{Name: "monitoring"}},
			args:         map[string]any{"category": "monitoring", "patterns": []any{"   "}},
			wantContains: "at least one non-empty pattern",
			wantNoWrite:  true,
		},
		{
			name:         "rejects a wildcard that is not trailing",
			categories:   []toolcatalog.CategoryWithPatterns{{Name: "monitoring"}},
			args:         map[string]any{"category": "monitoring", "patterns": []any{"*_query"}},
			wantContains: "wildcard must be a trailing *",
			wantNoWrite:  true,
		},
		{
			name:         "rejects an unknown action",
			categories:   []toolcatalog.CategoryWithPatterns{{Name: "monitoring"}},
			args:         map[string]any{"category": "monitoring", "patterns": []any{"grafana_*"}, "action": "delete"},
			wantContains: `unknown action "delete"`,
			wantNoWrite:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &stubToolCategoryLister{categories: tt.categories}
			svc := &ChatService{toolCategorySvc: lister}

			out, err := svc.updateToolCategoryHandler()(context.Background(), tt.args)
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if !strings.Contains(out, tt.wantContains) {
				t.Fatalf("output %q does not contain %q", out, tt.wantContains)
			}

			if tt.wantNoWrite {
				if lister.setName != "" {
					t.Fatalf("expected no write, got patterns for %q: %v", lister.setName, lister.setPatterns)
				}
				return
			}
			if lister.setName != tt.wantName {
				t.Fatalf("wrote category %q, want %q", lister.setName, tt.wantName)
			}
			if len(lister.setPatterns) != len(tt.wantPatterns) {
				t.Fatalf("wrote patterns %v, want %v", lister.setPatterns, tt.wantPatterns)
			}
			for i, want := range tt.wantPatterns {
				if lister.setPatterns[i] != want {
					t.Fatalf("wrote patterns %v, want %v", lister.setPatterns, tt.wantPatterns)
				}
			}
		})
	}
}

func TestUpdateToolCategoryHandler_ListErrorFails(t *testing.T) {
	lister := &stubToolCategoryLister{listErr: errors.New("boom")}
	svc := &ChatService{toolCategorySvc: lister}

	if _, err := svc.updateToolCategoryHandler()(context.Background(), map[string]any{
		"category": "monitoring",
		"patterns": []any{"grafana_*"},
	}); err == nil {
		t.Fatal("expected an error when the category list cannot be read")
	}
}

func TestUpdateToolCategoryHandler_WriteErrorFails(t *testing.T) {
	lister := &stubToolCategoryLister{
		categories: []toolcatalog.CategoryWithPatterns{{Name: "monitoring"}},
		setErr:     errors.New("boom"),
	}
	svc := &ChatService{toolCategorySvc: lister}

	if _, err := svc.updateToolCategoryHandler()(context.Background(), map[string]any{
		"category": "monitoring",
		"patterns": []any{"grafana_*"},
	}); err == nil {
		t.Fatal("expected an error when the patterns cannot be written")
	}
}

// The tool edits the categories every plan inherits, so it must not reach a
// mode other than discuss even when that mode's allow list carries the name.
func TestUpdateToolCategoryTool_OnlyRegisteredForDiscuss(t *testing.T) {
	allow := map[string]struct{}{UpdateToolCategoryToolName: {}}
	svc := &ChatService{toolCategorySvc: &stubToolCategoryLister{}}

	for _, modeName := range []string{"discuss", "plan", "execute", "main", "decompose", "incident"} {
		t.Run(modeName, func(t *testing.T) {
			catalog := newToolCatalog()
			b := newToolBinding(uuid.New())
			b.mode = modeName
			svc.addLocalTools(catalog, allow, b)

			_, ok := catalog.localHandlers[UpdateToolCategoryToolName]
			if want := modeName == "discuss"; ok != want {
				t.Fatalf("mode %q: registered=%v, want %v", modeName, ok, want)
			}
		})
	}
}

func TestEnforceModeToolLimits_WithdrawsFromOtherModes(t *testing.T) {
	for _, modeName := range []string{"main", "decompose", "plan", "execute", "incident"} {
		allow := map[string]struct{}{UpdateToolCategoryToolName: {}, "tool_search": {}}
		enforceModeToolLimits(modeName, allow)
		if _, ok := allow[UpdateToolCategoryToolName]; ok {
			t.Fatalf("mode %q kept %s", modeName, UpdateToolCategoryToolName)
		}
		if _, ok := allow["tool_search"]; !ok {
			t.Fatalf("mode %q lost an unrelated tool", modeName)
		}
	}

	allow := map[string]struct{}{UpdateToolCategoryToolName: {}}
	enforceModeToolLimits("discuss", allow)
	if _, ok := allow[UpdateToolCategoryToolName]; !ok {
		t.Fatalf("discuss lost %s", UpdateToolCategoryToolName)
	}
}
