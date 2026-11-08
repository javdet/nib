import { describe, expect, it } from 'vitest'
import type { ActionExecRun } from '@/features/dialogs/api/dialogs'
import {
	canContinueRun,
	continueActionMessage,
	executeActionMessage,
} from './execute-action-message'

describe('executeActionMessage', () => {
	it('names the item by its operator-facing number', () => {
		expect(executeActionMessage('1.1')).toBe('Execute plan item 1.1')
		expect(executeActionMessage('R2')).toBe('Execute plan item R2')
	})
})

describe('continueActionMessage', () => {
	it('asks to carry on rather than restart', () => {
		expect(continueActionMessage('2.3')).toBe(
			'Continue plan item 2.3 from where its last run stopped',
		)
	})
})

describe('canContinueRun', () => {
	const run = (over: Partial<ActionExecRun>): ActionExecRun => ({
		status: 'failed',
		startedAt: 1,
		attempt: 1,
		...over,
	})

	it('offers a sub-agent run that ended without finishing', () => {
		expect(canContinueRun(run({ status: 'failed' }))).toBe(true)
		expect(canContinueRun(run({ status: 'cancelled' }))).toBe(true)
		expect(canContinueRun(run({ status: 'blocked', kind: 'subagent' }))).toBe(true)
	})

	it('withholds a finished, running, container or missing run', () => {
		expect(canContinueRun(run({ status: 'done' }))).toBe(false)
		expect(canContinueRun(run({ status: 'running' }))).toBe(false)
		expect(canContinueRun(run({ status: 'failed', kind: 'container' }))).toBe(false)
		expect(canContinueRun(undefined)).toBe(false)
	})
})
