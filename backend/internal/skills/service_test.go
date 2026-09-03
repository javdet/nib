package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/javdet/nib/internal/skills"
)

func TestServiceCRUD(t *testing.T) {
	dir := t.TempDir()
	svc := skills.NewService(dir, "")

	content := `---
name: test-skill
description: A test skill
---

do something
`

	if err := svc.Create("test-skill", content); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list.Skills) != 1 || list.Skills[0].Name != "test-skill" {
		t.Fatalf("List() = %+v, want [test-skill]", list)
	}

	got, err := svc.Get("test-skill")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Content != content {
		t.Fatalf("Get().Content = %q, want %q", got.Content, content)
	}

	updated := content + "\nextra line"
	if err := svc.Set("test-skill", updated); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err = svc.Get("test-skill")
	if err != nil {
		t.Fatalf("Get() after Set error = %v", err)
	}
	if got.Content != updated {
		t.Fatalf("Get().Content after Set = %q, want %q", got.Content, updated)
	}

	if err := svc.Delete("test-skill"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "test-skill.md")); !os.IsNotExist(err) {
		t.Fatalf("file still exists after Delete(): err = %v", err)
	}
}
