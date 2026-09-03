package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// A DAG stage may legitimately be called "Rollback plan". The rollback agent is
// addressed by kind and the stage by title, so neither can write the other's row.
func TestSetFanoutRollback_isNotReachableByTitle(t *testing.T) {
	t.Parallel()

	svc := &ChatService{planFanoutDir: t.TempDir()}
	planID := uuid.New()
	if err := svc.writeFanoutRun(planID, FanoutRun{
		RunID:  uuid.NewString(),
		Status: FanoutRunRunning,
		Stages: []FanoutStage{
			{Title: rollbackStageTitle, Status: FanoutStagePending},
			{Title: rollbackStageTitle, Kind: FanoutStageKindRollback, Status: FanoutStagePending},
		},
	}); err != nil {
		t.Fatalf("write run: %v", err)
	}

	if err := svc.setFanoutStage(planID, rollbackStageTitle, func(st *FanoutStage) {
		st.Status = FanoutStageDone
	}); err != nil {
		t.Fatalf("set stage: %v", err)
	}
	if err := svc.setFanoutRollback(planID, func(st *FanoutStage) {
		st.Status = FanoutStageFailed
	}); err != nil {
		t.Fatalf("set rollback: %v", err)
	}

	run, _, err := svc.ReadFanoutRun(planID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if run.Stages[0].Status != FanoutStageDone {
		t.Fatalf("stage status = %q, want done", run.Stages[0].Status)
	}
	if run.Stages[1].Status != FanoutStageFailed {
		t.Fatalf("rollback status = %q, want failed", run.Stages[1].Status)
	}
}

// Answers come back to the agent that asked, not to whoever shares its title.
func TestStageAnswersSection_ignoresTheRollbackAgentsBlockers(t *testing.T) {
	t.Parallel()

	svc := &ChatService{planFanoutDir: t.TempDir()}
	planID := uuid.New()
	if err := svc.writeFanoutRun(planID, FanoutRun{
		RunID: uuid.NewString(),
		Blockers: []PlanBlocker{
			{Stage: rollbackStageTitle, Question: "Do snapshots exist?", Assumption: "yes", Answer: "no"},
			{
				Stage: rollbackStageTitle, Kind: FanoutStageKindRollback,
				Question: "Undo the migration how?", Assumption: "restore", Answer: "replay",
			},
		},
	}); err != nil {
		t.Fatalf("write run: %v", err)
	}

	stage := svc.stageAnswersSection(planID, rollbackStageTitle)
	if !strings.Contains(stage, "Do snapshots exist?") || strings.Contains(stage, "Undo the migration how?") {
		t.Fatalf("stage answers = %q", stage)
	}

	rollback := svc.rollbackAnswersSection(planID)
	if !strings.Contains(rollback, "Undo the migration how?") || strings.Contains(rollback, "Do snapshots exist?") {
		t.Fatalf("rollback answers = %q", rollback)
	}
}

func rollbackFanoutFixture(t *testing.T) (*ChatService, uuid.UUID) {
	t.Helper()

	dataDir := t.TempDir()
	toolsDir := filepath.Join(dataDir, "tools")
	if err := os.MkdirAll(toolsDir, 0o755); err != nil {
		t.Fatalf("mkdir tools: %v", err)
	}
	allow := `{"allow_tools":["knowledge_search","ask_question","create_action_plan","update_action_plan","update_rollback_plan"]}`
	if err := os.WriteFile(filepath.Join(toolsDir, "plan.json"), []byte(allow), 0o644); err != nil {
		t.Fatalf("write plan.json: %v", err)
	}

	planID := uuid.New()
	svc := &ChatService{
		allowToolsDir:  toolsDir,
		dagsDir:        t.TempDir(),
		actionPlansDir: t.TempDir(),
		summariesDir:   t.TempDir(),
		planFanoutDir:  t.TempDir(),
		planStateDir:   t.TempDir(),
		activity:       NewActivityBroker(),
		dialogRepo: &actionListDialogRepo{dialogs: map[uuid.UUID]domain.Dialog{
			planID: {ID: planID, Mode: "decompose"},
		}},
	}
	return svc, planID
}

// The rollback agent writes the rollback and nothing else, and like every fan-out
// subagent it can neither rewrite the plan nor suspend to ask the user.
func TestRollbackAllowSet(t *testing.T) {
	t.Parallel()
	svc, planID := rollbackFanoutFixture(t)

	allow, err := svc.rollbackAllowSet(context.Background(), planID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	for _, name := range []string{UpdateRollbackPlanToolName, ReportBlockerToolName} {
		if _, ok := allow[name]; !ok {
			t.Fatalf("allow set = %v, want %s", allow, name)
		}
	}
	for _, name := range []string{CreateActionPlanToolName, UpdateActionPlanToolName, AskQuestionToolName} {
		if _, ok := allow[name]; ok {
			t.Fatalf("allow set = %v, did not want %s", allow, name)
		}
	}
}

// The stage subagents keep the tool they had and gain none of the rollback's.
func TestStageAllowSet_hasNoRollbackTool(t *testing.T) {
	t.Parallel()
	svc, planID := rollbackFanoutFixture(t)

	allow, err := svc.stageAllowSet(context.Background(), planID)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if _, ok := allow[UpdateActionPlanToolName]; !ok {
		t.Fatalf("allow set = %v, want %s", allow, UpdateActionPlanToolName)
	}
	if _, ok := allow[UpdateRollbackPlanToolName]; ok {
		t.Fatalf("allow set = %v, did not want %s", allow, UpdateRollbackPlanToolName)
	}
}

func TestBuildRollbackSeed_carriesThePlanAndNoStageOfItsOwn(t *testing.T) {
	t.Parallel()
	svc, planID := rollbackFanoutFixture(t)

	if err := os.WriteFile(filepath.Join(svc.summariesDir, planID.String()+".txt"),
		[]byte("Bump the api chart"), 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	stages := []storedActionStage{{
		Number: 1,
		Title:  "Configure Cassandra JMX",
		Steps:  []storedActionStep{{Number: "1.1", Type: "shell", Action: "enable jmx"}},
	}}

	seed, err := svc.buildRollbackSeed(planID, stages)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(seed, "## The plan as written") {
		t.Fatalf("seed = %q", seed)
	}
	if !strings.Contains(seed, "Configure Cassandra JMX") || !strings.Contains(seed, "Bump the api chart") {
		t.Fatalf("seed = %q", seed)
	}
	if strings.Contains(seed, "## Your stage") {
		t.Fatalf("seed = %q, the rollback agent owns no stage", seed)
	}
}

// Every stage failing leaves nothing to undo, and the plan document is created on
// demand, so the agent must not run at all.
func TestRunRollbackAgent_skipsAPlanWithNoStages(t *testing.T) {
	t.Parallel()
	svc, planID := rollbackFanoutFixture(t)

	if err := svc.writeFanoutRun(planID, FanoutRun{
		RunID:  uuid.NewString(),
		Status: FanoutRunRunning,
		Stages: []FanoutStage{{Title: rollbackStageTitle, Kind: FanoutStageKindRollback, Status: FanoutStagePending}},
	}); err != nil {
		t.Fatalf("write run: %v", err)
	}

	svc.runRollbackAgent(context.Background(), planID)

	run, _, err := svc.ReadFanoutRun(planID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if run.Stages[0].Status != FanoutStageDone || !strings.Contains(run.Stages[0].Error, "nothing to roll back") {
		t.Fatalf("rollback row = %#v", run.Stages[0])
	}
	if run.Stages[0].DialogID != "" {
		t.Fatal("expected no subagent dialog to be created")
	}
}

// An operator part-way through executing the rollback is relying on the list in
// front of them.
func TestRunRollbackAgent_skipsAPlanBeingRolledBack(t *testing.T) {
	t.Parallel()
	svc, planID := rollbackFanoutFixture(t)

	plan, err := json.Marshal(map[string]any{
		"stages":   []any{map[string]any{"title": "Configure Cassandra JMX", "steps": []any{}}},
		"rollback": []any{},
	})
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	if _, err := svc.WriteActionPlan(planID, plan); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	if err := svc.WritePlanState(planID, PlanState{Status: ActionPlanStatusRolledBack}); err != nil {
		t.Fatalf("write plan state: %v", err)
	}
	if err := svc.writeFanoutRun(planID, FanoutRun{
		RunID:  uuid.NewString(),
		Status: FanoutRunRunning,
		Stages: []FanoutStage{{Title: rollbackStageTitle, Kind: FanoutStageKindRollback, Status: FanoutStagePending}},
	}); err != nil {
		t.Fatalf("write run: %v", err)
	}

	svc.runRollbackAgent(context.Background(), planID)

	run, _, err := svc.ReadFanoutRun(planID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	if !strings.Contains(run.Stages[0].Error, "being rolled back") {
		t.Fatalf("rollback row = %#v", run.Stages[0])
	}
}
