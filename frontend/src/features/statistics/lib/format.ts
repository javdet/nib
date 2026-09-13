/** formatTokens abbreviates large counts; charts and tiles must line up. */
export function formatTokens(value: number): string {
	if (!Number.isFinite(value)) return '—'
	if (Math.abs(value) >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
	if (Math.abs(value) >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
	if (Math.abs(value) >= 1_000) return `${(value / 1_000).toFixed(1)}K`
	return value.toLocaleString('en-US')
}

/**
 * formatCost renders an unreported cost as an em dash, never as $0.00.
 *
 * The backend sends null when no call in the range carried a price -- most
 * providers, including the OpenAI platform, never report one -- and showing
 * $0.00 there would claim the work was free rather than unpriced.
 */
export function formatCost(value: number | null | undefined): string {
	if (value == null || !Number.isFinite(value)) return '—'
	if (value === 0) return '$0.00'
	if (value < 0.01) return `$${value.toFixed(6)}`
	return `$${value.toFixed(2)}`
}

export function formatDuration(ms: number): string {
	if (!Number.isFinite(ms) || ms <= 0) return '—'
	if (ms < 1000) return `${Math.round(ms)}ms`
	if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
	return `${Math.floor(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`
}

/** formatBucketLabel keeps an axis readable at the bucket's own resolution. */
export function formatBucketLabel(ts: string, bucket: string): string {
	const d = new Date(ts)
	if (Number.isNaN(d.getTime())) return ts
	if (bucket === 'hour') {
		return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' })
	}
	if (bucket === 'month') {
		return d.toLocaleDateString('en-US', { month: 'short', year: '2-digit' })
	}
	return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

/** formatStatusLabel turns a snake_case status into something readable. */
export function formatStatusLabel(status: string): string {
	return status.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
}
