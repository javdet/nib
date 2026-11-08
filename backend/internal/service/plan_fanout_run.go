package service

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/metrics"
	"github.com/javdet/nib/internal/repository"
)

type FanoutStageStatus string

const (
	FanoutStagePending FanoutStageStatus = "pending"
	FanoutStageRunning FanoutStageStatus = "running"
	// FanoutStageAwaitingInput marks a stage whose subagent is blocked on a
	// question it put to the operator. It is not terminal: the agent is alive
	// and goes back to running the moment an answer lands.
	FanoutStageAwaitingInput FanoutStageStatus = "awaiting_input"
	FanoutStageDone          FanoutStageStatus = "done"
	FanoutStageFailed        FanoutStageStatus = "failed"
)

type FanoutRunStatus string

const (
	FanoutRunRunning FanoutRunStatus = "running"
	FanoutRunDone    FanoutRunStatus = "done"
	FanoutRunFailed  FanoutRunStatus = "failed"
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
	// Carried marks a row the current run does not plan, kept from the run
	// before it so the list is always the whole plan rather than the round. Its
	// status, dialog and error are the earlier run's.
	Carried bool `json:"carried,omitempty"`
}

// PlanBlocker is a question a stage subagent could not answer for itself. The
// subagent blocks on it: the question goes to the operator naming the stage that
// raised it, and the answer comes back as the tool's own result. The assumption
// is what it falls back on when no answer arrives, so a blocker never leaves a
// gap in the plan.
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

// FanoutRun is the record of one plan fan-out over a plan's DAG. It
// is persisted rather than held in memory so the browser can re-attach after a
// reload, which is also why the fan-out needs no database migration.
type FanoutRun struct {
	RunID      string          `json:"runId"`
	Status     FanoutRunStatus `json:"status"`
	StartedAt  int64           `json:"startedAt"`
	FinishedAt int64           `json:"finishedAt,omitempty"`
	Stages     []FanoutStage   `json:"stages"`
	Blockers   []PlanBlocker   `json:"blockers,omitempty"`
	Error      string          `json:"error,omitempty"`
	// Rebuilt marks a run that discarded an action plan built for an older DAG
	// before it started, so it plans over nothing rather than over that plan.
	Rebuilt bool `json:"rebuilt,omitempty"`
	// Owner is the backend process that started it. A sweep closes a running
	// record only once its owner is gone, so one process restarting never
	// closes what another is still running.
	Owner string `json:"owner,omitempty"`
}

// Active reports whether a run still owns the plan, so a second fan-out over the
// same dialog can be refused rather than interleaved.
func (r FanoutRun) Active() bool {
	return r.Status == FanoutRunRunning
}

func (s *ChatService) fanoutMutex(dialogID uuid.UUID) *namedLock {
	return s.lockFor("fanout:" + dialogID.String())
}

// ReadFanoutRun returns the stored run for a dialog, or false when none exists.
func (s *ChatService) ReadFanoutRun(dialogID uuid.UUID) (FanoutRun, bool, error) {
	var run FanoutRun
	found, err := s.loadRunState(repository.RunStateFanout, dialogID, &run)
	if err != nil {
		return FanoutRun{}, false, fmt.Errorf("read fanout run: %w", err)
	}
	return run, found, nil
}

func (s *ChatService) writeFanoutRun(dialogID uuid.UUID, run FanoutRun) error {
	if err := s.saveRunState(repository.RunStateFanout, dialogID, run); err != nil {
		return fmt.Errorf("write fanout run: %w", err)
	}
	return nil
}

// updateFanoutRun applies mutate to the stored run under the dialog's lock.
// Stage subagents report blockers while the runner marks stages done, so every
// read-modify-write of the record goes through here.
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
				recordFanoutStage(run.Stages[i])
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
				recordFanoutStage(run.Stages[i])
				return
			}
		}
	})
	return err
}

// setFanoutStageOfKind records the outcome of whichever unit of work a kind and
// a title name. The blocker handler is bound to one of the two and knows only
// that pair, so the choice between the addressing schemes is made here.
func (s *ChatService) setFanoutStageOfKind(dialogID uuid.UUID, title string, kind FanoutStageKind, apply func(*FanoutStage)) error {
	if kind == FanoutStageKindStage {
		return s.setFanoutStage(dialogID, title, apply)
	}
	return s.setFanoutRollback(dialogID, apply)
}

// recordFanoutStage counts a stage that has reached a terminal status. pending
// and running are skipped: a stage passes through them on the way, and counting
// them would make the totals a multiple of the stages rather than a count.
func recordFanoutStage(st FanoutStage) {
	switch st.Status {
	case FanoutStageDone, FanoutStageFailed:
	default:
		return
	}
	kind := "stage"
	if st.Kind == FanoutStageKindRollback {
		kind = "rollback"
	}
	metrics.RecordFanoutStage(kind, string(st.Status))
}
