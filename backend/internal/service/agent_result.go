package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

const agentRunMessageNamePrefix = "agent-run:"

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
	TotalCostUSD float64
	SessionID    string
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

	s.activity.Publish(dialogID, domain.AgentActivity{Kind: domain.ActivityAgentResult})
	return stored, nil
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
	if res.TotalCostUSD > 0 {
		usageParts = append(usageParts, fmt.Sprintf("$%.4f", res.TotalCostUSD))
	}
	if len(usageParts) > 0 {
		lines = append(lines, strings.Join(usageParts, " · "))
	}

	return strings.Join(lines, "\n")
}

// ErrAgentResultDialogNotFound is returned when chat_id does not match a dialog.
var ErrAgentResultDialogNotFound = repository.ErrNotFound
