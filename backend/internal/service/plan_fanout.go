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
	// defaultPlanFanoutTimeout bounds the whole run: every wave of stages, and
	// then the rollback agent that runs alone after them.
	defaultPlanFanoutTimeout = 60 * time.Minute
	// maxBlockerQuestionsPerRound follows the two-questions-at-a-time rule the
	// plan and decompose prompts already state. Leftover blockers keep their
	// place in the run and come back in the next round.
	maxBlockerQuestionsPerRound = 2
	// stagePlanMode is the mode a stage subagent runs under: it uses the plan
	// prompt and the plan allow list, narrowed to a single stage.
	stagePlanMode = "plan"
	// stagePromptName is the prompt appended to plan.md for a stage subagent.
	stagePromptName = "plan_stage"
	// rollbackPromptName is the prompt appended to plan.md for the agent that
	// writes the plan's rollback list once the stages are written.
	rollbackPromptName = "rollback_stage"
)

var (
	ErrFanoutInProgress = errors.New("a plan fan-out is already running for this dialog")
	ErrNoDAGStages      = errors.New("this dialog has no DAG stages to plan")
)

// FanoutTargets says which parts of the plan a run covers. Every field is
// explicit, so "replan these two stages and the rollback" and "plan the rollback
// alone" are both sayable and neither is the accident of an empty slice.
type FanoutTargets struct {
	// Stages lists the DAG stages to plan. It is read only when AllStages is
	// unset, and an empty list then means no stage runs at all -- which is how
	// the rollback is redone by itself.
	Stages []string
	// AllStages plans every stage of the DAG, whatever Stages holds.
	AllStages bool
	// Rollback runs the rollback agent after the last wave. Set on every
	// ordinary run, and on a replan round too, because the rollback is a
	// function of the stages: a stage rewritten under a new assumption leaves an
	// undo that no longer matches the plan.
	Rollback bool
}

// AllFanoutTargets is the whole plan: every stage of the DAG, then the rollback.
func AllFanoutTargets() FanoutTargets {
	return FanoutTargets{AllStages: true, Rollback: true}
}

// work resolves the waves a run plans and whether it ends with the rollback. It
// reports ErrNoDAGStages for a target set that turns out to hold no work: an
// empty one, or one whose named stages are not in the DAG.
func (t FanoutTargets) work(waves [][]string) ([][]string, bool, error) {
	if !t.AllStages {
		waves = filterWaves(waves, t.Stages)
	}
	if len(waves) == 0 && !t.Rollback {
		return nil, false, ErrNoDAGStages
	}
	return waves, t.Rollback, nil
}

// fanoutStages lays the run's work out in order: the stages wave by wave, then
// the rollback on its own, since it undoes the plan as a whole and can only be
// worked out once every stage is written.
func fanoutStages(waves [][]string, rollback bool) []FanoutStage {
	var stages []FanoutStage
	for waveIdx, wave := range waves {
		for _, title := range wave {
			stages = append(stages, FanoutStage{
				Title:  title,
				Wave:   waveIdx,
				Status: FanoutStagePending,
			})
		}
	}
	if rollback {
		stages = append(stages, FanoutStage{
			Title:  rollbackStageTitle,
			Wave:   len(waves),
			Kind:   FanoutStageKindRollback,
			Status: FanoutStagePending,
		})
	}
	return stages
}

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

// StartPlanFanout plans a decompose dialog's DAG with one subagent per stage,
// then hands the finished stages to one more agent that works out the rollback,
// and returns as soon as the run is recorded. The work continues in the
// background and reports progress over the dialog's SSE stream.
//
// targets says what the run covers; AllFanoutTargets() is the whole plan. A
// target set holding no work is refused with ErrNoDAGStages.
func (s *ChatService) StartPlanFanout(ctx context.Context, decomposeID uuid.UUID, targets FanoutTargets) (FanoutRun, error) {
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
	waves, rollback, err := targets.work(waves)
	if err != nil {
		return FanoutRun{}, err
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
	run.Stages = fanoutStages(waves, rollback)
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
		s.runPlanFanout(runCtx, decomposeID, waves, rollback)
	}()

	slog.Info("plan fanout started",
		"dialog_id", decomposeID, "run_id", run.RunID,
		"waves", len(waves), "stages", len(run.Stages), "rollback", rollback)
	return run, nil
}

// filterWaves keeps only the named stages, dropping waves left empty. An empty
// filter keeps nothing: callers that mean every stage set AllStages and do not
// come here at all.
func filterWaves(waves [][]string, only []string) [][]string {
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
func (s *ChatService) runPlanFanout(ctx context.Context, decomposeID uuid.UUID, waves [][]string, rollback bool) {
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

	// The rollback is derived from the stages, so it is written last and alone,
	// once every wave has landed.
	if rollback {
		s.runRollbackAgent(ctx, decomposeID)
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

	sysPrompt, err := s.fanoutSystemPrompt(ctx, stagePromptName)
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

// fanoutSystemPrompt is the plan prompt plus the subagent overlay named by
// promptName, which narrows it to one part of the plan and swaps ask_question for
// report_blocker.
func (s *ChatService) fanoutSystemPrompt(ctx context.Context, promptName string) (string, error) {
	base, err := s.resolveSystemPrompt(ctx, stagePlanMode)
	if err != nil {
		return "", err
	}
	if s.systemPromptsSvc == nil {
		return base, nil
	}
	overlay, err := s.systemPromptsSvc.Get(promptName)
	if err != nil {
		return "", fmt.Errorf("resolve %s prompt: %w", promptName, err)
	}
	rendered, err := RenderTemplateVariables(ctx, overlay.Content, s.variableRepo, s.selection)
	if err != nil {
		return "", err
	}
	return base + "\n\n" + rendered, nil
}

// stageAllowSet is the plan allow set narrowed for a stage subagent: it cannot
// rewrite the whole plan, it owns no rollback, and it cannot suspend to ask the
// user, so create_action_plan, update_rollback_plan and ask_question come out and
// report_blocker goes in.
func (s *ChatService) stageAllowSet(ctx context.Context, decomposeID uuid.UUID) (map[string]struct{}, error) {
	allow, err := s.planFanoutAllowSet(ctx, decomposeID)
	if err != nil {
		return nil, err
	}

	delete(allow, UpdateRollbackPlanToolName)
	allow[UpdateActionPlanToolName] = struct{}{}
	return allow, nil
}

// planFanoutAllowSet is what every fan-out subagent starts from: the plan allow
// set plus the dialog's tool categories, minus the two tools no subagent may have.
// create_action_plan rewrites the whole plan and would destroy a sibling's work;
// ask_question suspends the turn and would strand the agents running beside it.
func (s *ChatService) planFanoutAllowSet(ctx context.Context, decomposeID uuid.UUID) (map[string]struct{}, error) {
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
	return allow, nil
}
