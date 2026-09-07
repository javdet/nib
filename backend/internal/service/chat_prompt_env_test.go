package service

import (
	"context"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/skills"
	"github.com/javdet/nib/internal/systemprompts"
)

// promptTestService walks the real path: the embedded prompts, rendered with
// the variables a booted install would have.
func promptTestService(t *testing.T) *ChatService {
	t.Helper()

	dataDir := t.TempDir()
	selection := NewSelectionStore()
	selection.Set(domain.Selection{Project: "demo", Environment: "any", Cloud: "any", Location: "any"})

	return &ChatService{
		systemPromptsSvc: systemprompts.NewService(dataDir, "prompts"),
		skillsSvc:        skills.NewService(dataDir, "skills"),
		selection:        selection,
		variableRepo: &stubVariableRepo{vars: map[string]map[string]any{
			"global": {
				"CompanyName":    "AutomagicOps",
				"TaskTracker":    "Jira",
				"IssueProject":   "OPS",
				"toolCategories": []any{"cloud", "kubernetes"},
				"hostOS":         "Debian GNU/Linux 12 (bookworm), linux/arm64, kernel 6.10.14-linuxkit",
				"hostRuntime":    "Kubernetes pod (namespace nib)",
				"hostShell":      "/bin/bash",
				"hostWorkingDir": "/app",
				"hostDataDir":    "/app/data",
				"hostUser":       "root (uid 0)",
				"hostCommands":   "bash, curl, dig, jq",
			},
		}},
	}
}

// The block is only worth anything if the detected values actually reach the
// prompt the model reads, in every mode that can run or plan a command.
func TestResolveSystemPromptCarriesTheEnvironmentBlock(t *testing.T) {
	t.Parallel()

	svc := promptTestService(t)

	tests := []struct {
		mode string
		want bool
	}{
		{mode: "main", want: true},
		{mode: "plan", want: true},
		{mode: "execute", want: true},
		{mode: "discuss", want: true},
		// Deliberately left out: these plan no commands of their own. Flipping
		// either is a one-line change in the prompt file.
		{mode: "decompose", want: false},
		{mode: "incident", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.mode, func(t *testing.T) {
			t.Parallel()

			prompt, err := svc.resolveSystemPrompt(context.Background(), tt.mode)
			if err != nil {
				t.Fatalf("resolveSystemPrompt(%s): %v", tt.mode, err)
			}

			if got := strings.Contains(prompt, "## Environment"); got != tt.want {
				t.Fatalf("%s prompt has the environment block = %v, want %v", tt.mode, got, tt.want)
			}
			if !tt.want {
				return
			}
			for _, want := range []string{
				"Debian GNU/Linux 12 (bookworm)",
				"Kubernetes pod (namespace nib)",
				"/app/data",
				"root (uid 0)",
				"bash, curl, dig, jq",
			} {
				if !strings.Contains(prompt, want) {
					t.Fatalf("%s prompt does not carry %q", tt.mode, want)
				}
			}
		})
	}
}

// The modes that write or run shell steps have to know execute_command is not a
// shell, or they will plan pipes that silently cannot run.
func TestPlanAndExecutePromptsSayThereIsNoShell(t *testing.T) {
	t.Parallel()

	svc := promptTestService(t)

	for _, mode := range []string{"plan", "execute"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			prompt, err := svc.resolveSystemPrompt(context.Background(), mode)
			if err != nil {
				t.Fatalf("resolveSystemPrompt(%s): %v", mode, err)
			}
			if !strings.Contains(prompt, "There is no shell") {
				t.Fatalf("%s prompt does not say execute_command is not a shell", mode)
			}
		})
	}
}

// Self-identification is what turns "how do I connect an MCP server to you?"
// from a guess about other agent frameworks into a nib configuration answer.
func TestMainAndDiscussPromptsIdentifyNibAndListItsSkills(t *testing.T) {
	t.Parallel()

	svc := promptTestService(t)

	for _, mode := range []string{"main", "discuss"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			prompt, err := svc.resolveSystemPrompt(context.Background(), mode)
			if err != nil {
				t.Fatalf("resolveSystemPrompt(%s): %v", mode, err)
			}
			for _, want := range []string{
				"nib-configuration",
				"streamable HTTP",
				"stdio",
			} {
				if !strings.Contains(prompt, want) {
					t.Fatalf("%s prompt does not mention %q", mode, want)
				}
			}
			// The catalog has to name the system skills, or get_skill is a tool
			// the model has no reason to reach for.
			if !strings.Contains(prompt, "## Skills") {
				t.Fatalf("%s prompt has no skill catalog", mode)
			}
			for _, name := range skills.SystemNames() {
				if !strings.Contains(prompt, "- "+name+":") {
					t.Fatalf("%s skill catalog does not list %q", mode, name)
				}
			}
		})
	}
}

// The one prompt an operator can override is returned verbatim, so an install
// that customised discuss keeps its own text -- worth pinning, because it is
// the known gap in shipping the block through the prompt files.
func TestDiscussOverrideIsReturnedVerbatim(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	svc := promptTestService(t)
	svc.systemPromptsSvc = systemprompts.NewService(dataDir, "prompts")
	if err := svc.systemPromptsSvc.Set("discuss", "You are terse.\n"); err != nil {
		t.Fatalf("Set(discuss): %v", err)
	}

	prompt, err := svc.resolveSystemPrompt(context.Background(), "discuss")
	if err != nil {
		t.Fatalf("resolveSystemPrompt(discuss): %v", err)
	}
	if strings.Contains(prompt, "## Environment") {
		t.Fatal("an override unexpectedly gained the environment block; update the docs if this is now inherited")
	}
	if !strings.HasPrefix(prompt, "You are terse.") {
		t.Fatalf("the override was not used: %q", prompt)
	}
}
