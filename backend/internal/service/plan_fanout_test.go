package service

import (
	"errors"
	"strings"
	"testing"
)

func TestFilterWaves_keepsOnlyNamedStagesAndDropsEmptyWaves(t *testing.T) {
	t.Parallel()
	waves := [][]string{{"A", "B"}, {"C"}, {"D"}}

	got := filterWaves(waves, []string{"b", "D"})
	if len(got) != 2 || got[0][0] != "B" || got[1][0] != "D" {
		t.Fatalf("waves = %v, want [[B] [D]]", got)
	}
}

// Every stage is asked for with AllStages, which never reaches filterWaves, so
// an empty filter here means a run that names no stage -- the shape of a round
// that redoes the rollback alone.
func TestFilterWaves_emptyFilterKeepsNothing(t *testing.T) {
	t.Parallel()
	waves := [][]string{{"A"}, {"B"}}

	if got := filterWaves(waves, nil); len(got) != 0 {
		t.Fatalf("waves = %v, want none", got)
	}
}

func TestFanoutTargets_work(t *testing.T) {
	t.Parallel()
	waves := [][]string{{"A", "B"}, {"C"}}

	t.Run("all stages and the rollback", func(t *testing.T) {
		t.Parallel()
		got, rollback, err := AllFanoutTargets().work(waves)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != 2 || !rollback {
			t.Fatalf("waves = %v, rollback = %v, want both waves and the rollback", got, rollback)
		}
	})

	t.Run("named stages and the rollback", func(t *testing.T) {
		t.Parallel()
		got, rollback, err := FanoutTargets{Stages: []string{"C"}, Rollback: true}.work(waves)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != 1 || got[0][0] != "C" || !rollback {
			t.Fatalf("waves = %v, rollback = %v", got, rollback)
		}
	})

	// A round the user's answers start may have nothing but the rollback left to
	// redo, and that is work, not an empty run.
	t.Run("the rollback alone", func(t *testing.T) {
		t.Parallel()
		got, rollback, err := FanoutTargets{Rollback: true}.work(waves)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(got) != 0 || !rollback {
			t.Fatalf("waves = %v, rollback = %v, want the rollback only", got, rollback)
		}
	})

	t.Run("nothing at all", func(t *testing.T) {
		t.Parallel()
		for name, targets := range map[string]FanoutTargets{
			"the zero value":    {},
			"a stage of no DAG": {Stages: []string{"Z"}},
		} {
			if _, _, err := targets.work(waves); !errors.Is(err, ErrNoDAGStages) {
				t.Fatalf("%s: err = %v, want ErrNoDAGStages", name, err)
			}
		}
	})
}

// The rollback goes last, in a wave of its own, because it cannot be worked out
// until every stage is written.
func TestFanoutStages_putsTheRollbackAfterTheLastWave(t *testing.T) {
	t.Parallel()

	waves := [][]string{{"A", "B"}, {"C"}}
	got := fanoutStages(waves, stageTitleSet(waves), true, FanoutRun{})
	if len(got) != 4 {
		t.Fatalf("stages = %#v, want three stages and the rollback", got)
	}
	last := got[3]
	if last.Kind != FanoutStageKindRollback || last.Title != rollbackStageTitle {
		t.Fatalf("last = %#v, want the rollback", last)
	}
	if last.Wave != 2 {
		t.Fatalf("wave = %d, want 2: after both stage waves", last.Wave)
	}
	for _, st := range got[:3] {
		if st.Kind != FanoutStageKindStage {
			t.Fatalf("stage %q carries kind %q", st.Title, st.Kind)
		}
		if st.Carried {
			t.Fatalf("stage %q is carried, want planned", st.Title)
		}
	}
}

