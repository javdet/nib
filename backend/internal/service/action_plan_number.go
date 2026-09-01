package service

import "fmt"

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
