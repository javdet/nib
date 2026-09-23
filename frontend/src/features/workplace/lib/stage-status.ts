import type {
	ActionExecRuns,
	ActionPlan,
	FanoutRun,
	FanoutStage,
	StageRun,
	StageRunScope,
} from '@/features/dialogs/api/dialogs'
import { actionPlanNumberForKey } from './action-plan-number'
import { fanoutStageLabel } from './fanout-stage-status'

/** A stage's items in the order "run all" walks them: steps, then checks. */
export function stageItemKeys(plan: ActionPlan, stageIdx: number): string[] {
	const stage = plan.stages[stageIdx]
	if (!stage) return []
	return [
		...stage.steps.map((_, i) => `s${stageIdx}.step${i}`),
		...stage.checks.map((_, i) => `s${stageIdx}.check${i}`),
	]
}

export function rollbackItemKeys(plan: ActionPlan): string[] {
	return plan.rollback.map((_, i) => `rollback.${i}`)
}

/**
 * A stage is complete once every one of its items is ticked. The checkbox is
 * the operator's call, not the run's: a sub-agent finishing its turn does not
 * say the action worked. A stage with nothing in it is not complete.
 */
export function isItemsComplete(keys: readonly string[], checked: Set<string>): boolean {
	return keys.length > 0 && keys.every((k) => checked.has(k))
}

/** Mirrors the backend's normalizeStageTitle, which matches DAG stages. */
export function normalizeStageTitle(title: string): string {
	return title.trim().split(/\s+/).join(' ').toLowerCase()
}

export function findFanoutStage(
	run: FanoutRun | null | undefined,
	title: string,
): FanoutStage | undefined {
	if (!run) return undefined
	const want = normalizeStageTitle(title)
	return run.stages.find(
		(st) => st.kind !== 'rollback' && normalizeStageTitle(st.title) === want,
	)
}

export function findFanoutRollback(
	run: FanoutRun | null | undefined,
): FanoutStage | undefined {
	return run?.stages.find((st) => st.kind === 'rollback')
}

export interface PlanStageEntry {
	/** Position in the stored plan, or null for a stage still being planned. */
	index: number | null
	title: string
	fanout?: FanoutStage
}

/**
 * The stages to draw: the plan's own, in order, then any the planning run
 * knows about but has not written yet. Without the second half a stage being
 * planned for the first time would be invisible until it landed.
 */
export function mergePlanStages(
	plan: ActionPlan | null,
	run: FanoutRun | null | undefined,
): PlanStageEntry[] {
	const entries: PlanStageEntry[] = (plan?.stages ?? []).map((stage, index) => ({
		index,
		title: stage.title,
		fanout: findFanoutStage(run, stage.title),
	}))

	const known = new Set(entries.map((e) => normalizeStageTitle(e.title)))
	for (const st of run?.stages ?? []) {
		if (st.kind === 'rollback') continue
		const key = normalizeStageTitle(st.title)
		if (known.has(key)) continue
		known.add(key)
		entries.push({ index: null, title: st.title, fanout: st })
	}
	return entries
}

/** Whether run is the active "run all" of this stage (or of the rollback). */
export function isStageRunFor(
	run: StageRun | null | undefined,
	scope: StageRunScope,
	stageIdx: number,
): boolean {
	if (!run || run.status !== 'running' || run.scope !== scope) return false
	return scope === 'rollback' || run.stage === stageIdx
}

export type StageStatusKind =
	| 'running'
	| 'queued'
	| 'planning'
	| 'waiting'
	| 'complete'
	| 'planning_failed'
	| 'attention'
	| 'empty'
	| 'progress'

export interface StageStatus {
	kind: StageStatusKind
	done: number
	total: number
	tooltip: string
}

export interface StageStatusInput {
	keys: readonly string[]
	checked: Set<string>
	execRuns: ActionExecRuns
	fanout?: FanoutStage
	fanoutRunning: boolean
	/** This stage's "run all" is going. */
	runActive: boolean
}

const CARRIED_NOTE = 'Kept from an earlier round; this run does not replan it'

/**
 * The one status a stage header shows. Execution in flight wins, because it is
 * what the operator is watching; planning comes next, because until a stage is
 * planned there is nothing to execute; then how far the operator has got.
 */
export function deriveStageStatus({
	keys,
	checked,
	execRuns,
	fanout,
	fanoutRunning,
	runActive,
}: StageStatusInput): StageStatus {
	const total = keys.length
	const done = keys.filter((k) => checked.has(k)).length
	const status = (kind: StageStatusKind, tooltip: string): StageStatus => ({
		kind,
		done,
		total,
		tooltip,
	})

	const running = keys.find((k) => execRuns[k]?.status === 'running')
	if (runActive || running) {
		const at = running ? ` — ${actionPlanNumberForKey(running) || running}` : ''
		return status(
			'running',
			runActive ? `Running all items${at}` : `Running${at}`,
		)
	}

	if (fanout && !(fanout.carried && fanoutRunning)) {
		if (fanout.status === 'awaiting_input') {
			return status('waiting', 'Planning is waiting for your answer in the chat')
		}
		if (fanoutRunning && fanout.status === 'running') {
			return status('planning', 'Planning…')
		}
		if (fanoutRunning && fanout.status === 'pending') {
			return status('queued', `Planning: ${fanoutStageLabel(fanout.status)}`)
		}
	}

	const carried = fanout?.carried && fanoutRunning ? `. ${CARRIED_NOTE}` : ''

	if (isItemsComplete(keys, checked)) {
		return status('complete', `All ${total} items ticked${carried}`)
	}

	if (fanout?.status === 'failed' && !fanout.carried) {
		return status(
			'planning_failed',
			fanout.error ? `Planning failed: ${fanout.error}` : 'Planning failed',
		)
	}

	const troubled = keys.filter(
		(k) =>
			!checked.has(k) &&
			(execRuns[k]?.status === 'failed' || execRuns[k]?.status === 'blocked'),
	)
	if (troubled.length > 0) {
		const numbers = troubled.map((k) => actionPlanNumberForKey(k) || k)
		return status('attention', `Needs attention: ${numbers.join(', ')}${carried}`)
	}

	if (total === 0) {
		return status('empty', `No items yet${carried}`)
	}
	return status('progress', `${done} of ${total} items ticked${carried}`)
}

export interface RunAllInput {
	keys: readonly string[]
	checked: Set<string>
	execRuns: ActionExecRuns
	fanoutRunning: boolean
	/** Another stage's "run all" is going. */
	otherRun: StageRun | null
	/** An unticked code action would be run while the executor is disabled. */
	codeBlocked: boolean
}

/**
 * Why "run all" cannot start for this stage, or null when it can. The server
 * refuses the same things; saying so up front keeps the operator from pressing
 * a button that answers with an error.
 */
export function runAllDisabledReason({
	keys,
	checked,
	execRuns,
	fanoutRunning,
	otherRun,
	codeBlocked,
}: RunAllInput): string | null {
	if (keys.length === 0) return 'Nothing to run yet'
	if (keys.every((k) => checked.has(k))) return 'Every item is already ticked'
	if (fanoutRunning) return 'Wait for planning to finish'
	if (otherRun) {
		const what =
			otherRun.scope === 'rollback' ? 'The rollback' : `Stage ${otherRun.stage + 1}`
		return `${what} is already running all its items`
	}
	if (Object.values(execRuns).some((r) => r.status === 'running')) {
		return 'An action is running — wait for it to finish'
	}
	if (codeBlocked) {
		return 'A code action here cannot run while the executor is disabled'
	}
	return null
}
