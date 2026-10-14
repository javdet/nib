import { describe, expect, it } from 'vitest'
import { scrollProgress, showScrollTop } from './scroll-top'

describe('showScrollTop', () => {
	const cases: { name: string; scrollTop: number; want: boolean }[] = [
		{ name: 'at the top', scrollTop: 0, want: false },
		{ name: 'just past the fold', scrollTop: 600, want: false },
		{ name: 'beyond three quarters of a screen', scrollTop: 601, want: true },
	]

	for (const tc of cases) {
		it(tc.name, () => {
			expect(
				showScrollTop({
					scrollTop: tc.scrollTop,
					scrollHeight: 5000,
					clientHeight: 800,
				}),
			).toBe(tc.want)
		})
	}
})

describe('scrollProgress', () => {
	const cases: {
		name: string
		scrollTop: number
		scrollHeight: number
		want: number
	}[] = [
		{ name: 'top', scrollTop: 0, scrollHeight: 2800, want: 0 },
		{ name: 'halfway', scrollTop: 1000, scrollHeight: 2800, want: 0.5 },
		{ name: 'bottom', scrollTop: 2000, scrollHeight: 2800, want: 1 },
		{ name: 'overscroll below', scrollTop: 2100, scrollHeight: 2800, want: 1 },
		{ name: 'overscroll above', scrollTop: -40, scrollHeight: 2800, want: 0 },
		{ name: 'nothing to scroll', scrollTop: 0, scrollHeight: 800, want: 0 },
	]

	for (const tc of cases) {
		it(tc.name, () => {
			expect(
				scrollProgress({
					scrollTop: tc.scrollTop,
					scrollHeight: tc.scrollHeight,
					clientHeight: 800,
				}),
			).toBe(tc.want)
		})
	}
})
