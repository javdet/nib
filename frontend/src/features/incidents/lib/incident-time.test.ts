import { describe, expect, it } from 'vitest'
import {
	formatIncidentTime,
	fromLocalInputValue,
	incidentDuration,
	toLocalInputValue,
} from './incident-time'

describe('incident time helpers', () => {
	it('round-trips through a datetime-local value', () => {
		const stored = '2026-10-07T14:05:00Z'
		expect(fromLocalInputValue(toLocalInputValue(stored))).toBe(stored)
	})

	it('treats empty and unparseable values as empty', () => {
		expect(toLocalInputValue('')).toBe('')
		expect(toLocalInputValue('around lunch')).toBe('')
		expect(fromLocalInputValue('')).toBe('')
	})

	it('shows an unparseable time as written', () => {
		expect(formatIncidentTime('around lunch')).toBe('around lunch')
	})

	it('formats durations', () => {
		expect(incidentDuration('2026-10-07T14:00:00Z', '2026-10-07T14:45:00Z')).toBe('45 min')
		expect(incidentDuration('2026-10-07T14:00:00Z', '2026-10-07T16:00:00Z')).toBe('2 h')
		expect(incidentDuration('2026-10-01T00:00:00Z', '2026-10-04T05:00:00Z')).toBe('3 d 5 h')
		expect(incidentDuration('', '2026-10-07T14:00:00Z')).toBeNull()
		expect(incidentDuration('2026-10-07T15:00:00Z', '2026-10-07T14:00:00Z')).toBeNull()
	})
})
