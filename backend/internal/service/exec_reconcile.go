package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/repository"
)

// restartedReason is what a run left behind by a previous process is closed with.
// It is the operator-facing explanation, so it says what happened rather than
// naming the mechanism.
const restartedReason = "the server restarted while this was running"

// ReconcileResult reports what the startup sweep closed.
type ReconcileResult struct {
	Actions   int
	Fanouts   int
	CodeFixes int
	StageRuns int
	Incidents int
}

// Total is every record the sweep closed.
func (r ReconcileResult) Total() int {
	return r.Actions + r.Fanouts + r.CodeFixes + r.StageRuns + r.Incidents
}

// ReconcileStuckRuns closes every run record still marked running, which at the
// boot of a lone process can only be what the previous process left behind.
func (s *ChatService) ReconcileStuckRuns() (ReconcileResult, error) {
	return s.SweepOrphanedRuns(nil)
}

// SweepOrphanedRuns closes the run records whose process is gone: those whose
// owner alive does not recognise. A nil alive recognises nobody.
//
// A sub-agent's goroutine and a code action's webhook both die with the process,
// so without this sweep every row that was working stays "running" -- a
// permanent spinner in the plan view, and, now that only one execution runs at a
// time, a permanent block on every other action in every plan. A fan-out left
// running is the same bug one level up: it makes ErrFanoutInProgress refuse
// every replan forever.
//
// With several processes sharing the database a record of a live one is left
// alone: a process booting, or one noticing that a peer has died, must not
// close what another is still running. A record with no owner predates
// ownership and is treated as orphaned.
//
// A record that cannot be read or parsed is logged and stepped over: a plan
// nobody can parse is not a reason to refuse to start.
func (s *ChatService) SweepOrphanedRuns(alive func(owner string) bool) (ReconcileResult, error) {
	orphaned := func(owner string) bool {
		return owner == "" || alive == nil || !alive(owner)
	}
	var out ReconcileResult

	// The lease of a process that is gone goes with it, as an in-memory lease
	// went with a restart: its run is closed below, and a lease left behind
	// would refuse every execution for up to the action deadline in the name
	// of a run already marked failed.
	if held, ok := s.currentLease(); ok && orphaned(held.Owner) {
		if dropped, ok := s.dropLeaseIf(func(l ExecutionLease) bool { return l.token == held.token }); ok {
			slog.Info("dropped the execution lease of a process that is gone",
				"owner", dropped.Owner, "plan_id", dropped.PlanID, "key", dropped.Key)
		}
	}

	actions, err := s.reconcileStuckActionRuns(orphaned)
	if err != nil {
		return out, err
	}
	out.Actions = actions

	fanouts, err := s.reconcileStuckFanoutRuns(orphaned)
	if err != nil {
		return out, err
	}
	out.Fanouts = fanouts

	fixes, err := s.reconcileStuckCodeFixRuns(orphaned)
	if err != nil {
		return out, err
	}
	out.CodeFixes = fixes

	stageRuns, err := s.reconcileStuckStageRuns(orphaned)
	if err != nil {
		return out, err
	}
	out.StageRuns = stageRuns

	incidents, err := s.reconcileStuckIncidentRuns(orphaned)
	if err != nil {
		return out, err
	}
	out.Incidents = incidents

	return out, nil
}

// sweepLockWait bounds how long the sweep waits for one record's lock.
const sweepLockWait = 30 * time.Second

// sweepRunState applies close to every document of kind and stores the ones it
// changed. close reports how many runs it closed.
//
// Each document is re-read and rewritten under lock, the same lock its ordinary
// writers take: the sweep also runs while other processes are working, and
// writing back a stale copy would undo what they recorded since the list.
func sweepRunState[T any](
	s *ChatService,
	kind repository.RunStateKind,
	lock func(uuid.UUID) *namedLock,
	close func(id uuid.UUID, doc *T) int,
) (int, error) {
	entries, err := s.listRunningRunStates(kind)
	if err != nil {
		return 0, fmt.Errorf("list %s: %w", kind, err)
	}

	closed := 0
	for _, e := range entries {
		// A cheap look first, so documents with nothing to close are never
		// locked.
		var peek T
		if err := json.Unmarshal(e.Data, &peek); err != nil {
			slog.Warn("reconcile runs: parse record", "kind", kind, "dialog_id", e.DialogID, "error", err)
			continue
		}
		if close(e.DialogID, &peek) == 0 {
			continue
		}

		n, err := func() (int, error) {
			// Bounded: the sweep shares nothing with the heartbeat any more, but
			// a record whose lock is held for long by a live writer is better
			// left for the next pass than waited on.
			mu := lock(e.DialogID)
			ctx, cancel := context.WithTimeout(context.Background(), sweepLockWait)
			defer cancel()
			if err := mu.LockContext(ctx); err != nil {
				return 0, fmt.Errorf("lock: %w", err)
			}
			defer mu.Unlock()

			var doc T
			found, err := s.loadRunState(kind, e.DialogID, &doc)
			if err != nil || !found {
				return 0, err
			}
			n := close(e.DialogID, &doc)
			if n == 0 {
				return 0, nil
			}
			return n, s.saveRunState(kind, e.DialogID, doc)
		}()
		if err != nil {
			slog.Warn("reconcile runs: rewrite record", "kind", kind, "dialog_id", e.DialogID, "error", err)
			continue
		}
		closed += n
	}
	return closed, nil
}

