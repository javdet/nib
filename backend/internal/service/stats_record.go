package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/metrics"
	"github.com/javdet/nib/internal/repository"
)

// SetStatsWriter wires usage recording after construction.
//
// A setter rather than a 23rd constructor parameter: NewChatService already
// takes 22 positional arguments, which is the hazard the handler.Deps comment
// describes, and statistics are optional -- every service test constructs a
// ChatService without one and must keep working.
// model is the configured one, used only when a response omits its own.
func (s *ChatService) SetStatsWriter(w repository.StatsWriter, model string) {
	s.statsWriter = w
	s.configuredModel = model
}

// recordLLMUsage persists what one completion cost.
//
// Statistics are a side effect of a turn and never a condition of it, so the
// write is fired on its own goroutine and a failure is logged and dropped. The
// context is detached with WithoutCancel deliberately: a cancelled or
// force-stopped turn still spent the tokens, and that is exactly the turn whose
// cost an operator wants to see. Bounded in practice -- one execution at a time,
// at most maxIterations rounds per turn.
func (s *ChatService) recordLLMUsage(
	ctx context.Context,
	dialogID, planID *uuid.UUID,
	modeName string,
	usage llm.Usage,
	elapsed time.Duration,
	callErr error,
) {
	if s.statsWriter == nil {
		return
	}

	model := usage.Model
	if model == "" {
		// Some gateways omit the model on the response; the configured one is
		// the only other thing that could have served the call.
		model = s.configuredModel
	}

	rec := domain.LLMUsageRecord{
		// Copied, not aliased: the insert happens on another goroutine after
		// this returns, so the record must not point at a caller's local that
		// a later round could change underneath it.
		DialogID:           copyUUID(dialogID),
		PlanID:             copyUUID(planID),
		Mode:               modeName,
		Operation:          metrics.LLMOpCompleteWithTools,
		Model:              model,
		PromptTokens:       usage.PromptTokens,
		CachedPromptTokens: usage.CachedPromptTokens,
		CompletionTokens:   usage.CompletionTokens,
		ReasoningTokens:    usage.ReasoningTokens,
		TotalTokens:        usage.TotalTokens,
		CostUSD:            usage.CostUSD,
		DurationMS:         int(elapsed.Milliseconds()),
		Outcome:            llm.OutcomeFor(callErr),
	}

	writer := s.statsWriter
	go func() {
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statsWriteTimeout)
		defer cancel()
		if err := writer.InsertLLMUsage(wctx, rec); err != nil {
			slog.Warn("stats: record llm usage", "dialog_id", dialogID, "error", err)
		}
	}()
}

// recordPlanStatusTransition appends to the status ledger. Same fire-and-forget
// contract: a plan status change must not fail because statistics are down.
func (s *ChatService) recordPlanStatusTransition(planID uuid.UUID, from, to string) {
	if s.statsWriter == nil || from == to {
		return
	}
	writer := s.statsWriter
	id := planID.String()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), statsWriteTimeout)
		defer cancel()
		if err := writer.InsertPlanStatusTransition(ctx, id, from, to); err != nil {
			slog.Warn("stats: record plan status transition", "plan_id", id, "error", err)
		}
	}()
}

// statsWriteTimeout bounds a detached statistics write. Well under any request
// deadline, so a wedged database cannot pile goroutines up.
const statsWriteTimeout = 5 * time.Second

// recordAgentRun persists one finished container agent's usage.
//
// Attribution is resolved here rather than in the handler because the root
// dialog walk needs the repository, and the webhook only knows its chat id.
func (s *ChatService) recordAgentRun(ctx context.Context, dialogID uuid.UUID, res AgentRunResult) {
	if s.statsWriter == nil {
		return
	}

	planID := dialogID
	if root, err := s.resolveRootDialogID(ctx, dialogID); err == nil {
		planID = root
	}

	rec := domain.AgentRunUsageRecord{
		DialogID:     &dialogID,
		PlanID:       &planID,
		JobName:      res.JobName,
		SessionID:    res.SessionID,
		Status:       metrics.NormalizeAgentRunnerStatus(strings.TrimSpace(res.Status)),
		InputTokens:  res.InputTokens,
		OutputTokens: res.OutputTokens,
		NumTurns:     res.NumTurns,
		// A reported zero is treated as "not priced": the entrypoint fills the
		// field with jq '... // 0', so zero and absent are indistinguishable on
		// the wire, and a run that genuinely cost nothing is not a thing.
		CostUSD:    positiveCost(res.TotalCostUSD),
		DurationMS: res.DurationMS,
	}

	writer := s.statsWriter
	go func() {
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statsWriteTimeout)
		defer cancel()
		if err := writer.InsertAgentRunUsage(wctx, rec); err != nil {
			slog.Warn("stats: record agent run usage", "dialog_id", dialogID, "error", err)
		}
	}()
}

func positiveCost(cost *float64) *float64 {
	if cost == nil || *cost <= 0 {
		return nil
	}
	return cost
}

func copyUUID(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	v := *id
	return &v
}
