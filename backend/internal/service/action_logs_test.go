package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/executor"
)

// The happy path needs a Docker daemon -- executorSvc is a concrete
// *executor.Service with no interface behind it -- so what is covered here is
// the refusals, which are what the plan view actually shows an operator.
func TestReadActionContainerLogsRefusals(t *testing.T) {
	t.Parallel()

	const key = "s0.step1"

	tests := []struct {
		name string
		runs ActionExecRuns
		want error
	}{
		{
			name: "row never run",
			runs: ActionExecRuns{},
			want: ErrActionExecRunNotFound,
		},
		{
			name: "sub-agent run has no container",
			runs: ActionExecRuns{key: {Status: ActionExecRunning, Kind: ExecutionKindSubagent}},
			want: ErrActionNotContainerRun,
		},
		{
			name: "record from before container runs were told apart",
			runs: ActionExecRuns{key: {Status: ActionExecRunning}},
			want: ErrActionNotContainerRun,
		},
		{
			name: "finished container run",
			runs: ActionExecRuns{key: {
				Status:      ActionExecDone,
				Kind:        ExecutionKindContainer,
				JobName:     "nib-12345678",
				ContainerID: "abc123",
			}},
			want: ErrActionNotRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := execTestService(t)
			svc.executorSvc = newTestExecutorService(t, executor.Config{Type: executor.TypeLocal})
			planID := uuid.New()
			if err := svc.WriteActionPlanExecRuns(planID, tt.runs); err != nil {
				t.Fatalf("seed exec runs: %v", err)
			}

			_, err := svc.ReadActionContainerLogs(context.Background(), planID, key)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestReadActionContainerLogsWithoutExecutor(t *testing.T) {
	t.Parallel()

	svc := execTestService(t)
	_, err := svc.ReadActionContainerLogs(context.Background(), uuid.New(), "s0.step1")
	if err == nil {
		t.Fatal("error = nil, want the unconfigured-executor failure")
	}
}
