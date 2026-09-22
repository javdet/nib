package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/executor"
)

// launchCodeSubagent hands an ad-hoc code change to the coding agent.
//
// It is the one sub-agent that is not an agent of nib's own: the work happens in
// an agent-runner container, so the launch returns as soon as the container is
// started and the result arrives later through the webhook, posted into this
// chat. That makes it a "started" launch, like plan and execute.
//
// Every precondition an operator can fix -- a disabled executor, an unselected
// secret, another execution already running -- comes back as a refusal rather
// than an error, so the orchestrator can relay it in a sentence.
func (s *ChatService) launchCodeSubagent(
	ctx context.Context,
	rootID uuid.UUID,
	req SubagentRequest,
) (SubagentResult, error) {
	if _, err := s.resolveOrchestratorRoot(ctx, rootID); err != nil {
		return SubagentResult{}, err
	}

	dialog, run, err := s.StartCodeFix(ctx, rootID, CodeFixRequest{
		Task:       req.Task,
		Repository: req.Repository,
		Branch:     req.Branch,
		PRTitle:    req.PRTitle,
	})
	switch {
	case err == nil:
	case errors.Is(err, executor.ErrExecutorDisabled):
		return SubagentResult{
			Status:  SubagentRefused,
			Summary: "the executor is disabled, so no coding agent can be launched; switch it on in executor settings",
		}, nil
	case errors.Is(err, ErrExecutorTokenSecretRequired),
		errors.Is(err, ErrExecutorGitTokenSecretRequired),
		errors.Is(err, ErrExecutorSecretMissing),
		errors.Is(err, ErrCodeFixTaskRequired),
		errors.Is(err, ErrCodeFixRepositoryRequired):
		return SubagentResult{Status: SubagentRefused, Summary: err.Error()}, nil
	case errors.Is(err, ErrExecutionBusy):
		// The error already names what holds the slot.
		return SubagentResult{Status: SubagentRefused, Summary: executionBusyMessage(err)}, nil
	default:
		return SubagentResult{}, err
	}

	return SubagentResult{
		Status: SubagentStarted,
		Summary: fmt.Sprintf(
			"handed the change to the coding agent (job %s, repository %s, branch %s); "+
				"the pull request and the agent's account of the work will be posted in this chat when it finishes, "+
				"and its progress is in chat %s",
			run.JobName, strings.TrimSpace(req.Repository), run.TargetBranch, dialog.ID,
		),
	}, nil
}
