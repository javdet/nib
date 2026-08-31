package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/google/uuid"
)

// PlanContract holds the cross-stage decisions taken during decomposition.
//
// Stage subagents plan concurrently, so any name two stages have to agree on --
// a user, a secret path, a namespace, a version -- must be decided here. At plan
// time no real value exists yet, only the name of one, and a name invented twice
// independently is a name that diverges: stage 1 writes "create user jmx" while
// stage 2 configures "cassandra_exporter". Deciding once, up front, is what lets
// the stages run in parallel at all.
type PlanContract struct {
	Shared []PlanContractValue          `json:"shared"`
	Stages map[string]PlanContractStage `json:"stages"`
}

// PlanContractValue is one agreed name. Value is what every stage must use
// verbatim; it is a name or a location, never a live secret.
type PlanContractValue struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Kind      string `json:"kind,omitempty"`
	DecidedBy string `json:"decidedBy,omitempty"`
}

type PlanContractStage struct {
	Provides []string `json:"provides,omitempty"`
	Requires []string `json:"requires,omitempty"`
	// Blocking marks a requirement the contract cannot pre-resolve: this stage
	// cannot be planned until it sees what the producing stage settled on after
	// its own research. Such a stage waits for a later wave; every other stage
	// is answered by Value above and plans immediately.
	Blocking bool `json:"blocking,omitempty"`
}

// planWaves groups stage titles into the order they may be planned in. Stages
// within a wave are independent and run concurrently.
//
// Only blocking requirements create an edge. An ordinary requirement is already
// answered by the contract, and the DAG's own edges describe operational
// ordering rather than information flow -- scheduling on those would serialise a
// near-linear DAG and give up the parallelism this exists for.
//
// Stages caught in a dependency cycle cannot be ordered, so they are returned in
// the first wave together with the reported error: a bad contract degrades to
// planning everything at once rather than planning nothing.
func planWaves(titles []string, contract PlanContract) ([][]string, error) {
	if len(titles) == 0 {
		return nil, nil
	}

	index := make(map[string]int, len(titles))
	for i, t := range titles {
		index[normalizeStageTitle(t)] = i
	}

	// Which stage settles each contract key.
	producer := make(map[string]int)
	for name, stage := range contract.Stages {
		i, ok := index[normalizeStageTitle(name)]
		if !ok {
			continue
		}
		for _, key := range stage.Provides {
			producer[key] = i
		}
	}

	deps := make([]map[int]struct{}, len(titles))
	for i := range deps {
		deps[i] = make(map[int]struct{})
	}
	for name, stage := range contract.Stages {
		if !stage.Blocking {
			continue
		}
		i, ok := index[normalizeStageTitle(name)]
		if !ok {
			continue
		}
		for _, key := range stage.Requires {
			if p, ok := producer[key]; ok && p != i {
				deps[i][p] = struct{}{}
			}
		}
	}

	remaining := make(map[int]struct{}, len(titles))
	for i := range titles {
		remaining[i] = struct{}{}
	}

	var waves [][]string
	for len(remaining) > 0 {
		var ready []int
		for i := range remaining {
			satisfied := true
			for d := range deps[i] {
				if _, pending := remaining[d]; pending {
					satisfied = false
					break
				}
			}
			if satisfied {
				ready = append(ready, i)
			}
		}

		if len(ready) == 0 {
			// A cycle: nothing can be ordered, so plan what is left together.
			for i := range remaining {
				ready = append(ready, i)
			}
			sort.Ints(ready)
			waves = append(waves, stageNames(titles, ready))
			return waves, fmt.Errorf("plan contract has a dependency cycle among %d stages", len(ready))
		}

		sort.Ints(ready)
		waves = append(waves, stageNames(titles, ready))
		for _, i := range ready {
			delete(remaining, i)
		}
	}
	return waves, nil
}

func stageNames(titles []string, idx []int) []string {
	out := make([]string, 0, len(idx))
	for _, i := range idx {
		out = append(out, titles[i])
	}
	return out
}

// ReadPlanContract returns the stored contract for a dialog, or false when the
// dialog was decomposed before contracts existed.
func (s *ChatService) ReadPlanContract(dialogID uuid.UUID) (PlanContract, bool, error) {
	path := filepath.Join(s.planContractsDir, dialogID.String()+".json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return PlanContract{}, false, nil
	}
	if err != nil {
		return PlanContract{}, false, err
	}

	var contract PlanContract
	if err := json.Unmarshal(b, &contract); err != nil {
		return PlanContract{}, false, fmt.Errorf("unmarshal plan contract: %w", err)
	}
	return contract, true, nil
}
