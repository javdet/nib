package service

import (
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

func TestFilterWaves_emptyFilterKeepsEverything(t *testing.T) {
	t.Parallel()
	waves := [][]string{{"A"}, {"B"}}

	if got := filterWaves(waves, nil); len(got) != 2 {
		t.Fatalf("waves = %v, want them all", got)
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
