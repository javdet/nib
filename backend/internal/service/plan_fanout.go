package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

const (
	defaultPlanFanoutConcurrency = 4
	defaultStageMaxIterations    = 15
	defaultPlanFanoutTimeout     = 45 * time.Minute
	// maxBlockerQuestionsPerRound follows the two-questions-at-a-time rule the
	// plan and decompose prompts already state. Leftover blockers keep their
	// place in the run and come back in the next round.
	maxBlockerQuestionsPerRound = 2
	// stagePlanMode is the mode a stage subagent runs under: it uses the plan
	// prompt and the plan allow list, narrowed to a single stage.
	stagePlanMode = "plan"
	// stagePromptName is the prompt appended to plan.md for a stage subagent.
	stagePromptName = "plan_stage"
)

var (
	ErrFanoutInProgress = errors.New("a plan fan-out is already running for this dialog")
	ErrNoDAGStages      = errors.New("this dialog has no DAG stages to plan")
)

// PlanFanoutConfig tunes the stage fan-out. The zero value is the shipped default.
type PlanFanoutConfig struct {
	Concurrency        int
	StageMaxIterations int
	TimeoutMinutes     int
}

func (c PlanFanoutConfig) withDefaults() PlanFanoutConfig {
	if c.Concurrency <= 0 {
		c.Concurrency = defaultPlanFanoutConcurrency
	}
	if c.StageMaxIterations <= 0 {
		c.StageMaxIterations = defaultStageMaxIterations
	}
	return c
}

func (c PlanFanoutConfig) timeout() time.Duration {
	if c.TimeoutMinutes <= 0 {
		return defaultPlanFanoutTimeout
	}
	return time.Duration(c.TimeoutMinutes) * time.Minute
}

// StartPlanFanout plans every stage of a decompose dialog's DAG with one subagent
// per stage and returns as soon as the run is recorded. The work continues in the
// background and reports progress over the dialog's SSE stream.
//
// only, when non-empty, restricts the run to those stages. That is how the stages
// whose blockers the user just answered are replanned without disturbing the rest.
func (s *ChatService) StartPlanFanout(ctx context.Context, decomposeID uuid.UUID, only []string) (FanoutRun, error) {
	if s.dialogRepo == nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: dialog repository is not configured")
	}
	if _, err := s.dialogRepo.GetDialog(ctx, decomposeID); err != nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: %w", err)
	}

	dag, found, err := s.ReadDAG(decomposeID)
	if err != nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: read dag: %w", err)
	}
	titles := dagStageTitles(dag)
	if !found || len(titles) == 0 {
		return FanoutRun{}, ErrNoDAGStages
	}

	if existing, found, err := s.ReadFanoutRun(decomposeID); err != nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: read run: %w", err)
	} else if found && existing.Active() {
		return FanoutRun{}, ErrFanoutInProgress
	}

	contract, _, err := s.ReadPlanContract(decomposeID)
	if err != nil {
		// A contract that cannot be parsed costs coordination, not the run.
		slog.Warn("plan fanout: read contract", "dialog_id", decomposeID, "error", err)
	}

	waves, err := planWaves(titles, contract)
	if err != nil {
		slog.Warn("plan fanout: wave order", "dialog_id", decomposeID, "error", err)
	}
	waves = filterWaves(waves, only)
	if len(waves) == 0 {
		return FanoutRun{}, ErrNoDAGStages
	}

	run := FanoutRun{
		RunID:     uuid.NewString(),
		Status:    FanoutRunRunning,
		StartedAt: time.Now().Unix(),
	}
	// Blockers already answered stay on the record so a stage being replanned can
	// be told what the user said.
	if previous, found, _ := s.ReadFanoutRun(decomposeID); found {
		run.Blockers = answeredBlockers(previous.Blockers)
	}
	for waveIdx, wave := range waves {
		for _, title := range wave {
			run.Stages = append(run.Stages, FanoutStage{
				Title:  title,
				Wave:   waveIdx,
				Status: FanoutStagePending,
			})
		}
	}
	if err := s.writeFanoutRun(decomposeID, run); err != nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: %w", err)
	}

	// The run outlives the request that started it, so it gets a deadline of its
	// own rather than inheriting one that is about to be cancelled.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.fanout.timeout())
	s.fanoutCancels.Store(decomposeID, cancel)
	go func() {
		defer cancel()
		defer s.fanoutCancels.Delete(decomposeID)
		s.runPlanFanout(runCtx, decomposeID, waves)
	}()

	slog.Info("plan fanout started",
		"dialog_id", decomposeID, "run_id", run.RunID, "waves", len(waves), "stages", len(run.Stages))
	return run, nil
}

