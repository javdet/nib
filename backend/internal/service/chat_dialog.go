package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mode"
	"github.com/google/uuid"
)

// SendInDialog appends the user turn to a persisted dialog, replays the full
// stored transcript to the LLM, and persists assistant/tool rows across the agent loop.
func (s *ChatService) SendInDialog(ctx context.Context, dialogID uuid.UUID, message string, attachmentIDs []string) (domain.ChatResponse, error) {
	if s.dialogRepo == nil {
		return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: dialog repository is not configured")
	}

	d, err := s.dialogRepo.GetDialog(ctx, dialogID)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: %w", err)
	}

	msgs, err := s.dialogRepo.ListMessages(ctx, dialogID)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: list messages: %w", err)
	}

	if len(msgs) == 0 {
		sysPrompt, err := s.resolveSystemPrompt(ctx, d.Mode)
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: resolve system prompt: %w", err)
		}
		if _, err := s.dialogRepo.AppendMessage(ctx, dialogID, domain.DialogMessage{
			Role:    "system",
			Content: sysPrompt,
		}); err != nil {
			return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: append system: %w", err)
		}
	}

	if _, err := s.dialogRepo.AppendMessage(ctx, dialogID, domain.DialogMessage{
		Role:    "user",
		Content: message,
	}); err != nil {
		return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: append user: %w", err)
	}

	parsedIDs, err := parseAttachmentIDs(attachmentIDs)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: %w", err)
	}
	if len(parsedIDs) > 0 {
		msgs, err := s.dialogRepo.ListMessages(ctx, dialogID)
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: list messages for link: %w", err)
		}
		var lastUser *domain.DialogMessage
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" {
				lastUser = &msgs[i]
				break
			}
		}
		if lastUser == nil {
			return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: user message not found for attachment link")
		}
		if err := s.attachmentRepo.LinkAttachmentsToMessage(ctx, dialogID, lastUser.ID, parsedIDs); err != nil {
			return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: link attachments: %w", err)
		}
	}

	catalog := newToolCatalog()
	if mode.IsValid(d.Mode) {
		allow, err := s.resolveDialogAllowSet(ctx, d)
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: load allow-tools: %w", err)
		}
		catalog, err = s.buildToolCatalog(ctx, allow, s.dialogToolBinding(ctx, d))
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: %w", err)
		}
	}

	slog.Info("chat send in dialog", "dialog_id", dialogID, "mode", d.Mode)

	resp, err := s.runPersistingAgentLoop(ctx, dialogID, d.Mode, catalog, loopConfig{})
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("chat send in dialog: %w", err)
	}
	return resp, nil
}

