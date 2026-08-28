package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/skills"
)

func TestGetSkillToolDef(t *testing.T) {
	t.Parallel()

	def := GetSkillToolDef()
	if def.Name != GetSkillToolName {
		t.Fatalf("Name = %q, want %q", def.Name, GetSkillToolName)
	}
	if def.Description == "" {
		t.Fatal("Description is empty")
	}
	if len(def.Parameters) == 0 {
		t.Fatal("Parameters is empty")
	}
}

func TestExecuteGetSkill(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	content := `---
name: test-skill
description: A test skill
---

do something useful`
	if err := os.WriteFile(filepath.Join(skillDir, "test-skill.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	skillSvc := skills.NewService(dir, "skills")
	chatSvc := &ChatService{
		skillsSvc: skillSvc,
	}

	out, err := chatSvc.ExecuteGetSkill(context.Background(), map[string]any{"name": "test-skill"})
	if err != nil {
		t.Fatalf("ExecuteGetSkill() error = %v", err)
	}
	if !strings.Contains(out, "do something useful") {
		t.Fatalf("output missing body: %s", out)
	}
}

func TestExecuteGetSkillNotFound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillSvc := skills.NewService(dir, "skills")
	chatSvc := &ChatService{skillsSvc: skillSvc}

	_, err := chatSvc.ExecuteGetSkill(context.Background(), map[string]any{"name": "missing"})
	if err == nil {
		t.Fatal("expected error for missing skill")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestBuildIncludedSkillsSection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	included := `---
name: included-skill
description: Included skill description
category: included
---
body`
	searchable := `---
name: searchable-skill
description: Searchable skill description
---
body`
	if err := os.WriteFile(filepath.Join(skillDir, "included-skill.md"), []byte(included), 0o644); err != nil {
		t.Fatalf("write included skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "searchable-skill.md"), []byte(searchable), 0o644); err != nil {
		t.Fatalf("write searchable skill: %v", err)
	}

	chatSvc := &ChatService{skillsSvc: skills.NewService(dir, "skills")}
	section, err := chatSvc.buildIncludedSkillsSection()
	if err != nil {
		t.Fatalf("buildIncludedSkillsSection() error = %v", err)
	}
	if !strings.Contains(section, "included-skill: Included skill description") {
		t.Fatalf("section missing included skill: %s", section)
	}
	if strings.Contains(section, "searchable-skill") {
		t.Fatalf("section must not list searchable skills: %s", section)
	}
}
