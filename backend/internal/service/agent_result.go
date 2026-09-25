package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/textutil"
)

const agentRunMessageNamePrefix = "agent-run:"

// codeActionReportMaxBytes caps what a failed code action spells out in the plan
// chat; the whole of it is in the execute dialog the report links to.
const codeActionReportMaxBytes = 4000

// AgentRunResult is the completion payload posted by the claude-code agent entrypoint.
type AgentRunResult struct {
	Status       string
	ExitCode     int
	Result       string
	LogTail      string
	TaskID       string
	ThreadRootID string
	ChatID       string
	JobName      string
	JobPod       string
	JobNamespace string
	ImageVersion string
	RepoURL      string
	BaseBranch   string
	TargetBranch string
	RepoPushed   bool
	PRURL        string
	DurationMS   int
	NumTurns     int
	TotalCostUSD *float64
	SessionID    string
	InputTokens  int
	OutputTokens int
}

// AppendAgentResult stores the agent's conclusion as an assistant message in the
// execute chat named by res.ChatID. Duplicate deliveries for the same job name
// are ignored so curl retries do not create duplicate rows.
func (s *ChatService) AppendAgentResult(ctx context.Context, dialogID uuid.UUID, res AgentRunResult) (domain.DialogMessage, error) {
	if s.dialogRepo == nil {
		return domain.DialogMessage{}, fmt.Errorf("append agent result: dialog repository is not configured")
	}

	dialog, err := s.dialogRepo.GetDialog(ctx, dialogID)
	if err != nil {
		return domain.DialogMessage{}, fmt.Errorf("append agent result: %w", err)
	}

	s.warnAttachActionPullRequest(ctx, dialog, res.PRURL)

	msgName := agentRunMessageName(res.JobName)
	if msgName != "" {
		existing, err := s.dialogRepo.ListMessages(ctx, dialogID)
		if err != nil {
			return domain.DialogMessage{}, fmt.Errorf("append agent result: list messages: %w", err)
		}
		for _, msg := range existing {
			if msg.Name == msgName {
				s.finishCodeActionRun(ctx, dialog, res)
				s.activity.Publish(dialogID, domain.AgentActivity{Kind: domain.ActivityAgentResult})
				return msg, nil
			}
		}
	}

	content := formatAgentResultMessage(res)
	msg := domain.DialogMessage{
		Role:    "assistant",
		Content: content,
	}
	if msgName != "" {
		msg.Name = msgName
	}

	stored, err := s.dialogRepo.AppendMessage(ctx, dialogID, msg)
	if err != nil {
		return domain.DialogMessage{}, fmt.Errorf("append agent result: %w", err)
	}

	// Deliberately after the dedup return above: the container retries this
	// webhook with curl, and recording before that check would multiply the
	// cost by the number of retries. The partial unique index on job_name is
	// the second line of defence, for deliveries that carry no job name.
	s.recordAgentRun(ctx, dialogID, res)

	s.finishCodeActionRun(ctx, dialog, res)

	s.activity.Publish(dialogID, domain.AgentActivity{Kind: domain.ActivityAgentResult})
	return stored, nil
}

// finishCodeActionRun closes the exec run of the code action this webhook belongs
// to and hands the execution lease back.
//
// Nothing did this before: launching a code action recorded "running" and only a
// sub-agent run ever recorded an ending, so every code action stayed running
// forever -- a permanent spinner in the plan view, and now, with one execution
// at a time, a permanent block on every other action in every plan.
//
// Both callers may reach it for the same delivery, so every step is idempotent:
// finishActionExecRun leaves an already-closed run alone and
// releaseExecutionLease only drops a lease still describing this run.
func (s *ChatService) finishCodeActionRun(ctx context.Context, exec domain.Dialog, res AgentRunResult) {
	if exec.ParentID == nil {
		return
	}
	planID := *exec.ParentID

	runs, err := s.ReadActionPlanRuns(planID)
	if err != nil {
		slog.Warn("finish code action run: read runs",
			"exec_dialog_id", exec.ID, "plan_dialog_id", planID, "error", err)
		return
	}
	key := findRunKeyByExecID(runs, exec.ID)
	if key == "" {
		// Not an action row. It may still be an ad-hoc code fix asked for in the
		// chat, which reports the same way and is recognised by its own record;
		// run_executor also lands here and has no record at all.
		s.finishCodeFixRun(context.WithoutCancel(ctx), planID, exec.ID, res)
		return
	}

	status, errMsg := ActionExecDone, ""
	if !strings.EqualFold(strings.TrimSpace(res.Status), "success") {
		status = ActionExecFailed
		errMsg = fmt.Sprintf("the coding agent exited with code %d", res.ExitCode)
	}

	run := s.finishActionExecRun(planID, key, status, errMsg)
	// A code action reports into its own execute dialog, which the actions after
	// it never see, so its account of the work is recorded on the action the same
	// way a sub-agent's is. Both callers may reach this for one delivery; writing
	// the same note twice is a no-op.
	s.recordActionNote(planID, key, codeActionNoteText(res))
	s.releaseExecutionLeaseForContainer(planID, key, res.JobName)

	// The container's own account lands in its execute dialog, and nothing in the
	// plan view links to it, so until now finishing a code action turned a row
	// green in silence while a sub-agent action posted its outcome in the plan
	// chat. They are the same event to the operator reading it, so they report
	// the same way -- done plus a link to the transcript, or the reason it is
	// not done. reportActionResult publishes the activity event this used to
	// publish on its own and skips a row it has already written, so the
	// duplicate-delivery caller adds nothing.
	s.reportActionResult(context.WithoutCancel(ctx), planID, key, run.Attempt, exec.ID,
		status, codeActionReportText(res, errMsg))
	s.onPlanItemClosed(planID, key)
}

