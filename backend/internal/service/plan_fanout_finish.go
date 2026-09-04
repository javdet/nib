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

// blockerQuestion is one question after folding together the stages that raised it.
type blockerQuestion struct {
	Question string
	Options  []string
	// Stages names the DAG stages that raised the question, and is empty when
	// only the rollback agent did: it owns no stage, so answering it replans
	// none.
	Stages []string
}

// groupBlockers folds identical questions raised by different stages into one, so
// the user answers a thing once instead of once per stage. Order follows the first
// stage that asked, which keeps the prompt stable across reads of the run.
func groupBlockers(blockers []PlanBlocker) []blockerQuestion {
	var out []blockerQuestion
	index := make(map[string]int)

	for _, b := range blockers {
		if strings.TrimSpace(b.Answer) != "" {
			continue
		}
		key := strings.ToLower(strings.Join(strings.Fields(b.Question), " "))
		if key == "" {
			continue
		}
		if i, ok := index[key]; ok {
			out[i].Options = mergeOptions(out[i].Options, b.Options)
			if b.Kind == FanoutStageKindStage {
				out[i].Stages = append(out[i].Stages, b.Stage)
			}
			continue
		}
		index[key] = len(out)
		q := blockerQuestion{
			Question: b.Question,
			Options:  append([]string(nil), b.Options...),
		}
		if b.Kind == FanoutStageKindStage {
			q.Stages = []string{b.Stage}
		}
		out = append(out, q)
	}
	return out
}

func mergeOptions(existing, extra []string) []string {
	seen := make(map[string]struct{}, len(existing))
	for _, o := range existing {
		seen[strings.ToLower(o)] = struct{}{}
	}
	for _, o := range extra {
		if _, dup := seen[strings.ToLower(o)]; dup {
			continue
		}
		seen[strings.ToLower(o)] = struct{}{}
		existing = append(existing, o)
	}
	return existing
}

// finishPlanFanout closes a run. When stages raised questions it puts up to two of
// them to the user as a single ask_question on the root dialog and parks the
// run; otherwise the run is simply done. Every stage was written either way.
func (s *ChatService) finishPlanFanout(ctx context.Context, rootID uuid.UUID) {
	run, found, err := s.ReadFanoutRun(rootID)
	if err != nil || !found {
		slog.Warn("plan fanout: finish without a run record", "dialog_id", rootID, "error", err)
		return
	}

	questions := groupBlockers(run.Blockers)
	if len(questions) == 0 {
		s.closeFanoutRun(rootID, FanoutRunDone, "")
		return
	}

	asked := questions
	if len(asked) > maxBlockerQuestionsPerRound {
		asked = asked[:maxBlockerQuestionsPerRound]
	}

	callID, err := s.appendFanoutQuestion(ctx, rootID, run, asked, len(questions))
	if err != nil {
		slog.Error("plan fanout: could not ask the user", "dialog_id", rootID, "error", err)
		s.closeFanoutRun(rootID, FanoutRunDone, "")
		return
	}

	var stages []string
	seen := make(map[string]struct{})
	for _, q := range asked {
		for _, stage := range q.Stages {
			if _, dup := seen[normalizeStageTitle(stage)]; dup {
				continue
			}
			seen[normalizeStageTitle(stage)] = struct{}{}
			stages = append(stages, stage)
		}
	}

	if _, err := s.updateFanoutRun(rootID, func(r *FanoutRun) {
		r.Status = FanoutRunAwaitingInput
		r.FinishedAt = time.Now().Unix()
		r.PendingAskID = callID
		r.PendingStages = stages
		// Always: the run the answers start redoes the rollback whether or not
		// the rollback agent was the one that asked.
		r.PendingRollback = true
	}); err != nil {
		slog.Error("plan fanout: record pending question", "dialog_id", rootID, "error", err)
	}

	s.activity.Publish(rootID, domain.AgentActivity{
		Kind:   domain.ActivityPlanFanoutDone,
		Status: string(FanoutRunAwaitingInput),
	})
}

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
	// A run that raised no question would otherwise finish in silence: the plan
	// appears, and the chat the operator asked in says nothing. The
	// awaiting_input path already posts its question and needs none of this.
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

