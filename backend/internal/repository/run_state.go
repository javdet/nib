package repository

import (
	"context"

	"github.com/google/uuid"
)

// RunStateKind names one kind of per-dialog run-state document.
type RunStateKind string

const (
	RunStatePlanState   RunStateKind = "plan_state"
	RunStateFanout      RunStateKind = "plan_fanout"
	RunStateSubagents   RunStateKind = "subagents"
	RunStateCodeFixes   RunStateKind = "code_fixes"
	RunStateIncidentRun RunStateKind = "incident_run"
	RunStateActionExec  RunStateKind = "action_exec"
	RunStateActionRuns  RunStateKind = "action_runs"
	RunStateActionNotes RunStateKind = "action_notes"
	RunStateStageRun    RunStateKind = "stage_run"
)

// RunStateKinds lists every kind, in no particular order.
var RunStateKinds = []RunStateKind{
	RunStatePlanState,
	RunStateFanout,
	RunStateSubagents,
	RunStateCodeFixes,
	RunStateIncidentRun,
	RunStateActionExec,
	RunStateActionRuns,
	RunStateActionNotes,
	RunStateStageRun,
}

// RunStateEntry is one stored document.
type RunStateEntry struct {
	DialogID uuid.UUID
	Data     []byte
}

// RunStateRepository stores run-state documents as opaque JSON, one per kind
// and dialog. It does no locking of its own: a read-modify-write is guarded by
// the caller's lock, which is what keeps it usable from more than one process.
type RunStateRepository interface {
	// GetRunState returns ErrNotFound when the dialog has no document of kind.
	GetRunState(ctx context.Context, kind RunStateKind, dialogID uuid.UUID) ([]byte, error)
	// GetRunStates returns the documents of the given dialogs that exist.
	GetRunStates(ctx context.Context, kind RunStateKind, dialogIDs []uuid.UUID) (map[uuid.UUID][]byte, error)
	PutRunState(ctx context.Context, kind RunStateKind, dialogID uuid.UUID, data []byte) error
	// InsertRunState stores data only when the dialog has no document of kind
	// yet, and reports whether it did.
	InsertRunState(ctx context.Context, kind RunStateKind, dialogID uuid.UUID, data []byte) (bool, error)
	// DeleteRunState is not an error for a document that does not exist.
	DeleteRunState(ctx context.Context, kind RunStateKind, dialogID uuid.UUID) error
	ListRunStates(ctx context.Context, kind RunStateKind) ([]RunStateEntry, error)
	// ListRunningRunStates returns at least every document of kind holding a
	// "running" value anywhere, and may return more: it narrows what a sweep
	// decodes, the caller still decides.
	ListRunningRunStates(ctx context.Context, kind RunStateKind) ([]RunStateEntry, error)
}