// SubmitToolResult appends the user's answers for a pending ask_question tool call
// and resumes the agent loop.
func (s *ChatService) SubmitToolResult(ctx context.Context, dialogID uuid.UUID, toolCallID string, answers []string) (domain.ChatResponse, error) {
	if s.dialogRepo == nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: dialog repository is not configured")
	}
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: toolCallId is required")
	}
	if len(answers) == 0 {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: answers must not be empty")
	}

	d, err := s.dialogRepo.GetDialog(ctx, dialogID)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: %w", err)
	}

	msgs, err := s.dialogRepo.ListMessages(ctx, dialogID)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: list messages: %w", err)
	}
	if toolResultExists(msgs, toolCallID) {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: tool call %q already has a result", toolCallID)
	}

	tc, err := findAskQuestionCall(msgs, toolCallID)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: %w", err)
	}

	questions, err := parseAskQuestionFromArguments(tc.Arguments)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: %w", err)
	}
	if len(answers) != len(questions) {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: expected %d answers, got %d", len(questions), len(answers))
	}

	content, err := formatAskQuestionResult(questions, answers)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: %w", err)
	}

	if _, err := s.dialogRepo.AppendMessage(ctx, dialogID, domain.DialogMessage{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolCallID,
		Name:       AskQuestionToolName,
	}); err != nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: append tool message: %w", err)
	}

	// A question raised by the plan fan-out is answered by replanning the stages
	// that raised it, not by resuming this dialog's own turn.
	resumed, err := s.resumeFanoutFromAnswers(ctx, dialogID, toolCallID, questions, answers)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: %w", err)
	}
	if resumed {
		slog.Info("plan fanout resumed from answers", "dialog_id", dialogID, "tool_call_id", toolCallID)
		return domain.ChatResponse{Response: "Replanning the stages affected by your answers."}, nil
	}

	// A question a sub-agent raised is answered by resuming that sub-agent. The
	// orchestrator asked on its behalf and has nothing of its own waiting on the
	// answer, so resuming its turn here would only make it narrate the question
	// back at the operator.
	if resp, resumed, err := s.resumeSubagentFromAnswers(ctx, d, toolCallID, answers); err != nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: %w", err)
	} else if resumed {
		slog.Info("subagent resumed from answers", "dialog_id", dialogID, "tool_call_id", toolCallID)
		return resp, nil
	}

	catalog := newToolCatalog()
	if mode.IsValid(d.Mode) {
		allow, err := s.resolveDialogAllowSet(ctx, d)
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("submit tool result: load allow-tools: %w", err)
		}
		catalog, err = s.buildToolCatalog(ctx, allow, s.dialogToolBinding(ctx, d))
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("submit tool result: %w", err)
		}
	}

	slog.Info("submit tool result", "dialog_id", dialogID, "tool_call_id", toolCallID)

	resp, err := s.runPersistingAgentLoop(ctx, dialogID, d.Mode, catalog, loopConfig{})
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("submit tool result: %w", err)
	}
	return resp, nil
}

// RetryLastResponse deletes the last agent turn (all messages after the last user
// message), resets generated dialog artifacts, and re-runs the agent loop.
func (s *ChatService) RetryLastResponse(ctx context.Context, dialogID uuid.UUID) (domain.ChatResponse, error) {
	if s.dialogRepo == nil {
		return domain.ChatResponse{}, fmt.Errorf("retry last response: dialog repository is not configured")
	}

	d, err := s.dialogRepo.GetDialog(ctx, dialogID)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("retry last response: %w", err)
	}

	// A retry wipes the exec records along with everything else the turn
	// produced, which would leave the lease held against a row nothing can
	// close. The operator stops the execution first, or waits for it.
	if held, running := s.ExecutionInProgress(); running {
		rootID, rootErr := s.resolveRootDialogID(ctx, dialogID)
		if rootErr == nil && held.PlanID == rootID {
			return domain.ChatResponse{}, fmt.Errorf("retry last response: %w",
				newExecutionBusyError(held))
		}
	}

	msgs, err := s.dialogRepo.ListMessages(ctx, dialogID)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("retry last response: list messages: %w", err)
	}

	lastUserSeq := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			lastUserSeq = msgs[i].Seq
			break
		}
	}
	if lastUserSeq < 0 {
		return domain.ChatResponse{}, fmt.Errorf("retry last response: no user message to retry from")
	}

	if err := s.dialogRepo.DeleteMessagesAfterSeq(ctx, dialogID, lastUserSeq); err != nil {
		return domain.ChatResponse{}, fmt.Errorf("retry last response: delete messages: %w", err)
	}

	if err := s.resetDialogArtifacts(ctx, dialogID); err != nil {
		return domain.ChatResponse{}, fmt.Errorf("retry last response: reset artifacts: %w", err)
	}

	catalog := newToolCatalog()
	if mode.IsValid(d.Mode) {
		allow, err := s.resolveDialogAllowSet(ctx, d)
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("retry last response: load allow-tools: %w", err)
		}
		catalog, err = s.buildToolCatalog(ctx, allow, s.dialogToolBinding(ctx, d))
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("retry last response: %w", err)
		}
	}

	slog.Info("retry last response", "dialog_id", dialogID, "mode", d.Mode)

	resp, err := s.runPersistingAgentLoop(ctx, dialogID, d.Mode, catalog, loopConfig{})
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("retry last response: %w", err)
	}
	return resp, nil
}

