package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/executor"
)

// fakeStageRunStarter stands in for startPlanItem: it records what the run
// launched and writes the running record a real launch would, so the test can
// then finish that attempt the way a sub-agent or a webhook does.
type fakeStageRunStarter struct {
	svc *ChatService
	err func(key string) error

	mu      sync.Mutex
	started []string
}

func (f *fakeStageRunStarter) start(_ context.Context, planID uuid.UUID, key string) error {
	if f.err != nil {
		if err := f.err(key); err != nil {
			return err
		}
	}
	f.mu.Lock()
	f.started = append(f.started, key)
	f.mu.Unlock()
	_, err := f.svc.startActionExecRun(planID, key)
	return err
}

func (f *fakeStageRunStarter) keys() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.started...)
}

func stageRunTestPlan() map[string]any {
	return map[string]any{
		"stages": []any{
			map[string]any{
				"title": "Deploy",
				"steps": []any{
					map[string]any{"type": "shell", "action": "build"},
					map[string]any{"type": "shell", "action": "ship"},
				},
				"checks": []any{
					map[string]any{"check": "curl it", "expectation": "200"},
				},
			},
			map[string]any{
				"title":  "Verify",
				"steps":  []any{map[string]any{"type": "shell", "action": "look"}},
				"checks": []any{},
			},
		},
		"rollback": []any{
			map[string]any{"type": "shell", "action": "undo ship"},
			map[string]any{"type": "shell", "action": "undo build"},
		},
	}
}

func newStageRunService(t *testing.T) (*ChatService, *fakeStageRunStarter, *transcriptDialogRepo, uuid.UUID) {
	t.Helper()

	planID := uuid.New()
	repo := &transcriptDialogRepo{
		mapDialogRepo: mapDialogRepo{dialogs: map[uuid.UUID]domain.Dialog{
			planID: {ID: planID, Mode: "main"},
		}},
		messages: map[uuid.UUID][]domain.DialogMessage{},
	}
	svc := &ChatService{
		dialogRepo:     repo,
		actionPlansDir: t.TempDir(),
		activity:       NewActivityBroker(),
		actionExec:     ActionExecConfig{}.withDefaults(),
	}
	fake := &fakeStageRunStarter{svc: svc}
	svc.stageRunStart = fake.start

	writeStageRunPlan(t, svc, planID, stageRunTestPlan())
	return svc, fake, repo, planID
}

func writeStageRunPlan(t *testing.T, svc *ChatService, planID uuid.UUID, plan map[string]any) {
	t.Helper()
	b, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if _, err := svc.WriteActionPlan(planID, b); err != nil {
		t.Fatalf("write plan: %v", err)
	}
}

// finishItem closes key's running attempt the way its runner would and then
// fires the terminal hook, as every runner does once the lease is back.
func finishItem(t *testing.T, svc *ChatService, planID uuid.UUID, key string, status ActionExecStatus, errMsg string) {
	t.Helper()
	svc.finishActionExecRun(planID, key, status, errMsg)
	svc.advanceStageRun(planID, key)
}

func mustStageRun(t *testing.T, svc *ChatService, planID uuid.UUID) StageRun {
	t.Helper()
	run, err := svc.ReadStageRun(planID)
	if err != nil {
		t.Fatalf("ReadStageRun: %v", err)
	}
	if run == nil {
		t.Fatal("ReadStageRun = nil, want a run")
	}
	return *run
}

func stageRunMessages(repo *transcriptDialogRepo, planID uuid.UUID) []domain.DialogMessage {
	msgs, _ := repo.ListMessages(context.Background(), planID)
	var out []domain.DialogMessage
	for _, m := range msgs {
		if strings.HasPrefix(m.Name, "stage-run:") {
			out = append(out, m)
		}
	}
	return out
}

func equalKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStageRunWalksStepsThenChecks(t *testing.T) {
	t.Parallel()

	svc, fake, repo, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}

	for _, key := range []string{"s0.step0", "s0.step1", "s0.check0"} {
		if run := mustStageRun(t, svc, planID); run.Current != key || !run.Active() {
			t.Fatalf("run = %+v, want running on %s", run, key)
		}
		finishItem(t, svc, planID, key, ActionExecDone, "")
	}

	want := []string{"s0.step0", "s0.step1", "s0.check0"}
	if got := fake.keys(); !equalKeys(got, want) {
		t.Fatalf("started = %v, want %v", got, want)
	}
	run := mustStageRun(t, svc, planID)
	if run.Status != StageRunDone {
		t.Fatalf("status = %q, want done", run.Status)
	}
	if msgs := stageRunMessages(repo, planID); len(msgs) != 1 || !strings.Contains(msgs[0].Content, "ran to the end") {
		t.Fatalf("chat messages = %+v, want one saying the stage ran to the end", msgs)
	}
	// The slot is free again.
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 1); err != nil {
		t.Fatalf("second StartStageRun after done: %v", err)
	}
}

func TestStageRunSkipsTickedItems(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	if err := svc.WriteActionPlanChecks(planID, []string{"s0.step0", "s0.check0"}); err != nil {
		t.Fatalf("write checks: %v", err)
	}
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}
	finishItem(t, svc, planID, "s0.step1", ActionExecDone, "")

	if got, want := fake.keys(), []string{"s0.step1"}; !equalKeys(got, want) {
		t.Fatalf("started = %v, want %v", got, want)
	}
	if run := mustStageRun(t, svc, planID); run.Status != StageRunDone {
		t.Fatalf("status = %q, want done", run.Status)
	}
}

func TestStageRunNothingToRun(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	if err := svc.WriteActionPlanChecks(planID, []string{"s1.step0"}); err != nil {
		t.Fatalf("write checks: %v", err)
	}
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 1); !errors.Is(err, ErrStageRunNothingToRun) {
		t.Fatalf("err = %v, want ErrStageRunNothingToRun", err)
	}
	if len(fake.keys()) != 0 {
		t.Fatalf("started = %v, want nothing", fake.keys())
	}
}

func TestStageRunRejectsBadTarget(t *testing.T) {
	t.Parallel()

	svc, _, _, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, "everything", 0); !errors.Is(err, ErrInvalidStageRunScope) {
		t.Fatalf("bad scope err = %v, want ErrInvalidStageRunScope", err)
	}
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 7); !errors.Is(err, ErrActionPlanIndexOutOfRange) {
		t.Fatalf("bad stage err = %v, want ErrActionPlanIndexOutOfRange", err)
	}
}

func TestStageRunWalksRollback(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	// The stage index means nothing for the rollback.
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeRollback, 5); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}
	finishItem(t, svc, planID, "rollback.0", ActionExecDone, "")
	finishItem(t, svc, planID, "rollback.1", ActionExecDone, "")

	if got, want := fake.keys(), []string{"rollback.0", "rollback.1"}; !equalKeys(got, want) {
		t.Fatalf("started = %v, want %v", got, want)
	}
	run := mustStageRun(t, svc, planID)
	if run.Status != StageRunDone || run.Title != "Rollback" {
		t.Fatalf("run = %+v, want a done Rollback run", run)
	}
}

func TestStageRunStopsOnUnsuccessfulItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status ActionExecStatus
		errMsg string
		want   string
	}{
		{ActionExecFailed, "exit 2", "1.1 failed: exit 2"},
		{ActionExecBlocked, "", "1.1 needs a decision"},
		{ActionExecCancelled, "", "1.1 was cancelled"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()

			svc, fake, repo, planID := newStageRunService(t)
			if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
				t.Fatalf("StartStageRun: %v", err)
			}
			finishItem(t, svc, planID, "s0.step0", tt.status, tt.errMsg)

			if got := fake.keys(); !equalKeys(got, []string{"s0.step0"}) {
				t.Fatalf("started = %v, want only the first item", got)
			}
			run := mustStageRun(t, svc, planID)
			if run.Status != StageRunStopped || run.Reason != tt.want || run.Current != "s0.step0" {
				t.Fatalf("run = %+v, want stopped at s0.step0 with %q", run, tt.want)
			}
			msgs := stageRunMessages(repo, planID)
			if len(msgs) != 1 || !strings.Contains(msgs[0].Content, tt.want) {
				t.Fatalf("chat messages = %+v, want one carrying %q", msgs, tt.want)
			}
		})
	}
}

