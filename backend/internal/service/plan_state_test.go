package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

func TestReadWritePlanState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dialogID := uuid.New()
	svc := &ChatService{planStateDir: dir}

	state, err := svc.ReadPlanState(dialogID)
	if err != nil {
		t.Fatalf("read empty plan state: %v", err)
	}
	if state.Status != ActionPlanStatusDraft {
		t.Fatalf("status = %q, want draft", state.Status)
	}
	if state.ScheduledAt != 0 {
		t.Fatalf("scheduledAt = %d, want 0", state.ScheduledAt)
	}

	want := PlanState{
		Status:      ActionPlanStatusScheduled,
		ScheduledAt: 1735689600,
	}
	if err := svc.WritePlanState(dialogID, want); err != nil {
		t.Fatalf("write plan state: %v", err)
	}

	got, err := svc.ReadPlanState(dialogID)
	if err != nil {
		t.Fatalf("read plan state: %v", err)
	}
	if got.Status != want.Status {
		t.Fatalf("status = %q, want %q", got.Status, want.Status)
	}
	if got.ScheduledAt != want.ScheduledAt {
		t.Fatalf("scheduledAt = %d, want %d", got.ScheduledAt, want.ScheduledAt)
	}
}

func TestReadPlanStatesDefaultsToDraft(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	svc := &ChatService{planStateDir: dir}

	id1 := uuid.New()
	id2 := uuid.New()
	states := svc.ReadPlanStates([]uuid.UUID{id1, id2})

	if len(states) != 2 {
		t.Fatalf("states len = %d, want 2", len(states))
	}
	for _, id := range []uuid.UUID{id1, id2} {
		state, ok := states[id]
		if !ok {
			t.Fatalf("missing state for %s", id)
		}
		if state.Status != ActionPlanStatusDraft {
			t.Fatalf("status for %s = %q, want draft", id, state.Status)
		}
		if state.ScheduledAt != 0 {
			t.Fatalf("scheduledAt for %s = %d, want 0", id, state.ScheduledAt)
		}
	}
}

func TestSyncPlanStatusForChecks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()

	parentID := uuid.New()
	planChildID := uuid.New()

	repo := &actionListDialogRepo{
		dialogs: map[uuid.UUID]domain.Dialog{
			planChildID: {
				ID:       planChildID,
				Mode:     "plan",
				ParentID: &parentID,
			},
		},
	}

	plansDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(plansDir, planChildID.String()+".json"),
		sampleStoredActionPlan(),
		0o644,
	); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	// sampleStoredActionPlan has 2 steps + 1 check in one stage, plus 1 rollback
	// item that must not count towards completion.
	allItems := []string{"s0.step0", "s0.step1", "s0.check0"}

	svc := &ChatService{
		planStateDir:   dir,
		actionPlansDir: plansDir,
		dialogRepo:     repo,
	}

	t.Run("draft to in_progress when first checkbox checked", func(t *testing.T) {
		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, []string{"s0.step0"})
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusInProgress {
			t.Fatalf("status = %q, want in_progress", status)
		}

		state, err := svc.ReadPlanState(parentID)
		if err != nil {
			t.Fatalf("read plan state: %v", err)
		}
		if state.Status != ActionPlanStatusInProgress {
			t.Fatalf("persisted status = %q, want in_progress", state.Status)
		}
	})

	t.Run("scheduled to in_progress when checkbox checked", func(t *testing.T) {
		if err := svc.WritePlanState(parentID, PlanState{
			Status:      ActionPlanStatusScheduled,
			ScheduledAt: 1735689600,
		}); err != nil {
			t.Fatalf("write plan state: %v", err)
		}

		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, []string{"s0.step0"})
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusInProgress {
			t.Fatalf("status = %q, want in_progress", status)
		}
	})

	t.Run("empty checked leaves status unchanged", func(t *testing.T) {
		if err := svc.WritePlanState(parentID, PlanState{
			Status:      ActionPlanStatusDraft,
			ScheduledAt: 0,
		}); err != nil {
			t.Fatalf("write plan state: %v", err)
		}

		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, nil)
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusDraft {
			t.Fatalf("status = %q, want draft", status)
		}
	})

	t.Run("in_progress stays in_progress when checks cleared", func(t *testing.T) {
		if err := svc.WritePlanState(parentID, PlanState{
			Status:      ActionPlanStatusInProgress,
			ScheduledAt: 0,
		}); err != nil {
			t.Fatalf("write plan state: %v", err)
		}

		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, nil)
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusInProgress {
			t.Fatalf("status = %q, want in_progress", status)
		}
	})

	t.Run("partial progress stays in_progress", func(t *testing.T) {
		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, allItems[:2])
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusInProgress {
			t.Fatalf("status = %q, want in_progress", status)
		}
	})

	t.Run("all steps and checks done finishes the plan", func(t *testing.T) {
		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, allItems)
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusDone {
			t.Fatalf("status = %q, want done", status)
		}

		state, err := svc.ReadPlanState(parentID)
		if err != nil {
			t.Fatalf("read plan state: %v", err)
		}
		if state.Status != ActionPlanStatusDone {
			t.Fatalf("persisted status = %q, want done", state.Status)
		}
	})

	t.Run("unchecking a finished plan reopens it", func(t *testing.T) {
		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, allItems[:2])
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusReopened {
			t.Fatalf("status = %q, want reopened", status)
		}
	})

	t.Run("reopened stays reopened while incomplete", func(t *testing.T) {
		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, nil)
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusReopened {
			t.Fatalf("status = %q, want reopened", status)
		}
	})

	t.Run("reopened finishes again when all items checked", func(t *testing.T) {
		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, allItems)
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusDone {
			t.Fatalf("status = %q, want done", status)
		}
	})

	t.Run("rolled_back is never changed automatically", func(t *testing.T) {
		if err := svc.WritePlanState(parentID, PlanState{
			Status:      ActionPlanStatusRolledBack,
			ScheduledAt: 0,
		}); err != nil {
			t.Fatalf("write plan state: %v", err)
		}

		status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, allItems)
		if err != nil {
			t.Fatalf("sync plan status: %v", err)
		}
		if status != ActionPlanStatusRolledBack {
			t.Fatalf("status = %q, want rolled_back", status)
		}
	})
}

