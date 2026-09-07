package skills

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/repository"
)

func TestIsSystem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{"nib-configuration", true},
		{"nib-internals", true},
		{"categorize-tools", false},
		{"", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := IsSystem(tt.name); got != tt.want {
				t.Fatalf("IsSystem(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestSystemNamesAndMetaAreSortedAndDescribed(t *testing.T) {
	t.Parallel()

	names := SystemNames()
	if len(names) == 0 {
		t.Fatal("SystemNames() is empty")
	}
	if !slices.IsSorted(names) {
		t.Fatalf("SystemNames() = %v, want sorted", names)
	}

	metas := SystemMeta()
	if len(metas) != len(names) {
		t.Fatalf("SystemMeta() has %d entries, SystemNames() has %d", len(metas), len(names))
	}
	for _, meta := range metas {
		if meta.Description == "" {
			t.Fatalf("system skill %q has no description, so the prompt catalog cannot say what it is for", meta.Name)
		}
	}
}

func TestValidateSystemSkills(t *testing.T) {
	t.Parallel()

	if err := ValidateSystemSkills(); err != nil {
		t.Fatalf("ValidateSystemSkills() = %v, want nil", err)
	}

	for _, name := range SystemNames() {
		content, err := systemContent(name)
		if err != nil {
			t.Fatalf("systemContent(%q): %v", name, err)
		}
		if strings.TrimSpace(content) == "" {
			t.Fatalf("systemContent(%q) is blank", name)
		}
	}

	if _, err := systemContent("not-a-system-skill"); !errors.Is(err, ErrSystemSkill) {
		t.Fatalf("systemContent on an unknown name = %v, want ErrSystemSkill", err)
	}
}

// The two registries must stay disjoint: a name in both would be seeded onto the
// data volume as an editable copy of something the image owns.
func TestSystemAndDefaultNamesAreDisjoint(t *testing.T) {
	t.Parallel()

	for _, name := range DefaultNames() {
		if IsSystem(name) {
			t.Fatalf("%q is both a seeded default and a system skill", name)
		}
	}
}

func TestListAllMergesBothTiers(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	if err := svc.Create("postgres-upgrade", "---\nname: postgres-upgrade\ndescription: ours\n---\nbody"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	list, err := svc.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}

	got := make([]string, 0, len(list.Skills))
	for _, meta := range list.Skills {
		got = append(got, meta.Name)
	}
	if !slices.IsSorted(got) {
		t.Fatalf("ListAll() = %v, want sorted", got)
	}
	for _, want := range append(SystemNames(), "postgres-upgrade") {
		if !slices.Contains(got, want) {
			t.Fatalf("ListAll() = %v, want it to contain %q", got, want)
		}
	}

	// The operator-facing listing is what the HTTP API serves, so it must not
	// leak a system skill into the interface.
	onDisk, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, meta := range onDisk.Skills {
		if IsSystem(meta.Name) {
			t.Fatalf("List() exposed the system skill %q", meta.Name)
		}
	}
}

func TestGetAnyReachesBothTiersAndGetDoesNot(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	if err := svc.Create("ours", "body"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ours, err := svc.GetAny("ours")
	if err != nil {
		t.Fatalf("GetAny(ours): %v", err)
	}
	if ours.Content != "body" {
		t.Fatalf("GetAny(ours).Content = %q, want %q", ours.Content, "body")
	}

	system, err := svc.GetAny("nib-configuration")
	if err != nil {
		t.Fatalf("GetAny(nib-configuration): %v", err)
	}
	if !strings.Contains(system.Content, "streamable HTTP") {
		t.Fatal("GetAny(nib-configuration) does not carry the MCP transport answer")
	}

	if _, err := svc.Get("nib-configuration"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get(nib-configuration) = %v, want ErrNotFound: the HTTP handler must not reach a system skill", err)
	}
}

func TestWritesToASystemNameAreRefused(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	if err := svc.Create("ours", "body"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tests := []struct {
		name string
		call func() error
	}{
		{"create", func() error { return svc.Create("nib-configuration", "hijacked") }},
		{"set", func() error { return svc.Set("nib-configuration", "hijacked") }},
		{"delete", func() error { return svc.Delete("nib-configuration") }},
		{"rename from", func() error { return svc.Rename("nib-configuration", "ours", "") }},
		// Renaming *onto* a system name is how a file would come to shadow the
		// image's own answer.
		{"rename onto", func() error { return svc.Rename("ours", "nib-configuration", "") }},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, ErrSystemSkill) {
				t.Fatalf("%s = %v, want ErrSystemSkill", tt.name, err)
			}
		})
	}

	if _, err := os.Stat(filepath.Join(svc.Dir(), "nib-configuration.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a system skill landed on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(svc.Dir(), "ours.md")); err != nil {
		t.Fatalf("the refused rename moved the operator's own skill: %v", err)
	}
}

func TestSeedWritesNoSystemSkill(t *testing.T) {
	t.Parallel()

	svc := newTestService(t)
	if _, err := svc.Seed(); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	for _, name := range SystemNames() {
		if _, err := os.Stat(filepath.Join(svc.Dir(), name+".md")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Seed wrote the system skill %q to the data volume: %v", name, err)
		}
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(t.TempDir(), "skills")
}
