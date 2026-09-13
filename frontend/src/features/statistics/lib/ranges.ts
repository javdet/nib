import type { StatsBucket, StatisticsParams } from '../api/statistics'

export type RangePresetId = '24h' | '7d' | '30d' | '90d' | '12mo'

interface RangePreset {
	id: RangePresetId
	label: string
	/** Hours back from now. */
	hours: number
	/** Chosen so every preset stays well inside the backend's bucket cap. */
	bucket: StatsBucket
}

export const RANGE_PRESETS: readonly [RangePreset, ...RangePreset[]] = [
	{ id: '24h', label: 'Last 24 hours', hours: 24, bucket: 'hour' },
	{ id: '7d', label: 'Last 7 days', hours: 24 * 7, bucket: 'day' },
	{ id: '30d', label: 'Last 30 days', hours: 24 * 30, bucket: 'day' },
	{ id: '90d', label: 'Last 90 days', hours: 24 * 90, bucket: 'week' },
	{ id: '12mo', label: 'Last 12 months', hours: 24 * 365, bucket: 'month' },
]

export const DEFAULT_PRESET: RangePresetId = '30d'

/** presetToParams resolves a preset against a fixed "now" so it is testable. */
export function presetToParams(
	id: RangePresetId,
	now: Date = new Date(),
): StatisticsParams {
	const preset =
		RANGE_PRESETS.find((p) => p.id === id) ??
		RANGE_PRESETS.find((p) => p.id === DEFAULT_PRESET) ??
		RANGE_PRESETS[0]
	const to = now
	const from = new Date(now.getTime() - preset.hours * 60 * 60 * 1000)
	return {
		from: from.toISOString(),
		to: to.toISOString(),
		bucket: preset.bucket,
	}
}
