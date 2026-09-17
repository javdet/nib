package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The seed files are copied to the data volume only when absent, so an install
// that predates these fields keeps a schema file without them. The stamping is
// the only route to that install, which makes this the test that matters.
func TestWithStepFields_StampsImpactOnSeedSchemas(t *testing.T) {
	for _, file := range seedSchemaFiles {
		t.Run(file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "..", "seed", "tools", "schemas", file))
			if err != nil {
				t.Fatalf("read seed schema: %v", err)
			}

			var doc any
			if err := json.Unmarshal(withStepFields(json.RawMessage(raw), []string{"kubernetes"}), &doc); err != nil {
				t.Fatalf("unmarshal injected schema: %v", err)
			}

			steps := collectStepSchemas(doc)
			if len(steps) == 0 {
				t.Fatalf("no step-shaped schema found in %s", file)
			}
			for _, step := range steps {
				props := step["properties"].(map[string]any)
				for _, key := range []string{stepDowntimeKey, stepDegradedKey} {
					prop, ok := props[key].(map[string]any)
					if !ok {
						t.Fatalf("%s property missing: %v", key, props)
					}
					if prop["type"] != "string" {
						t.Fatalf("%s type = %v, want string", key, prop["type"])
					}
					if desc, _ := prop["description"].(string); desc == "" {
						t.Fatalf("%s carries no description", key)
					}
					// A required string forces the model to send "" on every
					// action, and a label on every row is a label on none.
					if got := requiredNames(step); contains(got, key) {
						t.Fatalf("required = %v, want it to leave out %q", got, key)
					}
				}
			}
		})
	}
}

// The stamping must not overwrite an operator's own wording, the same bargain
// setStepCategoriesProperty makes. Nothing here has to follow a runtime value,
// so a property the file already describes is left exactly as it is.
func TestWithStepFields_LeavesExistingImpactDescriptionAlone(t *testing.T) {
	in := json.RawMessage(`{
	  "type": "object",
	  "properties": {
	    "type": { "type": "string" },
	    "action": { "type": "string" },
	    "downtime": { "type": "string", "description": "operator wording" }
	  },
	  "required": ["type", "action"]
	}`)

	var doc map[string]any
	if err := json.Unmarshal(withStepFields(in, []string{"cloud"}), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	props := doc["properties"].(map[string]any)
	if got := props[stepDowntimeKey].(map[string]any)["description"]; got != "operator wording" {
		t.Fatalf("description = %v, want the wording already on the property", got)
	}
	// The sibling it did not describe is still added.
	if _, ok := props[stepDegradedKey].(map[string]any); !ok {
		t.Fatalf("degraded property missing: %v", props)
	}
}