func (s *ChatService) resetDialogArtifacts(ctx context.Context, dialogID uuid.UUID) error {
	if err := s.dialogRepo.UpdateTitle(ctx, dialogID, ""); err != nil {
		return fmt.Errorf("reset title: %w", err)
	}
	if err := s.dialogRepo.SetDialogCategories(ctx, dialogID, nil); err != nil {
		return fmt.Errorf("reset categories: %w", err)
	}

	id := dialogID.String()
	files := []string{
		filepath.Join(s.dagsDir, id+".md"),
		filepath.Join(s.summariesDir, id+".txt"),
		filepath.Join(s.actionPlansDir, id+".json"),
		actionPlanChecksPath(s.actionPlansDir, dialogID),
		actionPlanCommentsPath(s.actionPlansDir, dialogID),
		actionPlanRunsPath(s.actionPlansDir, dialogID),
		actionPlanExecPath(s.actionPlansDir, dialogID),
	}
	for _, path := range files {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return nil
}

// loopConfig tunes a single run of the agent loop. The zero value describes an
// ordinary turn: the dialog owns its own action plan, may write any stage of it,
// and gets the service-wide iteration budget.
type loopConfig struct {
	// planID owns the action plan this turn writes. Differs from the dialog only
	// for a plan stage subagent, whose plan belongs to the decompose dialog.
	planID uuid.UUID
	// stage is the single DAG stage a subagent is responsible for.
	stage string
	// kind says which part of the plan a fan-out subagent owns. Empty for an
	// ordinary turn and for a stage subagent.
	kind FanoutStageKind
	// maxIterations overrides the service-wide completion round budget.
	maxIterations int
}

func (c loopConfig) withDefaults(dialogID uuid.UUID, serviceMax int) loopConfig {
	if c.planID == uuid.Nil {
		c.planID = dialogID
	}
	if c.maxIterations <= 0 {
		c.maxIterations = serviceMax
	}
	return c
}

// transcriptMutex returns the lock guarding appends to one dialog's transcript.
//
// A round is written as an assistant row carrying tool_calls followed by one
// tool row per call, with a network round trip between them. A foreign append
// landing in that gap — an action sub-agent posting its result, a fan-out
// posting its questions — separates the assistant row from its results, and
// repairOrphanToolCalls then answers every call twice, which the completion API
// rejects outright. Everything that writes to a dialog another agent may be
// mid-round in takes this lock.
func (s *ChatService) transcriptMutex(dialogID uuid.UUID) *sync.Mutex {
	mu, _ := s.transcriptWriteMu.LoadOrStore(dialogID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// appendMessageLocked appends one row under the transcript lock. It is for
// writers outside the agent loop; the loop holds the lock across a whole round.
func (s *ChatService) appendMessageLocked(ctx context.Context, dialogID uuid.UUID, msg domain.DialogMessage) (domain.DialogMessage, error) {
	mu := s.transcriptMutex(dialogID)
	mu.Lock()
	defer mu.Unlock()
	return s.dialogRepo.AppendMessage(ctx, dialogID, msg)
}

func (s *ChatService) runPersistingAgentLoop(ctx context.Context, dialogID uuid.UUID, modeName string, catalog *toolCatalog, cfg loopConfig) (domain.ChatResponse, error) {
	cfg = cfg.withDefaults(dialogID, s.maxIterations)
	logCtx := newAgentLogCtx(&dialogID, modeName)
	actionPlanUpdated := false
	defer s.activity.Publish(dialogID, domain.AgentActivity{Kind: domain.ActivityTurnEnd})

	stored, err := s.dialogRepo.ListMessages(ctx, dialogID)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("list messages: %w", err)
	}
	attachments, err := s.listDialogAttachments(ctx, dialogID)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("list attachments: %w", err)
	}
	messages, err := s.dialogMessagesToLLM(stored, attachments)
	if err != nil {
		return domain.ChatResponse{}, fmt.Errorf("convert messages: %w", err)
	}

	reminders := 0
	toolFailures := 0

	// The lock is taken per round rather than for the whole turn, so a sub-agent
	// result lands between rounds instead of waiting out the entire turn. One
	// deferred release covers every early return inside the locked stretch.
	transcriptLocked := false
	unlockTranscript := func() {
		if transcriptLocked {
			transcriptLocked = false
			s.transcriptMutex(dialogID).Unlock()
		}
	}
	defer unlockTranscript()

	for round := 0; round < cfg.maxIterations; round++ {
		roundNum := round + 1
		roundLog := logCtx.withRound(roundNum)

		logAgentRoundStart(roundNum, len(messages), len(catalog.tools), logCtx)

		logSendingCompletionRequest(roundNum, logCtx)
		asst, err := s.provider.CompleteWithTools(ctx, messages, catalog.tools)
		if err != nil {
			logCompletionError(roundNum, err, logCtx)
			return domain.ChatResponse{}, fmt.Errorf("completion round %d: %w", roundNum, err)
		}

		logCompletionParsed(roundNum, asst.ToolCalls, logCtx)

		if len(asst.ToolCalls) == 0 {
			if reminders < maxActionPlanReminders && s.needsActionPlanReminder(cfg, modeName, catalog, actionPlanUpdated) {
				reminders++
				logActionPlanReminder(roundNum, logCtx)
				// The reply is research narration, not an answer, so it is kept
				// out of the transcript and only replayed for the next round.
				if strings.TrimSpace(asst.Content) != "" {
					messages = append(messages, llm.Message{
						Role:      "assistant",
						Content:   asst.Content,
						Reasoning: asst.Reasoning,
					})
				}
				messages = append(messages, llm.Message{
					Role:    "user",
					Content: actionPlanReminderFor(cfg),
				})
				continue
			}

			if _, err := s.dialogRepo.AppendMessage(ctx, dialogID, domain.DialogMessage{
				Role:    "assistant",
				Content: asst.Content,
			}); err != nil {
				return domain.ChatResponse{}, fmt.Errorf("append final assistant round %d: %w", roundNum, err)
			}
			return domain.ChatResponse{
				Response:          asst.Content,
				ActionPlanUpdated: actionPlanUpdated,
			}, nil
		}

		if round+1 >= cfg.maxIterations {
			logMaxIterationsWithPendingTools(cfg.maxIterations, roundNum, logCtx)
			return domain.ChatResponse{}, fmt.Errorf("max iterations (%d) exceeded with pending tool_calls", cfg.maxIterations)
		}

		logToolBatch(roundNum, asst.ToolCalls, logCtx)

		toolCallsJSON, err := marshalToolCalls(asst.ToolCalls)
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("marshal tool_calls round %d: %w", roundNum, err)
		}
		s.transcriptMutex(dialogID).Lock()
		transcriptLocked = true
		if _, err := s.dialogRepo.AppendMessage(ctx, dialogID, domain.DialogMessage{
			Role:      "assistant",
			Content:   asst.Content,
			ToolCalls: toolCallsJSON,
		}); err != nil {
			return domain.ChatResponse{}, fmt.Errorf("append assistant round %d: %w", roundNum, err)
		}
		// Reasoning is kept in memory only: it is replayed for the remaining
		// rounds of this turn and dropped once the turn ends, since the next
		// turn rebuilds history from the database.
		messages = append(messages, llm.Message{
			Role:      "assistant",
			Content:   asst.Content,
			ToolCalls: asst.ToolCalls,
			Reasoning: asst.Reasoning,
		})

		execCount := 0
		for _, tc := range asst.ToolCalls {
			if tc.Name != AskQuestionToolName {
				execCount++
			}
		}
		if execCount > 0 {
			s.activity.Publish(dialogID, domain.AgentActivity{
				Kind:  domain.ActivityToolsStart,
				Round: roundNum,
				Count: execCount,
			})
		}

		var pendingAsk *llm.ToolCall
		for _, tc := range asst.ToolCalls {
			if tc.Name == AskQuestionToolName {
				if pendingAsk == nil {
					pending := tc
					pendingAsk = &pending
				}
				continue
			}

			logCallTool(tc.Name, tc.ID, tc.Arguments, roundLog)
			out, err := s.executeToolCall(ctx, catalog, tc)
			if err != nil {
				logCallToolFailed(tc.Name, tc.ID, err, roundLog)
				if isTurnFatalToolError(ctx, err) {
					return domain.ChatResponse{}, fmt.Errorf("tool %q (id %s) round %d: %w", tc.Name, tc.ID, roundNum, err)
				}
				out = toolErrorPayload(tc.Name, err)
				toolFailures++
			} else {
				logCallToolResult(tc.ID, out, roundLog)
				switch tc.Name {
				case CreateActionPlanToolName, UpdateActionPlanToolName, UpdateRollbackPlanToolName:
					actionPlanUpdated = true
				}
			}
			if _, err := s.dialogRepo.AppendMessage(ctx, dialogID, domain.DialogMessage{
				Role:       "tool",
				Content:    out,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			}); err != nil {
				return domain.ChatResponse{}, fmt.Errorf("append tool %q round %d: %w", tc.Name, roundNum, err)
			}
			messages = append(messages, llm.Message{
				Role:       "tool",
				Content:    out,
				ToolCallID: tc.ID,
			})
			if toolFailures > maxToolFailuresPerTurn {
				return domain.ChatResponse{}, fmt.Errorf("round %d: %w", roundNum, ErrTooManyToolFailures)
			}
		}

		// Every tool row answering this round's assistant row is now stored, so
		// the pairing is complete and a foreign append is safe again.
		unlockTranscript()

		if execCount > 0 {
			s.activity.Publish(dialogID, domain.AgentActivity{
				Kind:  domain.ActivityToolsEnd,
				Round: roundNum,
			})
		}

		if pendingAsk != nil {
			logCallTool(pendingAsk.Name, pendingAsk.ID, pendingAsk.Arguments, roundLog)
			questions, err := parseAskQuestionFromArguments(pendingAsk.Arguments)
			if err != nil {
				return domain.ChatResponse{}, fmt.Errorf("parse ask_question round %d: %w", roundNum, err)
			}
			// A question the orchestrator puts to the operator may be one it is
			// relaying for a paused sub-agent. Binding it here is the only place
			// both ids exist: the sub-agent's dangling call was recorded when it
			// suspended, and this call id only exists now.
			if modeName == mainDialogMode {
				s.bindSubagentPause(dialogID, pendingAsk.ID, questions)
			}
			return domain.ChatResponse{
				Status:            "awaiting_input",
				ToolCallID:        pendingAsk.ID,
				Questions:         questions,
				ActionPlanUpdated: actionPlanUpdated,
			}, nil
		}
	}
	return domain.ChatResponse{}, fmt.Errorf("max iterations (%d) exhausted without final assistant message", cfg.maxIterations)
}

