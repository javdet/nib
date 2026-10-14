package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// planChangeNext is what the orchestrator is told after a refused plan change.
// It rides on the result rather than only in the prompt because the moment the
// orchestrator needs it is the moment it has just tried to replan on its own.
const planChangeNext = "Do not change the plan on your own initiative. If the operator reported a failure or asked " +
	"for help with one, troubleshoot it: read the row's notes in get_action_list, find the cause, and fix it with " +
	"execute (rerun: true) or code, or tell the operator what has to change outside the plan. If you believe the " +
	"stages or steps themselves are wrong, say why and ask the operator with ask_question; launch again only once " +
	"they agree, passing their answer in operator_request."

// guardPlanChange refuses a decompose or plan launch that would rewrite a DAG or
// an action plan the plan already has, unless the operator asked for it.
//
// Once the stages and steps exist they are what the operator agreed to, and
// replanning a stage discards the checks, runs and notes recorded against its
// rows. The orchestrator is a model that may decide on its own that a failed
// step means a wrong plan, so the prompt alone is not enough: it has to quote
// the operator's latest message, and that quote is checked against the
// transcript. The UI's own buttons do not come through here.
func (s *ChatService) guardPlanChange(ctx context.Context, rootID uuid.UUID, req SubagentRequest) (SubagentResult, bool, error) {
	var artifact string
	switch req.Name {
	case SubagentDecompose:
		_, found, err := s.ReadDAG(rootID)
		if err != nil {
			return SubagentResult{}, false, fmt.Errorf("guard plan change: read dag: %w", err)
		}
		if !found {
			return SubagentResult{}, false, nil
		}
		// Relaying an answer to a question decompose is suspended on is the same
		// conversation carrying on, not a new change.
		suspended, err := s.decomposeSuspended(ctx, rootID)
		if err != nil {
			return SubagentResult{}, false, err
		}
		if suspended {
			return SubagentResult{}, false, nil
		}
		artifact = "DAG"
	case SubagentPlan:
		_, found, err := s.ReadActionPlan(rootID)
		if err != nil {
			return SubagentResult{}, false, fmt.Errorf("guard plan change: read action plan: %w", err)
		}
		if !found {
			return SubagentResult{}, false, nil
		}
		artifact = "action plan"
	default:
		return SubagentResult{}, false, nil
	}

	quote := normalizeOperatorText(req.OperatorRequest)
	if quote == "" {
		return SubagentResult{
			Status: SubagentRefused,
			Summary: fmt.Sprintf("this plan already has its %s, and %s rewrites it. Once the plan exists it changes "+
				"only when the operator asks for that change; pass their request in operator_request, quoted from "+
				"their latest message.", artifact, req.Name),
			Next: planChangeNext,
		}, true, nil
	}

	latest, err := s.latestOperatorInput(ctx, rootID)
	if err != nil {
		return SubagentResult{}, false, err
	}
	if !strings.Contains(normalizeOperatorText(latest), quote) {
		return SubagentResult{
			Status: SubagentRefused,
			Summary: fmt.Sprintf("operator_request %q is not in the operator's latest message, so it is not their "+
				"request to change this plan's %s. Quote their words exactly, untranslated.",
				strings.TrimSpace(req.OperatorRequest), artifact),
			Next: planChangeNext,
		}, true, nil
	}
	return SubagentResult{}, false, nil
}

// decomposeSuspended reports whether this plan's decompose transcript is waiting
// on an ask_question it raised.
func (s *ChatService) decomposeSuspended(ctx context.Context, rootID uuid.UUID) (bool, error) {
	children, err := s.dialogRepo.ListChildren(ctx, rootID)
	if err != nil {
		return false, fmt.Errorf("guard plan change: list plan children: %w", err)
	}
	for _, child := range children {
		if child.Mode != decomposeSubagentMode {
			continue
		}
		msgs, err := s.dialogRepo.ListMessages(ctx, child.ID)
		if err != nil {
			return false, fmt.Errorf("guard plan change: list decompose messages: %w", err)
		}
		return danglingAskQuestionID(msgs) != "", nil
	}
	return false, nil
}

// latestOperatorInput is the newest thing the operator said in the root chat:
// a message they sent, or their answers to an ask_question. Only the operator
// writes either into a root transcript; sub-agents report as assistant rows.
func (s *ChatService) latestOperatorInput(ctx context.Context, rootID uuid.UUID) (string, error) {
	msgs, err := s.dialogRepo.ListMessages(ctx, rootID)
	if err != nil {
		return "", fmt.Errorf("guard plan change: list messages: %w", err)
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		switch {
		case m.Role == "user":
			return m.Content, nil
		case m.Role == "tool" && m.Name == AskQuestionToolName:
			return askQuestionAnswerText(m), nil
		}
	}
	return "", nil
}

// askQuestionAnswerText keeps only the operator's answers out of a stored
// ask_question result: the questions beside them are the model's own words, and
// quoting those would pass for an operator request.
func askQuestionAnswerText(m domain.DialogMessage) string {
	var parsed struct {
		Answers []struct {
			Answer string `json:"answer"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(m.Content), &parsed); err != nil || len(parsed.Answers) == 0 {
		return m.Content
	}
	answers := make([]string, 0, len(parsed.Answers))
	for _, a := range parsed.Answers {
		answers = append(answers, a.Answer)
	}
	return strings.Join(answers, "\n")
}

// normalizeOperatorText folds case, whitespace and the quote marks a model wraps
// a citation in, so a faithful quote is not refused over typography.
func normalizeOperatorText(text string) string {
	text = strings.Trim(strings.TrimSpace(text), "\"'`«»“”„")
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}
