package systemprompts

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/prompttpl"
)

// builtinRef matches a `{{ .builtin.Field }}` reference, including the ones
// inside `{{ if ne .builtin.Field "any" }}` guards.
var builtinRef = regexp.MustCompile(`\.builtin\.(\w+)`)

// globalRef matches a `{{ .global.Name }}` reference, so a prompt can be
// rendered without enumerating every prompt variable it happens to use.
var globalRef = regexp.MustCompile(`\.global\.(\w+)`)

// TestDefaultsRenderEveryBuiltinTheyGuardOn renders each embedded prompt with a
// distinct sentinel per selection field and requires every field the prompt
// mentions to reach the output. A prompt that branches on one field and then
// prints another -- the copy-paste that made the planner ask for a region the
// operator had already selected -- leaves its sentinel missing and fails here.
func TestDefaultsRenderEveryBuiltinTheyGuardOn(t *testing.T) {
	t.Parallel()

	sentinels := map[string]string{
		"Project":     "sentinel-project",
		"Environment": "sentinel-environment",
		"Cloud":       "sentinel-cloud",
		"Location":    "sentinel-location",
	}

	for _, name := range DefaultNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			content, ok := Default(name)
			if !ok {
				t.Fatalf("Default(%q) missing", name)
			}

			wanted := make(map[string]struct{})
			for _, m := range builtinRef.FindAllStringSubmatch(content, -1) {
				field := m[1]
				if _, known := sentinels[field]; !known {
					t.Fatalf("prompt references unknown selection field .builtin.%s", field)
				}
				wanted[field] = struct{}{}
			}
			if len(wanted) == 0 {
				return
			}

			builtin := make(map[string]any, len(sentinels))
			for field, value := range sentinels {
				builtin[field] = value
			}

			rendered, err := prompttpl.Render(content, map[string]map[string]any{
				"global":  globalStubs(content),
				"builtin": builtin,
			})
			if err != nil {
				t.Fatalf("Render(%s) = %v, want nil", name, err)
			}

			for field := range wanted {
				if !strings.Contains(rendered, sentinels[field]) {
					t.Errorf("prompt %s mentions .builtin.%s but never renders it; check that the branch guarding on %s also prints %s", name, field, field, field)
				}
			}
		})
	}
}

// globalStubs fills every `.global.X` the prompt uses, since rendering runs with
// missingkey=error. toolCategories is the one variable read as a list.
func globalStubs(content string) map[string]any {
	out := make(map[string]any)
	for _, m := range globalRef.FindAllStringSubmatch(content, -1) {
		name := m[1]
		if name == "toolCategories" {
			out[name] = []string{"kubernetes"}
			continue
		}
		out[name] = fmt.Sprintf("stub-%s", name)
	}
	return out
}
