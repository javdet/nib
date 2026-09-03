package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

const (
	decomposeSubagentMode  = "decompose"
	decomposeSubagentTitle = "Decompose"
	// decomposeSubagentPromptName narrows the decompose prompt for a sub-agent:
	// its answer is relayed, and running the plan is the orchestrator's job.
	decomposeSubagentPromptName = "decompose_subagent"
)

var (
	// ErrNotMainDialog is returned when a sub-agent launch is asked of a dialog
	// that is not an orchestrator root.
	ErrNotMainDialog = errors.New("only a main dialog can launch sub-agents")
)

// launchDecomposeSubagent runs the decompose sub-agent inside the orchestrator's
// turn.
//
// It blocks on purpose. Decomposition is a conversation: it puts two questions
// at a time to the operator and cannot go further without the answers, so there
// is nothing for the orchestrator to do while it runs. Planning and execution do
// not have that shape and start asynchronously instead.
//
// One decompose dialog per plan, created on the first call and reused by every
// later one, so the DAG is revised in the conversation that produced it rather
// than by an agent starting from nothing.
func (s *ChatService) launchDecomposeSubagent(ctx context.Context, rootID uuid.UUID, task string) (SubagentResult, error) {
	if _, err := s.resolveOrchestratorRoot(ctx, rootID); err != nil {
		return SubagentResult{}, err
	}

	claim := s.subagentClaim(rootID, SubagentDecompose)
	if !claim.tryLock() {
		return SubagentResult{
			Status:  SubagentRefused,
			Summary: "the decompose sub-agent is already working on this plan; wait for it to come back",
		}, nil
	}
	defer claim.unlock()

	decomposeID, err := s.decomposeSubagentDialog(ctx, rootID)
	if err != nil {
		return SubagentResult{}, err
	}

	if err := s.seedDecomposeSubagent(ctx, rootID, decomposeID, task); err != nil {
		return SubagentResult{}, err
	}

	return s.runDecomposeSubagentTurn(ctx, rootID, decomposeID)
}

// decomposeSubagentDialog finds this plan's decompose transcript, or starts one.
func (s *ChatService) decomposeSubagentDialog(ctx context.Context, rootID uuid.UUID) (uuid.UUID, error) {
	children, err := s.dialogRepo.ListChildren(ctx, rootID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("list plan children: %w", err)
	}
	for _, child := range children {
		if child.Mode == decomposeSubagentMode {
			return child.ID, nil
		}
	}

	// A fixed title, because chat_name belongs to the orchestrator: a sub-agent
	// naming its own transcript is fine, naming the operator's is not.
	created, err := s.dialogRepo.CreateDialog(ctx, decomposeSubagentMode, decomposeSubagentTitle, &rootID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create decompose dialog: %w", err)
	}
	return created.ID, nil
}

// seedDecomposeSubagent adds task to the sub-agent's transcript in whichever of
// three shapes fits where that transcript currently stands.
func (s *ChatService) seedDecomposeSubagent(ctx context.Context, rootID, decomposeID uuid.UUID, task string) error {
	msgs, err := s.dialogRepo.ListMessages(ctx, decomposeID)
	if err != nil {
		return fmt.Errorf("list decompose messages: %w", err)
	}

	// A dangling ask_question means the sub-agent is suspended and this task is
	// the operator's answer arriving the long way round -- the orchestrator
	// relaying it as prose rather than through the structured resume path. A
	// tool result is a free-text string everywhere else here, so prose is a
	// legitimate answer.
	if callID := danglingAskQuestionID(msgs); callID != "" {
		if _, err := s.appendMessageLocked(ctx, decomposeID, domain.DialogMessage{
			Role:       "tool",
			Content:    task,
			ToolCallID: callID,
			Name:       AskQuestionToolName,
		}); err != nil {
			return fmt.Errorf("answer dangling question: %w", err)
		}
		// The pause is settled either way now; leaving it would let a later
		// answer write a second result for the same call.
		s.discardSubagentPauseFor(rootID, decomposeID, callID)
		return nil
	}

	if len(msgs) == 0 {
		sysPrompt, err := s.subagentSystemPrompt(ctx, decomposeSubagentMode, decomposeSubagentPromptName)
		if err != nil {
			return err
		}
		if _, err := s.appendMessageLocked(ctx, decomposeID, domain.DialogMessage{
			Role:    "system",
			Content: sysPrompt,
		}); err != nil {
			return fmt.Errorf("append decompose system prompt: %w", err)
		}
	}

	if _, err := s.appendMessageLocked(ctx, decomposeID, domain.DialogMessage{
		Role:    "user",
		Content: task,
	}); err != nil {
		return fmt.Errorf("append decompose task: %w", err)
	}
	return nil
}

