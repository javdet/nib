package service

import (
	"time"

	"github.com/javdet/nib/internal/metrics"
	"github.com/javdet/nib/internal/mode"
)

// normalizeMode bounds the mode label.
//
// POST /chat takes a mode name from the request and runAgentLoop never
// validates it, so an arbitrary string can reach the agent loop -- and would
// reach a metric label with it.
func normalizeMode(name string) string {
	if mode.IsValid(name) {
		return name
	}
	return "other"
}

// planStatusLabels is the plans-by-status label set as the metrics package
// spells it. It exists so a test can assert the two lists agree: metrics is a
// leaf package and cannot import the ActionPlanStatus constants.
func planStatusLabels() []string {
	return metrics.PlanStatuses
}

// agentTurn accumulates what one agent turn is worth measuring.
//
// Both agent loops have a dozen error returns each, so the outcome starts
// pessimistic and the success paths overwrite it -- that way a return added
// later is counted as a failure rather than silently going unrecorded.
type agentTurn struct {
	mode         string
	start        time.Time
	outcome      string
	rounds       int
	toolFailures int
}

func beginAgentTurn(modeName string) *agentTurn {
	t := &agentTurn{
		mode:    normalizeMode(modeName),
		start:   time.Now(),
		outcome: metrics.OutcomeError,
	}
	metrics.IncAgentTurnsInFlight(t.mode)
	return t
}

func (t *agentTurn) round(n int)          { t.rounds = n }
func (t *agentTurn) toolFailed()          { t.toolFailures++ }
func (t *agentTurn) succeeded()           { t.outcome = metrics.OutcomeSuccess }
func (t *agentTurn) awaitingInput()       { t.outcome = metrics.OutcomeAwaitingInput }
func (t *agentTurn) hitMaxIterations()    { t.outcome = metrics.OutcomeMaxIterations }
func (t *agentTurn) hitToolFailureLimit() { t.outcome = metrics.OutcomeToolFailures }

func (t *agentTurn) finish() {
	metrics.DecAgentTurnsInFlight(t.mode)
	metrics.AddAgentToolFailures(t.mode, t.toolFailures)
	metrics.RecordAgentTurn(t.mode, t.outcome, t.rounds, time.Since(t.start))
}
