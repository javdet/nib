package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/llm"
)

const (
	oldDAG = "```mermaid\nflowchart TD\n  a[\"Build\"] --> b[\"Deploy\"]\n```"
	newDAG = "flowchart TD\n  a[\"Build\"] --> c[\"Monitor\"]"
)

// rebuildTestService is decomposeTestService plus the stores a fan-out and a
// plan status write touch.
func rebuildTestService(t *testing.T, repo *multiDialogRepo, provider llm.Provider) *ChatService {
	t.Helper()

	svc := decomposeTestService(t, repo, provider)
	dir := t.TempDir()
	svc.planFanoutDir = filepath.Join(dir, "plan_fanout")
	svc.planStateDir = filepath.Join(dir, "plan_state")
	svc.fanout = PlanFanoutConfig{}.withDefaults()
	return svc
}

// seedPlannedRoot gives root the old DAG and an action plan with a checkbox, a
// note and an in-progress status: everything a rebuild has to take away.
func seedPlannedRoot(t *testing.T, svc *ChatService, root uuid.UUID) {
	t.Helper()

	writeFile(t, filepath.Join(svc.dagsDir, root.String()+".md"), oldDAG)
	writeFile(t, filepath.Join(svc.actionPlansDir, root.String()+".json"),
		`{"stages":[{"number":1,"title":"Build","description":"b","steps":[{"type":"command","action":"make"}],"checks":[]},`+
			`{"number":2,"title":"Deploy","description":"d","steps":[{"type":"command","action":"ship"}],"checks":[]}],"rollback":[]}`)
	if err := svc.WriteActionPlanChecks(root, []string{"s0.step0"}); err != nil {
		t.Fatalf("WriteActionPlanChecks: %v", err)
	}
	if err := svc.writeActionPlanNotes(root, map[string]string{"s0.step0": "built image v1"}); err != nil {
		t.Fatalf("writeActionPlanNotes: %v", err)
	}
	if err := svc.WritePlanState(root, PlanState{Status: ActionPlanStatusInProgress}); err != nil {
		t.Fatalf("WritePlanState: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// waitForFanout lets a background run finish before the temp directory it
// writes into is removed.
func waitForFanout(t *testing.T, svc *ChatService, root uuid.UUID) FanoutRun {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		run, found, err := svc.ReadFanoutRun(root)
		if err != nil {
			t.Fatalf("ReadFanoutRun: %v", err)
		}
		if found && !run.Active() {
			if _, running := svc.fanoutRuns.Load(root); !running {
				return run
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the fan-out did not finish")
	return FanoutRun{}
}

func createDAGTurn(mermaid string) llm.AssistantMessage {
	args, _ := json.Marshal(map[string]string{"mermaid": mermaid})
	return llm.AssistantMessage{ToolCalls: []llm.ToolCall{
		{ID: "dag", Name: CreateDAGToolName, Arguments: string(args)},
	}}
}

func TestSameDAG(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b string
		want bool
	}{
		{"fenced and bare", oldDAG, "flowchart TD\n  a[\"Build\"] --> b[\"Deploy\"]", true},
		{"indentation and blank lines", oldDAG, "flowchart TD\n\n\ta[\"Build\"]   -->   b[\"Deploy\"]\n", true},
		{"a stage renamed", oldDAG, "flowchart TD\n  a[\"Build\"] --> b[\"Release\"]", false},
		{"an edge added", oldDAG, "flowchart TD\n  a[\"Build\"] --> b[\"Deploy\"]\n  b --> c[\"Monitor\"]", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sameDAG(tt.a, tt.b); got != tt.want {
				t.Fatalf("sameDAG = %v, want %v", got, tt.want)
			}
		})
	}
}

// Only a DAG that actually changed under an existing action plan owes a
// rebuild: the first DAG has nothing to discard, and a re-save of the same
// diagram must not cost the operator their checks.
func TestCreateDAGMarksTheActionPlanForRebuild(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		priorDAG bool
		plan     bool
		mermaid  string
		want     bool
	}{
		{"changed under a plan", true, true, newDAG, true},
		{"same diagram re-saved", true, true, "flowchart TD\n a[\"Build\"] --> b[\"Deploy\"]", false},
		{"changed before anything was planned", true, false, newDAG, false},
		{"first DAG", false, false, newDAG, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := rebuildTestService(t, newMultiDialogRepo(), &scriptedProvider{})
			root := uuid.New()
			if tt.priorDAG {
				writeFile(t, filepath.Join(svc.dagsDir, root.String()+".md"), oldDAG)
			}
			if tt.plan {
				writeFile(t, filepath.Join(svc.actionPlansDir, root.String()+".json"), `{"stages":[],"rollback":[]}`)
			}

			out, err := svc.createDAGHandler(root)(t.Context(), map[string]any{"mermaid": tt.mermaid})
			if err != nil {
				t.Fatalf("create_dag err = %v", err)
			}
			owed, err := svc.actionPlanRebuildOwed(root)
			if err != nil {
				t.Fatalf("actionPlanRebuildOwed: %v", err)
			}
			if owed != tt.want {
				t.Fatalf("rebuild owed = %v, want %v", owed, tt.want)
			}
			if mentions := strings.Contains(out, "planned again from scratch"); mentions != tt.want {
				t.Errorf("tool output = %q, mentions the rebuild = %v, want %v", out, mentions, tt.want)
			}
		})
	}
}

// The heart of it: a decompose that rewrites the DAG of a planned plan leaves
// it with no trace of the old action list, and planning of the new DAG under
// way.
func TestDecomposeChangingTheDAGRebuildsTheActionPlan(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := rebuildTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		createDAGTurn(newDAG),
		{Content: "Replaced Deploy with Monitor."},
	}})
	seedPlannedRoot(t, svc, root)

	res, err := svc.launchDecomposeSubagent(t.Context(), root, "swap deploy for monitoring")
	if err != nil {
		t.Fatalf("launchDecomposeSubagent err = %v", err)
	}
	run := waitForFanout(t, svc, root)

	if res.Status != SubagentCompleted {
		t.Fatalf("status = %q, want %q", res.Status, SubagentCompleted)
	}
	if !strings.HasPrefix(res.Summary, "Replaced Deploy with Monitor.") {
		t.Errorf("summary = %q, want the sub-agent's own answer first", res.Summary)
	}
	if !strings.Contains(res.Summary, "built again from scratch") {
		t.Errorf("summary = %q, want it to say the action list is being rebuilt", res.Summary)
	}
	if res.Next != actionPlanRebuildNext {
		t.Errorf("next = %q, want the orchestrator told not to launch plan itself", res.Next)
	}

	if !run.Rebuilt {
		t.Error("the fan-out does not record that it started from scratch")
	}
	var titles []string
	for _, st := range run.Stages {
		if st.Carried {
			t.Errorf("stage %q was carried from the old run, want every stage planned afresh", st.Title)
		}
		titles = append(titles, st.Title)
	}
	if got := strings.Join(titles, ","); got != "Build,Monitor,"+rollbackStageTitle {
		t.Errorf("planned stages = %s, want the new DAG's", got)
	}

	if checked, _ := svc.ReadActionPlanChecks(root); len(checked) != 0 {
		t.Errorf("checks = %v, want none left from the old plan", checked)
	}
	if notes, _ := svc.readActionPlanNotes(root); len(notes) != 0 {
		t.Errorf("notes = %v, want none left from the old plan", notes)
	}
	if raw, found, _ := svc.ReadActionPlan(root); found && strings.Contains(string(raw), "Deploy") {
		t.Errorf("action plan = %s, still holds a stage the new DAG dropped", raw)
	}
	if state, _ := svc.ReadPlanState(root); state.Status != ActionPlanStatusDraft {
		t.Errorf("plan status = %q, want %q once nothing on it has been carried out", state.Status, ActionPlanStatusDraft)
	}
	if owed, _ := svc.actionPlanRebuildOwed(root); owed {
		t.Error("the rebuild is still marked owed after it started")
	}
}