// filterWaves keeps only the named stages, dropping waves left empty. An empty
// filter keeps everything.
func filterWaves(waves [][]string, only []string) [][]string {
	if len(only) == 0 {
		return waves
	}
	keep := make(map[string]struct{}, len(only))
	for _, name := range only {
		keep[normalizeStageTitle(name)] = struct{}{}
	}

	var out [][]string
	for _, wave := range waves {
		var kept []string
		for _, title := range wave {
			if _, ok := keep[normalizeStageTitle(title)]; ok {
				kept = append(kept, title)
			}
		}
		if len(kept) > 0 {
			out = append(out, kept)
		}
	}
	return out
}

func answeredBlockers(blockers []PlanBlocker) []PlanBlocker {
	var out []PlanBlocker
	for _, b := range blockers {
		if strings.TrimSpace(b.Answer) != "" {
			out = append(out, b)
		}
	}
	return out
}

// runPlanFanout plans one wave at a time. Stages inside a wave run together; a
// stage that fails is recorded and its siblings carry on, since a half-written
// plan is worth more than none.
func (s *ChatService) runPlanFanout(ctx context.Context, decomposeID uuid.UUID, waves [][]string) {
	for waveIdx, wave := range waves {
		if ctx.Err() != nil {
			break
		}

		sem := make(chan struct{}, s.fanout.Concurrency)
		var wg sync.WaitGroup
		for _, title := range wave {
			wg.Add(1)
			go func(title string) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()
				s.runPlanStage(ctx, decomposeID, title, waveIdx)
			}(title)
		}
		wg.Wait()
	}

	// A cancelled or timed-out context cannot append the closing question, so the
	// run is closed as failed and whatever landed stays on the plan.
	if ctx.Err() != nil {
		s.closeFanoutRun(decomposeID, FanoutRunFailed, "run cancelled or timed out")
		return
	}
	s.finishPlanFanout(ctx, decomposeID)
}

// CancelPlanFanout stops a running fan-out. Stages already written stay on the
// plan; stages still running are abandoned. It reports whether a run was stopped.
func (s *ChatService) CancelPlanFanout(decomposeID uuid.UUID) bool {
	cancel, ok := s.fanoutCancels.LoadAndDelete(decomposeID)
	if !ok {
		return false
	}
	cancel.(context.CancelFunc)()
	return true
}

