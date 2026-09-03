package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// ErrActionAlreadyRunning is returned when a subagent already holds an action row.
var ErrActionAlreadyRunning = errors.New("an action sub-agent is already running for this action")

// actionAgentSystemPrompt is the execute prompt plus the subagent overlay that
// narrows it to one action and takes away its ability to ask the operator.
func (s *ChatService) actionAgentSystemPrompt(ctx context.Context) (string, error) {
	base, err := s.resolveSystemPrompt(ctx, executeDialogMode)
	if err != nil {
		return "", err
	}
	if s.systemPromptsSvc == nil {
		return base, nil
	}
	overlay, err := s.systemPromptsSvc.Get(actionExecPromptName)
	if err != nil {
		return "", fmt.Errorf("resolve %s prompt: %w", actionExecPromptName, err)
	}
	rendered, err := RenderTemplateVariables(ctx, overlay.Content, s.variableRepo, s.selection)
	if err != nil {
		return "", err
	}
	return base + "\n\n" + rendered, nil
}

// actionAgentAllowSet is the execute allow set narrowed for a subagent. It holds
// one row of somebody else's plan, so every tool that could rewrite the plan,
// suspend the turn, spawn another subagent, or start work with nowhere to report
// back comes out.
func (s *ChatService) actionAgentAllowSet(ctx context.Context, planID uuid.UUID) (map[string]struct{}, error) {
	allow, err := s.resolveAllowSet(executeDialogMode)
	if err != nil {
		return nil, err
	}

	if s.toolCategorySvc != nil && s.dialogRepo != nil {
		d, err := s.dialogRepo.GetDialog(ctx, planID)
		if err != nil {
			return nil, err
		}
		for _, name := range d.Categories {
			tools, err := s.toolCategorySvc.ListToolsByCategory(ctx, name)
			if err != nil {
				slog.Warn("action agent allow set: list tools by category", "category", name, "error", err)
				continue
			}
			for _, t := range tools {
				allow[t.Name] = struct{}{}
			}
		}
	}

	// Category inheritance only ever adds MCP tools, so these deletes guard
	// against an operator's edit to the execute allow list rather than against
	// inheritance -- but the cost of being wrong here is a subagent rewriting
	// the plan it was asked to carry out one line of.
	stripSubagentTools(allow)
	delete(allow, AskQuestionToolName)
	delete(allow, CreateActionPlanToolName)
	delete(allow, UpdateActionPlanToolName)
	delete(allow, UpdateRollbackPlanToolName)
	// run_executor's request carries no chat id, so the agent-runner it starts
	// has no dialog to report into and its work would vanish. A code action goes
	// through ExecuteCodeAction, which does pass one.
	delete(allow, RunExecutorToolName)
	return allow, nil
}

// buildActionSeed is the first user message of an action subagent: the plan
// context every subagent gets, then the one action this run owns.
func (s *ChatService) buildActionSeed(
	ctx context.Context,
	planID uuid.UUID,
	key, number string,
	step storedActionStep,
) (string, error) {
	parts, _, err := s.sharedSeedSections(ctx, planID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("## Your action\n\n")
	if number != "" {
		fmt.Fprintf(&b, "Number: %s\n", number)
	}
	if t := strings.TrimSpace(step.Type); t != "" {
		fmt.Fprintf(&b, "Type: %s\n", t)
	}
	if r := strings.TrimSpace(step.Repository); r != "" {
		fmt.Fprintf(&b, "Repository: %s\n", r)
	}
	b.WriteString("\n" + strings.TrimSpace(step.Action) + "\n")
	if c := strings.TrimSpace(step.Command); c != "" {
		b.WriteString("\n### Command\n\n```\n" + c + "\n```\n")
	}
	parts = append(parts, strings.TrimRight(b.String(), "\n"))

	comments, err := s.ReadActionPlanComments(planID)
	if err != nil {
		return "", fmt.Errorf("read comments: %w", err)
	}
	if c := strings.TrimSpace(comments[key]); c != "" {
		parts = append(parts, "## Comment\n\n"+c)
	}

	return strings.Join(parts, "\n\n"), nil
}

// reportActionResult posts the outcome of a subagent run into the plan chat, so
// the operator sees it where they asked for it rather than having to open the
// subagent's own dialog.
//
// The row is named after the action so a retried report cannot double-post, the
// same guard AppendAgentResult uses for webhook retries.
func (s *ChatService) reportActionResult(
	ctx context.Context,
	planID uuid.UUID,
	key string,
	attempt int,
	dialogID uuid.UUID,
	status ActionExecStatus,
	body string,
) {
	kind := domain.ActivityActionExecDone
	if status != ActionExecDone {
		kind = domain.ActivityActionExecFailed
	}
	defer s.activity.Publish(planID, domain.AgentActivity{
		Kind:   kind,
		Action: key,
		Status: string(status),
	})

	if s.dialogRepo == nil {
		return
	}

	name := actionExecMessageName(key, attempt)
	existing, err := s.dialogRepo.ListMessages(ctx, planID)
	if err != nil {
		slog.Warn("action result: list messages", "plan_id", planID, "key", key, "error", err)
		return
	}
	for _, m := range existing {
		if m.Name == name {
			return
		}
	}

	if _, err := s.appendMessageLocked(ctx, planID, domain.DialogMessage{
		Role:    "assistant",
		Content: formatActionResultMessage(key, dialogID, status, body),
		Name:    name,
	}); err != nil {
		slog.Warn("action result: append", "plan_id", planID, "key", key, "error", err)
	}
}

// actionExecMessageName identifies one report of one attempt. The attempt is
// part of it because a restart must post its own result: keying on the row alone
// made the retry look like a duplicate of the run it replaced, and the operator
// was left reading the outcome of the attempt they had just abandoned.
func actionExecMessageName(key string, attempt int) string {
	return fmt.Sprintf("action-exec:%s#%d", key, attempt)
}

// formatActionResultMessage renders the outcome. The link is a fragment the chat
// panel intercepts to open the subagent's dialog: there is no route to a dialog
// by id, so a plain URL would go nowhere.
func formatActionResultMessage(key string, dialogID uuid.UUID, status ActionExecStatus, body string) string {
	number := actionPlanNumberForKey(key)
	if number == "" {
		number = key
	}

	var b strings.Builder
	switch status {
	case ActionExecDone:
		fmt.Fprintf(&b, "**%s executed** ✅\n", number)
	case ActionExecBlocked:
		fmt.Fprintf(&b, "**%s needs a decision** ⏸\n", number)
	case ActionExecCancelled:
		fmt.Fprintf(&b, "**%s cancelled** ⏹\n", number)
	default:
		fmt.Fprintf(&b, "**%s failed** ❌\n", number)
	}
	if text := strings.TrimSpace(body); text != "" {
		b.WriteString("\n" + text + "\n")
	}
	if dialogID != uuid.Nil {
		fmt.Fprintf(&b, "\n[View sub-agent transcript](#dialog:%s)\n", dialogID)
	}
	return strings.TrimRight(b.String(), "\n")
}

func subagentCannotAskPayload(toolCallID string) string {
	const text = "this sub-agent runs one action and cannot put a question to the operator; " +
		"the run ended here and the question was reported in the plan chat"
	payload := map[string]any{
		"isError": true,
		"content": []map[string]string{{"type": "text", "text": text}},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf(`{"isError":true,"content":[{"type":"text","text":%q}]}`, text)
	}
	return string(b)
}
