package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
)

const (
	// fanoutAskIDPrefix marks a question raised by a plan fan-out, so answering
	// it replans the stages that asked rather than resuming a turn.
	fanoutAskIDPrefix = "fanout_ask_"
	// subagentAskIDPrefix marks a question relayed from a suspended subagent, so
	// answering it resumes that subagent rather than the chat it was asked in.
	subagentAskIDPrefix = "subagent_ask_"
)

// appendSyntheticAskQuestion writes an assistant row carrying one ask_question
// call and no tool result, which is exactly the shape the chat panel already
// renders as a pending question. Nothing in the frontend needs to know what
// produced it, and the id prefix is what routes the answers back.
//
// It appends under the transcript lock. The dialog it writes to is the one the
// operator is talking to, so it may well be mid-round, and landing between an
// assistant row and its tool results is precisely what that lock exists to
// prevent.
func (s *ChatService) appendSyntheticAskQuestion(
	ctx context.Context,
	dialogID uuid.UUID,
	preamble string,
	questions []domain.Question,
	idPrefix string,
) (string, error) {
	items := make([]map[string]any, 0, len(questions))
	for _, q := range questions {
		item := map[string]any{"question": q.Question}
		if len(q.Options) > 0 {
			item["options"] = q.Options
		}
		items = append(items, item)
	}
	args, err := json.Marshal(map[string]any{"questions": items})
	if err != nil {
		return "", fmt.Errorf("marshal questions: %w", err)
	}

	callID := idPrefix + uuid.NewString()
	toolCalls, err := marshalToolCalls([]llm.ToolCall{{
		ID:        callID,
		Name:      AskQuestionToolName,
		Arguments: string(args),
	}})
	if err != nil {
		return "", fmt.Errorf("marshal tool calls: %w", err)
	}

	if _, err := s.appendMessageLocked(ctx, dialogID, domain.DialogMessage{
		Role:      "assistant",
		Content:   preamble,
		ToolCalls: toolCalls,
	}); err != nil {
		return "", fmt.Errorf("append question: %w", err)
	}
	return callID, nil
}
