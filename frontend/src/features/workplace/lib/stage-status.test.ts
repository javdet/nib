import { describe, expect, it } from 'vitest'
import type {
	ActionPlan,
	FanoutRun,
	FanoutStage,
	StageRun,
} from '@/features/dialogs/api/dialogs'
import {
	deriveStageStatus,
	findFanoutRollback,
	findFanoutStage,
	isItemsComplete,
	isStageRunFor,
	mergePlanStages,
	rollbackItemKeys,
	runAllDisabledReason,
	stageItemKeys,
	type StageStatusInput,
} from './stage-status'

const plan: ActionPlan = {
	stages: [
		{
			number: 1,
			title: 'Deploy',
			description: '',
			steps: [
				{ type: 'shell', action: 'apply' },
				{ type: 'code', action: 'bump' },
			],
			checks: [{ check: 'pods ready', expectation: 'Running' }],
		},
		{
			number: 2,
			title: 'Verify',
			description: '',
			steps: [],
			checks: [],
		},
	],
	rollback: [{ type: 'shell', action: 'undo' }],
}

function fanout(stages: FanoutStage[], status: FanoutRun['status'] = 'done'): FanoutRun {
	return { runId: 'r', status, startedAt: 0, stages }
}

function stageRun(over: Partial<StageRun> = {}): StageRun {
	return {
		runId: 'x',
		scope: 'stage',
		stage: 0,
		title: 'Deploy',
		status: 'running',
		startedAt: 0,
		...over,
	}
}

describe('stageItemKeys', () => {
	it('lists steps before checks', () => {
		expect(stageItemKeys(plan, 0)).toEqual(['s0.step0', 's0.step1', 's0.check0'])
	})

	it('is empty for a stage with nothing in it, or none at all', () => {
		expect(stageItemKeys(plan, 1)).toEqual([])
		expect(stageItemKeys(plan, 9)).toEqual([])
	})

	it('keys the rollback on its own', () => {
		expect(rollbackItemKeys(plan)).toEqual(['rollback.0'])
	})
})

describe('isItemsComplete', () => {
	it('needs every item ticked', () => {
		const keys = stageItemKeys(plan, 0)
		expect(isItemsComplete(keys, new Set(['s0.step0', 's0.step1']))).toBe(false)
		expect(isItemsComplete(keys, new Set(keys))).toBe(true)
	})

	it('never calls an empty stage complete', () => {
		expect(isItemsComplete([], new Set())).toBe(false)
	})
})

describe('fan-out matching', () => {
	const run = fanout([
		{ title: '  deploy ', wave: 0, status: 'done' },
		{ title: 'Rollback', wave: 1, status: 'done', kind: 'rollback' },
	])

	it('matches a stage by title, ignoring case and spacing', () => {
		expect(findFanoutStage(run, 'Deploy')?.title).toBe('  deploy ')
	})

	it('never takes the rollback agent for a stage titled like it', () => {
		expect(findFanoutStage(run, 'Rollback')).toBeUndefined()
		expect(findFanoutRollback(run)?.kind).toBe('rollback')
	})
})

describe('mergePlanStages', () => {
	it('keeps plan order and appends stages still being planned', () => {
		const run = fanout(
			[
				{ title: 'Verify', wave: 0, status: 'done' },
				{ title: 'Migrate', wave: 1, status: 'running' },
				{ title: 'Rollback', wave: 2, status: 'pending', kind: 'rollback' },
			],
			'running',
		)
		const entries = mergePlanStages(plan, run)
		expect(entries.map((e) => [e.index, e.title])).toEqual([
			[0, 'Deploy'],
			[1, 'Verify'],
			[null, 'Migrate'],
		])
		expect(entries[1]?.fanout?.status).toBe('done')
		expect(entries[0]?.fanout).toBeUndefined()
	})

	it('draws the planning run alone before any plan exists', () => {
		const run = fanout([{ title: 'Deploy', wave: 0, status: 'pending' }], 'running')
		expect(mergePlanStages(null, run).map((e) => e.index)).toEqual([null])
	})
})

describe('isStageRunFor', () => {
	it('matches the running stage only', () => {
		expect(isStageRunFor(stageRun(), 'stage', 0)).toBe(true)
		expect(isStageRunFor(stageRun(), 'stage', 1)).toBe(false)
		expect(isStageRunFor(stageRun({ status: 'done' }), 'stage', 0)).toBe(false)
		expect(isStageRunFor(null, 'stage', 0)).toBe(false)
	})

	it('ignores the stage index for the rollback', () => {
		expect(isStageRunFor(stageRun({ scope: 'rollback' }), 'rollback', 3)).toBe(true)
		expect(isStageRunFor(stageRun({ scope: 'rollback' }), 'stage', 0)).toBe(false)
	})
})

