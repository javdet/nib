import { describe, expect, it } from 'vitest'
import { dialogHref, dialogIdFromHref } from './dialog-link'

describe('dialogIdFromHref', () => {
	it('reads the id out of a dialog link', () => {
		expect(dialogIdFromHref('#dialog:6f1c2d3e')).toBe('6f1c2d3e')
	})

	it('ignores ordinary links', () => {
		expect(dialogIdFromHref('https://example.com')).toBeNull()
		expect(dialogIdFromHref('#section')).toBeNull()
		expect(dialogIdFromHref(undefined)).toBeNull()
	})

	it('round-trips a href it built', () => {
		expect(dialogIdFromHref(dialogHref('abc-123'))).toBe('abc-123')
	})

	it('treats an empty id as no link', () => {
		expect(dialogIdFromHref('#dialog:')).toBeNull()
		expect(dialogIdFromHref('#dialog:   ')).toBeNull()
	})
})
