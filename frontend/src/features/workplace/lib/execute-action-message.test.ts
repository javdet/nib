import { describe, expect, it } from 'vitest'
import { executeActionMessage } from './execute-action-message'

describe('executeActionMessage', () => {
	it('names the item by its operator-facing number', () => {
		expect(executeActionMessage('1.1')).toBe('Execute plan item 1.1')
		expect(executeActionMessage('R2')).toBe('Execute plan item R2')
	})
})
