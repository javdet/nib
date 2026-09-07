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
	"github.com/javdet/nib/internal/metrics"
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
	// notPlannedYetReason explains a carried row nothing has written: a stage the
	// DAG gained since the last run, or one an earlier run never reached.
	notPlannedYetReason = "not planned yet; replan it to fill this in"
	// abandonedReason closes a carried row a finished run left mid-flight.
	abandonedReason = "the run that was planning this ended before it finished"
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

// fanoutStages lays the plan's work out in order: the stages wave by wave, then
// the rollback on its own, since it undoes the plan as a whole and can only be
// worked out once every stage is written.
//
// allWaves is the whole DAG rather than the slice of it this run replans, and
// running holds the titles the run does plan. A unit of work outside it is
// carried from the run before, so the list an operator reads is always the plan
// and never the round: answering one question must not make the stages nobody
// is replanning disappear.
func fanoutStages(allWaves [][]string, running map[string]struct{}, rollback bool, previous FanoutRun) []FanoutStage {
	var stages []FanoutStage
	for waveIdx, wave := range allWaves {
		for _, title := range wave {
			if _, plans := running[normalizeStageTitle(title)]; !plans {
				stages = append(stages, carriedStage(previous, waveIdx, title, FanoutStageKindStage))
				continue
			}
			stages = append(stages, FanoutStage{
				Title:  title,
				Wave:   waveIdx,
				Status: FanoutStagePending,
			})
		}
	}

	if !rollback {
		return append(stages, carriedStage(previous, len(allWaves), rollbackStageTitle, FanoutStageKindRollback))
	}
	return append(stages, FanoutStage{
		Title:  rollbackStageTitle,
		Wave:   len(allWaves),
		Kind:   FanoutStageKindRollback,
		Status: FanoutStagePending,
	})
}

// carriedStage is the row for a unit of work this run leaves alone: whatever the
// run before it recorded, marked so nothing mistakes it for work in hand. The
// earlier subagent's dialog comes with it, which is what keeps the "Open" button
// on a stage planned two rounds ago working.
func carriedStage(previous FanoutRun, wave int, title string, kind FanoutStageKind) FanoutStage {
	st := FanoutStage{Title: title, Wave: wave, Kind: kind, Carried: true}

	prior, found := findFanoutStage(previous, title, kind)
	if !found {
		// A stage the DAG gained since the last run, or a first run that leaves
		// the rollback out: it is in the list because it is part of the plan, not
		// because anything has written it.
		st.Status = FanoutStagePending
		st.Error = notPlannedYetReason
		return st
	}

	st.Status = prior.Status
	st.DialogID = prior.DialogID
	st.Error = prior.Error
	switch prior.Status {
	case FanoutStageRunning:
		// A run that is over left nothing behind that is still working, and
		// carrying "running" would show a spinner with no agent under it -- the
		// reasoning the boot-time sweep in ReconcileStuckRuns follows too.
		st.Status = FanoutStageFailed
		st.Error = abandonedReason
	case FanoutStagePending:
		st.Error = notPlannedYetReason
	}
	return st
}

// findFanoutStage looks a row up the way the run's writers address it: a DAG
// stage by title among the rows carrying no kind, the rollback by kind alone.
func findFanoutStage(run FanoutRun, title string, kind FanoutStageKind) (FanoutStage, bool) {
	for _, st := range run.Stages {
		if kind != FanoutStageKindStage {
			if st.Kind == kind {
				return st, true
			}
			continue
		}
		if st.Kind == FanoutStageKindStage && normalizeStageTitle(st.Title) == normalizeStageTitle(title) {
			return st, true
		}
	}
	return FanoutStage{}, false
}