func (s *ChatService) dialogMessagesToLLM(msgs []domain.DialogMessage, attachments []domain.Attachment) ([]llm.Message, error) {
	byMessage := groupAttachmentsByMessage(attachments)
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		lm := llm.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			calls, err := parseStoredToolCalls(m.ToolCalls)
			if err != nil {
				return nil, fmt.Errorf("seq %d: %w", m.Seq, err)
			}
			lm.ToolCalls = calls
		}
		if m.Role == "user" {
			if err := s.applyAttachmentsToLLMMessage(&lm, byMessage[m.ID]); err != nil {
				return nil, fmt.Errorf("seq %d attachments: %w", m.Seq, err)
			}
		}
		out = append(out, lm)
	}
	return repairOrphanToolCalls(out), nil
}

func (s *ChatService) listDialogAttachments(ctx context.Context, dialogID uuid.UUID) ([]domain.Attachment, error) {
	if s.attachmentRepo == nil {
		return nil, nil
	}
	return s.attachmentRepo.ListAttachmentsByDialog(ctx, dialogID)
}

func groupAttachmentsByMessage(attachments []domain.Attachment) map[int64][]domain.Attachment {
	out := make(map[int64][]domain.Attachment)
	for _, a := range attachments {
		if a.MessageID == nil {
			continue
		}
		out[*a.MessageID] = append(out[*a.MessageID], a)
	}
	return out
}

