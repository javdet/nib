package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/toolschema"
)

func TestSanitizeToolName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "already safe", in: "get_skill", want: "get_skill"},
		{name: "hyphens are kept", in: "list-issues", want: "list-issues"},
		{name: "dots become underscores", in: "jira.search.issues", want: "jira_search_issues"},
		{name: "slashes become underscores", in: "tools/list", want: "tools_list"},
		{name: "spaces collapse", in: "search   issues", want: "search_issues"},
		{name: "mixed separators collapse to one", in: "a./ b", want: "a_b"},
		{name: "leading digit is prefixed", in: "2fa_verify", want: "t_2fa_verify"},
		{name: "leading separator is trimmed", in: "__weird", want: "weird"},
		{name: "unicode is replaced", in: "поиск", want: ""},
		{name: "unicode mixed with ascii", in: "search_поиск", want: "search"},
		{name: "empty", in: "", want: ""},
		{name: "whitespace only", in: "   ", want: ""},
		{name: "separators only", in: "///", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeToolName(tc.in); got != tc.want {
				t.Errorf("sanitizeToolName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestSanitizeToolNameLength covers the cap and, more importantly, that two
// long names sharing a prefix do not collide -- a collision silently drops the
// second tool from the catalog.
func TestSanitizeToolNameLength(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 80)
	got := sanitizeToolName(long)
	if len(got) > maxToolNameLen {
		t.Errorf("len = %d, want <= %d", len(got), maxToolNameLen)
	}

	a := sanitizeToolName(strings.Repeat("x", 70) + "_one")
	b := sanitizeToolName(strings.Repeat("x", 70) + "_two")
	if a == b {
		t.Errorf("two long names collided: both became %q", a)
	}
	if len(a) > maxToolNameLen || len(b) > maxToolNameLen {
		t.Errorf("lengths = %d and %d, want <= %d", len(a), len(b), maxToolNameLen)
	}
}

// TestSanitizeToolNameIsIdempotent matters because a name may be sanitized
// again on the way through the catalog.
func TestSanitizeToolNameIsIdempotent(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"jira.search", "tools/list", "2fa", strings.Repeat("a", 80), "get_skill"} {
		once := sanitizeToolName(in)
		if twice := sanitizeToolName(once); twice != once {
			t.Errorf("sanitizeToolName(%q): %q then %q", in, once, twice)
		}
	}
}

// TestSanitizeToolNameOutputIsProviderSafe asserts the character rules every
// provider nib targets shares, rather than re-stating the implementation.
func TestSanitizeToolNameOutputIsProviderSafe(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"jira.search.issues", "tools/list", "2fa_verify", "a b c",
		strings.Repeat("z", 90), "get_skill", "list-issues",
	}
	for _, in := range inputs {
		got := sanitizeToolName(in)
		if got == "" {
			continue
		}
		assertProviderSafeName(t, got)
	}
}

func assertProviderSafeName(t *testing.T, name string) {
	t.Helper()
	if len(name) == 0 || len(name) > maxToolNameLen {
		t.Errorf("name %q has length %d, want 1..%d", name, len(name), maxToolNameLen)
		return
	}
	if c := name[0]; !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_') {
		t.Errorf("name %q does not start with a letter or underscore", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			t.Errorf("name %q contains %q, which at least one provider rejects", name, r)
			return
		}
	}
}

// TestLocalToolsAreProviderSafe walks the real local tool registry and holds
// every compiled-in tool to the same rules MCP tools are rewritten onto. Local
// names are never sanitized -- they go to the provider exactly as declared --
// so a future tool spelled `create-plan.contract` has to fail here rather than
// at the first turn that offers it.
func TestLocalToolsAreProviderSafe(t *testing.T) {
	t.Parallel()

	catalog := buildLocalToolCatalogForTest(t)
	if len(catalog.tools) == 0 {
		t.Fatal("no local tools were registered; the walk proves nothing")
	}

	for _, def := range catalog.tools {
		t.Run(def.Name, func(t *testing.T) {
			assertProviderSafeName(t, def.Name)
			if sanitizeToolName(def.Name) != def.Name {
				t.Errorf("local tool %q is not already provider-safe (would become %q)",
					def.Name, sanitizeToolName(def.Name))
			}
		})
	}
}

// TestLocalToolSchemasSurviveSanitizing checks every compiled-in schema still
// describes an object with properties once the sanitizer has run. A tool whose
// arguments vanish would be callable but unusable.
func TestLocalToolSchemasSurviveSanitizing(t *testing.T) {
	t.Parallel()

	catalog := buildLocalToolCatalogForTest(t)
	for _, def := range catalog.tools {
		t.Run(def.Name, func(t *testing.T) {
			clean := toolschema.Sanitize(def.Parameters)
			if clean["type"] != "object" {
				t.Errorf("type = %v, want object", clean["type"])
			}
			if _, ok := clean["properties"].(map[string]any); !ok {
				t.Errorf("properties missing or not an object: %v", clean)
			}
			// The sanitized schema must still be valid JSON, since it is
			// marshalled straight into the request body.
			if _, err := json.Marshal(clean); err != nil {
				t.Errorf("sanitized schema does not marshal: %v", err)
			}
		})
	}
}

// buildLocalToolCatalogForTest registers every local tool against a bare
// ChatService. Registrars that need a collaborator simply do not register, and
// the ones that do are the ones worth checking.
func buildLocalToolCatalogForTest(t *testing.T) *toolCatalog {
	t.Helper()
	s := &ChatService{}
	catalog := newToolCatalog()
	b := newToolBinding(uuid.New())
	b.mode = "main"
	s.addLocalTools(catalog, nil, b)
	return catalog
}
