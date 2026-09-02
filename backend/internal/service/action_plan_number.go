package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// The operator-facing numbering of a plan. Every label is derived from the
// item's position, the same way actionPlanItemKey derives its storage key, so
// the two always agree and a number sent by the model is never trusted.
//
//	stage           1
//	step of stage 1 1.1, 1.2
//	check of stage 1 1.C1, 1.C2
//	rollback entry  R1, R2 (numbered across the plan, the list is plan-level)

// actionPlanStageNumber returns the 1-based label of a stage.
func actionPlanStageNumber(stage int) int {
	return stage + 1
}

// actionPlanItemNumber returns the label of a step or check inside a stage.
func actionPlanItemNumber(stage int, scope ActionPlanScope, index int) string {
	switch scope {
	case ActionPlanScopeSteps:
		return fmt.Sprintf("%d.%d", actionPlanStageNumber(stage), index+1)
	case ActionPlanScopeChecks:
		return fmt.Sprintf("%d.C%d", actionPlanStageNumber(stage), index+1)
	default:
		return ""
	}
}

// actionPlanRollbackNumber returns the label of a rollback entry. Rollback is a
// plan-level list, so it carries no stage prefix.
func actionPlanRollbackNumber(index int) string {
	return fmt.Sprintf("R%d", index+1)
}

// renumberActionPlan stamps the derived numbers onto a plan document in place.
// Values supplied by the model are overruled rather than trusted: position is
// the only source. Anything that is not a plan object — a stage that decoded to
// something other than an object, an unrecognised key — is stepped over so a
// hand-edited or future-shaped document survives a write intact.
func renumberActionPlan(plan map[string]any) {
	for i, rawStage := range asArray(plan["stages"]) {
		stage, ok := rawStage.(map[string]any)
		if !ok {
			continue
		}
		stage["number"] = actionPlanStageNumber(i)
		renumberActionPlanItems(stage["steps"], i, ActionPlanScopeSteps)
		renumberActionPlanItems(stage["checks"], i, ActionPlanScopeChecks)
	}

	for i, rawStep := range asArray(plan["rollback"]) {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		step["number"] = actionPlanRollbackNumber(i)
	}
}

func renumberActionPlanItems(raw any, stage int, scope ActionPlanScope) {
	for i, rawItem := range asArray(raw) {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		item["number"] = actionPlanItemNumber(stage, scope, i)
	}
}

func asArray(raw any) []any {
	items, _ := raw.([]any)
	return items
}

// The operator-facing labels, matched lowercased: "1.2" for a step, "1.c1" for a
// check, "r1" for a rollback entry. A label may be zero-padded or carry a leading
// "#", because these come back from a model quoting what it saw in the web
// interface rather than from code.
var (
	actionPlanStepNumberPattern     = regexp.MustCompile(`^(\d+)\.(\d+)$`)
	actionPlanCheckNumberPattern    = regexp.MustCompile(`^(\d+)\.c(\d+)$`)
	actionPlanRollbackNumberPattern = regexp.MustCompile(`^r(\d+)$`)
)

// parseActionPlanNumber turns an operator-facing label into the row key the plan
// side files are indexed by. It is the inverse of actionPlanItemNumber and
// actionPlanRollbackNumber, and it also passes a raw row key straight through so
// a model that quotes one back is not punished for it.
//
// The inverse is only partial: "9.4" parses to a well-formed key for a row that
// may not exist. Callers must follow this with a lookup — findActionPlanStep
// already reports ErrActionNotFound — and never treat a parse as proof.
func parseActionPlanNumber(number string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(number))
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}

	// A row key is already what the caller wants; accept it unchanged.
	if actionPlanKeyPattern.MatchString(s) || actionPlanRollbackKeyPattern.MatchString(s) {
		return s, true
	}

	if m := actionPlanStepNumberPattern.FindStringSubmatch(s); m != nil {
		stage, index, ok := parseActionPlanLabelPair(m[1], m[2])
		if !ok {
			return "", false
		}
		return actionPlanItemKey(stage, ActionPlanScopeSteps, index), true
	}
	if m := actionPlanCheckNumberPattern.FindStringSubmatch(s); m != nil {
		stage, index, ok := parseActionPlanLabelPair(m[1], m[2])
		if !ok {
			return "", false
		}
		return actionPlanItemKey(stage, ActionPlanScopeChecks, index), true
	}
	if m := actionPlanRollbackNumberPattern.FindStringSubmatch(s); m != nil {
		index, err := strconv.Atoi(m[1])
		if err != nil || index < 1 {
			return "", false
		}
		return fmt.Sprintf("rollback.%d", index-1), true
	}
	return "", false
}

// parseActionPlanLabelPair converts a "stage.item" label pair to zero-based
// indexes. Labels are one-based on both halves, so a zero in either is not a
// label this plan could ever have produced.
func parseActionPlanLabelPair(rawStage, rawIndex string) (stage, index int, ok bool) {
	stage, err := strconv.Atoi(rawStage)
	if err != nil || stage < 1 {
		return 0, 0, false
	}
	index, err = strconv.Atoi(rawIndex)
	if err != nil || index < 1 {
		return 0, 0, false
	}
	return stage - 1, index - 1, true
}

// actionPlanNumberForKey renders the operator-facing label of a row key. It is
// the pair of parseActionPlanNumber and, like every number here, is derived from
// position rather than read back from the document.
func actionPlanNumberForKey(key string) string {
	if m := actionPlanKeyPattern.FindStringSubmatch(key); m != nil {
		stage, err := strconv.Atoi(m[1])
		if err != nil {
			return ""
		}
		index, err := strconv.Atoi(m[3])
		if err != nil {
			return ""
		}
		switch m[2] {
		case "step":
			return actionPlanItemNumber(stage, ActionPlanScopeSteps, index)
		case "check":
			return actionPlanItemNumber(stage, ActionPlanScopeChecks, index)
		}
		return ""
	}
	if strings.HasPrefix(key, "rollback.") {
		index, err := strconv.Atoi(strings.TrimPrefix(key, "rollback."))
		if err != nil || index < 0 {
			return ""
		}
		return actionPlanRollbackNumber(index)
	}
	return ""
}