// appendFanoutQuestion puts the questions a fan-out raised to the operator in
// the chat that asked for the planning.
func (s *ChatService) appendFanoutQuestion(
	ctx context.Context,
	rootID uuid.UUID,
	run FanoutRun,
	asked []blockerQuestion,
	total int,
) (string, error) {
	questions := make([]domain.Question, 0, len(asked))
	for _, q := range asked {
		questions = append(questions, domain.Question{
			Question: q.Question,
			Options:  q.Options,
		})
	}
	return s.appendSyntheticAskQuestion(ctx, rootID,
		fanoutQuestionPreamble(run, asked, total), questions, fanoutAskIDPrefix)
}

func fanoutQuestionPreamble(run FanoutRun, asked []blockerQuestion, total int) string {
	done, failed := 0, 0
	for _, st := range run.Stages {
		switch st.Status {
		case FanoutStageDone:
			done++
		case FanoutStageFailed:
			failed++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Planned %d of %d stages.", done, len(run.Stages))
	if failed > 0 {
		fmt.Fprintf(&b, " %d failed and can be retried.", failed)
	}
	b.WriteString(" Every stage was written, but some rest on assumptions I had to make while planning.\n\n")

	var stages []string
	for _, q := range asked {
		stages = append(stages, q.Stages...)
	}
	// The rollback is derived from the stages, so it is redone whether or not it
	// was the agent that asked.
	if replans := dedupeStages(stages); len(replans) > 0 {
		fmt.Fprintf(&b, "Answering the questions below replans %s, and redoes the rollback with them.",
			strings.Join(replans, ", "))
	} else {
		b.WriteString("Answering the questions below redoes the rollback.")
	}
	if total > len(asked) {
		fmt.Fprintf(&b, " %d further question(s) follow once these are settled.", total-len(asked))
	}
	return b.String()
}

func dedupeStages(stages []string) []string {
	seen := make(map[string]struct{}, len(stages))
	var out []string
	for _, s := range stages {
		key := normalizeStageTitle(s)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

// resumeFanoutFromAnswers records the user's answers against the blockers that
// asked for them and replans exactly the stages affected. It reports false when
// toolCallID is not a fan-out question, leaving the ordinary resume path to it.
func (s *ChatService) resumeFanoutFromAnswers(
	ctx context.Context,
	rootID uuid.UUID,
	toolCallID string,
	questions []domain.Question,
	answers []string,
) (bool, error) {
	run, found, err := s.ReadFanoutRun(rootID)
	if err != nil || !found || run.PendingAskID == "" || run.PendingAskID != toolCallID {
		return false, nil
	}

	answerFor := make(map[string]string, len(questions))
	for i, q := range questions {
		if i < len(answers) {
			answerFor[strings.ToLower(strings.Join(strings.Fields(q.Question), " "))] = answers[i]
		}
	}

	updated, err := s.updateFanoutRun(rootID, func(r *FanoutRun) {
		for i := range r.Blockers {
			key := strings.ToLower(strings.Join(strings.Fields(r.Blockers[i].Question), " "))
			if answer, ok := answerFor[key]; ok {
				r.Blockers[i].Answer = answer
			}
		}
		r.PendingAskID = ""
		r.PendingRollback = false
	})
	if err != nil {
		return false, err
	}

	if _, err := s.StartPlanFanout(ctx, rootID, FanoutTargets{
		Stages: updated.PendingStages,
		// The rollback undoes whatever the stages end up saying, so a replanned
		// stage leaves it stale even when the answer was never about it.
		Rollback: true,
	}); err != nil {
		return false, fmt.Errorf("replan answered stages: %w", err)
	}
	return true, nil
}
