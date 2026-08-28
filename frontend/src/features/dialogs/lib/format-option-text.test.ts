import { describe, expect, it } from 'vitest'
import {
	formatOptionLabel,
	OPTION_LONG_THRESHOLD,
	shouldStackOptions,
} from './format-option-text'

describe('shouldStackOptions', () => {
	it('returns false when all options are at or below the threshold', () => {
		expect(shouldStackOptions(['Yes', 'No', 'Maybe'])).toBe(false)
		expect(
			shouldStackOptions(['a'.repeat(OPTION_LONG_THRESHOLD)]),
		).toBe(false)
	})

	it('returns true when any option exceeds the threshold', () => {
		expect(
			shouldStackOptions([
				'Short',
				'a'.repeat(OPTION_LONG_THRESHOLD + 1),
			]),
		).toBe(true)
	})
})

describe('formatOptionLabel', () => {
	it('returns short text unchanged', () => {
		const text = 'Deploy to staging'
		expect(formatOptionLabel(text)).toBe(text)
	})

	it('inserts break opportunities in long unbroken runs', () => {
		const longPath = '/very/long/path/without/any/spaces/that/keeps/going'
		const formatted = formatOptionLabel(longPath)

		expect(formatted).not.toBe(longPath)
		expect(formatted.replace(/\u200B/g, '')).toBe(longPath)
		expect(formatted).toContain('\u200B')
	})

	it('preserves spaces between words for long sentences', () => {
		const sentence =
			'This is a long sentence that should still contain normal spaces between words for readability'
		const formatted = formatOptionLabel(sentence)

		expect(formatted).toContain(' ')
		expect(formatted.replace(/\u200B/g, '')).toBe(sentence)
	})
})

describe('display vs stored value', () => {
	it('keeps the original option string separate from the formatted label', () => {
		const original = 'https://example.com/very/long/url/without/spaces/at/all/continuing'
		const label = formatOptionLabel(original)

		expect(label).not.toBe(original)
		expect(label.replace(/\u200B/g, '')).toBe(original)
	})
})
