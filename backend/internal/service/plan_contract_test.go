package service

import (
	"strings"
	"testing"
)

func contractStage(provides, requires []string, blocking bool) PlanContractStage {
	return PlanContractStage{Provides: provides, Requires: requires, Blocking: blocking}
}

func waveTitles(waves [][]string) string {
	parts := make([]string, 0, len(waves))
	for _, w := range waves {
		parts = append(parts, strings.Join(w, "+"))
	}
	return strings.Join(parts, " | ")
}

// Without a contract nothing is known to depend on anything, so every stage is
// planned at once. This is also the path older dialogs take.
func TestPlanWaves_noContractPlansEverythingAtOnce(t *testing.T) {
	t.Parallel()
	titles := []string{"A", "B", "C"}

	waves, err := planWaves(titles, PlanContract{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := waveTitles(waves); got != "A+B+C" {
		t.Fatalf("waves = %q, want one wave of everything", got)
	}
}

// The Cassandra Reaper case: stage 2 needs a user and a secret path from stage 1,
// but the contract already names both, so the stages still plan concurrently.
func TestPlanWaves_nonBlockingRequirementKeepsStagesTogether(t *testing.T) {
	t.Parallel()
	titles := []string{"Configure Cassandra JMX", "Install Cassandra Reaper"}
	contract := PlanContract{
		Shared: []PlanContractValue{
			{Key: "cassandra.jmx.user", Value: "reaper_jmx"},
			{Key: "cassandra.jmx.secret", Value: "vault:secret/cassandra/jmx#password"},
		},
		Stages: map[string]PlanContractStage{
			"Configure Cassandra JMX": contractStage([]string{"cassandra.jmx.user", "cassandra.jmx.secret"}, nil, false),
			"Install Cassandra Reaper": contractStage(
				nil, []string{"cassandra.jmx.user", "cassandra.jmx.secret"}, false),
		},
	}

	waves, err := planWaves(titles, contract)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(waves) != 1 {
		t.Fatalf("waves = %q, want a single wave", waveTitles(waves))
	}
}

// A blocking requirement is the one case that costs a wave.
func TestPlanWaves_blockingRequirementDefersTheConsumer(t *testing.T) {
	t.Parallel()
	titles := []string{"Provision cluster", "Configure autoscaling", "Write runbook"}
	contract := PlanContract{
		Stages: map[string]PlanContractStage{
			"Provision cluster":     contractStage([]string{"cluster.id"}, nil, false),
			"Configure autoscaling": contractStage(nil, []string{"cluster.id"}, true),
		},
	}

	waves, err := planWaves(titles, contract)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := waveTitles(waves); got != "Provision cluster+Write runbook | Configure autoscaling" {
		t.Fatalf("waves = %q", got)
	}
}

// A diamond collapses to three waves, with the independent middle pair together.
func TestPlanWaves_diamond(t *testing.T) {
	t.Parallel()
	titles := []string{"Base", "Left", "Right", "Join"}
	contract := PlanContract{
		Stages: map[string]PlanContractStage{
			"Base":  contractStage([]string{"base"}, nil, false),
			"Left":  contractStage([]string{"left"}, []string{"base"}, true),
			"Right": contractStage([]string{"right"}, []string{"base"}, true),
			"Join":  contractStage(nil, []string{"left", "right"}, true),
		},
	}

	waves, err := planWaves(titles, contract)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := waveTitles(waves); got != "Base | Left+Right | Join" {
		t.Fatalf("waves = %q", got)
	}
}

// A cycle cannot be ordered. Rather than planning nothing, everything left is
// returned in one wave alongside the error.
func TestPlanWaves_cycleDegradesToOneWave(t *testing.T) {
	t.Parallel()
	titles := []string{"A", "B"}
	contract := PlanContract{
		Stages: map[string]PlanContractStage{
			"A": contractStage([]string{"a"}, []string{"b"}, true),
			"B": contractStage([]string{"b"}, []string{"a"}, true),
		},
	}

	waves, err := planWaves(titles, contract)
	if err == nil {
		t.Fatal("expected a cycle error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("err = %v", err)
	}
	if got := waveTitles(waves); got != "A+B" {
		t.Fatalf("waves = %q, want every stage in one wave", got)
	}
}

// Contract stages the DAG does not name carry no position and must not create
// phantom dependencies.
func TestPlanWaves_ignoresStagesMissingFromTheDAG(t *testing.T) {
	t.Parallel()
	titles := []string{"Real stage"}
	contract := PlanContract{
		Stages: map[string]PlanContractStage{
			"Real stage":  contractStage(nil, []string{"ghost"}, true),
			"Ghost stage": contractStage([]string{"ghost"}, nil, false),
		},
	}

	waves, err := planWaves(titles, contract)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := waveTitles(waves); got != "Real stage" {
		t.Fatalf("waves = %q", got)
	}
}

// Stage names are matched the same loose way update_action_plan matches them.
func TestPlanWaves_matchesStageNamesLoosely(t *testing.T) {
	t.Parallel()
	titles := []string{"Provision Cluster", "Configure Autoscaling"}
	contract := PlanContract{
		Stages: map[string]PlanContractStage{
			"provision  cluster":    contractStage([]string{"cluster.id"}, nil, false),
			"CONFIGURE AUTOSCALING": contractStage(nil, []string{"cluster.id"}, true),
		},
	}

	waves, err := planWaves(titles, contract)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got := waveTitles(waves); got != "Provision Cluster | Configure Autoscaling" {
		t.Fatalf("waves = %q", got)
	}
}