func (s *ChatService) applyAttachmentsToLLMMessage(lm *llm.Message, attachments []domain.Attachment) error {
	if len(attachments) == 0 {
		return nil
	}

	var textParts []string
	for _, a := range attachments {
		switch a.Kind {
		case domain.AttachmentKindText:
			absPath := filepath.Join(s.attachmentsDir, a.DialogID.String(), filepath.Base(a.Path))
			content, err := os.ReadFile(absPath)
			if err != nil {
				return fmt.Errorf("read text attachment %q: %w", a.Filename, err)
			}
			textParts = append(textParts, fmt.Sprintf("--- %s ---\n%s", a.Filename, string(content)))
		case domain.AttachmentKindImage:
			absPath := filepath.Join(s.attachmentsDir, a.DialogID.String(), filepath.Base(a.Path))
			content, err := os.ReadFile(absPath)
			if err != nil {
				return fmt.Errorf("read image attachment %q: %w", a.Filename, err)
			}
			ct := a.ContentType
			if ct == "" {
				ct = "image/png"
			}
			dataURL := fmt.Sprintf("data:%s;base64,%s", ct, base64.StdEncoding.EncodeToString(content))
			lm.Images = append(lm.Images, llm.ImageContent{DataURL: dataURL})
		}
	}

	if len(textParts) > 0 {
		joined := strings.Join(textParts, "\n\n")
		if lm.Content != "" {
			lm.Content = lm.Content + "\n\n" + joined
		} else {
			lm.Content = joined
		}
	}
	return nil
}

