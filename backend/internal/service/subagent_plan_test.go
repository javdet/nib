package service

import (
	"reflect"
	"strings"
	"testing"
)

// "let's work on stage 1" is what an operator actually says, so a bare number
// has to resolve as well as a title.
func TestResolveDAGStages(t *testing.T) {
	t.Parallel()

	titles := []string{"Deploy Centrifugo", "Add monitoring", "Verify"}

	for name, tc := range map[string]struct {
		requested []string
		want      []string
	}{
		"an exact title":        {[]string{"Add monitoring"}, []string{"Add monitoring"}},
		"a title cased loosely": {[]string{"add MONITORING"}, []string{"Add monitoring"}},
		"a number":              {[]string{"1"}, []string{"Deploy Centrifugo"}},
		"a number with a dot":   {[]string{"3."}, []string{"Verify"}},
		"several at once":       {[]string{"1", "Verify"}, []string{"Deploy Centrifugo", "Verify"}},
	} {
		got, miss := resolveDAGStages(titles, tc.requested)
		if miss != "" {
			t.Errorf("%s: unexpected miss %q", name, miss)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: resolveDAGStages = %v, want %v", name, got, tc.want)
		}
	}
}

// The sentence that reports the mistake is also the one that teaches the
// orchestrator how to retry, so it has to list the real stages.
func TestResolveDAGStagesReportsAMissWithTheRealTitles(t *testing.T) {
	t.Parallel()

	titles := []string{"Deploy Centrifugo", "Add monitoring"}

	for name, requested := range map[string][]string{
		"a stage that is not there": {"Delete the cluster"},
		"a number past the end":     {"9"},
		"nothing at all":            {"   "},
	} {
		got, miss := resolveDAGStages(titles, requested)
		if miss == "" {
			t.Errorf("%s: resolveDAGStages = %v, want a miss", name, got)
			continue
		}
		for _, want := range []string{"1. Deploy Centrifugo", "2. Add monitoring"} {
			if !strings.Contains(miss, want) {
				t.Errorf("%s: miss %q does not list %q", name, miss, want)
			}
		}
	}
}

// The orchestrator and the HTTP route agree on every shape but one, and that one
// is deliberate: "redo the rollback" must not replan every stage.
func TestPlanSubagentTargets(t *testing.T) {
	t.Parallel()

	yes, no := true, false

	for name, tc := range map[string]struct {
		stages   []string
		rollback *bool
		want     FanoutTargets
	}{
		"nothing named is the whole plan": {
			nil, nil,
			FanoutTargets{AllStages: true, Rollback: true},
		},
		"named stages are a replan, and the rollback follows them": {
			[]string{"Deploy"}, nil,
			FanoutTargets{Stages: []string{"Deploy"}, Rollback: true},
		},
		"named stages without the rollback": {
			[]string{"Deploy"}, &no,
			FanoutTargets{Stages: []string{"Deploy"}},
		},
		"the rollback on its own": {
			nil, &yes,
			FanoutTargets{Rollback: true},
		},
		"every stage but no rollback": {
			nil, &no,
			FanoutTargets{AllStages: true},
		},
	} {
		if got := planSubagentTargets(tc.stages, tc.rollback); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: planSubagentTargets = %#v, want %#v", name, got, tc.want)
		}
	}
}

// The summary is what the orchestrator tells the operator, so it has to name the
// work that actually started.
func TestPlannedStagesSummary(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		run  FanoutRun
		want string
	}{
		"stages and the rollback": {
			FanoutRun{Stages: []FanoutStage{
				{Title: "Deploy"},
				{Title: "Rollback plan", Kind: FanoutStageKindRollback},
			}},
			"planning Deploy, then redoing the rollback",
		},
		"the rollback alone": {
			FanoutRun{Stages: []FanoutStage{{Title: "Rollback plan", Kind: FanoutStageKindRollback}}},
			"redoing the rollback plan",
		},
		"stages alone": {
			FanoutRun{Stages: []FanoutStage{{Title: "Deploy"}, {Title: "Verify"}}},
			"planning Deploy, Verify",
		},
	} {
		if got := plannedStagesSummary(tc.run); got != tc.want {
			t.Errorf("%s: plannedStagesSummary = %q, want %q", name, got, tc.want)
		}
	}
}
