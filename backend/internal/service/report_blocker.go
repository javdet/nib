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
      "description": "What you are assuming in the meantime. Required: the stage is written either way, so it has to say which way you went."
    }
  },
  "required": ["question", "assumption"]
}`)

// ReportBlockerToolDef returns the LLM tool definition a stage subagent uses in
// place of ask_question.
//
// Stages are planned in parallel, so suspending one to ask the user would leave
// its siblings running against an answer that has not arrived. Instead the
// question is recorded, the subagent states an assumption and finishes its stage,
// and the runner puts every stage's questions to the user in one round at the end.
func ReportBlockerToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: ReportBlockerToolName,
		Description: "Record a question for the user without interrupting your work. " +
			"You are planning one stage of a plan alongside other agents, so you cannot ask the user directly. " +
			"State the question, two or three options, and the assumption you are proceeding under, then finish and store your stage as usual. " +
			"All stages' questions are put to the user together once the plan is drafted.",
		Parameters: reportBlockerParameters,
	}
}

// reportBlockerHandler returns a handler that appends to the fan-out run of planID.
func (s *ChatService) reportBlockerHandler(planID uuid.UUID, stage string) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		question := strings.TrimSpace(argString(args["question"]))
		if question == "" {
			return "question is required", nil
		}
		assumption := strings.TrimSpace(argString(args["assumption"]))
		if assumption == "" {
			return "assumption is required: your stage is stored either way, so it has to say which way you went", nil
		}

		var options []string
		if raw, ok := args["options"].([]any); ok {
			for _, o := range raw {
				if v := strings.TrimSpace(argString(o)); v != "" {
					options = append(options, v)
				}
			}
		}

		if _, err := s.updateFanoutRun(planID, func(run *FanoutRun) {
			// The same subagent may retry a round; one question per stage is
			// enough, so an identical one is not recorded twice.
			for _, b := range run.Blockers {
				if b.Stage == stage && strings.EqualFold(b.Question, question) {
					return
				}
			}
			run.Blockers = append(run.Blockers, PlanBlocker{
				Stage:      stage,
				Question:   question,
				Options:    options,
				Assumption: assumption,
			})
		}); err != nil {
			return "", fmt.Errorf("record blocker: %w", err)
		}

		return "Question recorded for the user. Continue planning under your stated assumption and store the stage with " +
			UpdateActionPlanToolName + " before your turn ends.", nil
	}
}