// reconcileStuckIncidentRuns closes investigations and closures a dead process
// left running, without which ErrInvestigationInProgress would refuse every
// later run of those incidents.
func (s *ChatService) reconcileStuckIncidentRuns(orphaned func(string) bool) (int, error) {
	now := time.Now().Unix()
	lock := func(id uuid.UUID) *namedLock { return s.lockFor("incident-run:" + id.String()) }
	return sweepRunState(s, repository.RunStateIncidentRun, lock, func(_ uuid.UUID, run *IncidentRun) int {
		if !run.Active() || !orphaned(run.Owner) {
			return 0
		}
		run.Status = IncidentRunFailed
		run.FinishedAt = now
		run.Error = restartedReason
		for i := range run.Steps {
			if run.Steps[i].Status == IncidentStepPending || run.Steps[i].Status == IncidentStepRunning {
				run.Steps[i].Status = IncidentStepFailed
				run.Steps[i].FinishedAt = now
				run.Steps[i].Error = restartedReason
			}
		}
		return 1
	})
}

func (s *ChatService) reconcileStuckActionRuns(orphaned func(string) bool) (int, error) {
	now := time.Now().Unix()
	lock := func(id uuid.UUID) *namedLock { return s.planMutex(id) }
	return sweepRunState(s, repository.RunStateActionExec, lock, func(_ uuid.UUID, runs *ActionExecRuns) int {
		closed := 0
		for key, run := range *runs {
			if !run.Active() || !orphaned(run.Owner) {
				continue
			}
			run.Status = ActionExecFailed
			run.FinishedAt = now
			run.Error = restartedReason
			(*runs)[key] = run
			closed++
		}
		return closed
	})
}

func (s *ChatService) reconcileStuckFanoutRuns(orphaned func(string) bool) (int, error) {
	now := time.Now().Unix()
	lock := func(id uuid.UUID) *namedLock { return s.fanoutMutex(id) }
	return sweepRunState(s, repository.RunStateFanout, lock, func(_ uuid.UUID, run *FanoutRun) int {
		if !run.Active() || !orphaned(run.Owner) {
			return 0
		}
		run.Status = FanoutRunFailed
		run.FinishedAt = now
		run.Error = restartedReason
		for i := range run.Stages {
			// A stage waiting on an answer is as dead as one mid-completion:
			// the channel the answer would have arrived on went with the
			// process.
			switch run.Stages[i].Status {
			case FanoutStageRunning, FanoutStageAwaitingInput:
				run.Stages[i].Status = FanoutStageFailed
				run.Stages[i].Error = restartedReason
			}
		}
		return 1
	})
}

// reconcileStuckCodeFixRuns closes the ad-hoc code fixes a dead process left
// running. Their containers report through the same webhook as a code action's,
// so they are stranded by a restart in the same way -- and, holding the one
// execution slot, would block every action in every plan.
func (s *ChatService) reconcileStuckCodeFixRuns(orphaned func(string) bool) (int, error) {
	now := time.Now().Unix()
	lock := func(id uuid.UUID) *namedLock { return s.codeFixMutex(id) }
	return sweepRunState(s, repository.RunStateCodeFixes, lock, func(_ uuid.UUID, runs *CodeFixRuns) int {
		closed := 0
		for id, run := range *runs {
			if !run.Active() || !orphaned(run.Owner) {
				continue
			}
			run.Status = ActionExecFailed
			run.FinishedAt = now
			run.Error = restartedReason
			(*runs)[id] = run
			closed++
		}
		return closed
	})
}

// reconcileStuckStageRuns stops the stage runs a dead process left going. The
// item a run was on has just been closed by reconcileStuckActionRuns, and the
// hook that would have moved the run on died with the process -- so without
// this the run would claim to be going forever, and refuse every other one.
func (s *ChatService) reconcileStuckStageRuns(orphaned func(string) bool) (int, error) {
	now := time.Now().Unix()
	lock := func(uuid.UUID) *namedLock { return s.stageRunLock() }
	return sweepRunState(s, repository.RunStateStageRun, lock, func(_ uuid.UUID, run *StageRun) int {
		if !run.Active() || !orphaned(run.Owner) {
			return 0
		}
		run.Status = StageRunStopped
		run.FinishedAt = now
		run.Reason = restartedReason
		return 1
	})
}
