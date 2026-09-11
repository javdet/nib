package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

// ActionPlanScope identifies which list within a stage is being reordered.
type ActionPlanScope string

const (
	ActionPlanScopeSteps ActionPlanScope = "steps"
	ActionPlanScopeChecks ActionPlanScope = "checks"
)

// ErrInvalidActionPlanScope is returned when scope is not "steps" or "checks".
var ErrInvalidActionPlanScope = errors.New("scope must be steps or checks")

// ErrActionPlanIndexOutOfRange is returned when stage/from/to indices are invalid.
var ErrActionPlanIndexOutOfRange = errors.New("action plan index out of range")

var actionPlanKeyPattern = regexp.MustCompile(`^s(\d+)\.(step|check)(\d+)$`)

// moveIndex returns a permutation mapping old indices to new indices after moving
// the element at from to to within a list of length n.
func moveIndex(n, from, to int) ([]int, error) {
	if from < 0 || from >= n || to < 0 || to >= n {
		return nil, ErrActionPlanIndexOutOfRange
	}
	if from == to {
		perm := make([]int, n)
		for i := range perm {
			perm[i] = i
		}
		return perm, nil
	}

	perm := make([]int, n)
	for old := range perm {
		newIdx := old
		if old == from {
			newIdx = to
		} else if from < to {
			if old > from && old <= to {
				newIdx = old - 1
			}
		} else {
			if old >= to && old < from {
				newIdx = old + 1
			}
		}
		perm[old] = newIdx
	}
	return perm, nil
}

func actionPlanItemKey(stage int, scope ActionPlanScope, index int) string {
	switch scope {
	case ActionPlanScopeSteps:
		return fmt.Sprintf("s%d.step%d", stage, index)
	case ActionPlanScopeChecks:
		return fmt.Sprintf("s%d.check%d", stage, index)
	default:
		return ""
	}
}

func remapCheckedKeys(checked []string, stage int, scope ActionPlanScope, perm []int) []string {
	if len(checked) == 0 {
		return checked
	}

	oldToNew := make(map[string]string, len(perm))
	for oldIdx, newIdx := range perm {
		oldKey := actionPlanItemKey(stage, scope, oldIdx)
		newKey := actionPlanItemKey(stage, scope, newIdx)
		oldToNew[oldKey] = newKey
	}

	out := make([]string, 0, len(checked))
	for _, key := range checked {
		m := actionPlanKeyPattern.FindStringSubmatch(key)
		if m == nil {
			out = append(out, key)
			continue
		}
		keyStage := atoi(m[1])
		keyKind := m[2]
		wantKind := "step"
		if scope == ActionPlanScopeChecks {
			wantKind = "check"
		}
		if keyStage != stage || keyKind != wantKind {
			out = append(out, key)
			continue
		}
		if newKey, ok := oldToNew[key]; ok {
			out = append(out, newKey)
		}
	}
	return out
}