// A webhook retry, or a force stop followed by the cancelled goroutine's own
// finish, reports the same item closed twice. Only the first may move the run.
func TestStageRunIgnoresDuplicateAndEarlyHooks(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}

	// Still running: nothing to move on from.
	svc.advanceStageRun(planID, "s0.step0")
	if got := fake.keys(); !equalKeys(got, []string{"s0.step0"}) {
		t.Fatalf("after early hook started = %v, want only the first item", got)
	}

	finishItem(t, svc, planID, "s0.step0", ActionExecDone, "")
	svc.advanceStageRun(planID, "s0.step0")
	// A hook for an item the run is not on is somebody else's.
	svc.advanceStageRun(planID, "s1.step0")

	if got, want := fake.keys(), []string{"s0.step0", "s0.step1"}; !equalKeys(got, want) {
		t.Fatalf("started = %v, want %v", got, want)
	}
}

// A finished record left over from an earlier attempt is not this run's result.
func TestStageRunIgnoresEarlierAttempt(t *testing.T) {
	t.Parallel()

	svc, _, _, planID := newStageRunService(t)
	if err := svc.WriteActionPlanExecRuns(planID, ActionExecRuns{
		"s0.step0": {Status: ActionExecDone, Attempt: 3, StartedAt: time.Now().Unix()},
	}); err != nil {
		t.Fatalf("write exec runs: %v", err)
	}

	// A launch that has not recorded its attempt yet.
	svc.stageRunStart = func(context.Context, uuid.UUID, string) error { return nil }
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}
	svc.advanceStageRun(planID, "s0.step0")

	if run := mustStageRun(t, svc, planID); run.Current != "s0.step0" || !run.Active() || run.BaseAttempt != 3 {
		t.Fatalf("run = %+v, want still on s0.step0 from attempt 3", run)
	}
}

func TestStageRunStopsWhenPlanIsRestructured(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}

	plan := stageRunTestPlan()
	stage := plan["stages"].([]any)[0].(map[string]any)
	stage["steps"] = append(stage["steps"].([]any), map[string]any{"type": "shell", "action": "extra"})
	writeStageRunPlan(t, svc, planID, plan)

	finishItem(t, svc, planID, "s0.step0", ActionExecDone, "")

	if got := fake.keys(); !equalKeys(got, []string{"s0.step0"}) {
		t.Fatalf("started = %v, want only the first item", got)
	}
	if run := mustStageRun(t, svc, planID); run.Status != StageRunStopped || run.Reason != stageRunPlanChanged {
		t.Fatalf("run = %+v, want stopped because the plan changed", run)
	}
}

// Rewording an item the run has not reached, or the webhook attaching a pull
// request, leaves the keys naming the same items.
func TestStageRunSurvivesContentEdits(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}

	plan := stageRunTestPlan()
	steps := plan["stages"].([]any)[0].(map[string]any)["steps"].([]any)
	steps[0].(map[string]any)["pr_url"] = "https://example.test/pr/1"
	steps[1].(map[string]any)["action"] = "ship carefully"
	writeStageRunPlan(t, svc, planID, plan)

	finishItem(t, svc, planID, "s0.step0", ActionExecDone, "")

	if got, want := fake.keys(), []string{"s0.step0", "s0.step1"}; !equalKeys(got, want) {
		t.Fatalf("started = %v, want %v", got, want)
	}
}

func TestStageRunStopsWhenStageIsReordered(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}
	if _, _, _, err := svc.ReorderActionPlanItems(planID, ActionPlanScopeSteps, 0, 0, 1); err != nil {
		t.Fatalf("ReorderActionPlanItems: %v", err)
	}
	if run := mustStageRun(t, svc, planID); run.Status != StageRunStopped || run.Reason != stageRunPlanChanged {
		t.Fatalf("run = %+v, want stopped because the plan changed", run)
	}
	if got := fake.keys(); !equalKeys(got, []string{"s0.step0"}) {
		t.Fatalf("started = %v, want only the first item", got)
	}
}

