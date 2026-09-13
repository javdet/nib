import { describe, expect, it } from 'vitest'
import { DEFAULT_PRESET, RANGE_PRESETS, presetToParams } from './ranges'

const NOW = new Date('2026-09-13T12:00:00.000Z')

describe('presetToParams', () => {
	it('resolves a preset to an inclusive window ending now', () => {
		const params = presetToParams('7d', NOW)
		expect(params.to).toBe('2026-09-13T12:00:00.000Z')
		expect(params.from).toBe('2026-09-06T12:00:00.000Z')
		expect(params.bucket).toBe('day')
	})

	it('falls back to 30 days for an unknown preset', () => {
		// @ts-expect-error deliberately out of the union
		const params = presetToParams('nonsense', NOW)
		expect(params.bucket).toBe('day')
		expect(params.from).toBe('2026-08-14T12:00:00.000Z')
	})

	// Every preset has to stay inside the backend's 400-bucket cap or the page
	// 400s on a range the UI itself offers.
	it('keeps every preset inside the backend bucket cap', () => {
		const bucketHours: Record<string, number> = {
			hour: 1,
			day: 24,
			week: 24 * 7,
			month: 24 * 30,
		}
		for (const preset of RANGE_PRESETS) {
			const hours = bucketHours[preset.bucket] ?? 1
			const buckets = preset.hours / hours
			expect(buckets).toBeLessThanOrEqual(400)
		}
	})

	it('offers the default preset', () => {
		expect(RANGE_PRESETS.some((p) => p.id === DEFAULT_PRESET)).toBe(true)
	})
})
