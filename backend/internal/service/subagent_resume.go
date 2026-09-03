package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// resumeSubagentFromAnswers writes the operator's answers against the dangling
// ask_question of the paused sub-agent, runs it on to its next stop, and reports
// what happened into the orchestrator's chat.
//
// It reports false when toolCallID is not a relayed question, leaving the
// ordinary resume path to it.
//
// The orchestrator's own model is deliberately not re-entered: it asked on the
// sub-agent's behalf and has nothing of its own waiting on the answer, so
// resuming its turn would only make it narrate the question back.
func (s *ChatService) resumeSubagentFromAnswers(
	ctx context.Context,
	d domain.Dialog,
	toolCallID string,
	answers []string,
) (domain.ChatResponse, bool, error) {
	if !strings.HasPrefix(toolCallID, subagentAskIDPrefix) && !strings.EqualFold(d.Mode, mainDialogMode) {
		// Cheap exit for every dialog that has no orchestrator behind it.
		return domain.ChatResponse{}, false, nil
	}

	pause, found, err := s.takeSubagentPause(d.ID, toolCallID)
	if err != nil {
		return domain.ChatResponse{}, false, err
	}
	if !found {
		return domain.ChatResponse{}, false, nil
	}

	pauseDialogID, err := uuid.Parse(pause.DialogID)
	if err != nil {
		return domain.ChatResponse{}, false, fmt.Errorf("paused sub-agent dialog %q: %w", pause.DialogID, err)
	}

	msgs, err := s.dialogRepo.ListMessages(ctx, pauseDialogID)
	if err != nil {
		return domain.ChatResponse{}, false, fmt.Errorf("list sub-agent messages: %w", err)
	}
	if toolResultExists(msgs, pause.ToolCallID) {
		// Something already answered it -- a relayed answer that came through as
		// a fresh task, most likely. The pause is gone now; let the ordinary
		// path handle this call.
		return domain.ChatResponse{}, false, nil
	}

	content, err := s.formatRelayedAnswers(msgs, pause, answers)
	if err != nil {
		return domain.ChatResponse{}, false, err
	}

	if _, err := s.appendMessageLocked(ctx, pauseDialogID, domain.DialogMessage{
		Role:       "tool",
		Content:    content,
		ToolCallID: pause.ToolCallID,
		Name:       AskQuestionToolName,
	}); err != nil {
		return domain.ChatResponse{}, false, fmt.Errorf("append relayed answers: %w", err)
	}

	res, err := s.runDecomposeSubagentTurn(ctx, d.ID, pauseDialogID)
	if err != nil {
		return domain.ChatResponse{}, false, err
	}

	if res.Status == SubagentAwaitingInput {
		resp, err := s.relaySubagentQuestions(ctx, d.ID, pause.Subagent, res.Questions)
		if err != nil {
			return domain.ChatResponse{}, false, err
		}
		return resp, true, nil
	}

	return s.reportSubagentResult(ctx, d.ID, pause, res), true, nil
}

// formatRelayedAnswers pairs the answers with the questions the sub-agent
// actually asked.
//
// The sub-agent's own wording is used when the counts line up and the relayed
// wording when they do not: a model that merged two questions into one should
// still get the answer it was given rather than an error.
func (s *ChatService) formatRelayedAnswers(
	msgs []domain.DialogMessage,
	pause SubagentPause,
	answers []string,
) (string, error) {
	questions := pause.Questions

	if tc, err := findAskQuestionCall(msgs, pause.ToolCallID); err == nil {
		if original, err := parseAskQuestionFromArguments(tc.Arguments); err == nil && len(original) == len(answers) {
			questions = original
		}
	}

	if len(questions) != len(answers) {
		return "", fmt.Errorf("relayed answers: expected %d answers, got %d", len(questions), len(answers))
	}
	return formatAskQuestionResult(questions, answers)
}

// relaySubagentQuestions puts a suspended sub-agent's next questions to the
// operator without a round trip through the orchestrator's model, and records the
// pause already bound to the call it wrote.
func (s *ChatService) relaySubagentQuestions(
	ctx context.Context,
	rootID uuid.UUID,
	name SubagentName,
	questions []domain.Question,
) (domain.ChatResponse, error) {
	preamble := fmt.Sprintf("The %s sub-agent needs one more decision before it can finish.", name)

	callID, err := s.appendSyntheticAskQuestion(ctx, rootID, preamble, questions, subagentAskIDPrefix)
	if err != nil {
		return domain.ChatResponse{}, err
	}
	if !s.bindSubagentPause(rootID, callID, questions) {
		slog.Warn("relay sub-agent questions: no pause to bind", "root_id", rootID, "subagent", name)
	}

	return domain.ChatResponse{
		Status:     "awaiting_input",
		ToolCallID: callID,
		Questions:  questions,
	}, nil
}

// reportSubagentResult posts a finished sub-agent's answer into the
// orchestrator's chat, named so a retried request cannot double-post.
func (s *ChatService) reportSubagentResult(
	ctx context.Context,
	rootID uuid.UUID,
	pause SubagentPause,
	res SubagentResult,
) domain.ChatResponse {
	if pause.Subagent == SubagentDecompose {
		if dialogID, err := uuid.Parse(pause.DialogID); err == nil {
			s.adoptDecomposeTitle(ctx, rootID, dialogID)
		}
	}

	body := formatSubagentReport(pause, res)
	name := fmt.Sprintf("subagent:%s#%s", pause.Subagent, pause.ToolCallID)

	if msgs, err := s.dialogRepo.ListMessages(ctx, rootID); err == nil {
		for _, m := range msgs {
			if m.Name == name {
				return domain.ChatResponse{Response: body}
			}
		}
	}

	if _, err := s.appendMessageLocked(ctx, rootID, domain.DialogMessage{
		Role:    "assistant",
		Content: body,
		Name:    name,
	}); err != nil {
		slog.Warn("report sub-agent result", "root_id", rootID, "subagent", pause.Subagent, "error", err)
	}
	return domain.ChatResponse{Response: body}
}

func formatSubagentReport(pause SubagentPause, res SubagentResult) string {
	var b strings.Builder
	if text := strings.TrimSpace(res.Summary); text != "" {
		b.WriteString(text)
	} else {
		fmt.Fprintf(&b, "The %s sub-agent finished without saying anything.", pause.Subagent)
	}

	if dialogID, err := uuid.Parse(pause.DialogID); err == nil && dialogID != uuid.Nil {
		fmt.Fprintf(&b, "\n\n[View sub-agent transcript](#dialog:%s)", dialogID)
	}
	// A later turn reads this row and must not mistake it for work still to do.
	b.WriteString("\n\n_This work is finished. Do not launch it again unless the operator asks._")
	return b.String()
}
