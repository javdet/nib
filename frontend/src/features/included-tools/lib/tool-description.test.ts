import { describe, expect, it } from 'vitest'
import { summarizeToolDescription } from './tool-description'

describe('summarizeToolDescription', () => {
	it('returns empty string for blank input', () => {
		expect(summarizeToolDescription('')).toBe('')
		expect(summarizeToolDescription('   ')).toBe('')
	})

	it('leaves a short phrase unchanged', () => {
		expect(summarizeToolDescription('Get a GitHub Actions workflow run.')).toBe(
			'Get a GitHub Actions workflow run.',
		)
	})

	it('collapses whitespace', () => {
		expect(
			summarizeToolDescription('Get   a workflow.\n\nSecond line.'),
		).toBe('Get a workflow. Second line.')
	})

	it('keeps only the first two sentences', () => {
		const input =
			'First sentence here. Second sentence follows. Third should be dropped.'
		expect(summarizeToolDescription(input)).toBe(
			'First sentence here. Second sentence follows.',
		)
	})

	it('clips long run-on text at a word boundary', () => {
		const long = 'Alpha '.repeat(80)
		const summary = summarizeToolDescription(long)
		expect(summary.length).toBeLessThanOrEqual(281)
		expect(summary.endsWith('…')).toBe(true)
	})
})