// Decompose may write a DAG and then ask the operator something. The plan is
// left alone until it finishes, since the DAG it settles on may differ again.
func TestDecomposeAskingKeepsThePlanUntilItFinishes(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := rebuildTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		createDAGTurn(newDAG),
		askQuestionTurn("q1", "Which monitoring stack?"),
	}})
	seedPlannedRoot(t, svc, root)

	res, err := svc.launchDecomposeSubagent(t.Context(), root, "swap deploy for monitoring")
	if err != nil {
		t.Fatalf("launchDecomposeSubagent err = %v", err)
	}
	if res.Status != SubagentAwaitingInput {
		t.Fatalf("status = %q, want %q", res.Status, SubagentAwaitingInput)
	}
	if _, found, _ := svc.ReadActionPlan(root); !found {
		t.Error("the action plan was discarded while decompose was still asking")
	}
	if owed, _ := svc.actionPlanRebuildOwed(root); !owed {
		t.Error("the rebuild was forgotten while decompose was still asking")
	}
}

// A DAG decompose leaves as it was is no reason to throw a plan away.
func TestDecomposeLeavingTheDAGAloneKeepsThePlan(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := rebuildTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		createDAGTurn(oldDAG),
		{Content: "The DAG already covers that."},
	}})
	seedPlannedRoot(t, svc, root)

	res, err := svc.launchDecomposeSubagent(t.Context(), root, "does it cover deploys?")
	if err != nil {
		t.Fatalf("launchDecomposeSubagent err = %v", err)
	}
	if res.Summary != "The DAG already covers that." {
		t.Errorf("summary = %q, want the sub-agent's answer alone", res.Summary)
	}
	if checked, _ := svc.ReadActionPlanChecks(root); len(checked) != 1 {
		t.Errorf("checks = %v, want the operator's checkbox kept", checked)
	}
	if _, found, _ := svc.ReadFanoutRun(root); found {
		t.Error("a fan-out started for a DAG that did not change")
	}
}

