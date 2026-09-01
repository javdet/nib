import {
	CircleEllipsis,
	CircleHelp,
	FileCode2,
	Globe,
	Send,
	SquareTerminal,
} from 'lucide-react'
import { describe, expect, it } from 'vitest'
import { actionTypeIcon } from './action-type-icon'

describe('actionTypeIcon', () => {
	it.each([
		['code', FileCode2],
		['web', Globe],
		['curl', Send],
		['shell', SquareTerminal],
		['other', CircleEllipsis],
	])('maps %s to its icon', (type, expected) => {
		expect(actionTypeIcon(type)).toBe(expected)
	})

	it('is case-insensitive and trims whitespace', () => {
		expect(actionTypeIcon('  CODE  ')).toBe(FileCode2)
	})

	it('falls back for unknown types', () => {
		expect(actionTypeIcon('custom')).toBe(CircleHelp)
	})
})
