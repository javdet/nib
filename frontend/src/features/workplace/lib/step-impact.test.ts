import { describe, expect, it } from 'vitest'
import type { ActionStep } from '@/features/dialogs/api/dialogs'
import { impactedIndexes, stepImpact, strongestImpact } from './step-impact'

function step(fields: Partial<ActionStep> = {}): ActionStep {
	return { type: 'shell', action: 'do a thing', ...fields }
}

describe('stepImpact', () => {
	it('reads the downtime sentence as the label text', () => {
		expect(stepImpact(step({ downtime: 'orders-db is down for ~10 min' }))).toEqual({
			kind: 'downtime',
			text: 'orders-db is down for ~10 min',
		})
	})

	it('reads the degraded sentence when there is no downtime', () => {
		expect(stepImpact(step({ degraded: 'reads are slower' }))).toEqual({
			kind: 'degraded',
			text: 'reads are slower',
		})
	})

	// The prompt forbids setting both. The UI does not trust that: one row gets
	// one label, and it is the severe one.
	it('lets downtime supersede degraded when a plan carries both', () => {
		expect(
			stepImpact(step({ downtime: 'api is down', degraded: 'api is slow' })),
		).toEqual({ kind: 'downtime', text: 'api is down' })
	})

	it('treats a missing or whitespace-only value as no impact', () => {
		expect(stepImpact(step())).toBeNull()
		expect(stepImpact(step({ downtime: '   ', degraded: '\n' }))).toBeNull()
	})

	it('trims the sentence it renders', () => {
		expect(stepImpact(step({ degraded: '  slower  ' }))?.text).toBe('slower')
	})
})

describe('strongestImpact', () => {
	it('returns downtime when any step causes one, wherever it sits', () => {
		expect(
			strongestImpact([
				step({ degraded: 'slower' }),
				step(),
				step({ downtime: 'down' }),
			]),
		).toBe('downtime')
	})

	it('returns degraded when that is the worst of them', () => {
		expect(strongestImpact([step(), step({ degraded: 'slower' })])).toBe(
			'degraded',
		)
	})

	it('returns null for a stage that costs nothing, and for an empty one', () => {
		expect(strongestImpact([step(), step()])).toBeNull()
		expect(strongestImpact([])).toBeNull()
	})
})

describe('impactedIndexes', () => {
	it('lists the positions carrying the kind asked for', () => {
		const steps = [
			step({ downtime: 'down' }),
			step({ degraded: 'slower' }),
			step(),
			step({ downtime: 'down again' }),
		]
		expect(impactedIndexes(steps, 'downtime')).toEqual([0, 3])
		expect(impactedIndexes(steps, 'degraded')).toEqual([1])
	})

	// A step carrying both is a downtime row and nothing else, so the degraded
	// rollup must not name it.
	it('counts a step with both as downtime only', () => {
		const steps = [step({ downtime: 'down', degraded: 'slower' })]
		expect(impactedIndexes(steps, 'downtime')).toEqual([0])
		expect(impactedIndexes(steps, 'degraded')).toEqual([])
	})
})
