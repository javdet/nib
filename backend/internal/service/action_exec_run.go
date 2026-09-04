package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/metrics"
)

// ActionExecStatus is where a per-action sub-agent run ended up.
type ActionExecStatus string

const (
	// ActionExecRunning means a sub-agent holds the row and is working.
	ActionExecRunning ActionExecStatus = "running"
	// ActionExecDone means the sub-agent finished its turn without erroring.
	// It is not a claim that the action succeeded — only the operator's
	// checkbox says that.
	ActionExecDone ActionExecStatus = "done"
	// ActionExecFailed means the run errored or timed out.
	ActionExecFailed ActionExecStatus = "failed"
	// ActionExecBlocked means the sub-agent tried to ask the operator a
	// question. It cannot suspend, so the run ends here rather than pretending
	// to have finished the work.
	ActionExecBlocked ActionExecStatus = "blocked"
	// ActionExecCancelled means a restart or a shutdown took the run away.
	ActionExecCancelled ActionExecStatus = "cancelled"
)

// ActionExecRun is the record of one attempt at one action row.
//
// It deliberately carries neither the row's number nor its dialog id. The number
// is derived from position and goes stale the moment a stage is reordered, and
// the dialog id lives in {plan}.runs.json, which the agent-runner webhook
// reverse-looks-up to attach a pull request. Two maps of the same fact would
// drift.
type ActionExecRun struct {
	Status     ActionExecStatus `json:"status"`
	StartedAt  int64            `json:"startedAt"`
	FinishedAt int64            `json:"finishedAt,omitempty"`
	Error      string           `json:"error,omitempty"`
	Attempt    int              `json:"attempt"`
	// Kind separates a sub-agent run from a container run: the two are stopped
	// in completely different ways.
	Kind ExecutionKind `json:"kind,omitempty"`
	// JobName, ContainerID and Namespace identify a code action's agent-runner
	// container. They are here because there is nowhere else: the container
	// outlives the process that started it, and before this the job name only
	// ever appeared in the sentence the tool returned to the model -- which is
	// exactly why a code action could not be stopped.
	JobName     string `json:"jobName,omitempty"`
	ContainerID string `json:"containerId,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
}

// Active reports whether a run still holds its row.
func (r ActionExecRun) Active() bool {
	return r.Status == ActionExecRunning
}

// ActionExecRuns maps an action row key to its latest run.
type ActionExecRuns map[string]ActionExecRun

func actionPlanExecPath(dir string, dialogID uuid.UUID) string {
	return filepath.Join(dir, dialogID.String()+".exec.json")
}

// ReadActionPlanExecRuns returns the latest sub-agent run per action row key.
func (s *ChatService) ReadActionPlanExecRuns(dialogID uuid.UUID) (ActionExecRuns, error) {
	path := actionPlanExecPath(s.actionPlansDir, dialogID)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ActionExecRuns{}, nil
	}
	if err != nil {
		return nil, err
	}

	var runs ActionExecRuns
	if err := json.Unmarshal(b, &runs); err != nil {
		return nil, fmt.Errorf("unmarshal action plan exec runs: %w", err)
	}
	if runs == nil {
		return ActionExecRuns{}, nil
	}
	return runs, nil
}

// WriteActionPlanExecRuns persists the run record.
func (s *ChatService) WriteActionPlanExecRuns(dialogID uuid.UUID, runs ActionExecRuns) error {
	if runs == nil {
		runs = ActionExecRuns{}
	}

	if err := os.MkdirAll(s.actionPlansDir, 0o755); err != nil {
		return fmt.Errorf("create action_plans directory: %w", err)
	}

	data, err := json.Marshal(runs)
	if err != nil {
		return fmt.Errorf("marshal action plan exec runs: %w", err)
	}

	return writeActionPlanFile(actionPlanExecPath(s.actionPlansDir, dialogID), data)
}

// updateActionPlanExecRun applies mutate to one row's record and stores the
// result, returning what was written. It takes the plan lock: the read and the
// write are a single logical step, and sub-agent goroutines run concurrently.
func (s *ChatService) updateActionPlanExecRun(
	planID uuid.UUID,
	key string,
	mutate func(*ActionExecRun),
) (ActionExecRun, error) {
	mu := s.planMutex(planID)
	mu.Lock()
	defer mu.Unlock()

	runs, err := s.ReadActionPlanExecRuns(planID)
	if err != nil {
		return ActionExecRun{}, fmt.Errorf("read action plan exec runs: %w", err)
	}

	run := runs[key]
	mutate(&run)
	runs[key] = run

	if err := s.WriteActionPlanExecRuns(planID, runs); err != nil {
		return ActionExecRun{}, fmt.Errorf("write action plan exec runs: %w", err)
	}
	return run, nil
}

// startActionExecRun records a fresh attempt at a row, carrying the attempt
// counter forward so a restart is visible as such.
func (s *ChatService) startActionExecRun(planID uuid.UUID, key string) (ActionExecRun, error) {
	return s.updateActionPlanExecRun(planID, key, func(run *ActionExecRun) {
		attempt := run.Attempt + 1
		*run = ActionExecRun{
			Status:    ActionExecRunning,
			StartedAt: time.Now().Unix(),
			Attempt:   attempt,
			Kind:      ExecutionKindSubagent,
		}
		metrics.RecordActionExecStarted(string(ExecutionKindSubagent))
	})
}

// finishActionExecRun closes a run and returns what the record ended up holding.
// A run already closed by something else — a restart that cancelled it, say — is
// left as it is, so the record reflects what actually stopped it.
func (s *ChatService) finishActionExecRun(planID uuid.UUID, key string, status ActionExecStatus, errMsg string) ActionExecRun {
	run, err := s.updateActionPlanExecRun(planID, key, func(run *ActionExecRun) {
		if !run.Active() {
			return
		}
		run.Status = status
		run.FinishedAt = time.Now().Unix()
		run.Error = errMsg
		// Recorded inside the closure, past the Active guard: a run something
		// else already closed must not be counted a second time.
		var ran time.Duration
		if run.StartedAt > 0 {
			ran = time.Duration(run.FinishedAt-run.StartedAt) * time.Second
		}
		metrics.RecordActionExecFinished(string(status), ran)
	})
	if err != nil {
		// The sub-agent has already done its work; failing to record the
		// outcome is worth a log, not a lost result.
		slog.Warn("finish action exec run", "plan_id", planID, "key", key, "error", err)
	}
	return run
}