func TestStageRunStopsWhenNextLaunchIsRefused(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	busy := newExecutionBusyError(subagentLease(uuid.New(), "s3.step0", "4.1"))
	fake.err = func(key string) error {
		if key == "s0.step1" {
			return busy
		}
		return nil
	}
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}
	finishItem(t, svc, planID, "s0.step0", ActionExecDone, "")

	run := mustStageRun(t, svc, planID)
	if run.Status != StageRunStopped || run.Current != "s0.step1" || !strings.Contains(run.Reason, "4.1 is already running") {
		t.Fatalf("run = %+v, want stopped at s0.step1 naming what holds the slot", run)
	}
}

func TestStageRunFirstLaunchFailureIsReturned(t *testing.T) {
	t.Parallel()

	svc, fake, repo, planID := newStageRunService(t)
	fake.err = func(string) error { return executor.ErrExecutorDisabled }

	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); !errors.Is(err, executor.ErrExecutorDisabled) {
		t.Fatalf("err = %v, want ErrExecutorDisabled", err)
	}
	if run := mustStageRun(t, svc, planID); run.Status != StageRunStopped {
		t.Fatalf("status = %q, want stopped", run.Status)
	}
	// The caller is told; the chat is not.
	if msgs := stageRunMessages(repo, planID); len(msgs) != 0 {
		t.Fatalf("chat messages = %+v, want none", msgs)
	}
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 1); err == nil || errors.Is(err, ErrStageRunActive) {
		t.Fatalf("second start err = %v, want the slot released", err)
	}
}

func TestStageRunOneAtATime(t *testing.T) {
	t.Parallel()

	svc, _, _, planID := newStageRunService(t)
	other := uuid.New()
	writeStageRunPlan(t, svc, other, stageRunTestPlan())

	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}
	_, err := svc.StartStageRun(t.Context(), other, StageRunScopeStage, 1)
	if !errors.Is(err, ErrStageRunActive) || !strings.Contains(err.Error(), "Deploy") {
		t.Fatalf("err = %v, want ErrStageRunActive naming the running stage", err)
	}
}

func TestStageRunRefusedWhileExecutionBusy(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	if _, ok := svc.acquireExecutionLease(subagentLease(uuid.New(), "s0.step0", "1.1")); !ok {
		t.Fatal("acquire lease failed")
	}
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); !errors.Is(err, ErrExecutionBusy) {
		t.Fatalf("err = %v, want ErrExecutionBusy", err)
	}
	if run, _ := svc.ReadStageRun(planID); run != nil {
		t.Fatalf("run = %+v, want none recorded", run)
	}
	if len(fake.keys()) != 0 {
		t.Fatalf("started = %v, want nothing", fake.keys())
	}
}

func TestCancelExecutionStopsStageRun(t *testing.T) {
	t.Parallel()

	svc, fake, _, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}
	if _, ok := svc.acquireExecutionLease(subagentLease(planID, "s0.step0", "1.1")); !ok {
		t.Fatal("acquire lease failed")
	}

	if _, err := svc.CancelExecution(t.Context()); err != nil {
		t.Fatalf("CancelExecution: %v", err)
	}
	// The cancelled goroutine's own hook arrives afterwards and must not
	// restart anything.
	svc.advanceStageRun(planID, "s0.step0")

	run := mustStageRun(t, svc, planID)
	if run.Status != StageRunStopped || run.Reason != stageRunStoppedByOperator {
		t.Fatalf("run = %+v, want stopped by the operator", run)
	}
	if got := fake.keys(); !equalKeys(got, []string{"s0.step0"}) {
		t.Fatalf("started = %v, want only the first item", got)
	}
}

