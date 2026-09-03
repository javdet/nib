package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The schema is generated from the registry, so the two cannot disagree about
// which sub-agents exist or which arguments each one reads.
func TestRunSubagentToolDefMatchesTheRegistry(t *testing.T) {
	t.Parallel()

	def := RunSubagentToolDef()
	if def.Name != RunSubagentToolName {
		t.Fatalf("def.Name = %q, want %q", def.Name, RunSubagentToolName)
	}

	var schema struct {
		Properties map[string]struct {
			Enum        []string `json:"enum"`
			Description string   `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("the generated schema is not valid JSON: %v", err)
	}

	name, ok := schema.Properties["name"]
	if !ok {
		t.Fatal("the schema has no name discriminator")
	}
	if strings.Join(name.Enum, ",") != strings.Join(subagentNames(), ",") {
		t.Errorf("name enum = %v, want %v", name.Enum, subagentNames())
	}
	if len(schema.Required) != 1 || schema.Required[0] != "name" {
		t.Errorf("required = %v, want only the discriminator", schema.Required)
	}

	// The model picks a sub-agent from this description alone, so every entry's
	// purpose and arguments have to be in it.
	for _, spec := range subagentSpecs {
		if !strings.Contains(name.Description, string(spec.Name)) {
			t.Errorf("the name description never mentions %q", spec.Name)
		}
		if !strings.Contains(name.Description, spec.Description) {
			t.Errorf("the name description omits %q's own description", spec.Name)
		}
		for _, param := range spec.Params {
			if _, ok := schema.Properties[param]; !ok {
				t.Errorf("%q reads %q, which the schema does not declare", spec.Name, param)
			}
			if !strings.Contains(name.Description, param) {
				t.Errorf("the name description does not say %q reads %q", spec.Name, param)
			}
		}
	}
}

// A missing or misspelled argument costs a sentence, never the turn: the handler
// answers with prose and a nil error the way every other tool here does.
func TestRunSubagentHandlerRejectsBadArgumentsAsToolOutput(t *testing.T) {
	t.Parallel()

	svc := &ChatService{}
	handler := svc.runSubagentHandler(newToolBinding(uuid.New()))

	for name, tc := range map[string]struct {
		args map[string]any
		want string
	}{
		"no name":            {map[string]any{}, "is not one of your sub-agents"},
		"unknown name":       {map[string]any{"name": "refactor"}, "is not one of your sub-agents"},
		"decompose, no task": {map[string]any{"name": "decompose"}, `needs "task"`},
		"execute, no item":   {map[string]any{"name": "execute"}, `needs "item"`},
	} {
		out, err := handler(t.Context(), tc.args)
		if err != nil {
			t.Errorf("%s: err = %v, want the mistake reported as output", name, err)
			continue
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: out = %q, want it to contain %q", name, out, tc.want)
		}
	}
}

// The binding is the recursion guard: a sub-agent runs with its transcript and
// its plan on different dialogs, so only a root turn is ever offered the tools
// that launch and stop sub-agents.
func TestOrchestratorToolsAreOfferedOnlyToARootTurn(t *testing.T) {
	t.Parallel()

	svc := &ChatService{dialogRepo: &actionListDialogRepo{}}
	allow := map[string]struct{}{
		RunSubagentToolName:   {},
		StopExecutionToolName: {},
	}

	root := uuid.New()
	catalog := newToolCatalog()
	svc.addLocalTools(catalog, allow, newToolBinding(root))
	for _, want := range []string{RunSubagentToolName, StopExecutionToolName} {
		if _, ok := catalog.localHandlers[want]; !ok {
			t.Errorf("a root turn was not offered %q", want)
		}
	}

	subagent := newToolCatalog()
	svc.addLocalTools(subagent, allow, toolBinding{dialogID: uuid.New(), planID: root})
	for _, forbidden := range []string{RunSubagentToolName, StopExecutionToolName} {
		if _, ok := subagent.localHandlers[forbidden]; ok {
			t.Errorf("a sub-agent transcript was offered %q", forbidden)
		}
	}
}

// The allow-list strip is the second guard, and it has to be code: SeedAllowLists
// never takes a name out of a list already on a data volume, so an upgraded
// install still names execute_action in data/tools/decompose.json.
func TestStripSubagentToolsRemovesTheOrchestratorsOwn(t *testing.T) {
	t.Parallel()

	allow := map[string]struct{}{
		RunSubagentToolName:      {},
		StopExecutionToolName:    {},
		ExecuteActionToolName:    {},
		KnowledgeSearchToolName:  {},
		UpdateActionPlanToolName: {},
	}

	stripSubagentTools(allow)

	for _, gone := range []string{RunSubagentToolName, StopExecutionToolName, ExecuteActionToolName} {
		if _, ok := allow[gone]; ok {
			t.Errorf("%q survived the strip", gone)
		}
	}
	for _, kept := range []string{KnowledgeSearchToolName, UpdateActionPlanToolName} {
		if _, ok := allow[kept]; !ok {
			t.Errorf("%q was stripped and should not have been", kept)
		}
	}
}