func TestSyncPlanStatusForChecksWithoutPlanFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	parentID := uuid.New()
	planChildID := uuid.New()

	svc := &ChatService{
		planStateDir:   t.TempDir(),
		actionPlansDir: t.TempDir(),
		dialogRepo: &actionListDialogRepo{
			dialogs: map[uuid.UUID]domain.Dialog{
				planChildID: {ID: planChildID, Mode: "plan", ParentID: &parentID},
			},
		},
	}

	status, err := svc.SyncPlanStatusForChecks(ctx, planChildID, []string{"s0.step0"})
	if err != nil {
		t.Fatalf("sync plan status: %v", err)
	}
	if status != ActionPlanStatusInProgress {
		t.Fatalf("status = %q, want in_progress", status)
	}
}

func TestPlanProgressIgnoresRollback(t *testing.T) {
	t.Parallel()

	var plan storedActionPlan
	if err := json.Unmarshal(sampleStoredActionPlan(), &plan); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}

	done, total := planProgress(plan, []string{"s0.step0", "rollback.0", "s9.step9"})
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if done != 1 {
		t.Fatalf("done = %d, want 1", done)
	}
}

func TestNextPlanStatusForChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current ActionPlanStatus
		done    int
		total   int
		want    ActionPlanStatus
	}{
		{"draft untouched", ActionPlanStatusDraft, 0, 3, ActionPlanStatusDraft},
		{"draft started", ActionPlanStatusDraft, 1, 3, ActionPlanStatusInProgress},
		{"scheduled started", ActionPlanStatusScheduled, 1, 3, ActionPlanStatusInProgress},
		{"draft completed", ActionPlanStatusDraft, 3, 3, ActionPlanStatusDone},
		{"in_progress completed", ActionPlanStatusInProgress, 3, 3, ActionPlanStatusDone},
		{"in_progress partial", ActionPlanStatusInProgress, 2, 3, ActionPlanStatusInProgress},
		{"done unchecked", ActionPlanStatusDone, 2, 3, ActionPlanStatusReopened},
		{"done still complete", ActionPlanStatusDone, 3, 3, ActionPlanStatusDone},
		{"reopened completed", ActionPlanStatusReopened, 3, 3, ActionPlanStatusDone},
		{"reopened partial", ActionPlanStatusReopened, 1, 3, ActionPlanStatusReopened},
		{"rolled_back completed", ActionPlanStatusRolledBack, 3, 3, ActionPlanStatusRolledBack},
		{"unknown plan size", ActionPlanStatusDraft, 0, 0, ActionPlanStatusDraft},
		{"unknown plan size started", ActionPlanStatusDraft, 1, 0, ActionPlanStatusInProgress},
		{"unknown plan size never finishes", ActionPlanStatusInProgress, 3, 0, ActionPlanStatusInProgress},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := nextPlanStatusForChecks(tt.current, tt.done, tt.total)
			if got != tt.want {
				t.Fatalf("next(%q, %d/%d) = %q, want %q",
					tt.current, tt.done, tt.total, got, tt.want)
			}
		})
	}
}