func dialogMessagesToLLM(msgs []domain.DialogMessage) ([]llm.Message, error) {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		lm := llm.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			calls, err := parseStoredToolCalls(m.ToolCalls)
			if err != nil {
				return nil, fmt.Errorf("seq %d: %w", m.Seq, err)
			}
			lm.ToolCalls = calls
		}
		out = append(out, lm)
	}
	return repairOrphanToolCalls(out), nil
}

func marshalToolCalls(calls []llm.ToolCall) (json.RawMessage, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	items := make([]map[string]any, len(calls))
	for i, tc := range calls {
		items[i] = map[string]any{
			"id":   tc.ID,
			"type": "function",
			"function": map[string]string{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		}
	}
	return json.Marshal(items)
}

func parseStoredToolCalls(raw json.RawMessage) ([]llm.ToolCall, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, nil
	}

	var items []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, fmt.Errorf("parse tool_calls: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}

	out := make([]llm.ToolCall, 0, len(items))
	for i, it := range items {
		if it.ID == "" {
			return nil, fmt.Errorf("tool_calls[%d]: missing id", i)
		}
		if it.Type != "" && it.Type != "function" {
			return nil, fmt.Errorf("tool_calls[%d]: unsupported type %q", i, it.Type)
		}
		if it.Function.Name == "" {
			return nil, fmt.Errorf("tool_calls[%d]: missing function.name", i)
		}
		out = append(out, llm.ToolCall{
			ID:        it.ID,
			Name:      it.Function.Name,
			Arguments: it.Function.Arguments,
		})
	}
	return out, nil
}
