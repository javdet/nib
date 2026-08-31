package systemprompts

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/mode"
)

// TestEmbeddedDefaultsCoverAllModes is the build-time guarantee that every mode
// exposed by the API has a prompt compiled into the binary.
func TestEmbeddedDefaultsCoverAllModes(t *testing.T) {
	t.Parallel()

	for _, m := range mode.Modes {
		m := m
		t.Run(m, func(t *testing.T) {
			t.Parallel()
			content, ok := Default(m)
			if !ok {
				t.Fatalf("no prompt compiled into the binary for mode %q; expected internal/systemprompts/defaults/%s.md", m, m)
			}
			if strings.TrimSpace(content) == "" {
				t.Fatalf("embedded prompt for mode %q is blank", m)
			}
		})
	}
}

// TestEmbeddedDefaultsHaveNoExtras catches a stray default.md/assistant.md
// reappearing in the embedded FS.
func TestEmbeddedDefaultsHaveNoExtras(t *testing.T) {
	t.Parallel()

	want := append([]string(nil), mode.Modes...)
	want = append(want, Auxiliary...)
	sort.Strings(want)

	if got := DefaultNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultNames() = %v, want %v", got, want)
	}
}

func TestValidateEmbeddedDefaults(t *testing.T) {
	t.Parallel()

	if err := ValidateEmbeddedDefaults(mode.Modes); err != nil {
		t.Fatalf("ValidateEmbeddedDefaults(mode.Modes) = %v, want nil", err)
	}

	err := ValidateEmbeddedDefaults(append(append([]string(nil), mode.Modes...), "ghost"))
	if err == nil {
		t.Fatal("ValidateEmbeddedDefaults with a missing prompt = nil, want error")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error %q does not name the missing prompt", err)
	}
}

func TestEditableNameIsAMode(t *testing.T) {
	t.Parallel()

	if !mode.IsValid(EditableName) {
		t.Fatalf("EditableName %q is not a valid mode", EditableName)
	}
}
