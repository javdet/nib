package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// restartedReason is what a run left behind by a previous process is closed with.
// It is the operator-facing explanation, so it says what happened rather than
// naming the mechanism.
const restartedReason = "the server restarted while this was running"

// ReconcileResult reports what the startup sweep closed.
type ReconcileResult struct {
	Actions int
	Fanouts int
}

// ReconcileStuckRuns closes the run records a previous process left behind.
//
// A sub-agent's goroutine and a code action's webhook both die with the process,
// so without this sweep every row that was working stays "running" -- a
// permanent spinner in the plan view, and, now that only one execution runs at a
// time, a permanent block on every other action in every plan. A fan-out left
// running is the same bug one level up: it makes ErrFanoutInProgress refuse
// every replan forever.
//
// A file that cannot be read or parsed is logged and stepped over: a plan nobody
// can parse is not a reason to refuse to start.
func (s *ChatService) ReconcileStuckRuns() (ReconcileResult, error) {
	var out ReconcileResult

	actions, err := s.reconcileStuckActionRuns()
	if err != nil {
		return out, err
	}
	out.Actions = actions

	fanouts, err := s.reconcileStuckFanoutRuns()
	if err != nil {
		return out, err
	}
	out.Fanouts = fanouts

	return out, nil
}

func (s *ChatService) reconcileStuckActionRuns() (int, error) {
	paths, err := filepath.Glob(filepath.Join(s.actionPlansDir, "*.exec.json"))
	if err != nil {
		return 0, fmt.Errorf("list action exec records: %w", err)
	}

	closed := 0
	now := time.Now().Unix()
	for _, path := range paths {
		var runs ActionExecRuns
		if !readJSONFileOrWarn(path, &runs) {
			continue
		}

		changed := false
		for key, run := range runs {
			if !run.Active() {
				continue
			}
			run.Status = ActionExecFailed
			run.FinishedAt = now
			run.Error = restartedReason
			runs[key] = run
			changed = true
			closed++
		}
		if !changed {
			continue
		}

		writeJSONFileOrWarn(path, runs)
	}
	return closed, nil
}

func (s *ChatService) reconcileStuckFanoutRuns() (int, error) {
	paths, err := filepath.Glob(filepath.Join(s.planFanoutDir, "*.json"))
	if err != nil {
		return 0, fmt.Errorf("list plan fan-out records: %w", err)
	}

	closed := 0
	now := time.Now().Unix()
	for _, path := range paths {
		var run FanoutRun
		if !readJSONFileOrWarn(path, &run) {
			continue
		}
		if !run.Active() {
			continue
		}

		run.Status = FanoutRunFailed
		run.FinishedAt = now
		run.Error = restartedReason
		for i := range run.Stages {
			if run.Stages[i].Status == FanoutStageRunning {
				run.Stages[i].Status = FanoutStageFailed
				run.Stages[i].Error = restartedReason
			}
		}
		closed++

		writeJSONFileOrWarn(path, run)
	}
	return closed, nil
}

// readJSONFileOrWarn decodes path into v. It reports false, with a warning, for
// anything it could not read -- a file removed between the glob and the read
// included.
func readJSONFileOrWarn(path string, v any) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("reconcile runs: read record", "path", path, "error", err)
		}
		return false
	}
	if err := json.Unmarshal(b, v); err != nil {
		slog.Warn("reconcile runs: parse record", "path", path, "error", err)
		return false
	}
	return true
}

func writeJSONFileOrWarn(path string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Warn("reconcile runs: marshal record", "path", path, "error", err)
		return
	}
	if err := writeActionPlanFile(path, data); err != nil {
		slog.Warn("reconcile runs: write record", "path", path, "error", err)
	}
}
