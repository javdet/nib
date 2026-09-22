package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

const (
	// maxBlockerAnswerWait caps a single question. It is the only way a wait can
	// end that leaves the stage able to carry on: a stopped or out-of-budget run
	// cancels the context the agent loop is running under, so its next
	// completion fails and the stage is abandoned rather than written.
	maxBlockerAnswerWait = 2 * time.Hour
	// fanoutBudgetTick is how often the work budget is charged. Coarse on
	// purpose: the error is one tick against a budget measured in hours.
	fanoutBudgetTick = 30 * time.Second
	// maxBlockersPerStage stops one confused planner from holding the operator's
	// chat hostage. Past it the tool answers with the agent's own assumption.
	maxBlockersPerStage = 2
	// waitingForAnswerReason is the stage row's tooltip while it is blocked. The
	// UI shows Error as the tooltip whatever the status, which is where an
	// operator looks for "why is this one not moving".
	waitingForAnswerReason = "waiting for your answer in the chat"
)

// fanoutRunState is what a live fan-out owns beyond its record on disk.
type fanoutRunState struct {
	cancel context.CancelFunc
	// desk admits one question to the chat at a time. The panel renders only the
	// newest unanswered ask_question, so a second question posted beside the
	// first would strand it with no way for anyone to answer it.
	desk chan struct{}
	// waiting counts the stages blocked on the operator. The run's work budget
	// does not run down while it is above zero: an answer that takes an
	// afternoon is not time the run spent working.
	waiting atomic.Int64
}

func newFanoutRunState(cancel context.CancelFunc) *fanoutRunState {
	return &fanoutRunState{cancel: cancel, desk: make(chan struct{}, 1)}
}

// fanoutWait is one stage subagent suspended on a question, keyed by the
// synthetic call the operator answers it under.
type fanoutWait struct {
	label string
	// ch is buffered so a delivery racing a timeout is dropped rather than
	// blocking the request that carried the answer.
	ch chan string
}

// blockerAskLabel is what the question calls the agent that raised it. The
// rollback agent owns no stage, and its display title ("Rollback plan") reads
// oddly in a sentence, so it is named for what it is.
func blockerAskLabel(stage string, kind FanoutStageKind) string {
	if kind == FanoutStageKindRollback {
		return "rollback"
	}
	return stage
}

func blockerAskPreamble(label string) string {
	return fmt.Sprintf(
		"The **%s** planner is waiting on a decision before it can finish. "+
			"The other stages keep planning while you decide.", label)
}

// awaitBlockerAnswer puts one stage subagent's question to the operator and
// blocks until it is answered, the wait times out, or the run is stopped. It
// returns the tool result the subagent reads, never an error: a planner that
// cannot reach the operator falls back to its own assumption rather than losing
// its stage.
//
// The run record's lock must not be held across this call. A stage waiting with
// it held would freeze every sibling's status write, which is exactly the
// deadlock that turns "one stage waits" into "the whole plan waits".
func (s *ChatService) awaitBlockerAnswer(ctx context.Context, planID uuid.UUID, b PlanBlocker) string {
	label := blockerAskLabel(b.Stage, b.Kind)

	v, ok := s.fanoutRuns.Load(planID)
	if !ok {
		// No live run behind this call, so there is nobody to post the question
		// for. Degrade to recording it and carrying on.
		return blockerFallbackResult(b)
	}
	st := v.(*fanoutRunState)

	st.waiting.Add(1)
	s.setBlockerStageStatus(planID, b, FanoutStageAwaitingInput, waitingForAnswerReason)

	var (
		callID string
		posted bool
	)
	// The unwind is one closure rather than a stack of defers because its order
	// is what the interface reads: the stage has to be back to running before
	// the event that makes the page re-read it, or the row stays "waiting for
	// you" until something else happens.
	defer func() {
		if callID != "" {
			s.fanoutWaits.Delete(callID)
		}
		s.setBlockerStageStatus(planID, b, FanoutStageRunning, "")
		st.waiting.Add(-1)
		if posted {
			s.publishPlanQuestion(planID, b.Stage, "settled")
		}
	}()

	// Queued behind another stage's question still counts as waiting: the agent
	// is blocked on the operator either way, and the budget must not run.
	select {
	case st.desk <- struct{}{}:
	case <-ctx.Done():
		return blockerFallbackResult(b)
	}
	// Released before the unwind above, so the next stage can post while this
	// one is still writing its status back.
	defer func() { <-st.desk }()

	callID = newAskID(fanoutAskIDPrefix)
	w := &fanoutWait{label: label, ch: make(chan string, 1)}
	// Registered before the row is written, so an operator who answers the
	// instant it appears finds a waiter rather than an orphan.
	s.fanoutWaits.Store(callID, w)

	question := domain.Question{
		// The preamble is a separate bubble the operator may scroll past, and
		// formatAskQuestionResult archives only this string -- so the stage is
		// named in both or the stored answer loses what it belonged to.
		Question: "[" + label + "] " + b.Question,
		Options:  b.Options,
	}
	if err := s.appendSyntheticAskQuestionWithID(
		ctx, planID, callID, blockerAskPreamble(label), []domain.Question{question},
	); err != nil {
		slog.Error("plan fanout: post blocker question",
			"dialog_id", planID, "stage", b.Stage, "error", err)
		return blockerFallbackResult(b)
	}

	posted = true
	s.publishPlanQuestion(planID, b.Stage, string(FanoutStageAwaitingInput))

	timer := time.NewTimer(maxBlockerAnswerWait)
	defer timer.Stop()

	select {
	case answer := <-w.ch:
		s.recordBlockerAnswer(planID, b, answer)
		return blockerAnsweredResult(b, answer)
	case <-ctx.Done():
		return blockerFallbackResult(b)
	case <-timer.C:
		slog.Warn("plan fanout: blocker question timed out",
			"dialog_id", planID, "stage", b.Stage, "waited", maxBlockerAnswerWait)
		return blockerFallbackResult(b)
	}
}

