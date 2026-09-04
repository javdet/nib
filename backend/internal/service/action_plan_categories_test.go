package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/toolcatalog"
)

// seedSchemaFiles are the schemas shipped for the plan tools. The injector runs
// over whatever is on disk, so it is tested against the real documents rather
// than a hand-written fragment.
var seedSchemaFiles = []string{
	"create_action_plan.json",
	"update_action_plan.json",
	"update_rollback_plan.json",
}

func TestWithStepCategories_SeedSchemas(t *testing.T) {
	names := []string{"kubernetes", "monitoring"}

	for _, file := range seedSchemaFiles {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "..", "seed", "tools", "schemas", file))
			if err != nil {
				t.Fatalf("read seed schema: %v", err)
			}

			var doc any
			if err := json.Unmarshal(withStepCategories(json.RawMessage(raw), names), &doc); err != nil {
				t.Fatalf("unmarshal injected schema: %v", err)
			}

			steps := collectStepSchemas(doc)
			if len(steps) == 0 {
				t.Fatalf("no step-shaped schema found in %s", file)
			}
			for _, step := range steps {
				props := step["properties"].(map[string]any)
				prop, ok := props[stepCategoriesKey].(map[string]any)
				if !ok {
					t.Fatalf("categories property missing: %v", props)
				}
				item, ok := prop["items"].(map[string]any)
				if !ok {
					t.Fatalf("categories items missing: %v", prop)
				}
				enum, ok := item["enum"].([]any)
				if !ok {
					t.Fatalf("categories enum missing: %v", item)
				}
				if len(enum) != len(names) || enum[0] != names[0] {
					t.Fatalf("enum = %v, want %v", enum, names)
				}
				if got := requiredNames(step); !contains(got, stepCategoriesKey) {
					t.Fatalf("required = %v, want it to hold %q", got, stepCategoriesKey)
				}
			}
		})
	}
}

