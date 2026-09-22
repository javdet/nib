package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/metrics"
)

// closeFanoutRun records the end of a run and reports it into the chat that
// asked for the planning.
func (s *ChatService) closeFanoutRun(rootID uuid.UUID, status FanoutRunStatus, errMsg string) {
	run, err := s.updateFanoutRun(rootID, func(r *FanoutRun) {
		r.Status = status
		r.FinishedAt = time.Now().Unix()
		r.Error = errMsg
	})
	var ran time.Duration
	if run.StartedAt > 0 && run.FinishedAt > 0 {
		ran = time.Duration(run.FinishedAt-run.StartedAt) * time.Second
	}
	metrics.RecordFanoutFinished(string(status), ran)
	metrics.AddFanoutBlockers(len(run.Blockers))
	if err != nil {
		slog.Error("plan fanout: close run", "dialog_id", rootID, "error", err)
	}
	s.activity.Publish(rootID, domain.AgentActivity{
		Kind:   domain.ActivityPlanFanoutDone,
		Status: string(status),
	})
	// A run would otherwise finish in silence: the plan appears, and the chat
	// the operator asked in says nothing.
	s.reportFanoutResult(context.Background(), rootID, run, status)
	slog.Info("plan fanout finished", "dialog_id", rootID, "status", status)
}

// reportFanoutResult posts the outcome of a planning run into the chat that
// asked for it, so the operator reads it where they asked.
//
// Named after the run, so a close that happens twice cannot double-post -- the
// same guard the action sub-agent's report uses.
func (s *ChatService) reportFanoutResult(
	ctx context.Context,
	rootID uuid.UUID,
	run FanoutRun,
	status FanoutRunStatus,
) {
	if s.dialogRepo == nil || run.RunID == "" {
		return
	}

	name := "plan-fanout:" + run.RunID
	msgs, err := s.dialogRepo.ListMessages(ctx, rootID)
	if err != nil {
		slog.Warn("plan fanout: list messages before reporting", "dialog_id", rootID, "error", err)
		return
	}
	for _, msg := range msgs {
		if msg.Name == name {
			return
		}
	}

	// Carried rows count too, so the number is the plan's coverage rather than
	// the round's: "4 of 5" after a one-stage replan is what the operator sees in
	// the stage list, and a sentence that counted only the round would disagree
	// with it.
	planned, failed := 0, 0
	for _, st := range run.Stages {
		switch st.Status {
		case FanoutStageDone:
			planned++
		case FanoutStageFailed:
			failed++
		}
	}

	var body string
	switch {
	case status == FanoutRunFailed:
		body = fmt.Sprintf("**Planning failed** after %d of %d stages.", planned, len(run.Stages))
		if strings.TrimSpace(run.Error) != "" {
			body += "\n\n" + run.Error
		}
	default:
		body = fmt.Sprintf("**Action plan written** — %d of %d stages planned.", planned, len(run.Stages))
		if failed > 0 {
			body += fmt.Sprintf(" %d failed and can be replanned.", failed)
		}
	}
	body += "\n\nThe plan is in the Action List. This work is finished — do not plan it again unless the operator asks."

	if _, err := s.appendMessageLocked(ctx, rootID, domain.DialogMessage{
		Role:    "assistant",
		Content: body,
		Name:    name,
	}); err != nil {
		slog.Warn("plan fanout: report result", "dialog_id", rootID, "error", err)
	}
}