// deliverFanoutAnswer hands an operator's answer to the stage subagent blocked
// on it. It reports handled for every fan-out question, answered or not, so a
// stale one cannot fall through and set the orchestrator narrating it back.
func (s *ChatService) deliverFanoutAnswer(toolCallID string, answers []string) (domain.ChatResponse, bool) {
	if !strings.HasPrefix(toolCallID, fanoutAskIDPrefix) || len(answers) == 0 {
		return domain.ChatResponse{}, false
	}

	v, ok := s.fanoutWaits.LoadAndDelete(toolCallID)
	if !ok {
		// The run ended, the wait timed out, or the process restarted under it.
		return domain.ChatResponse{Response: "Nothing is waiting on that question any more — " +
			"the planning run it came from has stopped. Its stage says in the plan what " +
			"became of it."}, true
	}

	w := v.(*fanoutWait)
	select {
	case w.ch <- answers[0]:
	default:
	}
	return domain.ChatResponse{Response: fmt.Sprintf("Answer passed to the %s planner.", w.label)}, true
}

// watchFanoutBudget cancels a run once its agents have spent the configured
// budget working. Time a stage spends waiting on the operator is not work: the
// run's deadline is a bound on what nib does, not on how long a person takes to
// answer.
//
// budget and tick are arguments rather than constants so a test can run it in
// milliseconds.
func (s *ChatService) watchFanoutBudget(ctx context.Context, st *fanoutRunState, budget, tick time.Duration) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	var worked time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if st.waiting.Load() > 0 {
				continue
			}
			worked += tick
			if worked >= budget {
				slog.Warn("plan fanout: work budget exhausted", "budget", budget)
				st.cancel()
				return
			}
		}
	}
}

func (s *ChatService) setBlockerStageStatus(planID uuid.UUID, b PlanBlocker, status FanoutStageStatus, reason string) {
	if err := s.setFanoutStageOfKind(planID, b.Stage, b.Kind, func(sg *FanoutStage) {
		sg.Status = status
		sg.Error = reason
	}); err != nil {
		slog.Warn("plan fanout: set stage status",
			"dialog_id", planID, "stage", b.Stage, "status", status, "error", err)
	}
}

// recordBlockerAnswer stores the answer against the blocker this wait was
// started for, matched against the record itself. The question put to the
// operator carries the stage's name in front of it, so matching on what came
// back would find nothing.
func (s *ChatService) recordBlockerAnswer(planID uuid.UUID, b PlanBlocker, answer string) {
	if _, err := s.updateFanoutRun(planID, func(run *FanoutRun) {
		for i := range run.Blockers {
			if run.Blockers[i].Kind == b.Kind &&
				run.Blockers[i].Stage == b.Stage &&
				run.Blockers[i].Question == b.Question {
				run.Blockers[i].Answer = answer
				return
			}
		}
	}); err != nil {
		slog.Warn("plan fanout: record blocker answer",
			"dialog_id", planID, "stage", b.Stage, "error", err)
	}
}

func (s *ChatService) publishPlanQuestion(planID uuid.UUID, stage, status string) {
	s.activity.Publish(planID, domain.AgentActivity{
		Kind:   domain.ActivityPlanQuestion,
		Stage:  stage,
		Status: status,
	})
}

// blockerStoreTool is the call the agent has to end its turn with, which differs
// by which part of the plan it owns.
func blockerStoreTool(kind FanoutStageKind) (what, store string) {
	if kind == FanoutStageKindRollback {
		return "the rollback", UpdateRollbackPlanToolName
	}
	return "the stage", UpdateActionPlanToolName
}

func blockerAnsweredResult(b PlanBlocker, answer string) string {
	what, store := blockerStoreTool(b.Kind)
	return fmt.Sprintf(
		"The user answered: %s\n\nThis supersedes the assumption you stated (%q). "+
			"Plan the rest of %s on the answer and store it with %s before your turn ends.",
		answer, b.Assumption, what, store)
}

func blockerFallbackResult(b PlanBlocker) string {
	what, store := blockerStoreTool(b.Kind)
	return fmt.Sprintf(
		"No answer came back — the question timed out or the planning run was stopped. "+
			"Proceed under your stated assumption (%q), say so where a reader will see it, "+
			"and store %s with %s before your turn ends.",
		b.Assumption, what, store)
}
