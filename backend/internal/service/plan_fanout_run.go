package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
)

type FanoutStageStatus string

const (
	FanoutStagePending FanoutStageStatus = "pending"
	FanoutStageRunning FanoutStageStatus = "running"
	FanoutStageDone    FanoutStageStatus = "done"
	FanoutStageFailed  FanoutStageStatus = "failed"
)

type FanoutRunStatus string

const (
	FanoutRunRunning       FanoutRunStatus = "running"
	FanoutRunAwaitingInput FanoutRunStatus = "awaiting_input"
	FanoutRunDone          FanoutRunStatus = "done"
	FanoutRunFailed        FanoutRunStatus = "failed"
)

// FanoutStageKind separates the units of planning work a run holds. A DAG stage
// is addressed by title; the rollback agent is addressed by kind alone, so a DAG
// stage that happens to be titled "Rollback plan" cannot be mistaken for it.
type FanoutStageKind string

const (
	// FanoutStageKindStage is the zero value: an ordinary DAG stage.
	FanoutStageKindStage FanoutStageKind = ""
	// FanoutStageKindRollback marks the agent that writes the plan's rollback
	// list once every stage has been planned.
	FanoutStageKindRollback FanoutStageKind = "rollback"
)

// FanoutStage is one unit of planning work handed to its own subagent: a DAG
// stage, or -- once they are all written -- the plan's rollback list.
type FanoutStage struct {
	Title  string            `json:"title"`
	Wave   int               `json:"wave"`
	Status FanoutStageStatus `json:"status"`
	// Kind is empty for a DAG stage. Entries that carry one are addressed by it
	// and never by title.
	Kind FanoutStageKind `json:"kind,omitempty"`
	// DialogID is the subagent's own dialog, kept so its research can be read
	// back after the fact.
	DialogID string `json:"dialogId,omitempty"`
	// Error explains a failed entry, and on a done one why it was skipped
	// without running an agent. The interface shows it as the row's tooltip
	// either way, which is where the operator looks for "why is this empty".
	Error string `json:"error,omitempty"`
}

// PlanBlocker is a question a stage subagent could not answer for itself. It
// never stops that stage: the subagent records the assumption it made instead
// and stores its stage anyway, so a blocker costs a correction, not a gap.
type PlanBlocker struct {
	Stage string `json:"stage"`
	// Kind is empty for a question raised by a DAG stage, and matches the
	// FanoutStage that raised it otherwise.
	Kind       FanoutStageKind `json:"kind,omitempty"`
	Question   string          `json:"question"`
	Options    []string        `json:"options,omitempty"`
	Assumption string          `json:"assumption,omitempty"`
	Answer     string          `json:"answer,omitempty"`
}

// FanoutRun is the record of one plan fan-out over a decompose dialog's DAG. It
// is persisted rather than held in memory so the browser can re-attach after a
// reload, which is also why the fan-out needs no database migration.
type FanoutRun struct {
	RunID      string          `json:"runId"`
	Status     FanoutRunStatus `json:"status"`
	StartedAt  int64           `json:"startedAt"`
	FinishedAt int64           `json:"finishedAt,omitempty"`
	Stages     []FanoutStage   `json:"stages"`
	Blockers   []PlanBlocker   `json:"blockers,omitempty"`
	// PendingAskID is the synthesised ask_question tool call whose answers the
	// next run consumes, and PendingStages the stages that run will redo.
	PendingAskID  string   `json:"pendingAskId,omitempty"`
	PendingStages []string `json:"pendingStages,omitempty"`
	// PendingRollback records that the rollback agent is among the work the next
	// run redoes, so a reloaded page knows what the pending question covers.
	PendingRollback bool   `json:"pendingRollback,omitempty"`
	Error           string `json:"error,omitempty"`
}

// Active reports whether a run still owns the plan, so a second fan-out over the
// same dialog can be refused rather than interleaved.
func (r FanoutRun) Active() bool {
	return r.Status == FanoutRunRunning
}

func (s *ChatService) fanoutMutex(dialogID uuid.UUID) *sync.Mutex {
	mu, _ := s.fanoutWriteMu.LoadOrStore(dialogID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

func (s *ChatService) fanoutRunPath(dialogID uuid.UUID) string {
	return filepath.Join(s.planFanoutDir, dialogID.String()+".json")
}

// ReadFanoutRun returns the stored run for a dialog, or false when none exists.
func (s *ChatService) ReadFanoutRun(dialogID uuid.UUID) (FanoutRun, bool, error) {
	b, err := os.ReadFile(s.fanoutRunPath(dialogID))
	if errors.Is(err, os.ErrNotExist) {
		return FanoutRun{}, false, nil
	}
	if err != nil {
		return FanoutRun{}, false, err
	}

	var run FanoutRun
	if err := json.Unmarshal(b, &run); err != nil {
		return FanoutRun{}, false, fmt.Errorf("unmarshal fanout run: %w", err)
	}
	return run, true, nil
}

func (s *ChatService) writeFanoutRun(dialogID uuid.UUID, run FanoutRun) error {
	if err := os.MkdirAll(s.planFanoutDir, 0o755); err != nil {
		return fmt.Errorf("create plan_fanout directory: %w", err)
	}
	data, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("marshal fanout run: %w", err)
	}
	return atomicfile.Write(s.fanoutRunPath(dialogID), data)
}

// updateFanoutRun applies mutate to the stored run under the dialog's lock.
// Stage subagents report blockers while the runner marks stages done, so every
// read-modify-write of this file goes through here.
func (s *ChatService) updateFanoutRun(dialogID uuid.UUID, mutate func(*FanoutRun)) (FanoutRun, error) {
	mu := s.fanoutMutex(dialogID)
	mu.Lock()
	defer mu.Unlock()

	run, found, err := s.ReadFanoutRun(dialogID)
	if err != nil {
		return FanoutRun{}, err
	}
	if !found {
		return FanoutRun{}, fmt.Errorf("no plan fan-out run for dialog %s", dialogID)
	}

	mutate(&run)
	if err := s.writeFanoutRun(dialogID, run); err != nil {
		return FanoutRun{}, err
	}
	return run, nil
}

// setFanoutStage records the outcome of one DAG stage. Entries that carry a kind
// are stepped over: they are addressed by kind, and a DAG stage may legitimately
// share their title.
func (s *ChatService) setFanoutStage(dialogID uuid.UUID, title string, apply func(*FanoutStage)) error {
	_, err := s.updateFanoutRun(dialogID, func(run *FanoutRun) {
		for i := range run.Stages {
			if run.Stages[i].Kind != FanoutStageKindStage {
				continue
			}
			if normalizeStageTitle(run.Stages[i].Title) == normalizeStageTitle(title) {
				apply(&run.Stages[i])
				return
			}
		}
	})
	return err
}

// setFanoutRollback records the outcome of the rollback agent. It matches on kind
// alone, so the title it is displayed under is free to collide with a DAG stage.
func (s *ChatService) setFanoutRollback(dialogID uuid.UUID, apply func(*FanoutStage)) error {
	_, err := s.updateFanoutRun(dialogID, func(run *FanoutRun) {
		for i := range run.Stages {
			if run.Stages[i].Kind == FanoutStageKindRollback {
				apply(&run.Stages[i])
				return
			}
		}
	})
	return err
}