// stageTitleSet is the normalized titles a run plans, which is what separates a
// row being replanned from one only being carried.
func stageTitleSet(waves [][]string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, wave := range waves {
		for _, title := range wave {
			out[normalizeStageTitle(title)] = struct{}{}
		}
	}
	return out
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

// StartPlanFanout plans the root dialog's DAG with one subagent per stage,
// then hands the finished stages to one more agent that works out the rollback,
// and returns as soon as the run is recorded. The work continues in the
// background and reports progress over the dialog's SSE stream.
//
// targets says what the run covers; AllFanoutTargets() is the whole plan. A
// target set holding no work is refused with ErrNoDAGStages.
func (s *ChatService) StartPlanFanout(ctx context.Context, rootID uuid.UUID, targets FanoutTargets) (FanoutRun, error) {
	if s.dialogRepo == nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: dialog repository is not configured")
	}
	if _, err := s.dialogRepo.GetDialog(ctx, rootID); err != nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: %w", err)
	}

	dag, found, err := s.ReadDAG(rootID)
	if err != nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: read dag: %w", err)
	}
	titles := dagStageTitles(dag)
	if !found || len(titles) == 0 {
		metrics.RecordFanoutRejected("no_dag_stages")
		return FanoutRun{}, ErrNoDAGStages
	}

	// The run before this one is both the guard against a second fan-out and the
	// source of every row and blocker this one carries, so it is read once.
	previous, _, err := s.ReadFanoutRun(rootID)
	if err != nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: read run: %w", err)
	}
	if previous.Active() {
		metrics.RecordFanoutRejected("in_progress")
		return FanoutRun{}, ErrFanoutInProgress
	}

	contract, _, err := s.ReadPlanContract(rootID)
	if err != nil {
		// A contract that cannot be parsed costs coordination, not the run.
		slog.Warn("plan fanout: read contract", "dialog_id", rootID, "error", err)
	}

	allWaves, err := planWaves(titles, contract)
	if err != nil {
		slog.Warn("plan fanout: wave order", "dialog_id", rootID, "error", err)
	}
	waves, rollback, err := targets.work(allWaves)
	if err != nil {
		return FanoutRun{}, err
	}
	running := stageTitleSet(waves)

	run := FanoutRun{
		RunID:     uuid.NewString(),
		Status:    FanoutRunRunning,
		StartedAt: time.Now().Unix(),
	}
	run.Blockers = carryBlockers(previous.Blockers, running, rollback)
	// The whole DAG, not just this run's share of it: what a round replans is the
	// waves below, what the operator watches is every stage plus the rollback.
	run.Stages = fanoutStages(allWaves, running, rollback, previous)
	if err := s.writeFanoutRun(rootID, run); err != nil {
		return FanoutRun{}, fmt.Errorf("start plan fanout: %w", err)
	}
	metrics.RecordFanoutStarted()

	// The run outlives the request that started it, so it gets a deadline of its
	// own rather than inheriting one that is about to be cancelled.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.fanout.timeout())
	s.fanoutCancels.Store(rootID, cancel)
	go func() {
		defer cancel()
		defer s.fanoutCancels.Delete(rootID)
		s.runPlanFanout(runCtx, rootID, waves, rollback)
	}()

	slog.Info("plan fanout started",
		"dialog_id", rootID, "run_id", run.RunID,
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

// carryBlockers is the blockers a new run starts with.
//
// An answered one stays, so the stage being replanned can be told what the user
// said. An unanswered one stays too unless the run replans whoever raised it:
// that agent raises again whatever it still cannot answer, while an agent nobody
// is rerunning gets no second chance to, and dropping its blocker would lose the
// question for good -- which is what used to happen to every question past the
// two a round asks.
func carryBlockers(blockers []PlanBlocker, running map[string]struct{}, rollback bool) []PlanBlocker {
	var out []PlanBlocker
	for _, b := range blockers {
		if strings.TrimSpace(b.Answer) != "" {
			out = append(out, b)
			continue
		}

		reraised := rollback
		if b.Kind == FanoutStageKindStage {
			_, reraised = running[normalizeStageTitle(b.Stage)]
		}
		if !reraised {
			out = append(out, b)
		}
	}
	return out
}

// runPlanFanout plans one wave at a time. Stages inside a wave run together; a
// stage that fails is recorded and its siblings carry on, since a half-written
// plan is worth more than none.
func (s *ChatService) runPlanFanout(ctx context.Context, rootID uuid.UUID, waves [][]string, rollback bool) {
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
				s.runPlanStage(ctx, rootID, title, waveIdx)
			}(title)
		}
		wg.Wait()
	}

	// The rollback is derived from the stages, so it is written last and alone,
	// once every wave has landed.
	if rollback {
		s.runRollbackAgent(ctx, rootID)
	}

	// A cancelled or timed-out context cannot append the closing question, so the
	// run is closed as failed and whatever landed stays on the plan.
	if ctx.Err() != nil {
		s.closeFanoutRun(rootID, FanoutRunFailed, "run cancelled or timed out")
		return
	}
	s.finishPlanFanout(ctx, rootID)
}