// remapActionPlanKeys moves a key-addressed side store onto the row keys a
// reorder produced. Checkboxes, comments, agent-runner dialogs and execution
// records are all indexed by position, so each one is remapped in the same pass
// that renumbers the plan -- a store left behind points at somebody else's row.
func remapActionPlanKeys[V any](store map[string]V, stage int, scope ActionPlanScope, perm []int) map[string]V {
	if len(store) == 0 {
		return store
	}

	out := make(map[string]V, len(store))
	for key, value := range store {
		m := actionPlanKeyPattern.FindStringSubmatch(key)
		if m == nil {
			out[key] = value
			continue
		}
		keyStage := atoi(m[1])
		keyKind := m[2]
		wantKind := "step"
		if scope == ActionPlanScopeChecks {
			wantKind = "check"
		}
		if keyStage != stage || keyKind != wantKind {
			out[key] = value
			continue
		}
		oldIdx := atoi(m[3])
		if oldIdx < 0 || oldIdx >= len(perm) {
			out[key] = value
			continue
		}
		newKey := actionPlanItemKey(stage, scope, perm[oldIdx])
		out[newKey] = value
	}
	return out
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func parseActionPlanScope(scope ActionPlanScope) (string, error) {
	switch scope {
	case ActionPlanScopeSteps:
		return "steps", nil
	case ActionPlanScopeChecks:
		return "checks", nil
	default:
		return "", ErrInvalidActionPlanScope
	}
}

func movePlanItems(plan map[string]any, scope ActionPlanScope, stage, from, to int) error {
	scopeKey, err := parseActionPlanScope(scope)
	if err != nil {
		return err
	}

	stagesRaw, ok := plan["stages"]
	if !ok {
		return ErrActionPlanIndexOutOfRange
	}
	stages, ok := stagesRaw.([]any)
	if !ok {
		return ErrActionPlanIndexOutOfRange
	}
	if stage < 0 || stage >= len(stages) {
		return ErrActionPlanIndexOutOfRange
	}

	stageObj, ok := stages[stage].(map[string]any)
	if !ok {
		return ErrActionPlanIndexOutOfRange
	}

	itemsRaw, ok := stageObj[scopeKey]
	if !ok {
		return ErrActionPlanIndexOutOfRange
	}
	items, ok := itemsRaw.([]any)
	if !ok {
		return ErrActionPlanIndexOutOfRange
	}

	n := len(items)
	if from < 0 || from >= n || to < 0 || to >= n {
		return ErrActionPlanIndexOutOfRange
	}
	if from == to {
		return nil
	}

	moved := make([]any, n)
	copy(moved, items)
	item := moved[from]
	if from < to {
		for i := from; i < to; i++ {
			moved[i] = moved[i+1]
		}
	} else {
		for i := from; i > to; i-- {
			moved[i] = moved[i-1]
		}
	}
	moved[to] = item
	stageObj[scopeKey] = moved
	stages[stage] = stageObj
	plan["stages"] = stages
	return nil
}

// ReorderActionPlanItems moves an item within a stage's steps or checks list and
// remaps persisted checkbox and comment keys atomically.
func (s *ChatService) ReorderActionPlanItems(
	dialogID uuid.UUID,
	scope ActionPlanScope,
	stage, from, to int,
) (json.RawMessage, []string, map[string]string, error) {
	planRaw, found, err := s.ReadActionPlan(dialogID)
	if err != nil {
		return nil, nil, nil, err
	}
	if !found {
		return nil, nil, nil, fmt.Errorf("action plan not found")
	}

	var plan map[string]any
	if err := json.Unmarshal(planRaw, &plan); err != nil {
		return nil, nil, nil, fmt.Errorf("unmarshal action plan: %w", err)
	}

	scopeKey, err := parseActionPlanScope(scope)
	if err != nil {
		return nil, nil, nil, err
	}

	stagesRaw, ok := plan["stages"]
	if !ok {
		return nil, nil, nil, ErrActionPlanIndexOutOfRange
	}
	stages, ok := stagesRaw.([]any)
	if !ok {
		return nil, nil, nil, ErrActionPlanIndexOutOfRange
	}
	if stage < 0 || stage >= len(stages) {
		return nil, nil, nil, ErrActionPlanIndexOutOfRange
	}
	stageObj, ok := stages[stage].(map[string]any)
	if !ok {
		return nil, nil, nil, ErrActionPlanIndexOutOfRange
	}
	itemsRaw, ok := stageObj[scopeKey]
	if !ok {
		return nil, nil, nil, ErrActionPlanIndexOutOfRange
	}
	items, ok := itemsRaw.([]any)
	if !ok {
		return nil, nil, nil, ErrActionPlanIndexOutOfRange
	}

	perm, err := moveIndex(len(items), from, to)
	if err != nil {
		return nil, nil, nil, err
	}

	if err := movePlanItems(plan, scope, stage, from, to); err != nil {
		return nil, nil, nil, err
	}

	planData, err := json.Marshal(plan)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("marshal action plan: %w", err)
	}

	// The write renumbers the moved items, so the stored document — not the one
	// assembled above — is what the caller returns to the client.
	planData, err = s.WriteActionPlan(dialogID, planData)
	if err != nil {
		return nil, nil, nil, err
	}

	checked, err := s.ReadActionPlanChecks(dialogID)
	if err != nil {
		return nil, nil, nil, err
	}
	checked = remapCheckedKeys(checked, stage, scope, perm)

	if err := s.WriteActionPlanChecks(dialogID, checked); err != nil {
		return nil, nil, nil, err
	}

	comments, err := s.ReadActionPlanComments(dialogID)
	if err != nil {
		return nil, nil, nil, err
	}
	comments = remapActionPlanKeys(comments, stage, scope, perm)

	if err := s.WriteActionPlanComments(dialogID, comments); err != nil {
		return nil, nil, nil, err
	}

	runs, err := s.ReadActionPlanRuns(dialogID)
	if err != nil {
		return nil, nil, nil, err
	}
	runs = remapActionPlanKeys(runs, stage, scope, perm)

	if err := s.WriteActionPlanRuns(dialogID, runs); err != nil {
		return nil, nil, nil, err
	}

	execRuns, err := s.ReadActionPlanExecRuns(dialogID)
	if err != nil {
		return nil, nil, nil, err
	}
	execRuns = remapActionPlanKeys(execRuns, stage, scope, perm)

	if err := s.WriteActionPlanExecRuns(dialogID, execRuns); err != nil {
		return nil, nil, nil, err
	}

	notes, err := s.readActionPlanNotes(dialogID)
	if err != nil {
		return nil, nil, nil, err
	}
	notes = remapActionPlanKeys(notes, stage, scope, perm)

	if err := s.writeActionPlanNotes(dialogID, notes); err != nil {
		return nil, nil, nil, err
	}

	return json.RawMessage(planData), checked, comments, nil
}
