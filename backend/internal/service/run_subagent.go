package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
)

const RunSubagentToolName = "run_subagent"

// RunSubagentToolDef generates the schema from the registry, so adding a
// specialist is one entry in subagentSpecs and nothing else.
//
// The schema is one flat object with a name enum rather than a oneOf per
// sub-agent. llm.ToolDef.Parameters is passed to the provider verbatim, and
// oneOf support is uneven across the two APIs this backend speaks
// (/v1/chat/completions and /v1/responses) and across the OpenAI-compatible
// gateways behind them. A flat object with a discriminator is the lowest common
// denominator, and the shape execute_action already uses. Arguments meant for
// another sub-agent are a mistake the handler reports as tool output, the way
// every other tool here reports a bad argument.
func RunSubagentToolDef() llm.ToolDef {
	var b strings.Builder
	b.WriteString("Which specialist to launch.")
	for _, spec := range subagentSpecs {
		fmt.Fprintf(&b, "\n\n- %s: %s", spec.Name, spec.Description)
		if len(spec.Params) > 0 {
			fmt.Fprintf(&b, " (reads: %s)", strings.Join(spec.Params, ", "))
		}
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"enum":        subagentNames(),
				"description": b.String(),
			},
			"task": map[string]any{
				"type": "string",
				"description": "decompose only. The operator's request verbatim, plus any context you already have. " +
					"When you are relaying an answer to a question decompose raised, put their answer here.",
			},
			"stages": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
				"description": "plan only. DAG stage titles as the DAG spells them, or the numbers the operator " +
					"counted off the diagram (\"1\", \"2\"). Omit to plan every stage.",
			},
			"rollback": map[string]any{
				"type": "boolean",
				"description": "plan only. Whether the run ends by redoing the rollback. Omit for the usual " +
					"behaviour: the rollback follows whatever the planned stages say.",
			},
			"item": map[string]any{
				"type": "string",
				"description": "execute only. The item's number as the web interface shows it: \"1.1\" for an " +
					"action, \"R1\" for a rollback entry.",
			},
			"rerun": map[string]any{
				"type": "boolean",
				"description": "execute only. Run the item again from scratch, replacing an attempt still in " +
					"progress. Use it when the operator asks to restart, repeat or retry.",
			},
		},
		"required": []string{"name"},
	}

	params, err := json.Marshal(schema)
	if err != nil {
		// The schema is a literal; a marshal failure is a programming error, and
		// an empty object is still a valid tool the model can be told off for
		// misusing.
		params = json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
	}

	return llm.ToolDef{
		Name: RunSubagentToolName,
		Description: "Hand this conversation's work to one of your specialists. " +
			"Decompose runs to completion inside the call and may come back needing an answer; " +
			"planning and execution start in the background and report into this chat as they land. " +
			"The result says which happened in its `status`, and what to do next in its `next`.",
		Parameters: params,
	}
}

// runSubagentHandler launches one of the orchestrator's specialists.
//
// Everything an operator or a model can get wrong -- an unknown name, a missing
// argument, a number that is not in the plan, an execution already running --
// comes back as tool output with a nil error, so a mistake costs a sentence
// rather than the turn.
func (s *ChatService) runSubagentHandler(b toolBinding) localToolHandler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		name := SubagentName(strings.ToLower(strings.TrimSpace(argString(args["name"]))))
		spec, ok := subagentSpec(name)
		if !ok {
			return fmt.Sprintf("%q is not one of your sub-agents; choose one of: %s",
				name, strings.Join(subagentNames(), ", ")), nil
		}

		req, msg := parseSubagentRequest(spec, args)
		if msg != "" {
			return msg, nil
		}

		res, err := spec.Launch(s, ctx, b.planID, req)
		if err != nil {
			return "", err
		}
		return marshalSubagentResult(name, res), nil
	}
}

// parseSubagentRequest reads the arguments this specialist cares about and
// reports, as a sentence, the first required one that is missing.
func parseSubagentRequest(spec SubagentSpec, args map[string]any) (SubagentRequest, string) {
	req := SubagentRequest{Name: spec.Name}

	reads := make(map[string]struct{}, len(spec.Params))
	for _, p := range spec.Params {
		reads[p] = struct{}{}
	}

	if _, ok := reads["task"]; ok {
		req.Task = strings.TrimSpace(argString(args["task"]))
	}
	if _, ok := reads["stages"]; ok {
		req.Stages = argStringSlice(args["stages"])
	}
	if _, ok := reads["rollback"]; ok {
		if raw, present := args["rollback"]; present && raw != nil {
			v := argBool(raw)
			req.Rollback = &v
		}
	}
	if _, ok := reads["item"]; ok {
		req.Item = strings.TrimSpace(argString(args["item"]))
	}
	if _, ok := reads["rerun"]; ok {
		req.Rerun = argBool(args["rerun"])
	}

	for _, needed := range spec.Required {
		switch needed {
		case "task":
			if req.Task == "" {
				return req, `the decompose sub-agent needs "task": the operator's request, in their own words`
			}
		case "item":
			if req.Item == "" {
				return req, `the execute sub-agent needs "item": the number the web interface shows beside the row, for example 1.1`
			}
		}
	}

	return req, ""
}

// subagentToolResult is the JSON the orchestrator reads back. Data is JSON and
// guidance is prose, the same split get_action_list uses -- except that the
// guidance here depends on the status, so it rides along in `next`.
type subagentToolResult struct {
	Subagent  SubagentName      `json:"subagent"`
	Status    SubagentStatus    `json:"status"`
	Summary   string            `json:"summary,omitempty"`
	Questions []domain.Question `json:"questions,omitempty"`
	Next      string            `json:"next,omitempty"`
}

func marshalSubagentResult(name SubagentName, res SubagentResult) string {
	out := subagentToolResult{
		Subagent:  name,
		Status:    res.Status,
		Summary:   res.Summary,
		Questions: res.Questions,
		Next:      res.Next,
	}
	if out.Next == "" {
		out.Next = defaultSubagentNext(res.Status)
	}

	b, err := json.Marshal(out)
	if err != nil {
		return fmt.Sprintf("the %s sub-agent finished with status %q", name, res.Status)
	}
	return string(b)
}

func defaultSubagentNext(status SubagentStatus) string {
	switch status {
	case SubagentAwaitingInput:
		return "Put these questions to the operator with ask_question -- the same questions, " +
			"the same order, the same options. Do not answer them yourself."
	case SubagentStarted:
		return "Tell the operator the work has started. It reports into this chat on its own; " +
			"do not launch it again and do not say it is finished."
	case SubagentRefused:
		return "Relay this to the operator. It is a precondition they can fix, not something to retry."
	default:
		return "Relay this to the operator as your answer."
	}
}
