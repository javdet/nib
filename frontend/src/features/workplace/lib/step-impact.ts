import type { ActionStep } from '@/features/dialogs/api/dialogs'

/** What carrying an action out costs the people using the system. */
export type ImpactKind = 'downtime' | 'degraded'

export interface StepImpact {
	kind: ImpactKind
	/** The planner's sentence, shown as the label's tooltip. */
	text: string
}

/**
 * stepImpact returns the impact a step declares, or null when it declares none.
 *
 * downtime supersedes degraded, the same way the plan prompt orders them: a
 * model that set both against the rule gets the more severe label, never two.
 * A whitespace-only value counts as absent -- a label with nothing behind it
 * tells the operator less than no label at all.
 */
export function stepImpact(step: ActionStep): StepImpact | null {
	const downtime = step.downtime?.trim()
	if (downtime) return { kind: 'downtime', text: downtime }

	const degraded = step.degraded?.trim()
	if (degraded) return { kind: 'degraded', text: degraded }

	return null
}

/** The most severe impact across a list of steps, for a stage heading. */
export function strongestImpact(steps: ActionStep[]): ImpactKind | null {
	let degraded = false
	for (const step of steps) {
		const impact = stepImpact(step)
		if (impact?.kind === 'downtime') return 'downtime'
		if (impact) degraded = true
	}
	return degraded ? 'degraded' : null
}

/**
 * impactedIndexes lists the positions of the steps carrying kind, so a stage
 * rollup can name the rows instead of leaving the operator to hunt for them.
 */
export function impactedIndexes(
	steps: ActionStep[],
	kind: ImpactKind,
): number[] {
	const out: number[] = []
	steps.forEach((step, index) => {
		if (stepImpact(step)?.kind === kind) out.push(index)
	})
	return out
}
