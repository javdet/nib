import { describe, expect, it } from 'vitest'
import {
	executeActionMessage,
	restartActionMessage,
} from './execute-action-message'

describe('executeActionMessage', () => {
	it('names the item by its operator-facing number', () => {
		expect(executeActionMessage('1.1')).toBe('Execute plan item 1.1')
		expect(executeActionMessage('R2')).toBe('Execute plan item R2')
	})
})

describe('restartActionMessage', () => {
	it('asks for a fresh sub-agent so the agent passes rerun', () => {
		expect(restartActionMessage('1.1')).toBe(
			'Restart plan item 1.1 with a new sub-agent',
		)
	})

	it('is distinguishable from a plain execute', () => {
		expect(restartActionMessage('1.1')).not.toBe(executeActionMessage('1.1'))
	})
})
