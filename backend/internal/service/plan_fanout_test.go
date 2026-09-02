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

	got := fanoutStages([][]string{{"A", "B"}, {"C"}}, true)
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
	}

	if got := fanoutStages([][]string{{"A"}}, false); len(got) != 1 {
		t.Fatalf("stages = %#v, want no rollback row when it was not asked for", got)
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

func TestAnsweredBlockers_keepsOnlyThoseWithAnswers(t *testing.T) {
	t.Parallel()
	got := answeredBlockers([]PlanBlocker{
		{Stage: "A", Answer: "yes"},
		{Stage: "B"},
		{Stage: "C", Answer: "  "},
	})
	if len(got) != 1 || got[0].Stage != "A" {
		t.Fatalf("blockers = %#v", got)
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