func TestStopStageRunStopsOnlyItsOwnItem(t *testing.T) {
	t.Parallel()

	svc, _, repo, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}
	stranger := subagentLease(uuid.New(), "s0.step0", "1.1")
	if _, ok := svc.acquireExecutionLease(stranger); !ok {
		t.Fatal("acquire lease failed")
	}

	run, err := svc.StopStageRun(t.Context(), planID)
	if err != nil {
		t.Fatalf("StopStageRun: %v", err)
	}
	if run.Status != StageRunStopped || run.Reason != stageRunStoppedByOperator {
		t.Fatalf("run = %+v, want stopped by the operator", run)
	}
	if held, ok := svc.ExecutionInProgress(); !ok || held.PlanID != stranger.PlanID {
		t.Fatalf("lease = %+v, %v; want another plan's lease left alone", held, ok)
	}
	if msgs := stageRunMessages(repo, planID); len(msgs) != 1 {
		t.Fatalf("chat messages = %d, want 1", len(msgs))
	}
	if _, err := svc.StopStageRun(t.Context(), planID); !errors.Is(err, ErrNoStageRunActive) {
		t.Fatalf("second stop err = %v, want ErrNoStageRunActive", err)
	}
}

func TestReconcileStopsStageRuns(t *testing.T) {
	t.Parallel()

	svc, _, _, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}

	// A fresh process: nothing in memory, only the files.
	fresh := &ChatService{actionPlansDir: svc.actionPlansDir}
	res, err := fresh.ReconcileStuckRuns()
	if err != nil {
		t.Fatalf("ReconcileStuckRuns: %v", err)
	}
	if res.StageRuns != 1 {
		t.Fatalf("StageRuns = %d, want 1", res.StageRuns)
	}
	if run := mustStageRun(t, fresh, planID); run.Status != StageRunStopped || run.Reason != restartedReason {
		t.Fatalf("run = %+v, want stopped by the restart", run)
	}
}

// A plan reset under a running run must not hold the slot for good.
func TestStageRunSlotSurvivesRemovedRecord(t *testing.T) {
	t.Parallel()

	svc, _, _, planID := newStageRunService(t)
	if _, err := svc.StartStageRun(t.Context(), planID, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun: %v", err)
	}
	other := uuid.New()
	writeStageRunPlan(t, svc, other, stageRunTestPlan())

	// Removed behind the service's back, the way a deleted plan leaves it.
	if err := os.Remove(stageRunPath(svc.actionPlansDir, planID)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := svc.StartStageRun(t.Context(), other, StageRunScopeStage, 0); err != nil {
		t.Fatalf("StartStageRun on another plan: %v", err)
	}
}

func TestNextStageRunKey(t *testing.T) {
	t.Parallel()

	keys := []string{"s0.step0", "s0.step1", "s0.check0"}
	tests := []struct {
		name    string
		after   string
		checked []string
		want    string
		wantOK  bool
	}{
		{"from the start", "", nil, "s0.step0", true},
		{"skips ticked", "", []string{"s0.step0"}, "s0.step1", true},
		{"after an item", "s0.step0", nil, "s0.step1", true},
		{"does not go back", "s0.step1", []string{"s0.check0"}, "", false},
		{"all ticked", "", keys, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := nextStageRunKey(keys, tt.after, checkedSet(tt.checked))
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("nextStageRunKey = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// A code action's row has to show as running the moment its container starts,
// not only once its result comes back.
func TestRecordCodeActionRunPublishesStart(t *testing.T) {
	t.Parallel()

	svc, _, _, planID := newStageRunService(t)
	events, unsubscribe := svc.activity.Subscribe(planID)
	defer unsubscribe()

	svc.recordCodeActionRun(planID, "s0.step0", executor.ActionRunResult{JobName: "nib-job"})

	select {
	case ev := <-events:
		if ev.Kind != domain.ActivityActionExecStarted || ev.Action != "s0.step0" {
			t.Fatalf("event = %+v, want action_exec_started for s0.step0", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no activity published")
	}
	runs, err := svc.ReadActionPlanExecRuns(planID)
	if err != nil {
		t.Fatalf("read exec runs: %v", err)
	}
	if rec := runs["s0.step0"]; !rec.Active() || rec.JobName != "nib-job" || rec.Attempt != 1 {
		t.Fatalf("record = %+v, want a running container attempt 1", rec)
	}
}
