package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/executor"
)

// planContainerCleanupTimeout bounds the container sweep a cancel runs. It is
// detached from the request, so an operator closing the tab cannot leave it
// half done, and the bound is what keeps a wedged Docker daemon from holding it.
const planContainerCleanupTimeout = 30 * time.Second

// PlanStatusChange is what setting a plan's status did.
type PlanStatusChange struct {
	Status ActionPlanStatus
	// ContainersRemoved and CleanupErr report the agent containers removed
	// because the plan was cancelled. A failed cleanup does not undo the
	// cancel: the operator's decision stands, and the error says what is
	// still left on the host.
	ContainersRemoved int
	CleanupErr        error
}

// SetPlanStatus sets a plan's status as the operator asked.
//
// Cancelling also removes the plan's agent containers when the local executor
// is set to. That runs on every cancel, not only the first, so pressing it
// again retries a cleanup that failed.
func (s *ChatService) SetPlanStatus(ctx context.Context, planID uuid.UUID, status ActionPlanStatus) (PlanStatusChange, error) {
	state, err := s.ReadPlanState(planID)
	if err != nil {
		return PlanStatusChange{}, err
	}
	state.Status = status
	if err := s.WritePlanState(planID, state); err != nil {
		return PlanStatusChange{}, err
	}

	change := PlanStatusChange{Status: state.Status}
	if status == ActionPlanStatusCancelled {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), planContainerCleanupTimeout)
		defer cancel()
		change.ContainersRemoved, change.CleanupErr = s.removeCancelledPlanContainers(cleanupCtx, planID)
		if change.CleanupErr != nil {
			slog.Warn("remove cancelled plan containers",
				"plan_id", planID, "removed", change.ContainersRemoved, "error", change.CleanupErr)
		} else if change.ContainersRemoved > 0 {
			slog.Info("removed cancelled plan containers",
				"plan_id", planID, "removed", change.ContainersRemoved)
		}
	}
	return change, nil
}

// removeCancelledPlanContainers stops the plan's running agent container and
// removes every container the plan launched, when the local executor is set
// to remove them on cancel.
func (s *ChatService) removeCancelledPlanContainers(ctx context.Context, planID uuid.UUID) (int, error) {
	if s.executorSvc == nil {
		return 0, nil
	}
	cfg, err := s.executorSvc.ConfigStore().Get()
	if err != nil {
		return 0, fmt.Errorf("read executor config: %w", err)
	}
	if cfg.Type != executor.TypeLocal || !cfg.RemoveContainersOnPlanCancel {
		return 0, nil
	}

	// A container still holding the lease is stopped through it rather than
	// just removed: that closes its row or fix as cancelled and frees the
	// slot now, where a bare removal would leave the lease held against a run
	// that can never report back until the lease deadline.
	if held, ok := s.takeExecutionLeaseIf(func(l ExecutionLease) bool {
		return l.PlanID == planID && l.Kind == ExecutionKindContainer
	}); ok {
		if !held.Fix {
			s.stopStageRunOnItem(held.PlanID, held.Key, stageRunStoppedByOperator)
		}
		if _, err := s.cancelHeldExecution(ctx, held); err != nil {
			// The sweep below force-removes it anyway.
			slog.Warn("stop running container of a cancelled plan",
				"plan_id", planID, "job", held.JobName, "error", err)
		}
	}

	return s.executorSvc.RemovePlanContainers(ctx, executor.RemovePlanContainersRequest{
		PlanID:  planID.String(),
		Targets: s.recordedPlanContainers(planID),
	})
}

// recordedPlanContainers lists the containers the plan's own records name.
// The label finds every container launched since it was introduced; these
// reach the latest one per row and per fix launched before that.
func (s *ChatService) recordedPlanContainers(planID uuid.UUID) []string {
	var targets []string
	add := func(containerID, jobName string) {
		if id := strings.TrimSpace(containerID); id != "" {
			targets = append(targets, id)
			return
		}
		if name := strings.TrimSpace(jobName); name != "" {
			targets = append(targets, name)
		}
	}

	if runs, err := s.ReadActionPlanExecRuns(planID); err == nil {
		for _, run := range runs {
			if run.Kind == ExecutionKindContainer {
				add(run.ContainerID, run.JobName)
			}
		}
	} else {
		slog.Warn("read exec runs of a cancelled plan", "plan_id", planID, "error", err)
	}

	if fixes, err := s.ReadCodeFixRuns(planID); err == nil {
		for _, fix := range fixes {
			add(fix.ContainerID, fix.JobName)
		}
	} else {
		slog.Warn("read code fixes of a cancelled plan", "plan_id", planID, "error", err)
	}
	return targets
}
