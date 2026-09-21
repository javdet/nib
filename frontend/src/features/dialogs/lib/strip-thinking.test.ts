import { describe, expect, it } from 'vitest'
import { stripThinking } from './strip-thinking'

describe('stripThinking', () => {
	it('leaves a message without narration alone', () => {
		expect(stripThinking('Deployed centrifugo 5.4.1.')).toBe(
			'Deployed centrifugo 5.4.1.',
		)
	})

	it('drops a block before the answer', () => {
		expect(
			stripThinking(
				'<thinking>I need the chart version first.</thinking>\n\nDeployed.',
			),
		).toBe('Deployed.')
	})

	it('drops every block, whatever the casing or attributes', () => {
		expect(
			stripThinking('<Thinking>one</Thinking>a<thinking id="2">two</thinking >b'),
		).toBe('ab')
	})

	it('drops a multiline block', () => {
		expect(stripThinking('<thinking>\nunknowns:\n- version\n</thinking>\nok')).toBe(
			'ok',
		)
	})

	it('takes the tail with an unclosed block', () => {
		expect(stripThinking('Deployed.\n<thinking>now I should check the')).toBe(
			'Deployed.',
		)
	})

	it('returns nothing for a message that was only narration', () => {
		expect(stripThinking('<thinking>listing variables next</thinking>')).toBe('')
	})
})
