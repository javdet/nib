import { describe, expect, it } from 'vitest'
import { parseAnsweredQuestions } from './answered-questions'

function stored(answers: unknown): string {
	return JSON.stringify({ answers })
}

describe('parseAnsweredQuestions', () => {
	it('parses a well-formed answer set', () => {
		const content = stored([
			{ question: 'Which cloud provider?', answer: 'AWS' },
			{ question: 'Which region?', answer: 'eu-central-1' },
		])

		expect(parseAnsweredQuestions(content)).toEqual([
			{ question: 'Which cloud provider?', answer: 'AWS' },
			{ question: 'Which region?', answer: 'eu-central-1' },
		])
	})

	it('trims surrounding whitespace', () => {
		const content = stored([
			{ question: '  Which region?  ', answer: '  eu-central-1  ' },
		])

		expect(parseAnsweredQuestions(content)).toEqual([
			{ question: 'Which region?', answer: 'eu-central-1' },
		])
	})

	it('keeps a pair whose answer is empty', () => {
		const content = stored([{ question: 'Anything else?', answer: '' }])

		expect(parseAnsweredQuestions(content)).toEqual([
			{ question: 'Anything else?', answer: '' },
		])
	})

	it('skips a pair with no question text', () => {
		const content = stored([
			{ question: '   ', answer: 'orphan' },
			{ question: 'Which region?', answer: 'eu-central-1' },
		])

		expect(parseAnsweredQuestions(content)).toEqual([
			{ question: 'Which region?', answer: 'eu-central-1' },
		])
	})

	it('returns null when every pair is skipped', () => {
		expect(parseAnsweredQuestions(stored([{ question: '', answer: 'x' }]))).toBeNull()
	})

	it('returns null for malformed JSON', () => {
		expect(parseAnsweredQuestions('not json')).toBeNull()
		expect(parseAnsweredQuestions('')).toBeNull()
	})

	it('returns null when the payload is not an object', () => {
		expect(parseAnsweredQuestions('null')).toBeNull()
		expect(parseAnsweredQuestions('"a string"')).toBeNull()
		expect(parseAnsweredQuestions('[]')).toBeNull()
	})

	it('returns null when answers is missing, empty or not an array', () => {
		expect(parseAnsweredQuestions('{}')).toBeNull()
		expect(parseAnsweredQuestions(stored([]))).toBeNull()
		expect(parseAnsweredQuestions(stored('AWS'))).toBeNull()
	})

	it('returns null when an item has the wrong shape', () => {
		expect(parseAnsweredQuestions(stored(['AWS']))).toBeNull()
		expect(parseAnsweredQuestions(stored([null]))).toBeNull()
		expect(parseAnsweredQuestions(stored([{ question: 'Q' }]))).toBeNull()
		expect(parseAnsweredQuestions(stored([{ answer: 'A' }]))).toBeNull()
		expect(
			parseAnsweredQuestions(stored([{ question: 1, answer: 2 }])),
		).toBeNull()
	})

	it('ignores an unrelated tool result payload', () => {
		const createTable = JSON.stringify({
			columns: ['a'],
			rows: [['1']],
		})
		expect(parseAnsweredQuestions(createTable)).toBeNull()
	})
})
