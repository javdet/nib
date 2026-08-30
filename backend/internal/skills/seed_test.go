package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/skills"
)

func TestDefaultsAreLoadable(t *testing.T) {
	names := skills.DefaultNames()
	if len(names) == 0 {
		t.Fatal("DefaultNames() is empty, want at least one skill compiled into the binary")
	}
	for _, name := range names {
		content, ok := skills.Default(name)
		if !ok || strings.TrimSpace(content) == "" {
			t.Fatalf("Default(%q) = %q, %v; want non-empty content", name, content, ok)
		}
		if !strings.HasPrefix(content, "---") {
			t.Fatalf("Default(%q) has no frontmatter block", name)
		}
	}
}

func TestSeedWritesDefaultsOnFirstStart(t *testing.T) {
	dir := t.TempDir()
	svc := skills.NewService(dir, "")

	result, err := svc.Seed()
	if err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	if len(result.Created) != len(skills.DefaultNames()) {
		t.Fatalf("Seed().Created = %v, want %v", result.Created, skills.DefaultNames())
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list.Skills) != len(skills.DefaultNames()) {
		t.Fatalf("List() = %+v, want the built-in skills only", list.Skills)
	}
	// The marker file must not surface as a skill.
	for _, m := range list.Skills {
		if strings.HasPrefix(m.Name, ".") {
			t.Fatalf("List() returned bookkeeping file %q", m.Name)
		}
		if m.Description == "" {
			t.Fatalf("built-in skill %q has no description in its frontmatter", m.Name)
		}
		// Only included skills reach the system prompt; a searchable built-in
		// would ship invisible to the agent.
		if m.Category != skills.CategoryIncluded {
			t.Fatalf("built-in skill %q has category %q, want included", m.Name, m.Category)
		}
	}
}

func TestSeedIsIdempotentAndKeepsDeletions(t *testing.T) {
	dir := t.TempDir()
	svc := skills.NewService(dir, "")

	if _, err := svc.Seed(); err != nil {
		t.Fatalf("first Seed() error = %v", err)
	}

	name := skills.DefaultNames()[0]
	edited := "edited by the operator\n"
	if err := svc.Set(name, edited); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	result, err := svc.Seed()
	if err != nil {
		t.Fatalf("second Seed() error = %v", err)
	}
	if len(result.Created) != 0 {
		t.Fatalf("second Seed().Created = %v, want none", result.Created)
	}
	got, err := svc.Get(name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Content != edited {
		t.Fatalf("Get().Content = %q, want the operator edit preserved", got.Content)
	}

	if err := svc.Delete(name); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Seed(); err != nil {
		t.Fatalf("third Seed() error = %v", err)
	}
	if _, err := svc.Get(name); err == nil {
		t.Fatalf("Get(%q) succeeded after delete, want the deletion to survive seeding", name)
	}
}

func TestSeedLeavesAnExistingSkillAlone(t *testing.T) {
	dir := t.TempDir()
	svc := skills.NewService(dir, "")

	name := skills.DefaultNames()[0]
	own := "the operator's own skill\n"
	if err := svc.Create(name, own); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	result, err := svc.Seed()
	if err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	if len(result.Created) != 0 || len(result.Skipped) == 0 {
		t.Fatalf("Seed() = %+v, want the existing name skipped", result)
	}
	got, err := svc.Get(name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Content != own {
		t.Fatalf("Get().Content = %q, want %q", got.Content, own)
	}
}

func TestSeedRecordsNewDefaultsOnUpgrade(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	// An install from an older image: the marker names a skill that no longer
	// ships, and none of the current built-ins.
	if err := os.WriteFile(filepath.Join(skillsDir, ".seeded-defaults"), []byte("gone-skill\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := skills.NewService(dir, "")
	result, err := svc.Seed()
	if err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	if len(result.Created) != len(skills.DefaultNames()) {
		t.Fatalf("Seed().Created = %v, want every current built-in", result.Created)
	}

	marker, err := os.ReadFile(filepath.Join(skillsDir, ".seeded-defaults"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(marker), "gone-skill") {
		t.Fatalf("marker = %q, want the earlier entry kept", marker)
	}
}
