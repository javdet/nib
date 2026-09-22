import { describe, expect, it } from 'vitest'
import {
	fanoutStageClass,
	fanoutStageLabel,
} from '@/features/workplace/lib/fanout-stage-status'

describe('fanoutStageLabel', () => {
	it('asks for the operator rather than naming a machine state', () => {
		expect(fanoutStageLabel('awaiting_input')).toBe('waiting for you')
	})

	it('leaves every other status as it is stored', () => {
		expect(fanoutStageLabel('pending')).toBe('pending')
		expect(fanoutStageLabel('running')).toBe('running')
		expect(fanoutStageLabel('done')).toBe('done')
		expect(fanoutStageLabel('failed')).toBe('failed')
	})
})

describe('fanoutStageClass', () => {
	it('separates a stage waiting on the operator from one that stalled', () => {
		expect(fanoutStageClass('awaiting_input')).not.toBe(
			fanoutStageClass('failed'),
		)
		expect(fanoutStageClass('awaiting_input')).not.toBe('')
	})

	it('leaves the statuses that carry no verdict unstyled', () => {
		expect(fanoutStageClass('pending')).toBe('')
		expect(fanoutStageClass('running')).toBe('')
	})
})
