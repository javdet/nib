import { describe, expect, it } from 'vitest'
import { leavesPage } from './unsaved-changes'

describe('leavesPage', () => {
	const cases: {
		name: string
		current: string
		next: string
		want: boolean
	}[] = [
		{ name: 'other page', current: '/knowledge', next: '/rules', want: true },
		{ name: 'same page', current: '/knowledge', next: '/knowledge', want: false },
		{ name: 'other tab', current: '/tools?tab=mcp', next: '/tools?tab=executor', want: true },
		{ name: 'hash only', current: '/knowledge', next: '/knowledge#upload', want: false },
	]

	for (const tc of cases) {
		it(tc.name, () => {
			const current = new URL(tc.current, 'http://nib')
			const next = new URL(tc.next, 'http://nib')
			expect(leavesPage(current, next)).toBe(tc.want)
		})
	}
})