// runDecomposeSubagentTurn runs the sub-agent to its next stop and maps that
// stop into something the orchestrator can act on. It is shared by a launch and
// by a resume, so the two cannot drift.
func (s *ChatService) runDecomposeSubagentTurn(ctx context.Context, rootID, decomposeID uuid.UUID) (SubagentResult, error) {
	allow, err := s.decomposeSubagentAllowSet()
	if err != nil {
		return SubagentResult{}, err
	}

	// The transcript is the sub-agent's; every plan artifact belongs to the
	// root. That split is what puts the DAG and the summary on the operator's
	// plan rather than on this transcript.
	catalog, err := s.buildToolCatalog(ctx, allow, toolBinding{dialogID: decomposeID, planID: rootID})
	if err != nil {
		return SubagentResult{}, fmt.Errorf("build decompose catalog: %w", err)
	}

	slog.Info("decompose subagent turn", "root_id", rootID, "dialog_id", decomposeID)

	resp, err := s.runPersistingAgentLoop(ctx, decomposeID, decomposeSubagentMode, catalog, loopConfig{
		planID: rootID,
	})
	if err != nil {
		return SubagentResult{}, err
	}

	if resp.Status == "awaiting_input" {
		if err := s.recordSubagentPause(rootID, SubagentPause{
			Subagent:   SubagentDecompose,
			DialogID:   decomposeID.String(),
			ToolCallID: resp.ToolCallID,
			Questions:  resp.Questions,
			PausedAt:   time.Now().Unix(),
		}); err != nil {
			// Without the record the answers cannot be routed back, so the
			// operator would answer into nothing. Better to say so.
			return SubagentResult{}, fmt.Errorf("record decompose pause: %w", err)
		}
		return SubagentResult{
			Status:    SubagentAwaitingInput,
			Questions: resp.Questions,
		}, nil
	}

	s.adoptDecomposeTitle(ctx, rootID, decomposeID)

	return SubagentResult{
		Status:  SubagentCompleted,
		Summary: resp.Response,
	}, nil
}

// decomposeSubagentAllowSet is the decompose allow set narrowed for a sub-agent.
//
// ask_question stays: this is the one sub-agent whose questions are relayed to
// the operator, so it is the one that can suspend. Categories are deliberately
// not inherited -- decompose is where they are chosen, so inheriting them would
// make its own catalog depend on its own earlier output, and tool_search is the
// designed way to reach anything else.
func (s *ChatService) decomposeSubagentAllowSet() (map[string]struct{}, error) {
	allow, err := s.resolveAllowSet(decomposeSubagentMode)
	if err != nil {
		return nil, err
	}
	return stripSubagentTools(allow), nil
}

// adoptDecomposeTitle copies the sub-agent's title and task id onto the plan when
// the orchestrator has not named it yet.
//
// decompose.md still tells the sub-agent to call chat_name at the start of its
// work, and that call names its own transcript. Without this the plans list
// would show "New plan" for a plan that has a perfectly good name one dialog
// away -- and the task id, which is what names the branch a code action pushes
// to, would sit where nothing reads it.
func (s *ChatService) adoptDecomposeTitle(ctx context.Context, rootID, decomposeID uuid.UUID) {
	root, err := s.dialogRepo.GetDialog(ctx, rootID)
	if err != nil {
		slog.Warn("adopt decompose title: read root", "root_id", rootID, "error", err)
		return
	}
	if strings.TrimSpace(root.Title) != "" {
		return
	}

	child, err := s.dialogRepo.GetDialog(ctx, decomposeID)
	if err != nil {
		slog.Warn("adopt decompose title: read subagent", "dialog_id", decomposeID, "error", err)
		return
	}
	title := strings.TrimSpace(child.Title)
	if title == "" || title == decomposeSubagentTitle {
		return
	}

	if err := s.dialogRepo.UpdateTitle(ctx, rootID, title); err != nil {
		slog.Warn("adopt decompose title: write title", "root_id", rootID, "error", err)
		return
	}
	if child.TaskID != nil {
		if taskID := strings.TrimSpace(*child.TaskID); taskID != "" {
			if err := s.dialogRepo.SetDialogTaskID(ctx, rootID, &taskID); err != nil {
				slog.Warn("adopt decompose title: write task id", "root_id", rootID, "error", err)
			}
		}
	}
}

// danglingAskQuestionID is the id of an ask_question call with no result, which
// is what a suspended sub-agent leaves behind.
func danglingAskQuestionID(msgs []domain.DialogMessage) string {
	answered := make(map[string]struct{}, len(msgs))
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID != "" {
			answered[m.ToolCallID] = struct{}{}
		}
	}

	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "assistant" || len(msgs[i].ToolCalls) == 0 {
			continue
		}
		calls, err := parseStoredToolCalls(msgs[i].ToolCalls)
		if err != nil {
			continue
		}
		for _, tc := range calls {
			if tc.Name != AskQuestionToolName {
				continue
			}
			if _, done := answered[tc.ID]; !done {
				return tc.ID
			}
		}
	}
	return ""
}

// discardSubagentPauseFor forgets a pause whose question has just been answered
// by another route.
func (s *ChatService) discardSubagentPauseFor(rootID, dialogID uuid.UUID, toolCallID string) {
	if _, err := s.updateSubagentState(rootID, func(state *SubagentState) {
		kept := make([]SubagentPause, 0, len(state.Pauses))
		for _, p := range state.Pauses {
			if p.DialogID == dialogID.String() && p.ToolCallID == toolCallID {
				continue
			}
			kept = append(kept, p)
		}
		state.Pauses = kept
	}); err != nil {
		slog.Warn("discard subagent pause", "root_id", rootID, "error", err)
	}
}
