package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func reconcileService(t *testing.T) *ChatService {
	t.Helper()

	dir := t.TempDir()
	return &ChatService{
		actionPlansDir: filepath.Join(dir, "action_plans"),
		planFanoutDir:  filepath.Join(dir, "plan_fanout"),
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// A goroutine and a webhook both die with the process, so a run left "running"
// has nothing that will ever close it -- a permanent spinner in the plan view
// and, with one execution at a time, a permanent block on every other action.
func TestReconcileStuckRunsClosesRunningActions(t *testing.T) {
	t.Parallel()

	svc := reconcileService(t)
	planID := uuid.New()
	writeJSON(t, filepath.Join(svc.actionPlansDir, planID.String()+".exec.json"), ActionExecRuns{
		"s0.step0": {Status: ActionExecRunning, Attempt: 1, StartedAt: 1},
		"s0.step1": {Status: ActionExecDone, Attempt: 1, StartedAt: 1, FinishedAt: 2},
		"s0.step2": {Status: ActionExecRunning, Attempt: 2, StartedAt: 1, Kind: ExecutionKindContainer},
	})

	rec, err := svc.ReconcileStuckRuns()
	if err != nil {
		t.Fatalf("ReconcileStuckRuns err = %v", err)
	}
	if rec.Actions != 2 {
		t.Errorf("closed %d actions, want 2", rec.Actions)
	}

	runs, err := svc.ReadActionPlanExecRuns(planID)
	if err != nil {
		t.Fatalf("ReadActionPlanExecRuns: %v", err)
	}
	for _, key := range []string{"s0.step0", "s0.step2"} {
		run := runs[key]
		if run.Status != ActionExecFailed {
			t.Errorf("%s status = %q, want failed", key, run.Status)
		}
		if !strings.Contains(run.Error, "restarted") {
			t.Errorf("%s error = %q, want it to say the server restarted", key, run.Error)
		}
		if run.FinishedAt == 0 {
			t.Errorf("%s was closed without a finish time", key)
		}
	}
	// A record that already had an ending keeps it: it says what actually
	// stopped that run.
	if got := runs["s0.step1"]; got.Status != ActionExecDone || got.Error != "" {
		t.Errorf("an already-closed run was rewritten: %+v", got)
	}
	// The attempt counter has to survive, or a restart looks like a first try.
	if runs["s0.step2"].Attempt != 2 {
		t.Errorf("attempt = %d, want it carried forward", runs["s0.step2"].Attempt)
	}
}

// The same bug one level up: a fan-out left running makes ErrFanoutInProgress
// refuse every replan forever.
func TestReconcileStuckRunsClosesRunningFanouts(t *testing.T) {
	t.Parallel()

	svc := reconcileService(t)
	planID := uuid.New()
	writeJSON(t, filepath.Join(svc.planFanoutDir, planID.String()+".json"), FanoutRun{
		RunID:  "run-1",
		Status: FanoutRunRunning,
		Stages: []FanoutStage{
			{Title: "Deploy", Status: FanoutStageRunning},
			{Title: "Verify", Status: FanoutStageDone},
			{Title: "Monitor", Status: FanoutStagePending},
		},
	})

	rec, err := svc.ReconcileStuckRuns()
	if err != nil {
		t.Fatalf("ReconcileStuckRuns err = %v", err)
	}
	if rec.Fanouts != 1 {
		t.Errorf("closed %d fan-outs, want 1", rec.Fanouts)
	}

	run, found, err := svc.ReadFanoutRun(planID)
	if err != nil || !found {
		t.Fatalf("ReadFanoutRun found = %v, err = %v", found, err)
	}
	if run.Status != FanoutRunFailed {
		t.Errorf("run status = %q, want failed", run.Status)
	}
	if run.Stages[0].Status != FanoutStageFailed {
		t.Errorf("the running stage is %q, want failed", run.Stages[0].Status)
	}
	if run.Stages[1].Status != FanoutStageDone || run.Stages[2].Status != FanoutStagePending {
		t.Error("a stage that was not running was rewritten")
	}
}

// A run parked on a question is not stuck: the operator can still answer it
// after a restart, which is the whole reason the record is a file.
func TestReconcileStuckRunsLeavesAParkedFanoutAlone(t *testing.T) {
	t.Parallel()

	svc := reconcileService(t)
	planID := uuid.New()
	writeJSON(t, filepath.Join(svc.planFanoutDir, planID.String()+".json"), FanoutRun{
		RunID: "run-1", Status: FanoutRunAwaitingInput, PendingAskID: "fanout_ask_1",
	})

	rec, err := svc.ReconcileStuckRuns()
	if err != nil {
		t.Fatalf("ReconcileStuckRuns err = %v", err)
	}
	if rec.Fanouts != 0 {
		t.Errorf("closed %d fan-outs, want the parked one left alone", rec.Fanouts)
	}
	if run, _, _ := svc.ReadFanoutRun(planID); run.Status != FanoutRunAwaitingInput {
		t.Errorf("run status = %q, want it still awaiting input", run.Status)
	}
}

// A plan nobody can parse is not a reason to refuse to start.
func TestReconcileStuckRunsSkipsAMalformedRecord(t *testing.T) {
	t.Parallel()

	svc := reconcileService(t)
	good := uuid.New()

	if err := os.MkdirAll(svc.actionPlansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.actionPlansDir, "broken.exec.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	writeJSON(t, filepath.Join(svc.actionPlansDir, good.String()+".exec.json"), ActionExecRuns{
		"s0.step0": {Status: ActionExecRunning, Attempt: 1},
	})

	rec, err := svc.ReconcileStuckRuns()
	if err != nil {
		t.Fatalf("ReconcileStuckRuns err = %v, want the bad file stepped over", err)
	}
	if rec.Actions != 1 {
		t.Errorf("closed %d actions, want the readable one still closed", rec.Actions)
	}
}

// An empty data directory is the first-boot case and must be silent.
func TestReconcileStuckRunsOnAnEmptyDataDir(t *testing.T) {
	t.Parallel()

	rec, err := reconcileService(t).ReconcileStuckRuns()
	if err != nil {
		t.Fatalf("ReconcileStuckRuns err = %v", err)
	}
	if rec.Actions != 0 || rec.Fanouts != 0 {
		t.Errorf("closed %+v, want nothing", rec)
	}
}