// The seed files already describe categories, so the injector must refresh the
// enum on a property that is there rather than skip it -- a static file cannot
// carry names that come from the toolCategories variable.
func TestWithStepCategories_RefreshesExistingEnum(t *testing.T) {
	in := json.RawMessage(`{
	  "type": "object",
	  "properties": {
	    "type": { "type": "string" },
	    "action": { "type": "string" },
	    "categories": {
	      "type": "array",
	      "description": "operator wording",
	      "items": { "type": "string", "enum": ["stale"] }
	    }
	  },
	  "required": ["type", "action"]
	}`)

	var doc map[string]any
	if err := json.Unmarshal(withStepCategories(in, []string{"cloud"}), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	prop := doc["properties"].(map[string]any)[stepCategoriesKey].(map[string]any)
	if got := prop["description"]; got != "operator wording" {
		t.Fatalf("description = %v, want the wording already on the property", got)
	}
	item := prop["items"].(map[string]any)
	if got := item["enum"].([]any); len(got) != 1 || got[0] != "cloud" {
		t.Fatalf("enum = %v, want [cloud]", got)
	}
	if got := requiredNames(doc); len(got) != 3 || !contains(got, stepCategoriesKey) {
		t.Fatalf("required = %v, want type, action and categories", got)
	}
}

// An empty enum would forbid every value, so a catalog that could not be read
// has to widen the field rather than close it.
func TestWithStepCategories_NoNamesDropsEnum(t *testing.T) {
	in := json.RawMessage(`{"properties":{"type":{},"action":{},"categories":{"items":{"enum":["stale"]}}}}`)

	var doc map[string]any
	if err := json.Unmarshal(withStepCategories(in, nil), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	prop := doc["properties"].(map[string]any)[stepCategoriesKey].(map[string]any)
	if _, ok := prop["items"].(map[string]any)["enum"]; ok {
		t.Fatalf("enum should be gone when no category names are known: %v", prop)
	}
}

func TestWithStepCategories_UnrecognisedDocumentUnchanged(t *testing.T) {
	tests := []struct {
		name string
		in   json.RawMessage
	}{
		{name: "no step shape", in: json.RawMessage(`{"properties":{"check":{},"expectation":{}}}`)},
		{name: "not json", in: json.RawMessage(`{"properties":`)},
		{name: "empty", in: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withStepCategories(tt.in, []string{"cloud"}); !reflect.DeepEqual([]byte(got), []byte(tt.in)) {
				t.Fatalf("got %s, want the input unchanged", got)
			}
		})
	}
}

func TestNormalizeActionPlanCategories(t *testing.T) {
	valid := []string{"kubernetes", "monitoring"}

	plan := map[string]any{
		"stages": []any{
			map[string]any{
				"steps": []any{
					// Lowercased, deduped, and the unknown name dropped.
					map[string]any{"type": "shell", "categories": []any{"Kubernetes", "kubernetes", "datadog"}},
					// An empty list loses the key entirely, matching omitempty.
					map[string]any{"type": "web", "categories": []any{}},
					// Every name unknown is the same as none.
					map[string]any{"type": "curl", "categories": []any{"pagerduty"}},
				},
				"checks": []any{map[string]any{"check": "c", "expectation": "e"}},
			},
		},
		"rollback": []any{
			map[string]any{"type": "shell", "categories": []any{"monitoring"}},
		},
	}

	unknown := normalizeActionPlanCategories(plan, valid)
	if want := []string{"datadog", "pagerduty"}; !reflect.DeepEqual(unknown, want) {
		t.Fatalf("unknown = %v, want %v", unknown, want)
	}

	steps := plan["stages"].([]any)[0].(map[string]any)["steps"].([]any)
	if got := steps[0].(map[string]any)[stepCategoriesKey]; !reflect.DeepEqual(got, []string{"kubernetes"}) {
		t.Fatalf("step categories = %v, want [kubernetes]", got)
	}
	for _, idx := range []int{1, 2} {
		if _, ok := steps[idx].(map[string]any)[stepCategoriesKey]; ok {
			t.Fatalf("step %d should have lost its empty categories key", idx)
		}
	}
	rollback := plan["rollback"].([]any)[0].(map[string]any)
	if got := rollback[stepCategoriesKey]; !reflect.DeepEqual(got, []string{"monitoring"}) {
		t.Fatalf("rollback categories = %v, want [monitoring]", got)
	}
}

// An empty valid list means the categories could not be read, not that none
// exist, so nothing may be stripped on the strength of it.
func TestNormalizeActionPlanCategories_NoValidListKeepsEverything(t *testing.T) {
	entries := []any{map[string]any{"type": "shell", "categories": []any{"Whatever"}}}

	if unknown := normalizeActionPlanCategories(entries, nil); unknown != nil {
		t.Fatalf("unknown = %v, want none", unknown)
	}
	got := entries[0].(map[string]any)[stepCategoriesKey]
	if !reflect.DeepEqual(got, []string{"whatever"}) {
		t.Fatalf("categories = %v, want [whatever]", got)
	}
}

func TestUnknownCategoryNote(t *testing.T) {
	if got := unknownCategoryNote(nil, []string{"cloud"}); got != "" {
		t.Fatalf("note = %q, want empty when nothing was dropped", got)
	}
	got := unknownCategoryNote([]string{"datadog"}, []string{"cloud", "kubernetes"})
	want := " Dropped unknown categories: datadog. Valid categories: cloud, kubernetes."
	if got != want {
		t.Fatalf("note = %q, want %q", got, want)
	}
}

// collectStepSchemas returns every step-shaped schema object in a document, the
// same shape the injector looks for.
func collectStepSchemas(node any) []map[string]any {
	var out []map[string]any
	switch n := node.(type) {
	case map[string]any:
		if isStepSchema(n) {
			out = append(out, n)
		}
		for _, child := range n {
			out = append(out, collectStepSchemas(child)...)
		}
	case []any:
		for _, child := range n {
			out = append(out, collectStepSchemas(child)...)
		}
	}
	return out
}

func requiredNames(schema map[string]any) []string {
	list, _ := schema["required"].([]any)
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func contains(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

// The plan tools are the last place a bad category can be caught: after this the
// name only surfaces as MCP tools the executor silently does not get.
func TestCreateActionPlanHandler_DropsUnknownCategories(t *testing.T) {
	planID := uuid.New()
	dir := t.TempDir()
	svc := &ChatService{
		actionPlansDir:  dir,
		activity:        NewActivityBroker(),
		toolCategorySvc: &stubToolCategoryLister{categories: []toolcatalog.CategoryWithPatterns{{Name: "kubernetes"}}},
	}

	out, err := svc.createActionPlanHandler(planID)(context.Background(), map[string]any{
		"plan": map[string]any{
			"stages": []any{map[string]any{
				"title": "Deploy",
				"steps": []any{map[string]any{
					"type":       "shell",
					"action":     "restart",
					"categories": []any{"Kubernetes", "datadog"},
				}},
				"checks": []any{},
			}},
			"rollback": []any{},
		},
	})
	if err != nil {
		t.Fatalf("createActionPlanHandler: %v", err)
	}
	if !strings.Contains(out, "Dropped unknown categories: datadog") {
		t.Fatalf("result = %q, want the dropped name in it", out)
	}
	if !strings.Contains(out, "Valid categories: kubernetes") {
		t.Fatalf("result = %q, want the valid names in it", out)
	}

	raw, found, err := svc.ReadActionPlan(planID)
	if err != nil || !found {
		t.Fatalf("ReadActionPlan: found=%v err=%v", found, err)
	}
	var stored storedActionPlan
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("unmarshal stored plan: %v", err)
	}
	got := stored.Stages[0].Steps[0].Categories
	if !reflect.DeepEqual(got, []string{"kubernetes"}) {
		t.Fatalf("stored categories = %v, want [kubernetes] lowercased and filtered", got)
	}
}

// A plan whose categories are all known says nothing about them, so the note
// never becomes noise on the happy path.
func TestCreateActionPlanHandler_NoNoteWhenAllKnown(t *testing.T) {
	planID := uuid.New()
	svc := &ChatService{
		actionPlansDir:  t.TempDir(),
		activity:        NewActivityBroker(),
		toolCategorySvc: &stubToolCategoryLister{categories: []toolcatalog.CategoryWithPatterns{{Name: "kubernetes"}}},
	}

	out, err := svc.createActionPlanHandler(planID)(context.Background(), map[string]any{
		"plan": map[string]any{
			"stages": []any{map[string]any{
				"title":  "Deploy",
				"steps":  []any{map[string]any{"type": "shell", "action": "restart", "categories": []any{"kubernetes"}}},
				"checks": []any{},
			}},
			"rollback": []any{},
		},
	})
	if err != nil {
		t.Fatalf("createActionPlanHandler: %v", err)
	}
	if strings.Contains(out, "Dropped") {
		t.Fatalf("result = %q, want no note", out)
	}
}
