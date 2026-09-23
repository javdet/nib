package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
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

func writeSkillFiles(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	skillDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(skillDir, name+".md"), []byte(content), 0o644); err != nil {
			t.Fatalf("write skill %s: %v", name, err)
		}
	}
	return dir
}

// An explicit skill is the operator's to start: the catalog leaves it out, and
// only the slash command gets it past get_skill. A disabled one never loads.
func TestBuildSkillsSectionHonoursAccess(t *testing.T) {
	t.Parallel()

	dir := writeSkillFiles(t, map[string]string{
		"open-skill":  "---\nname: open-skill\ndescription: Open to the agent\n---\nbody",
		"get-jira":    "---\nname: get-jira\ndescription: Only on command\naccess: explicit\n---\nbody",
		"retired-one": "---\nname: retired-one\ndescription: Switched off\naccess: disabled\n---\nbody",
	})
	chatSvc := &ChatService{skillsSvc: skills.NewService(dir, "skills")}

	section, err := chatSvc.buildSkillsSection()
	if err != nil {
		t.Fatalf("buildSkillsSection() error = %v", err)
	}
	if !strings.Contains(section, "- open-skill: Open to the agent") {
		t.Fatalf("section missing the enabled skill: %s", section)
	}
	for _, hidden := range []string{"- get-jira", "Only on command", "retired-one", "Switched off"} {
		if strings.Contains(section, hidden) {
			t.Fatalf("section carries %q: %s", hidden, section)
		}
	}
	if !strings.Contains(section, "slash command") {
		t.Fatalf("section does not tell the agent how to honour a slash command: %s", section)
	}
}

func TestBuildSkillsSectionWithoutExplicitSkillsHasNoSlashRule(t *testing.T) {
	t.Parallel()

	dir := writeSkillFiles(t, map[string]string{
		"open-skill": "---\nname: open-skill\ndescription: Open\n---\nbody",
		"off":        "---\nname: off\naccess: disabled\n---\nbody",
	})
	chatSvc := &ChatService{skillsSvc: skills.NewService(dir, "skills")}

	section, err := chatSvc.buildSkillsSection()
	if err != nil {
		t.Fatalf("buildSkillsSection() error = %v", err)
	}
	if strings.Contains(section, "slash command") {
		t.Fatalf("section explains slash commands with no explicit skill to invoke: %s", section)
	}
}

func TestGetSkillHonoursAccess(t *testing.T) {
	t.Parallel()

	dir := writeSkillFiles(t, map[string]string{
		"get-jira": "---\nname: get-jira\naccess: explicit\n---\nfetch the task",
		"off":      "---\nname: off\naccess: disabled\n---\nnever",
	})
	repo := newMultiDialogRepo()
	rootID := repo.add(domain.Dialog{Mode: "main"})
	chatSvc := &ChatService{skillsSvc: skills.NewService(dir, "skills"), dialogRepo: repo}
	ctx := context.Background()
	handler := chatSvc.getSkillHandler(toolBinding{dialogID: uuid.New(), planID: rootID})
	args := map[string]any{"name": "get-jira"}

	if _, err := handler(ctx, map[string]any{"name": "off"}); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("get_skill(off) error = %v, want disabled", err)
	}

	if _, err := handler(ctx, args); err == nil || !strings.Contains(err.Error(), "/get-jira") {
		t.Fatalf("get_skill before the command error = %v, want a refusal naming /get-jira", err)
	}

	// nib writes named user rows and the agent writes assistant rows; neither is
	// the operator invoking anything.
	for _, msg := range []domain.DialogMessage{
		{Role: "assistant", Content: "I could run /get-jira for you"},
		{Role: "user", Name: "stage-report", Content: "/get-jira"},
		{Role: "user", Content: "please use get-jira"},
	} {
		if _, err := repo.AppendMessage(ctx, rootID, msg); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	if _, err := handler(ctx, args); err == nil {
		t.Fatal("get_skill served an explicit skill nobody invoked with a slash command")
	}

	if _, err := repo.AppendMessage(ctx, rootID, domain.DialogMessage{Role: "user", Content: "/get-jira PROJ-42"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	out, err := handler(ctx, args)
	if err != nil {
		t.Fatalf("get_skill after the command error = %v", err)
	}
	if !strings.Contains(out, "fetch the task") {
		t.Fatalf("get_skill output = %q, want the skill body", out)
	}

	if _, err := chatSvc.ExecuteGetSkill(ctx, args); err == nil {
		t.Fatal("ExecuteGetSkill, which has no dialog, served an explicit skill")
	}
}

func TestInvokesSkill(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want bool
	}{
		{"/get-jira", true},
		{"/get-jira PROJ-1", true},
		{"please run /get-jira now", true},
		{"(/get-jira)", true},
		{"try /get-jira.", true},
		{"line one\n/get-jira", true},
		{"get-jira", false},
		{"/get-jira-task", false},
		{"/get-jiras", false},
		{"see docs/get-jira", false},
		{"https://example.com/get-jira", false},
		{"./get-jira", false},
		{"/Get-Jira", false},
		{"/get-jira-task and then /get-jira", true},
	}
	for _, tt := range tests {
		if got := invokesSkill(tt.text, "get-jira"); got != tt.want {
			t.Errorf("invokesSkill(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}