// runPlanStage plans a single stage in a dialog of its own. Every failure is
// recorded against the run rather than returned: the fan-out reports what landed.
func (s *ChatService) runPlanStage(ctx context.Context, decomposeID uuid.UUID, title string, wave int) {
	s.activity.Publish(decomposeID, domain.AgentActivity{
		Kind:  domain.ActivityPlanStageStarted,
		Stage: title,
	})

	fail := func(err error) {
		slog.Error("plan fanout: stage failed", "dialog_id", decomposeID, "stage", title, "error", err)
		_ = s.setFanoutStage(decomposeID, title, func(st *FanoutStage) {
			st.Status = FanoutStageFailed
			st.Error = err.Error()
		})
		s.activity.Publish(decomposeID, domain.AgentActivity{
			Kind:   domain.ActivityPlanStageFailed,
			Stage:  title,
			Status: string(FanoutStageFailed),
		})
	}

	stageDialog, err := s.dialogRepo.CreateDialog(ctx, stagePlanMode, title, &decomposeID)
	if err != nil {
		fail(fmt.Errorf("create stage dialog: %w", err))
		return
	}
	if err := s.setFanoutStage(decomposeID, title, func(st *FanoutStage) {
		st.Status = FanoutStageRunning
		st.DialogID = stageDialog.ID.String()
	}); err != nil {
		fail(err)
		return
	}

	sysPrompt, err := s.stageSystemPrompt(ctx)
	if err != nil {
		fail(err)
		return
	}
	seed, err := s.buildStageSeed(ctx, decomposeID, title)
	if err != nil {
		fail(err)
		return
	}

	if _, err := s.dialogRepo.AppendMessage(ctx, stageDialog.ID, domain.DialogMessage{
		Role:    "system",
		Content: sysPrompt,
	}); err != nil {
		fail(fmt.Errorf("append system: %w", err))
		return
	}
	if _, err := s.dialogRepo.AppendMessage(ctx, stageDialog.ID, domain.DialogMessage{
		Role:    "user",
		Content: seed,
	}); err != nil {
		fail(fmt.Errorf("append seed: %w", err))
		return
	}

	allow, err := s.stageAllowSet(ctx, decomposeID)
	if err != nil {
		fail(fmt.Errorf("stage allow set: %w", err))
		return
	}
	binding := toolBinding{
		dialogID: stageDialog.ID,
		planID:   decomposeID,
		stage:    title,
	}
	catalog, err := s.buildToolCatalog(ctx, allow, binding)
	if err != nil {
		fail(fmt.Errorf("build stage catalog: %w", err))
		return
	}

	if _, err := s.runPersistingAgentLoop(ctx, stageDialog.ID, stagePlanMode, catalog, loopConfig{
		planID:        decomposeID,
		stage:         title,
		maxIterations: s.fanout.StageMaxIterations,
	}); err != nil {
		fail(err)
		return
	}

	_ = s.setFanoutStage(decomposeID, title, func(st *FanoutStage) {
		st.Status = FanoutStageDone
	})
	s.activity.Publish(decomposeID, domain.AgentActivity{
		Kind:   domain.ActivityPlanStageDone,
		Stage:  title,
		Status: string(FanoutStageDone),
	})
}

// stageSystemPrompt is the plan prompt plus the stage subagent contract that
// narrows it to one stage and swaps ask_question for report_blocker.
func (s *ChatService) stageSystemPrompt(ctx context.Context) (string, error) {
	base, err := s.resolveSystemPrompt(ctx, stagePlanMode)
	if err != nil {
		return "", err
	}
	if s.systemPromptsSvc == nil {
		return base, nil
	}
	stage, err := s.systemPromptsSvc.Get(stagePromptName)
	if err != nil {
		return "", fmt.Errorf("resolve %s prompt: %w", stagePromptName, err)
	}
	rendered, err := RenderTemplateVariables(ctx, stage.Content, s.variableRepo, s.selection)
	if err != nil {
		return "", err
	}
	return base + "\n\n" + rendered, nil
}

// stageAllowSet is the plan allow set narrowed for a subagent: it cannot rewrite
// the whole plan and it cannot suspend to ask the user, so create_action_plan and
// ask_question come out and report_blocker goes in.
func (s *ChatService) stageAllowSet(ctx context.Context, decomposeID uuid.UUID) (map[string]struct{}, error) {
	allow, err := s.resolveAllowSet(stagePlanMode)
	if err != nil {
		return nil, err
	}

	if s.toolCategorySvc != nil {
		d, err := s.dialogRepo.GetDialog(ctx, decomposeID)
		if err != nil {
			return nil, err
		}
		for _, name := range d.Categories {
			tools, err := s.toolCategorySvc.ListToolsByCategory(ctx, name)
			if err != nil {
				slog.Warn("stage allow set: list tools by category", "category", name, "error", err)
				continue
			}
			for _, t := range tools {
				allow[t.Name] = struct{}{}
			}
		}
	}

	delete(allow, CreateActionPlanToolName)
	delete(allow, AskQuestionToolName)
	allow[ReportBlockerToolName] = struct{}{}
	allow[UpdateActionPlanToolName] = struct{}{}
	return allow, nil
}
