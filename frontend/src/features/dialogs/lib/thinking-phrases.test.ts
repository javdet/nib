import { describe, expect, it } from 'vitest'
import {
	EXECUTING_PHRASES,
	THINKING_PHRASES,
	phrasesForMode,
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

	it('speaks of doing rather than planning in execute mode', () => {
		expect(thinkingPhrase(0, 'execute')).toBe('Executing')
		expect(thinkingPhrase(-1, 'execute')).toBe('Checking')
	})
})

describe('phrasesForMode', () => {
	it('uses the executing words for execute only', () => {
		expect(phrasesForMode('execute')).toBe(EXECUTING_PHRASES)
		for (const mode of ['main', 'plan', 'decompose', 'discuss', null, undefined]) {
			expect(phrasesForMode(mode)).toBe(THINKING_PHRASES)
		}
	})

	it('never plans in execute mode', () => {
		for (const phrase of EXECUTING_PHRASES) {
			expect(phrase).not.toMatch(/plan|strateg|deliberat/i)
		}
	})
})

describe.each([
	['THINKING_PHRASES', THINKING_PHRASES],
	['EXECUTING_PHRASES', EXECUTING_PHRASES],
])('%s', (_, phrases) => {
	it('has no adjacent duplicates', () => {
		for (let i = 0; i < phrases.length - 1; i++) {
			expect(phrases[i]).not.toBe(phrases[i + 1])
		}
	})
})
