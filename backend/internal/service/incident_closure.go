package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// StartIncidentClosure closes an incident: the impact is assessed once more
// over the final window, then a writer composes the closure summary, and only
// when both have finished is the incident marked closed. A closure that fails
// leaves the incident open, so pressing Close again retries it.
//
// resolvedAt, when given, is recorded as the resolution time; otherwise an
// incident with none is taken to be resolved now.
func (s *ChatService) StartIncidentClosure(ctx context.Context, rootID uuid.UUID, resolvedAt string) (IncidentRun, error) {
	if _, err := s.resolveIncidentRoot(ctx, rootID); err != nil {
		return IncidentRun{}, err
	}
	if err := s.refuseBusyOrClosed(rootID); err != nil {
		return IncidentRun{}, err
	}

	// The impact agent needs the same tools the Diggers used to measure it.
	var categories []string
	if previous, ok, err := s.ReadIncidentRun(rootID); err == nil && ok {
		for _, st := range previous.Steps {
			categories = append(categories, st.Categories...)
		}
	}

	resolvedAt = normalizeIncidentTime(resolvedAt)
	if resolvedAt != "" {
		if _, err := validateScalar(pmFieldResolvedAt, resolvedAt); err != nil {
			return IncidentRun{}, fmt.Errorf("%w: %s", ErrInvalidPostmortemPatch, err.Error())
		}
	}
	doc, err := s.updatePostmortem(rootID, func(doc *Postmortem) error {
		if resolvedAt != "" {
			doc.ResolvedAt = resolvedAt
		} else if doc.ResolvedAt == "" {
			doc.ResolvedAt = time.Now().UTC().Format(time.RFC3339)
		}
		doc.Status = IncidentStatusResolved
		return nil
	})
	if err != nil {
		return IncidentRun{}, err
	}

	run := IncidentRun{
		RunID:       uuid.NewString(),
		Kind:        IncidentRunClosure,
		Status:      IncidentRunRunning,
		StartedAt:   time.Now().Unix(),
		Brief:       "Final impact and closure of the incident, now resolved.",
		WindowStart: doc.StartedAt,
		WindowEnd:   doc.ResolvedAt,
		Steps: []IncidentStep{
			{Kind: IncidentStepImpact, Categories: uniqueNonEmpty(categories), Status: IncidentStepPending},
			{Kind: IncidentStepClosure, Status: IncidentStepPending},
		},
	}
	if err := s.launchIncidentRun(ctx, rootID, run); err != nil {
		return IncidentRun{}, err
	}
	return run, nil
}

// finishIncidentClosure closes the incident once every step of its closure
// run finished. It is set in code rather than by the writer agent, which can
// end its turn without writing; a closure without its summary is not one.
func (s *ChatService) finishIncidentClosure(rootID uuid.UUID, run *IncidentRun) {
	if run.Status != IncidentRunDone {
		return
	}
	for _, st := range run.Steps {
		if st.Status != IncidentStepDone {
			run.Status = IncidentRunFailed
			return
		}
	}
	_, err := s.updatePostmortem(rootID, func(doc *Postmortem) error {
		if doc.Closure == "" {
			return fmt.Errorf("the closure writer ended without writing the closure summary")
		}
		doc.Status = IncidentStatusClosed
		doc.ClosedAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	})
	if err != nil {
		slog.Warn("incident closure: close the post-mortem", "dialog_id", rootID, "error", err)
		run.Status = IncidentRunFailed
		run.Error = err.Error()
	}
}