// The list is the plan, not the round: a run that leaves the rollback alone
// still shows its row, because a unit of work vanishing from the list reads as
// work that was lost.
func TestFanoutStages_carriesTheRollbackARunDoesNotRedo(t *testing.T) {
	t.Parallel()

	waves := [][]string{{"A"}}
	previous := FanoutRun{Stages: []FanoutStage{
		{Title: "A", Status: FanoutStageDone, DialogID: "d-a"},
		{Title: rollbackStageTitle, Kind: FanoutStageKindRollback, Status: FanoutStageDone, DialogID: "d-r"},
	}}

	got := fanoutStages(waves, stageTitleSet(waves), false, previous)
	if len(got) != 2 {
		t.Fatalf("stages = %#v, want the stage and the carried rollback", got)
	}
	rollback := got[1]
	if rollback.Kind != FanoutStageKindRollback || !rollback.Carried {
		t.Fatalf("rollback = %#v, want a carried rollback row", rollback)
	}
	if rollback.Status != FanoutStageDone || rollback.DialogID != "d-r" {
		t.Fatalf("rollback = %#v, want the earlier run's result and dialog", rollback)
	}
}

// Answering one question replans one stage. Every other stage stays in the list
// with what it already had -- including the dialog of the subagent that wrote
// it, which is the only way back to its research.
func TestFanoutStages_carriesTheStagesARunDoesNotReplan(t *testing.T) {
	t.Parallel()

	allWaves := [][]string{{"A", "B"}, {"C", "D"}}
	previous := FanoutRun{Stages: []FanoutStage{
		{Title: "A", Status: FanoutStageDone, DialogID: "d-a"},
		{Title: "B", Status: FanoutStageFailed, DialogID: "d-b", Error: "boom"},
		{Title: "C", Status: FanoutStageRunning, DialogID: "d-c"},
		{Title: rollbackStageTitle, Kind: FanoutStageKindRollback, Status: FanoutStageDone},
	}}

	got := fanoutStages(allWaves, stageTitleSet([][]string{{"A"}}), true, previous)
	if len(got) != 5 {
		t.Fatalf("stages = %d (%#v), want every DAG stage and the rollback", len(got), got)
	}

	byTitle := make(map[string]FanoutStage, len(got))
	for _, st := range got {
		byTitle[st.Title] = st
	}

	if a := byTitle["A"]; a.Carried || a.Status != FanoutStagePending || a.DialogID != "" {
		t.Errorf("A = %#v, want a fresh pending row: it is the stage being replanned", a)
	}
	if b := byTitle["B"]; !b.Carried || b.Status != FanoutStageFailed || b.DialogID != "d-b" || b.Error != "boom" {
		t.Errorf("B = %#v, want the earlier failure carried with its dialog", b)
	}
	// A row a finished run left running has no agent behind it any more.
	if c := byTitle["C"]; !c.Carried || c.Status != FanoutStageFailed || c.Error != abandonedReason {
		t.Errorf("C = %#v, want an abandoned row rather than a live one", c)
	}
	// The DAG has D, the previous run never did: it belongs in the list, and the
	// row says nobody has written it.
	if d := byTitle["D"]; !d.Carried || d.Status != FanoutStagePending || d.Error != notPlannedYetReason {
		t.Errorf("D = %#v, want a carried row explaining it is unplanned", d)
	}
	if r := byTitle[rollbackStageTitle]; r.Carried || r.Status != FanoutStagePending {
		t.Errorf("rollback = %#v, want a fresh pending row: every replan redoes it", r)
	}
	// Wave numbers stay the DAG's, so the rollback still sorts after every stage.
	if byTitle["C"].Wave != 1 || byTitle[rollbackStageTitle].Wave != 2 {
		t.Errorf("waves = %d and %d, want the DAG's own order", byTitle["C"].Wave, byTitle[rollbackStageTitle].Wave)
	}
}

// Two stages hitting the same wall is one question for the user, not two, and
// answering it has to replan both.
func TestGroupBlockers_foldsTheSameQuestionAcrossStages(t *testing.T) {
	t.Parallel()
	blockers := []PlanBlocker{
		{Stage: "Install Reaper", Question: "Which auth mode?", Options: []string{"jmx", "none"}, Assumption: "jmx"},
		{Stage: "Verify repairs", Question: "which  AUTH mode?", Options: []string{"none", "mtls"}, Assumption: "jmx"},
		{Stage: "Install Reaper", Question: "Which schedule?", Assumption: "daily"},
	}

	got := groupBlockers(blockers)
	if len(got) != 2 {
		t.Fatalf("questions = %d, want 2", len(got))
	}
	if len(got[0].Stages) != 2 {
		t.Fatalf("stages = %v, want both stages on the shared question", got[0].Stages)
	}
	// Options from both stages survive, deduplicated.
	if strings.Join(got[0].Options, ",") != "jmx,none,mtls" {
		t.Fatalf("options = %v", got[0].Options)
	}
}

