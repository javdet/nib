package service

import (
	"context"

	"github.com/google/uuid"
)

// launchExecuteSubagent carries out one item of the plan.
//
// It shares executePlanItem with the execute_action tool, so a request that
// arrives through the orchestrator and one that arrives from a legacy decompose
// chat cannot diverge on what a code action is, which numbers exist, or what
// counts as already running.
func (s *ChatService) launchExecuteSubagent(
	ctx context.Context,
	rootID uuid.UUID,
	item string,
	rerun bool,
) (SubagentResult, error) {
	if _, err := s.resolveOrchestratorRoot(ctx, rootID); err != nil {
		return SubagentResult{}, err
	}

	text, started, err := s.executePlanItem(ctx, rootID, item, rerun)
	if err != nil {
		return SubagentResult{}, err
	}
	if !started {
		return SubagentResult{Status: SubagentRefused, Summary: text}, nil
	}
	return SubagentResult{Status: SubagentStarted, Summary: text}, nil
}