// An item of the plan still running holds the rebuild off -- its result would
// land on whichever row took its position -- and the rebuild stays owed until a
// plan of every stage does it.
func TestRebuildWaitsForARunningItemOfThePlan(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := rebuildTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		createDAGTurn(newDAG),
		{Content: "Replaced Deploy with Monitor."},
	}})
	seedPlannedRoot(t, svc, root)

	lease, ok := svc.acquireExecutionLease(subagentLease(root, "s0.step0", "1.1"))
	if !ok {
		t.Fatal("acquire lease refused")
	}

	res, err := svc.launchDecomposeSubagent(t.Context(), root, "swap deploy for monitoring")
	if err != nil {
		t.Fatalf("launchDecomposeSubagent err = %v", err)
	}
	if !strings.Contains(res.Summary, "1.1 of this plan is still running") {
		t.Errorf("summary = %q, want it to name the running item", res.Summary)
	}
	if res.Next != actionPlanRebuildOwedNext {
		t.Errorf("next = %q, want the orchestrator told the list is out of date", res.Next)
	}
	if checked, _ := svc.ReadActionPlanChecks(root); len(checked) != 1 {
		t.Errorf("checks = %v, want the plan left alone while 1.1 runs", checked)
	}
	if owed, _ := svc.actionPlanRebuildOwed(root); !owed {
		t.Fatal("the rebuild is no longer owed after being held off")
	}

	svc.releaseExecutionLease(lease.token)

	run, err := svc.StartPlanFanout(t.Context(), root, AllFanoutTargets())
	if err != nil {
		t.Fatalf("StartPlanFanout err = %v", err)
	}
	waitForFanout(t, svc, root)
	if !run.Rebuilt {
		t.Error("a plan of every stage did not settle the rebuild it was owed")
	}
	if checked, _ := svc.ReadActionPlanChecks(root); len(checked) != 0 {
		t.Errorf("checks = %v, want none after the rebuild", checked)
	}
}

// Replanning named stages is a different request, and leaves an owed rebuild
// owed rather than discarding the plan behind the operator's back.
func TestReplanningNamedStagesDoesNotDiscardThePlan(t *testing.T) {
	t.Parallel()

	svc := rebuildTestService(t, newMultiDialogRepo(), &scriptedProvider{})
	root := mainRoot(svc.dialogRepo.(*multiDialogRepo))
	seedPlannedRoot(t, svc, root)
	if err := svc.setActionPlanRebuild(root, true); err != nil {
		t.Fatalf("setActionPlanRebuild: %v", err)
	}

	run, err := svc.StartPlanFanout(t.Context(), root, FanoutTargets{Stages: []string{"Build"}})
	if err != nil {
		t.Fatalf("StartPlanFanout err = %v", err)
	}
	waitForFanout(t, svc, root)
	if run.Rebuilt {
		t.Error("replanning one stage discarded the whole plan")
	}
	if owed, _ := svc.actionPlanRebuildOwed(root); !owed {
		t.Error("the rebuild is no longer owed after a one-stage replan")
	}
}
