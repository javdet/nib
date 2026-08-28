import { describe, expect, it } from 'vitest'
import {
	hideDelay,
	initialToolActivityState,
	MIN_VISIBLE_MS,
	reduceToolActivity,
} from './tool-activity'

describe('hideDelay', () => {
	it('waits out the remainder of the minimum visible window', () => {
		expect(hideDelay(1000, 1050)).toBe(MIN_VISIBLE_MS - 50)
	})

	it('returns zero when the tool call already exceeded the minimum', () => {
		expect(hideDelay(1000, 6000)).toBe(0)
	})
})

describe('reduceToolActivity', () => {
	it('shows on tools_start', () => {
		const next = reduceToolActivity(initialToolActivityState, {
			type: 'tools_start',
			now: 1000,
		})
		expect(next).toEqual({ visible: true, shownAt: 1000 })
	})

	it('keeps shownAt when tools_start arrives during the grace window', () => {
		const visible = { visible: true, shownAt: 1000 }
		const next = reduceToolActivity(visible, {
			type: 'tools_start',
			now: 2500,
		})
		expect(next).toBe(visible)
	})

	it('does not hide immediately on tools_end', () => {
		const visible = { visible: true, shownAt: 1000 }
		const next = reduceToolActivity(visible, {
			type: 'tools_end',
			now: 1050,
		})
		expect(next).toEqual(visible)
	})

	it('clears on hide and reset', () => {
		const visible = { visible: true, shownAt: 1000 }
		expect(reduceToolActivity(visible, { type: 'hide' })).toEqual(
			initialToolActivityState,
		)
		expect(reduceToolActivity(visible, { type: 'reset' })).toEqual(
			initialToolActivityState,
		)
	})
})
