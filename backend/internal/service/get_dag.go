package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/llm"
)

const GetDAGToolName = "get_dag"

var getDAGParameters = json.RawMessage(`{"type":"object","properties":{}}`)

// GetDAGToolDef returns the LLM tool definition for reading the stored DAG.
func GetDAGToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: GetDAGToolName,
		Description: "Return the stage-level plan bound to the current conversation: the summary, the stages of the DAG " +
			"in the order the diagram declares them, and the mermaid flowchart itself. " +
			"`stages[].number` is the position the operator counts off the diagram and is what run_subagent's `stages` takes; " +
			"`stages[].title` is the spelling every other tool matches a stage by. " +
			"Read it before answering anything about which stages exist or how they depend on each other, rather than " +
			"working from what was said earlier: the DAG is rebuilt in full every time it changes. " +
			"get_action_list is the step-level plan derived from these stages.",
		Parameters: getDAGParameters,
	}
}

type dagResponse struct {
	Summary string     `json:"summary,omitempty"`
	Stages  []dagStage `json:"stages"`
	Mermaid string     `json:"mermaid"`
}

type dagStage struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
}

func (s *ChatService) getDAGHandler(dialogID uuid.UUID) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		planID, err := s.resolveRootDialogID(ctx, dialogID)
		if err != nil {
			return "", err
		}

		dag, found, err := s.ReadDAG(planID)
		if err != nil {
			return "", fmt.Errorf("read dag: %w", err)
		}
		if !found {
			return "no DAG found for this dialog", nil
		}

		resp := dagResponse{
			Stages:  dagStagesOf(dag),
			Mermaid: strings.TrimSpace(dag),
		}

		// A failed or absent summary costs the headline, not the diagram:
		// decompose writes the two as separate files, so either can be the one
		// that landed.
		summary, ok, err := s.ReadSummary(planID)
		if err != nil {
			slog.Warn("read plan summary", "plan_id", planID, "error", err)
		} else if ok {
			resp.Summary = strings.TrimSpace(summary)
		}

		out, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal dag: %w", err)
		}
		return string(out), nil
	}
}

// dagStagesOf numbers the stage titles of a stored diagram the way the operator
// reads them off it, which is also how resolveDAGStages takes a bare number.
func dagStagesOf(markdown string) []dagStage {
	titles := dagStageTitles(markdown)
	stages := make([]dagStage, 0, len(titles))
	for i, title := range titles {
		stages = append(stages, dagStage{Number: i + 1, Title: title})
	}
	return stages
}
