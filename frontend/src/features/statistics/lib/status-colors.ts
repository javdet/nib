import type { PlanStatus } from '@/features/dialogs/api/dialogs'

/**
 * STATUS_CHART_COLORS is a second, colour-valued copy of the status palette.
 *
 * STATUS_STYLES in features/workplace holds Tailwind class strings, which
 * recharts cannot consume as a fill, so the colours are repeated here as CSS
 * values. Labels are NOT repeated -- those still come from STATUS_STYLES so the
 * wording cannot drift between a badge and a chart legend.
 */
export const STATUS_CHART_COLORS: Record<PlanStatus, string> = {
	draft: 'var(--color-zinc-500)',
	scheduled: 'var(--color-amber-500)',
	in_progress: 'var(--color-blue-500)',
	done: 'var(--color-lime-500)',
	reopened: 'var(--color-orange-500)',
	rolled_back: 'var(--color-red-500)',
	cancelled: 'var(--color-rose-500)',
}

/** CHART_SERIES are the theme's own categorical tokens, defined for both light
 *  and dark in index.css and previously unused. */
export const CHART_SERIES = [
	'var(--color-chart-1)',
	'var(--color-chart-2)',
	'var(--color-chart-3)',
	'var(--color-chart-4)',
	'var(--color-chart-5)',
]