describe('deriveStageStatus', () => {
	const keys = stageItemKeys(plan, 0)
	const base: StageStatusInput = {
		keys,
		checked: new Set(),
		execRuns: {},
		fanoutRunning: false,
		runActive: false,
	}
	const running = { status: 'running' as const, startedAt: 0, attempt: 1 }
	const failed = { status: 'failed' as const, startedAt: 0, attempt: 1, error: 'x' }

	it.each([
		['progress', base],
		['running', { ...base, runActive: true }],
		['running', { ...base, execRuns: { 's0.step1': running } }],
		[
			'running',
			// Execution in flight wins over everything else.
			{ ...base, runActive: true, checked: new Set(keys) },
		],
		[
			'planning',
			{
				...base,
				fanoutRunning: true,
				fanout: { title: 'Deploy', wave: 0, status: 'running' },
			},
		],
		[
			'queued',
			{
				...base,
				fanoutRunning: true,
				fanout: { title: 'Deploy', wave: 0, status: 'pending' },
			},
		],
		[
			'waiting',
			{
				...base,
				fanoutRunning: true,
				fanout: { title: 'Deploy', wave: 0, status: 'awaiting_input' },
			},
		],
		[
			// A pending row of a run that is over is not going to be planned.
			'progress',
			{ ...base, fanout: { title: 'Deploy', wave: 0, status: 'pending' } },
		],
		['complete', { ...base, checked: new Set(keys) }],
		[
			'planning_failed',
			{ ...base, fanout: { title: 'Deploy', wave: 0, status: 'failed', error: 'boom' } },
		],
		['attention', { ...base, execRuns: { 's0.step0': failed } }],
		[
			// A failure the operator has since ticked is settled.
			'progress',
			{ ...base, checked: new Set(['s0.step0']), execRuns: { 's0.step0': failed } },
		],
		['empty', { ...base, keys: [] }],
	] as [string, StageStatusInput][])('reports %s', (kind, input) => {
		expect(deriveStageStatus(input).kind).toBe(kind)
	})

	it('counts ticked items', () => {
		const status = deriveStageStatus({ ...base, checked: new Set(['s0.step0']) })
		expect([status.done, status.total]).toEqual([1, 3])
		expect(status.tooltip).toBe('1 of 3 items ticked')
	})

	it('names the running item and the failing ones', () => {
		expect(
			deriveStageStatus({ ...base, runActive: true, execRuns: { 's0.step1': running } })
				.tooltip,
		).toBe('Running all items — 1.2')
		expect(deriveStageStatus({ ...base, execRuns: { 's0.check0': failed } }).tooltip).toBe(
			'Needs attention: 1.C1',
		)
	})

	it('says when a carried stage is not being replanned', () => {
		const status = deriveStageStatus({
			...base,
			fanoutRunning: true,
			fanout: { title: 'Deploy', wave: 0, status: 'done', carried: true },
		})
		expect(status.kind).toBe('progress')
		expect(status.tooltip).toContain('does not replan it')
	})
})

describe('runAllDisabledReason', () => {
	const keys = stageItemKeys(plan, 0)
	const base = {
		keys,
		checked: new Set<string>(),
		execRuns: {},
		fanoutRunning: false,
		otherRun: null,
		codeBlocked: false,
	}

	it('allows a stage with work left and a free slot', () => {
		expect(runAllDisabledReason(base)).toBeNull()
	})

	it.each([
		['Nothing to run yet', { ...base, keys: [] }],
		['Every item is already ticked', { ...base, checked: new Set(keys) }],
		['Wait for planning to finish', { ...base, fanoutRunning: true }],
		['Stage 2 is already running all its items', { ...base, otherRun: stageRun({ stage: 1 }) }],
		[
			'The rollback is already running all its items',
			{ ...base, otherRun: stageRun({ scope: 'rollback' }) },
		],
		[
			'An action is running — wait for it to finish',
			{ ...base, execRuns: { 's1.step0': { status: 'running' as const, startedAt: 0, attempt: 1 } } },
		],
		[
			'A code action here cannot run while the executor is disabled',
			{ ...base, codeBlocked: true },
		],
	])('refuses: %s', (reason, input) => {
		expect(runAllDisabledReason(input)).toBe(reason)
	})
})
