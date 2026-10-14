package service

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/executor"
)

func planCancelTestService(t *testing.T, execCfg *executor.Config) *ChatService {
	t.Helper()
	svc := &ChatService{
		planStateDir:   t.TempDir(),
		actionPlansDir: t.TempDir(),
		codeFixesDir:   t.TempDir(),
		activity:       NewActivityBroker(),
	}
	if execCfg != nil {
		store := executor.NewConfigStore(t.TempDir(), "", "")
		if err := store.Set(*execCfg); err != nil {
			t.Fatalf("set executor config: %v", err)
		}
		svc.executorSvc = executor.NewService(store, executor.Secrets{})
	}
	return svc
}

func TestSetPlanStatus_cancelLeavesContainersWhenTheSettingIsOff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  *executor.Config
	}{
		{name: "no executor", cfg: nil},
		{name: "disabled executor", cfg: &executor.Config{Type: executor.TypeDisabled}},
		{name: "local, setting off", cfg: &executor.Config{Type: executor.TypeLocal, RemoveContainersOnPlanCancel: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := planCancelTestService(t, tt.cfg)
			planID := uuid.New()
			if _, ok := svc.acquireExecutionLease(ExecutionLease{
				PlanID: planID,
				Key:    "s0.step0",
				Kind:   ExecutionKindContainer,
				// Without a start time the lease reads as long expired.
				StartedAt: time.Now().Unix(),
			}); !ok {
				t.Fatal("acquire lease: refused")
			}

			change, err := svc.SetPlanStatus(context.Background(), planID, ActionPlanStatusCancelled)
			if err != nil {
				t.Fatalf("SetPlanStatus() error = %v", err)
			}
			if change.Status != ActionPlanStatusCancelled {
				t.Errorf("status = %q, want cancelled", change.Status)
			}
			if change.ContainersRemoved != 0 || change.CleanupErr != nil {
				t.Errorf("cleanup = %d, %v; want nothing attempted", change.ContainersRemoved, change.CleanupErr)
			}
			if _, held := svc.ExecutionInProgress(); !held {
				t.Error("the running container's lease was taken although cleanup is off")
			}

			state, err := svc.ReadPlanState(planID)
			if err != nil {
				t.Fatalf("ReadPlanState() error = %v", err)
			}
			if state.Status != ActionPlanStatusCancelled {
				t.Errorf("stored status = %q, want cancelled", state.Status)
			}
		})
	}
}

func TestSetPlanStatus_otherStatusesDoNotClean(t *testing.T) {
	t.Parallel()

	// A local executor with the setting on, so a cleanup would be attempted --
	// and fail, with no Docker daemon here -- if anything but a cancel ran it.
	svc := planCancelTestService(t, &executor.Config{Type: executor.TypeLocal, RemoveContainersOnPlanCancel: true})
	planID := uuid.New()

	change, err := svc.SetPlanStatus(context.Background(), planID, ActionPlanStatusDone)
	if err != nil {
		t.Fatalf("SetPlanStatus() error = %v", err)
	}
	if change.Status != ActionPlanStatusDone || change.CleanupErr != nil {
		t.Fatalf("SetPlanStatus() = %+v, want done with no cleanup", change)
	}
}

func TestSetPlanStatus_rejectsAnUnknownStatus(t *testing.T) {
	t.Parallel()

	svc := planCancelTestService(t, nil)
	if _, err := svc.SetPlanStatus(context.Background(), uuid.New(), "paused"); err == nil {
		t.Fatal("SetPlanStatus(paused) error = nil, want invalid status")
	}
}

func TestRecordedPlanContainers(t *testing.T) {
	t.Parallel()

	svc := planCancelTestService(t, nil)
	planID := uuid.New()

	if err := svc.WriteActionPlanExecRuns(planID, ActionExecRuns{
		"s0.step0": {Kind: ExecutionKindContainer, ContainerID: "cid-1", JobName: "nib-11111111"},
		"s0.step1": {Kind: ExecutionKindContainer, JobName: "nib-22222222"},
		// A sub-agent run has no container to remove.
		"s0.step2": {Kind: ExecutionKindSubagent},
	}); err != nil {
		t.Fatalf("write exec runs: %v", err)
	}
	if err := svc.recordCodeFixRun(planID, uuid.New(), CodeFixRun{
		Status:      ActionExecDone,
		ContainerID: "cid-fix",
	}); err != nil {
		t.Fatalf("record code fix: %v", err)
	}

	got := svc.recordedPlanContainers(planID)
	slices.Sort(got)
	want := []string{"cid-1", "cid-fix", "nib-22222222"}
	if !slices.Equal(got, want) {
		t.Fatalf("recordedPlanContainers() = %v, want %v", got, want)
	}
}
