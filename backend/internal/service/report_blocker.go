package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/llm"
)

const ReportBlockerToolName = "report_blocker"

var reportBlockerParameters = json.RawMessage(`{
  "type": "object",
  "properties": {
    "question": {
      "type": "string",
      "description": "The single thing you need the user to decide, phrased so someone who has not read your research can answer it."
    },
    "options": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Two or three concrete answers to choose between."
    },
    "assumption": {
      "type": "string",
      "description": "What you will fall back on if no answer comes back. Required: your part of the plan is stored either way, so it has to say which way you would go."
    }
  },
  "required": ["question", "assumption"]
}`)

// ReportBlockerToolDef returns the LLM tool definition a fan-out subagent uses in
// place of ask_question.
//
// It is a separate tool rather than ask_question because ask_question suspends
// the turn, and a stage subagent's turn cannot be re-entered: the loop's state
// is in memory and the wave it belongs to is waiting on its goroutine. This one
// blocks inside the call instead, so the answer arrives as its result and the
// same turn carries on with it.
func ReportBlockerToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: ReportBlockerToolName,
		Description: "Ask the user a question and wait for the answer. " +
			"You are working out one part of a plan alongside other agents, so the question is put to the user " +
			"under your stage's name and the other stages keep planning while you wait. " +
			"State the question, two or three options, and the assumption you would fall back on. " +
			"The call returns what the user said; if no answer comes back it says so and you proceed under your assumption.",
		Parameters: reportBlockerParameters,
	}
}

// reportBlockerHandler returns a handler that puts one question to the operator
// and waits for the answer. kind says which agent is asking, which decides both
// what the question is labelled with and which tool the agent is reminded to
// finish with.
func (s *ChatService) reportBlockerHandler(planID uuid.UUID, stage string, kind FanoutStageKind) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		question := strings.TrimSpace(argString(args["question"]))
		if question == "" {
			return "question is required", nil
		}
		assumption := strings.TrimSpace(argString(args["assumption"]))
		if assumption == "" {
			return "assumption is required: your part of the plan is stored either way, so it has to say which way you went", nil
		}

		var options []string
		if raw, ok := args["options"].([]any); ok {
			for _, o := range raw {
				if v := strings.TrimSpace(argString(o)); v != "" {
					options = append(options, v)
				}
			}
		}

		blocker := PlanBlocker{
			Stage:      stage,
			Kind:       kind,
			Question:   question,
			Options:    options,
			Assumption: assumption,
		}

		outcome, answer, err := s.recordBlocker(planID, blocker)
		if err != nil {
			return "", fmt.Errorf("record blocker: %w", err)
		}
		switch outcome {
		case blockerAnswered:
			// A retried round asking the same thing again: the operator has
			// already settled it, so hand back what they said rather than
			// putting it to them twice.
			return blockerAnsweredResult(blocker, answer), nil
		case blockerSettled:
			return blockerFallbackResult(blocker), nil
		}

		// The run record's lock is released by recordBlocker before the wait:
		// holding it would block every sibling stage's status write.
		return s.awaitBlockerAnswer(ctx, planID, blocker), nil
	}
}

// blockerOutcome says what recordBlocker decided about a question.
type blockerOutcome int

const (
	// blockerAsk means the question is on record and should go to the operator.
	blockerAsk blockerOutcome = iota
	// blockerAnswered means an identical question from this agent already
	// carries the operator's answer.
	blockerAnswered
	// blockerSettled means the question is not worth putting to the operator:
	// this agent has used up its questions for the run, or it is re-raising one
	// that already went unanswered once.
	blockerSettled
)

// recordBlocker decides whether b is worth putting to the operator and, when it
// is, appends it to the run. It takes the run's lock and releases it, so nothing
// holds that lock across the wait that follows.
func (s *ChatService) recordBlocker(planID uuid.UUID, b PlanBlocker) (blockerOutcome, string, error) {
	outcome := blockerAsk
	answer := ""

	if _, err := s.updateFanoutRun(planID, func(run *FanoutRun) {
		asked := 0
		for _, existing := range run.Blockers {
			if existing.Kind != b.Kind || existing.Stage != b.Stage {
				continue
			}
			asked++
			if sameBlockerQuestion(existing.Question, b.Question) {
				// The same subagent may retry a round; one record per question
				// is enough, so an identical one is neither stored twice nor
				// counted twice against the cap. A record with no answer is one
				// the operator has already let go by, so it is not put to them
				// a second time either.
				outcome, answer = blockerAnswered, existing.Answer
				if answer == "" {
					outcome = blockerSettled
				}
				return
			}
		}
		if asked >= maxBlockersPerStage {
			outcome = blockerSettled
			return
		}
		run.Blockers = append(run.Blockers, b)
	}); err != nil {
		return blockerAsk, "", err
	}
	return outcome, answer, nil
}

// sameBlockerQuestion reports whether two questions are the same one asked
// twice. A model that retries a round rarely reproduces its own wording to the
// character, so case and spacing are folded away.
func sameBlockerQuestion(a, b string) bool {
	return strings.EqualFold(strings.Join(strings.Fields(a), " "), strings.Join(strings.Fields(b), " "))
}
