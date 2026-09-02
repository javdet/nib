package service

import (
	"testing"

	"github.com/google/uuid"
)

func execTestService(t *testing.T) *ChatService {
	t.Helper()
	return &ChatService{actionPlansDir: t.TempDir()}
}

func TestActionExecRunLifecycle(t *testing.T) {
	t.Parallel()

	svc := execTestService(t)
	planID := uuid.New()

	run, err := svc.startActionExecRun(planID, "s0.step0")
	if err != nil {
		t.Fatalf("startActionExecRun: %v", err)
	}
	if run.Status != ActionExecRunning || run.Attempt != 1 || run.StartedAt == 0 {
		t.Fatalf("first attempt = %+v", run)
	}

	svc.finishActionExecRun(planID, "s0.step0", ActionExecDone, "")

	runs, err := svc.ReadActionPlanExecRuns(planID)
	if err != nil {
		t.Fatalf("ReadActionPlanExecRuns: %v", err)
	}
	got := runs["s0.step0"]
	if got.Status != ActionExecDone || got.FinishedAt == 0 {
		t.Fatalf("after finish = %+v", got)
	}
	if got.Attempt != 1 {
		t.Errorf("Attempt = %d, want 1", got.Attempt)
	}
}

// Restarting an action must be visible as a new attempt rather than looking like
// the first one, or an operator cannot tell a retry from an original run.
func TestActionExecRunCountsAttempts(t *testing.T) {
	t.Parallel()

	svc := execTestService(t)
	planID := uuid.New()

	if _, err := svc.startActionExecRun(planID, "s0.step0"); err != nil {
		t.Fatalf("first: %v", err)
	}
	svc.finishActionExecRun(planID, "s0.step0", ActionExecFailed, "boom")

	run, err := svc.startActionExecRun(planID, "s0.step0")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if run.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", run.Attempt)
	}
	if run.Error != "" {
		t.Errorf("the previous failure leaked into the new attempt: %q", run.Error)
	}
}

// A run already closed by a restart must not be reopened by the attempt it
// replaced reporting in late.
func TestFinishActionExecRunLeavesAClosedRunAlone(t *testing.T) {
	t.Parallel()

	svc := execTestService(t)
	planID := uuid.New()

	if _, err := svc.startActionExecRun(planID, "s0.step0"); err != nil {
		t.Fatalf("start: %v", err)
	}
	svc.finishActionExecRun(planID, "s0.step0", ActionExecCancelled, "restarted")
	svc.finishActionExecRun(planID, "s0.step0", ActionExecDone, "")

	runs, _ := svc.ReadActionPlanExecRuns(planID)
	if got := runs["s0.step0"].Status; got != ActionExecCancelled {
		t.Errorf("Status = %q, want %q", got, ActionExecCancelled)
	}
}

// Every key-addressed side store is remapped when a stage is reordered. An exec
// record left behind would put one action's status on another's row.
func TestRemapActionPlanKeysMovesExecRuns(t *testing.T) {
	t.Parallel()

	runs := ActionExecRuns{
		"s0.step0":  {Status: ActionExecDone, Attempt: 1},
		"s0.step1":  {Status: ActionExecFailed, Attempt: 2},
		"s1.step0":  {Status: ActionExecRunning},
		"s0.check0": {Status: ActionExecDone},
	}

	// Move step 0 to position 1 within stage 0.
	perm, err := moveIndex(2, 0, 1)
	if err != nil {
		t.Fatalf("moveIndex: %v", err)
	}
	got := remapActionPlanKeys(runs, 0, ActionPlanScopeSteps, perm)

	if got["s0.step1"].Status != ActionExecDone {
		t.Errorf("s0.step0 did not move to s0.step1: %+v", got)
	}
	if got["s0.step0"].Status != ActionExecFailed {
		t.Errorf("s0.step1 did not move to s0.step0: %+v", got)
	}
	if got["s1.step0"].Status != ActionExecRunning {
		t.Error("another stage was disturbed")
	}
	if got["s0.check0"].Status != ActionExecDone {
		t.Error("the checks scope was disturbed by a steps reorder")
	}
}

func TestReadActionPlanExecRunsMissingFile(t *testing.T) {
	t.Parallel()

	svc := execTestService(t)
	runs, err := svc.ReadActionPlanExecRuns(uuid.New())
	if err != nil {
		t.Fatalf("ReadActionPlanExecRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("runs = %v, want empty", runs)
	}
}

// A restart must report its own outcome. Naming the report after the row alone
// made every attempt after the first look like a duplicate of the run it
// replaced, so the operator kept reading a result they had already abandoned.
func TestActionExecMessageNameIsPerAttempt(t *testing.T) {
	t.Parallel()

	first := actionExecMessageName("s0.step0", 1)
	second := actionExecMessageName("s0.step0", 2)

	if first == second {
		t.Fatalf("attempts 1 and 2 share the name %q", first)
	}
	if actionExecMessageName("s0.step0", 1) != first {
		t.Error("the name is not stable for the same attempt")
	}
	if actionExecMessageName("s0.step1", 1) == first {
		t.Error("two different rows share a name")
	}
}
