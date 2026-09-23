package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/executor"
)

var (
	// ErrActionExecRunNotFound is returned when an action row has never been run.
	ErrActionExecRunNotFound = errors.New("this action has no recorded run")
	// ErrActionNotContainerRun is returned for a row a sub-agent ran: there is no
	// container behind it to read logs from.
	ErrActionNotContainerRun = errors.New("this action ran as a sub-agent; there is no container to show logs for")
	// ErrActionNotRunning is returned once a run has finished. The container
	// outlives it, but its account of the work is in the execute dialog by then.
	ErrActionNotRunning = errors.New("this action is not running; container logs are only available while it runs")
)

// ReadActionContainerLogs returns a snapshot of the agent-runner container's
// output for one running code action of the plan on planDialogID.
//
// A code action can run for tens of minutes and reports nothing until its
// webhook lands, so its container output is the only live account of what the
// coding agent is doing.
func (s *ChatService) ReadActionContainerLogs(
	ctx context.Context,
	planDialogID uuid.UUID,
	key string,
) (executor.ActionLogsResult, error) {
	if s.executorSvc == nil {
		return executor.ActionLogsResult{}, fmt.Errorf("read action container logs: executor service is not configured")
	}

	// The exec run is the authority on what is running, deliberately without a
	// second look at the action plan: a plan edited while its action runs must
	// not make the container unreachable.
	runs, err := s.ReadActionPlanExecRuns(planDialogID)
	if err != nil {
		return executor.ActionLogsResult{}, fmt.Errorf("read action container logs: %w", err)
	}

	run, ok := runs[key]
	switch {
	case !ok:
		return executor.ActionLogsResult{}, ErrActionExecRunNotFound
	// An empty Kind is an older record, from before container runs were told
	// apart from sub-agent ones; it has no container identity either.
	case run.Kind != ExecutionKindContainer:
		return executor.ActionLogsResult{}, ErrActionNotContainerRun
	case !run.Active():
		return executor.ActionLogsResult{}, ErrActionNotRunning
	}

	// Whether logs can be read at all is the executor's call, so the local
	// versus remote decision is made in exactly one place.
	return s.executorSvc.ReadActionLogs(ctx, executor.ActionLogsRequest{
		JobName:     run.JobName,
		ContainerID: run.ContainerID,
		Namespace:   run.Namespace,
	})
}
