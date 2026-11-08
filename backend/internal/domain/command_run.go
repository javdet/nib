package domain

import (
	"time"

	"github.com/google/uuid"
)

// CommandRunStatus is where one agent command job stands.
type CommandRunStatus string

const (
	CommandRunRunning   CommandRunStatus = "running"
	CommandRunSucceeded CommandRunStatus = "succeeded"
	CommandRunFailed    CommandRunStatus = "failed"
	CommandRunTimedOut  CommandRunStatus = "timed_out"
	CommandRunCancelled CommandRunStatus = "cancelled"
	// CommandRunLost is a run a restart stranded: the process died with the
	// backend, so nothing will ever report how it ended.
	CommandRunLost CommandRunStatus = "lost"
)

// Terminal reports whether nothing will change the run any more.
func (s CommandRunStatus) Terminal() bool {
	return s != CommandRunRunning
}

// CommandRunKind says which tool started a run.
type CommandRunKind string

const (
	CommandRunKindRun       CommandRunKind = "run"
	CommandRunKindWaitUntil CommandRunKind = "wait_until"
)

// CommandRun is the audit record of one command an agent ran through
// run_command or wait_until.
type CommandRun struct {
	ID uuid.UUID `json:"id"`
	// PlanID is the root dialog the run belongs to, uuid.Nil for the stateless
	// chat endpoint. DialogID is the transcript that ran it.
	PlanID    uuid.UUID      `json:"plan_id"`
	DialogID  uuid.UUID      `json:"dialog_id"`
	ActionKey string         `json:"action_key,omitempty"`
	Mode      string         `json:"mode"`
	Kind      CommandRunKind `json:"kind"`
	Script    string         `json:"script"`
	// Isolated is false when the run shared the backend's uid, which is what an
	// install without the capabilities to switch falls back to.
	Isolated     bool             `json:"isolated"`
	UID          int              `json:"uid"`
	Status       CommandRunStatus `json:"status"`
	ExitCode     *int             `json:"exit_code,omitempty"`
	Reason       string           `json:"reason,omitempty"`
	OutputBytes  int64            `json:"output_bytes"`
	OutputCapped bool             `json:"output_capped"`
	StartedAt    time.Time        `json:"started_at"`
	FinishedAt   *time.Time       `json:"finished_at,omitempty"`
	DurationMS   int64            `json:"duration_ms"`
}
