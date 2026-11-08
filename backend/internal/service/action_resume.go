package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// ActionLaunch says how a plan item starts relative to the attempts before it.
// The zero value is a plain first run.
type ActionLaunch struct {
	// Rerun starts from scratch, replacing an attempt still in progress.
	Rerun bool
	// Resume continues the row's last sub-agent run in its own transcript,
	// with a fresh round budget and deadline.
	Resume bool
	// Note is the operator's word to the resumed run -- typically the answer
	// to the question a blocked run ended on. Only read with Resume.
	Note string
}

// ErrActionLaunchConflict is returned when a launch asks to both restart an
// action and continue it.
var ErrActionLaunchConflict = errors.New("an action can be run again from scratch or continued, not both")

// ErrActionNothingToResume is returned when the row has no earlier sub-agent
// transcript to continue: it never ran, or its stage was replanned since.
var ErrActionNothingToResume = errors.New("this action has no earlier sub-agent run to continue")

// ErrActionResumeFinished is returned when the row's last run already finished.
// Continuing a finished run would only ask the agent to do the work twice.
var ErrActionResumeFinished = errors.New("this action's last run finished; run it again from scratch instead")

// ErrActionResumeCode is returned for a code action: its agent runs in a
// container, and nib keeps no transcript of it to continue from.
var ErrActionResumeCode = errors.New("a code action runs in a container and keeps no transcript to continue; run it again instead")

// actionResume is what a resumed run carries on from.
type actionResume struct {
	dialogID uuid.UUID
	previous ActionExecRun
	note     string
}

// prepareActionResume finds the transcript the row's last run left and checks
// that continuing it makes sense. It is called with the execution lease held.
func (s *ChatService) prepareActionResume(ctx context.Context, planID uuid.UUID, key, note string) (actionResume, error) {
	runs, err := s.ReadActionPlanExecRuns(planID)
	if err != nil {
		return actionResume{}, fmt.Errorf("resume action: read exec runs: %w", err)
	}
	prev, ok := runs[key]
	if !ok || prev.Status == "" {
		return actionResume{}, ErrActionNothingToResume
	}
	if prev.Kind == ExecutionKindContainer {
		return actionResume{}, ErrActionResumeCode
	}
	switch prev.Status {
	case ActionExecRunning:
		return actionResume{}, ErrActionAlreadyRunning
	case ActionExecDone:
		return actionResume{}, ErrActionResumeFinished
	}

	dialogs, err := s.ReadActionPlanRuns(planID)
	if err != nil {
		return actionResume{}, fmt.Errorf("resume action: read runs: %w", err)
	}
	dialogID, err := uuid.Parse(strings.TrimSpace(dialogs[key]))
	if err != nil {
		return actionResume{}, ErrActionNothingToResume
	}

	// A transcript without its system row would restart the agent with no
	// instructions at all; that is a fresh run's job, not a resume's.
	stored, err := s.dialogRepo.ListMessages(ctx, dialogID)
	if err != nil {
		return actionResume{}, fmt.Errorf("resume action: list messages: %w", err)
	}
	if len(stored) == 0 || stored[0].Role != "system" {
		return actionResume{}, ErrActionNothingToResume
	}

	return actionResume{dialogID: dialogID, previous: prev, note: strings.TrimSpace(note)}, nil
}

// appendActionResumeNote tells the agent why it is being woken in a transcript
// that already has an ending. Without it the model sees its own last tool
// result and nothing after, and has no way to know time has passed or that
// what it started may have moved on.
func (s *ChatService) appendActionResumeNote(ctx context.Context, resume *actionResume) error {
	if _, err := s.appendMessageLocked(ctx, resume.dialogID, domain.DialogMessage{
		Role:    "user",
		Content: buildActionResumeNote(resume.previous, resume.note),
	}); err != nil {
		return fmt.Errorf("append resume note: %w", err)
	}
	return nil
}

func buildActionResumeNote(prev ActionExecRun, note string) string {
	var b strings.Builder
	b.WriteString("## Continue this action\n\n")

	attempt := ""
	if prev.Attempt > 0 {
		attempt = fmt.Sprintf(" (attempt %d)", prev.Attempt)
	}
	reason := strings.TrimSpace(prev.Error)
	switch prev.Status {
	case ActionExecBlocked:
		if reason == "" {
			reason = "it needed a decision from the operator"
		} else {
			reason = "it needed a decision from the operator: " + reason
		}
	case ActionExecCancelled:
		if reason == "" {
			reason = "it was stopped"
		}
	default:
		if reason == "" {
			reason = "it failed"
		}
	}
	fmt.Fprintf(&b, "The previous attempt%s ended before it finished: %s.\n\n", attempt, reason)
	b.WriteString("The operator asked you to continue from where it stopped, not to start over. " +
		"Everything above is what that attempt did.\n\n")
	b.WriteString("- Do not repeat a step whose result is already in this conversation.\n")
	b.WriteString("- Time has passed since then. Before you change anything the earlier attempt touched, " +
		"check its current state: an operation it started may have finished, failed or still be running.\n")
	b.WriteString("- Command jobs end with the run that started them, so a `job_id` from the earlier attempt " +
		"is gone. Read the state again rather than waiting on it.\n")

	if note != "" {
		b.WriteString("\n## From the operator\n\n")
		b.WriteString(note)
		b.WriteString("\n")
	} else if prev.Status == ActionExecBlocked {
		b.WriteString("- The operator has not answered that question here. If the work still cannot go on " +
			"without the decision, stop and ask it again in your report; never guess at a change to a system.\n")
	}

	b.WriteString("\nCarry on with the action and finish with the report it asks for.")
	return b.String()
}
