package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// launchPlanSubagent fans the DAG out to one planning agent per stage and returns
// as soon as the run is recorded. Stages report into the orchestrator's chat as
// they land, so nothing waits here.
func (s *ChatService) launchPlanSubagent(
	ctx context.Context,
	rootID uuid.UUID,
	stages []string,
	rollback *bool,
) (SubagentResult, error) {
	if _, err := s.resolveOrchestratorRoot(ctx, rootID); err != nil {
		return SubagentResult{}, err
	}

	targets := planSubagentTargets(stages, rollback)

	// Stage names are resolved before the fan-out so a typo or a number comes
	// back as a sentence naming the real stages, rather than as ErrNoDAGStages.
	if !targets.AllStages && len(targets.Stages) > 0 {
		dag, found, err := s.ReadDAG(rootID)
		if err != nil {
			return SubagentResult{}, fmt.Errorf("launch plan sub-agent: read dag: %w", err)
		}
		if !found {
			return SubagentResult{
				Status:  SubagentRefused,
				Summary: "this plan has no DAG yet; launch the decompose sub-agent first",
			}, nil
		}

		resolved, miss := resolveDAGStages(dagStageTitles(dag), targets.Stages)
		if miss != "" {
			return SubagentResult{Status: SubagentRefused, Summary: miss}, nil
		}
		targets.Stages = resolved
	}

	run, err := s.StartPlanFanout(ctx, rootID, targets)
	switch {
	case err == nil:
	case errors.Is(err, ErrFanoutInProgress):
		return SubagentResult{Status: SubagentRefused, Summary: planningInProgressMessage(s, rootID)}, nil
	case errors.Is(err, ErrNoDAGStages):
		return SubagentResult{
			Status:  SubagentRefused,
			Summary: "this plan has no DAG stages to plan; launch the decompose sub-agent first",
		}, nil
	default:
		return SubagentResult{}, err
	}

	return SubagentResult{
		Status:  SubagentStarted,
		Summary: plannedStagesSummary(run),
	}, nil
}

// PlanFanoutTargetsFor turns "these stages, and maybe the rollback" into the
// target set a fan-out takes. rollback is a modifier on the stage selection
// here, not a selection of its own: asking for it with no stages named is still
// the whole plan.
func PlanFanoutTargetsFor(stages []string, rollback *bool) FanoutTargets {
	targets := AllFanoutTargets()
	if len(stages) > 0 {
		// Named stages are a replan; the rollback still follows whatever they
		// end up saying, unless the caller says otherwise.
		targets = FanoutTargets{Stages: stages, Rollback: true}
	}
	if rollback != nil {
		targets.Rollback = *rollback
	}
	return targets
}

// planSubagentTargets reads the orchestrator's arguments.
//
// It diverges from PlanFanoutTargetsFor on exactly one shape, deliberately: an
// explicit rollback with no stages named. "Redo the rollback" is a sentence an
// operator says on its own, and replanning every stage in answer to it would
// throw away work they never asked to lose. An empty Stages list with AllStages
// unset is how FanoutTargets spells "the rollback by itself".
func planSubagentTargets(stages []string, rollback *bool) FanoutTargets {
	if len(stages) == 0 && rollback != nil && *rollback {
		return FanoutTargets{Rollback: true}
	}
	return PlanFanoutTargetsFor(stages, rollback)
}

// resolveDAGStages maps what the orchestrator was told -- a stage title, or the
// number the operator counted off the diagram -- onto the titles the DAG spells.
//
// A miss lists the real titles, so the sentence that reports the mistake is also
// the one that teaches the orchestrator how to retry.
func resolveDAGStages(titles, requested []string) ([]string, string) {
	byTitle := make(map[string]string, len(titles))
	for _, t := range titles {
		byTitle[normalizeStageTitle(t)] = t
	}

	out := make([]string, 0, len(requested))
	for _, want := range requested {
		want = strings.TrimSpace(want)
		if want == "" {
			continue
		}

		if title, ok := byTitle[normalizeStageTitle(want)]; ok {
			out = append(out, title)
			continue
		}
		// A bare number is the position the operator read off the diagram.
		if n, err := strconv.Atoi(strings.TrimSuffix(want, ".")); err == nil && n >= 1 && n <= len(titles) {
			out = append(out, titles[n-1])
			continue
		}

		return nil, fmt.Sprintf("no DAG stage matches %q; this plan's stages are: %s",
			want, numberedStageList(titles))
	}

	if len(out) == 0 {
		return nil, fmt.Sprintf("no stage was named; this plan's stages are: %s", numberedStageList(titles))
	}
	return out, ""
}

func numberedStageList(titles []string) string {
	parts := make([]string, 0, len(titles))
	for i, t := range titles {
		parts = append(parts, fmt.Sprintf("%d. %s", i+1, t))
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
}

func plannedStagesSummary(run FanoutRun) string {
	var stages []string
	rollback := false
	for _, st := range run.Stages {
		if st.Kind == FanoutStageKindRollback {
			rollback = true
			continue
		}
		stages = append(stages, st.Title)
	}

	switch {
	case len(stages) == 0 && rollback:
		return "redoing the rollback plan"
	case rollback:
		return fmt.Sprintf("planning %s, then redoing the rollback", strings.Join(stages, ", "))
	case len(stages) > 0:
		return fmt.Sprintf("planning %s", strings.Join(stages, ", "))
	default:
		return "planning started"
	}
}

// planningInProgressMessage says how far the run that holds the plan has got, so
// the operator can decide between waiting and stopping it.
func planningInProgressMessage(s *ChatService, rootID uuid.UUID) string {
	run, found, err := s.ReadFanoutRun(rootID)
	if err != nil || !found {
		return "planning is already running for this plan; wait for it to finish or stop it in the Action List"
	}

	done := 0
	for _, st := range run.Stages {
		if st.Status == FanoutStageDone {
			done++
		}
	}
	return fmt.Sprintf("planning is already running for this plan -- %d of %d stages are done. "+
		"Wait for it to finish, or stop it in the Action List.", done, len(run.Stages))
}
