import { describe, expect, it } from 'vitest'
import {
	formatBucketLabel,
	formatCost,
	formatDuration,
	formatStatusLabel,
	formatTokens,
} from './format'

describe('formatCost', () => {
	// The whole point of the nullable cost column: most providers never report
	// one, and "$0.00" would claim the work was free rather than unpriced.
	it('renders an unreported cost as a dash, not as zero', () => {
		expect(formatCost(null)).toBe('—')
		expect(formatCost(undefined)).toBe('—')
	})

	it('renders a genuine zero as zero', () => {
		expect(formatCost(0)).toBe('$0.00')
	})

	it('keeps sub-cent costs visible instead of rounding them away', () => {
		expect(formatCost(0.0000002)).toBe('$0.000000')
		expect(formatCost(0.0012)).toBe('$0.001200')
	})

	it('formats ordinary amounts to two places', () => {
		expect(formatCost(12.3456)).toBe('$12.35')
	})
})

describe('formatTokens', () => {
	it('abbreviates by magnitude', () => {
		expect(formatTokens(999)).toBe('999')
		expect(formatTokens(1500)).toBe('1.5K')
		expect(formatTokens(2_500_000)).toBe('2.50M')
		expect(formatTokens(3_000_000_000)).toBe('3.00B')
	})

	it('handles zero and non-finite input', () => {
		expect(formatTokens(0)).toBe('0')
		expect(formatTokens(Number.NaN)).toBe('—')
	})
})

describe('formatDuration', () => {
	it('scales the unit to the magnitude', () => {
		expect(formatDuration(450)).toBe('450ms')
		expect(formatDuration(1500)).toBe('1.5s')
		expect(formatDuration(90_000)).toBe('1m 30s')
	})

	it('renders no duration as a dash', () => {
		expect(formatDuration(0)).toBe('—')
	})
})

describe('formatBucketLabel', () => {
	it('falls back to the raw value for an unparseable timestamp', () => {
		expect(formatBucketLabel('not-a-date', 'day')).toBe('not-a-date')
	})

	it('labels a month bucket by month', () => {
		expect(formatBucketLabel('2026-03-01T00:00:00Z', 'month')).toMatch(/\w{3} 26/)
	})
})

describe('formatStatusLabel', () => {
	it('turns a snake_case status into words', () => {
		expect(formatStatusLabel('in_progress')).toBe('In Progress')
		expect(formatStatusLabel('rolled_back')).toBe('Rolled Back')
	})
})