// CancelPlanFanout stops a running fan-out. Stages already written stay on the
// plan; stages still running are abandoned. It reports whether a run was stopped.
func (s *ChatService) CancelPlanFanout(rootID uuid.UUID) bool {
	cancel, ok := s.fanoutCancels.LoadAndDelete(rootID)
	if !ok {
		return false
	}
	cancel.(context.CancelFunc)()
	return true
}

// runPlanStage plans a single stage in a dialog of its own. Every failure is
// recorded against the run rather than returned: the fan-out reports what landed.
func (s *ChatService) runPlanStage(ctx context.Context, rootID uuid.UUID, title string, wave int) {
	s.activity.Publish(rootID, domain.AgentActivity{
		Kind:  domain.ActivityPlanStageStarted,
		Stage: title,
	})

	fail := func(err error) {
		slog.Error("plan fanout: stage failed", "dialog_id", rootID, "stage", title, "error", err)
		_ = s.setFanoutStage(rootID, title, func(st *FanoutStage) {
			st.Status = FanoutStageFailed
			st.Error = err.Error()
		})
		s.activity.Publish(rootID, domain.AgentActivity{
			Kind:   domain.ActivityPlanStageFailed,
			Stage:  title,
			Status: string(FanoutStageFailed),
		})
	}

	stageDialog, err := s.dialogRepo.CreateDialog(ctx, stagePlanMode, title, &rootID)
	if err != nil {
		fail(fmt.Errorf("create stage dialog: %w", err))
		return
	}
	if err := s.setFanoutStage(rootID, title, func(st *FanoutStage) {
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
	seed, err := s.buildStageSeed(rootID, title)
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

	allow, err := s.stageAllowSet(ctx, rootID)
	if err != nil {
		fail(fmt.Errorf("stage allow set: %w", err))
		return
	}
	binding := toolBinding{
		dialogID: stageDialog.ID,
		planID:   rootID,
		stage:    title,
		mode:     stagePlanMode,
	}
	catalog, err := s.buildToolCatalog(ctx, allow, binding)
	if err != nil {
		fail(fmt.Errorf("build stage catalog: %w", err))
		return
	}

	if _, err := s.runPersistingAgentLoop(ctx, stageDialog.ID, stagePlanMode, catalog, loopConfig{
		planID:        rootID,
		stage:         title,
		maxIterations: s.fanout.StageMaxIterations,
	}); err != nil {
		fail(err)
		return
	}

	_ = s.setFanoutStage(rootID, title, func(st *FanoutStage) {
		st.Status = FanoutStageDone
	})
	s.activity.Publish(rootID, domain.AgentActivity{
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
func (s *ChatService) stageAllowSet(ctx context.Context, rootID uuid.UUID) (map[string]struct{}, error) {
	allow, err := s.planFanoutAllowSet(ctx, rootID)
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
func (s *ChatService) planFanoutAllowSet(ctx context.Context, rootID uuid.UUID) (map[string]struct{}, error) {
	allow, err := s.resolveAllowSet(stagePlanMode)
	if err != nil {
		return nil, err
	}

	if s.toolCategorySvc != nil {
		d, err := s.dialogRepo.GetDialog(ctx, rootID)
		if err != nil {
			return nil, err
		}
		s.addCategoryTools(ctx, allow, d.Categories, "stage allow set")
	}

	stripSubagentTools(allow)
	delete(allow, CreateActionPlanToolName)
	delete(allow, AskQuestionToolName)
	allow[ReportBlockerToolName] = struct{}{}
	return allow, nil
}