// An answered blocker is history, not an open question.
func TestGroupBlockers_skipsAnsweredQuestions(t *testing.T) {
	t.Parallel()
	blockers := []PlanBlocker{
		{Stage: "A", Question: "Settled?", Assumption: "x", Answer: "yes"},
		{Stage: "B", Question: "Open?", Assumption: "y"},
	}

	got := groupBlockers(blockers)
	if len(got) != 1 || got[0].Question != "Open?" {
		t.Fatalf("questions = %#v, want only the open one", got)
	}
}

// The rollback agent's question folds into the shared one, but it is not a stage
// and must not be named as one: answering redoes the rollback because the run
// always does, not because it appears in PendingStages.
func TestGroupBlockers_marksTheRollbackAgentWithoutNamingItAStage(t *testing.T) {
	t.Parallel()
	blockers := []PlanBlocker{
		{Stage: "Install Reaper", Question: "Which auth mode?", Assumption: "jmx"},
		{
			Stage: rollbackStageTitle, Kind: FanoutStageKindRollback,
			Question: "which  AUTH mode?", Options: []string{"mtls"}, Assumption: "jmx",
		},
		{Stage: rollbackStageTitle, Kind: FanoutStageKindRollback, Question: "Snapshots?", Assumption: "yes"},
	}

	got := groupBlockers(blockers)
	if len(got) != 2 {
		t.Fatalf("questions = %d, want 2", len(got))
	}
	if len(got[0].Stages) != 1 || got[0].Stages[0] != "Install Reaper" {
		t.Fatalf("stages = %v, want only the stage that asked", got[0].Stages)
	}
	// A question only the rollback agent raised names no stage, so answering it
	// replans none -- the rollback is redone because every round redoes it.
	if len(got[1].Stages) != 0 {
		t.Fatalf("stages = %v, want none for a rollback-only question", got[1].Stages)
	}
}

// A round asks two questions. The rest have to survive it, or a question raised
// by a stage nobody is replanning could never be put to the operator at all.
func TestCarryBlockers_keepsWhatTheNextRoundStillHasToAsk(t *testing.T) {
	t.Parallel()

	blockers := []PlanBlocker{
		{Stage: "A", Question: "answered", Answer: "yes"},
		{Stage: "A", Question: "A raises this again"},
		{Stage: "B", Question: "nobody is replanning B"},
		{Stage: rollbackStageTitle, Kind: FanoutStageKindRollback, Question: "the rollback re-raises this"},
		{Stage: "C", Question: "blank answers are not answers", Answer: "  "},
	}

	got := carryBlockers(blockers, map[string]struct{}{"a": {}}, true)

	var kept []string
	for _, b := range got {
		kept = append(kept, b.Question)
	}
	want := []string{"answered", "nobody is replanning B", "blank answers are not answers"}
	if strings.Join(kept, "|") != strings.Join(want, "|") {
		t.Fatalf("kept = %v, want %v", kept, want)
	}
}

// With the rollback left alone, its own open question is nobody else's to raise.
func TestCarryBlockers_keepsARollbackQuestionWhenTheRollbackIsNotRedone(t *testing.T) {
	t.Parallel()

	got := carryBlockers([]PlanBlocker{
		{Stage: rollbackStageTitle, Kind: FanoutStageKindRollback, Question: "still open"},
	}, nil, false)
	if len(got) != 1 || got[0].Question != "still open" {
		t.Fatalf("blockers = %#v, want the rollback's question kept", got)
	}
}

func TestPlanFanoutConfig_defaults(t *testing.T) {
	t.Parallel()
	c := PlanFanoutConfig{}.withDefaults()
	if c.Concurrency != defaultPlanFanoutConcurrency || c.StageMaxIterations != defaultStageMaxIterations {
		t.Fatalf("config = %#v", c)
	}
	if c.timeout() != defaultPlanFanoutTimeout {
		t.Fatalf("timeout = %v", c.timeout())
	}
}
