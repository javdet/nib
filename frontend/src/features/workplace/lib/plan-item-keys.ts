import type { ActionPlan } from '@/features/dialogs/api/dialogs'

/**
 * Every checkable item key of a plan, in reading order.
 *
 * The shape mirrors the backend's `actionPlanItemKey`, and rollback items are
 * left out for the same reason `planProgress` leaves them out: they are the undo
 * path and are normally never executed, so counting them would make a plan
 * impossible to finish.
 */
export function buildPlanItemKeys(plan: ActionPlan | null): string[] {
	if (!plan) return []

	const keys: string[] = []
	plan.stages.forEach((stage, stageIdx) => {
		stage.steps.forEach((_, stepIdx) => {
			keys.push(`s${stageIdx}.step${stepIdx}`)
		})
		stage.checks.forEach((_, checkIdx) => {
			keys.push(`s${stageIdx}.check${checkIdx}`)
		})
	})
	return keys
}
