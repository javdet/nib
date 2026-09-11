package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/mode"
)

func TestReportAgentAllowSet(t *testing.T) {
	t.Parallel()
	allow := reportAgentAllowSet()

	if len(allow) != 2 {
		t.Fatalf("len(allow) = %d, want 2: the report agent reads the plan and writes prose, nothing else", len(allow))
	}
	for _, name := range []string{GetActionListToolName, SetReportToolName} {
		if _, ok := allow[name]; !ok {
			t.Fatalf("missing %q", name)
		}
	}
	// Naming the ones that would turn a report into a side effect, so a later
	// widening of this set is a deliberate act rather than an accident.
	for _, name := range []string{
		ExecuteCommandToolName,
		APICallToolName,
		GetSecretsToolName,
		RunExecutorToolName,
		AskQuestionToolName,
		UpdateActionPlanToolName,
	} {
		if _, ok := allow[name]; ok {
			t.Fatalf("report agent must not carry %q", name)
		}
	}
}

// TestActionAgentAllowSetWithholdsSetReport is the guard that keeps a per-action
// executor from overwriting the plan's report with its account of one row. Both
// run in execute mode, so the mode allow list cannot tell them apart.
func TestActionAgentAllowSetWithholdsSetReport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := mode.SeedAllowLists(dir); err != nil {
		t.Fatalf("seed allow lists: %v", err)
	}
	// An operator adding the name by hand is exactly what this defends against.
	execList := filepath.Join(dir, "execute.json")
	if err := os.WriteFile(execList, []byte(`{"allow_tools":["get_action_list","set_report"]}`), 0o644); err != nil {
		t.Fatalf("write execute allow list: %v", err)
	}

	svc := &ChatService{allowToolsDir: dir}
	allow, err := svc.actionAgentAllowSet(context.Background(), nil)
	if err != nil {
		t.Fatalf("actionAgentAllowSet err = %v", err)
	}

	if _, ok := allow[SetReportToolName]; ok {
		t.Fatal("action agent must never carry set_report, even when the execute allow list names it")
	}
	if _, ok := allow[GetActionListToolName]; !ok {
		t.Fatal("action agent lost get_action_list")
	}
}

func TestStartReportAgentSkipsWhenAReportExists(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	reportsDir := t.TempDir()
	svc := &ChatService{
		reportsDir: reportsDir,
		dialogRepo: &actionListDialogRepo{},
	}
	if err := svc.WriteReport(planID, "## Outcome\n\nAlready written."); err != nil {
		t.Fatalf("WriteReport err = %v", err)
	}

	// A nil llm client would panic if the run started, so returning cleanly is
	// itself the assertion that nothing was launched.
	if err := svc.StartReportAgent(context.Background(), planID); err != nil {
		t.Fatalf("StartReportAgent err = %v", err)
	}

	content, found, err := svc.ReadReport(planID)
	if err != nil {
		t.Fatalf("ReadReport err = %v", err)
	}
	if !found || content != "## Outcome\n\nAlready written." {
		t.Fatalf("report was rewritten: found=%v content=%q", found, content)
	}
}

func TestStartReportAgentWithoutADialogRepo(t *testing.T) {
	t.Parallel()

	svc := &ChatService{reportsDir: t.TempDir()}
	if err := svc.StartReportAgent(context.Background(), uuid.New()); err == nil {
		t.Fatal("expected an error when the dialog repository is not configured")
	}
}

func TestBuildReportSeed(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	svc := &ChatService{
		summariesDir:     t.TempDir(),
		dagsDir:          t.TempDir(),
		planContractsDir: t.TempDir(),
	}
	if err := svc.WriteSummary(rootID, "Roll billing out to stage."); err != nil {
		t.Fatalf("WriteSummary err = %v", err)
	}

	seed, err := svc.buildReportSeed(rootID)
	if err != nil {
		t.Fatalf("buildReportSeed err = %v", err)
	}

	for _, want := range []string{
		"## Summary",
		"Roll billing out to stage.",
		"## Your task",
		"`get_action_list`",
		"`set_report`",
	} {
		if !strings.Contains(seed, want) {
			t.Fatalf("seed missing %q:\n%s", want, seed)
		}
	}
	// The plan itself is deliberately absent: the agent pulls it with the tool,
	// so there is one shape of that data rather than two.
	if strings.Contains(seed, "## Action") {
		t.Fatalf("seed should not inline the action list:\n%s", seed)
	}
}
