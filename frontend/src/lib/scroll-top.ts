export interface ScrollMetrics {
	scrollTop: number
	scrollHeight: number
	clientHeight: number
}

// The button earns its place only once the way back up is longer than a
// screen; a page that merely nudges past the fold does not need it.
export function showScrollTop({ scrollTop, clientHeight }: ScrollMetrics) {
	return scrollTop > clientHeight * 0.75
}

// Fraction of the scrollable distance already travelled, clamped to [0, 1]
// because overscroll bounce reports values outside it.
export function scrollProgress({
	scrollTop,
	scrollHeight,
	clientHeight,
}: ScrollMetrics) {
	const range = scrollHeight - clientHeight
	if (range <= 0) return 0
	return Math.min(1, Math.max(0, scrollTop / range))
}