// codeActionReportText is the body of that report. It is dropped on success --
// the result is working notes for the actions that follow, not something the
// operator has to read -- so this only ever shows up on a failure, where the
// exit code alone says nothing about what went wrong and the agent's last words
// do. The log tail and the usage footer stay in the execute dialog, one link
// away, and the text is capped because the plan chat is also the orchestrator's
// context.
func codeActionReportText(res AgentRunResult, errMsg string) string {
	parts := make([]string, 0, 2)
	if errMsg != "" {
		parts = append(parts, errMsg)
	}
	if result := strings.TrimSpace(res.Result); result != "" {
		parts = append(parts, textutil.TruncateBytes(result, "\n\n_(truncated)_", codeActionReportMaxBytes))
	}
	return strings.Join(parts, "\n\n")
}

// codeActionNoteText is what a code action leaves for the actions after it. It
// is built from the agent's own result rather than reused from
// formatAgentResultMessage: the log tail, the cost and the turn count are there
// for the operator, while the branch is a value a later action genuinely needs
// and lives nowhere else. The pull request is left out on purpose -- it is
// written onto the action as `pr_url`, which get_action_list already returns.
func codeActionNoteText(res AgentRunResult) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(res.Result))
	if branch := strings.TrimSpace(res.TargetBranch); branch != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "Branch: %s", branch)
	}
	return b.String()
}

func agentRunMessageName(jobName string) string {
	jobName = strings.TrimSpace(jobName)
	if jobName == "" {
		return ""
	}
	return agentRunMessageNamePrefix + jobName
}

func formatAgentResultMessage(res AgentRunResult) string {
	var b strings.Builder

	if res.Status != "success" {
		fmt.Fprintf(&b, "**Agent run failed** (exit code %d)\n\n", res.ExitCode)
	}

	body := strings.TrimSpace(res.Result)
	if body != "" {
		b.WriteString(body)
	} else if res.Status != "success" {
		b.WriteString("_No result text was produced._")
	}

	if tail := strings.TrimSpace(res.LogTail); tail != "" && res.Status != "success" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("```\n")
		b.WriteString(tail)
		b.WriteString("\n```")
	}

	footer := formatAgentResultFooter(res)
	if footer != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n---\n\n")
		} else {
			b.WriteString("---\n\n")
		}
		b.WriteString(footer)
	}

	return strings.TrimSpace(b.String())
}

func formatAgentResultFooter(res AgentRunResult) string {
	var lines []string

	if pr := strings.TrimSpace(res.PRURL); pr != "" {
		lines = append(lines, fmt.Sprintf("**PR:** %s", pr))
	}
	if branch := strings.TrimSpace(res.TargetBranch); branch != "" {
		lines = append(lines, fmt.Sprintf("**Branch:** `%s`", branch))
	}

	usageParts := make([]string, 0, 3)
	if res.DurationMS > 0 {
		usageParts = append(usageParts, fmt.Sprintf("%ds", res.DurationMS/1000))
	}
	if res.NumTurns > 0 {
		usageParts = append(usageParts, fmt.Sprintf("%d turns", res.NumTurns))
	}
	if res.TotalCostUSD != nil && *res.TotalCostUSD > 0 {
		usageParts = append(usageParts, fmt.Sprintf("$%.4f", *res.TotalCostUSD))
	}
	if len(usageParts) > 0 {
		lines = append(lines, strings.Join(usageParts, " · "))
	}

	return strings.Join(lines, "\n")
}

// ErrAgentResultDialogNotFound is returned when chat_id does not match a dialog.
var ErrAgentResultDialogNotFound = repository.ErrNotFound
