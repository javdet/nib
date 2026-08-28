import { describe, expect, it } from 'vitest'
import {
	THINKING_PHRASES,
	thinkingPhrase,
} from './thinking-phrases'

describe('thinkingPhrase', () => {
	it('returns Thinking for turn 0', () => {
		expect(thinkingPhrase(0)).toBe('Thinking')
	})

	it('advances one word per turn', () => {
		expect(thinkingPhrase(1)).toBe('Planning')
		expect(thinkingPhrase(2)).toBe('Reasoning')
	})

	it('wraps past the end of the list', () => {
		const len = THINKING_PHRASES.length
		expect(thinkingPhrase(len)).toBe('Thinking')
		expect(thinkingPhrase(len + 1)).toBe('Planning')
	})

	it('handles negative turn values', () => {
		expect(thinkingPhrase(-1)).toBe('Strategizing')
	})
})

describe('THINKING_PHRASES', () => {
	it('has no adjacent duplicates', () => {
		for (let i = 0; i < THINKING_PHRASES.length - 1; i++) {
			expect(THINKING_PHRASES[i]).not.toBe(THINKING_PHRASES[i + 1])
		}
	})
})
