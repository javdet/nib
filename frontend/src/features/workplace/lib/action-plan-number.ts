// The operator-facing numbering of a plan, derived from each item's position:
//
//   stage             1
//   step of stage 1   1.1, 1.2
//   check of stage 1  1.C1, 1.C2
//   rollback entry    R1, R2 (numbered across the plan, the list is plan-level)
//
// The backend stamps the same labels into the stored plan on every write, but
// the UI derives them rather than reading them back: plan files written before
// numbering existed carry no label, and deriving is the only way the gutter and
// the Markdown export can never disagree with each other or with such a file.

export type ActionPlanItemKind = 'step' | 'check' | 'rollback'

export function actionPlanStageNumber(stageIdx: number): string {
	return String(stageIdx + 1)
}

export function actionPlanItemNumber(
	kind: ActionPlanItemKind,
	stageIdx: number,
	itemIdx: number,
): string {
	switch (kind) {
		case 'step':
			return `${actionPlanStageNumber(stageIdx)}.${itemIdx + 1}`
		case 'check':
			return `${actionPlanStageNumber(stageIdx)}.C${itemIdx + 1}`
		case 'rollback':
			return `R${itemIdx + 1}`
	}
}

// actionPlanNumberForKey labels an item addressed by its storage key, for the
// callers that already hold a key rather than a pair of indices.
export function actionPlanNumberForKey(key: string): string {
	const rollbackMatch = /^rollback\.(\d+)$/.exec(key)
	if (rollbackMatch) {
		return actionPlanItemNumber('rollback', 0, Number(rollbackMatch[1]))
	}

	const itemMatch = /^s(\d+)\.(step|check)(\d+)$/.exec(key)
	if (itemMatch) {
		const kind = itemMatch[2] === 'check' ? 'check' : 'step'
		return actionPlanItemNumber(kind, Number(itemMatch[1]), Number(itemMatch[3]))
	}

	return ''
}
