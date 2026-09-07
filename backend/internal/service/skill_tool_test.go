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

func TestBuildSkillsSection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	first := `---
name: first-skill
description: First skill description
---
body`
	second := `---
name: second-skill
description: Second skill description
---
body`
	if err := os.WriteFile(filepath.Join(skillDir, "first-skill.md"), []byte(first), 0o644); err != nil {
		t.Fatalf("write first skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "second-skill.md"), []byte(second), 0o644); err != nil {
		t.Fatalf("write second skill: %v", err)
	}

	chatSvc := &ChatService{skillsSvc: skills.NewService(dir, "skills")}
	section, err := chatSvc.buildSkillsSection()
	if err != nil {
		t.Fatalf("buildSkillsSection() error = %v", err)
	}
	if !strings.Contains(section, "first-skill: First skill description") {
		t.Fatalf("section missing first skill: %s", section)
	}
	if !strings.Contains(section, "second-skill: Second skill description") {
		t.Fatalf("section missing second skill: %s", section)
	}
}

func TestModeListsSkills(t *testing.T) {
	t.Parallel()

	for _, m := range []string{"main", "discuss"} {
		if !modeListsSkills(m) {
			t.Fatalf("modeListsSkills(%q) = false, want true", m)
		}
	}
	for _, m := range []string{"decompose", "plan", "execute", "incident", ""} {
		if modeListsSkills(m) {
			t.Fatalf("modeListsSkills(%q) = true, want false", m)
		}
	}
}

// A system skill is nib's own documentation, and that documentation quotes
// template syntax. prompttpl renders with missingkey=error, so putting it
// through the renderer would make get_skill fail on the very text it is there
// to serve -- while an operator's own skill must still be rendered.
func TestExecuteGetSkillDoesNotRenderSystemSkills(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	const templated = "see {{ .global.NoSuchVariable }} for details"
	if err := os.WriteFile(filepath.Join(skillDir, "ours.md"), []byte(templated), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	chatSvc := &ChatService{
		skillsSvc:    skills.NewService(dir, "skills"),
		variableRepo: &stubVariableRepo{vars: map[string]map[string]any{"global": {}}},
	}

	if _, err := chatSvc.ExecuteGetSkill(context.Background(), map[string]any{"name": "ours"}); err == nil {
		t.Fatal("an operator skill with an unknown variable rendered without error; the fixture no longer proves anything")
	}

	out, err := chatSvc.ExecuteGetSkill(context.Background(), map[string]any{"name": "nib-configuration"})
	if err != nil {
		t.Fatalf("ExecuteGetSkill(nib-configuration): %v", err)
	}
	if !strings.Contains(out, "streamable HTTP") {
		t.Fatal("the nib-configuration skill does not carry the MCP transport answer")
	}
}

// The catalog in the main and discuss prompts is where the model learns the
// system skills exist at all.
func TestBuildSkillsSectionListsSystemSkills(t *testing.T) {
	t.Parallel()

	chatSvc := &ChatService{skillsSvc: skills.NewService(t.TempDir(), "skills")}
	section, err := chatSvc.buildSkillsSection()
	if err != nil {
		t.Fatalf("buildSkillsSection() error = %v", err)
	}
	for _, meta := range skills.SystemMeta() {
		if !strings.Contains(section, "- "+meta.Name+": ") {
			t.Fatalf("section does not list the system skill %q: %s", meta.Name, section)
		}
	}
}
