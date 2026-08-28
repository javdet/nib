import { describe, expect, it } from 'vitest'
import type { DialogMessage } from '@/features/dialogs/api/dialogs'
import {
	hasLastUserMessage,
	hasToolResult,
	isTurnComplete,
} from './turn-state'

function askQuestionCall(id: string): DialogMessage {
	return {
		role: 'assistant',
		content: '',
		toolCalls: [
			{
				id,
				type: 'function',
				function: {
					name: 'ask_question',
					arguments: JSON.stringify({
						questions: [{ question: 'Which cluster?' }],
					}),
				},
			},
		],
	}
}

describe('isTurnComplete', () => {
	it('returns false for an empty transcript', () => {
		expect(isTurnComplete([])).toBe(false)
	})

	it('returns false while the agent is still calling tools', () => {
		const msgs: DialogMessage[] = [
			{ role: 'user', content: 'describe OPS-1801' },
			{
				role: 'assistant',
				content: '',
				toolCalls: [
					{
						id: 'call_1',
						type: 'function',
						function: { name: 'jira_search', arguments: '{}' },
					},
				],
			},
			{ role: 'tool', content: 'result', toolCallId: 'call_1' },
		]

		expect(isTurnComplete(msgs)).toBe(false)
	})

	it('returns true once a plain assistant reply lands', () => {
		const msgs: DialogMessage[] = [
			{ role: 'user', content: 'describe OPS-1801' },
			{ role: 'assistant', content: 'Here is the ticket.' },
		]

		expect(isTurnComplete(msgs)).toBe(true)
	})

	it('returns true when the agent is waiting for an answer', () => {
		const msgs: DialogMessage[] = [
			{ role: 'user', content: 'plan the migration' },
			askQuestionCall('call_ask'),
		]

		expect(isTurnComplete(msgs)).toBe(true)
	})

	it('returns false when the answered question is followed by more tool work', () => {
		const msgs: DialogMessage[] = [
			{ role: 'user', content: 'plan the migration' },
			askQuestionCall('call_ask'),
			{ role: 'tool', content: 'staging', toolCallId: 'call_ask' },
		]

		expect(isTurnComplete(msgs)).toBe(false)
	})
})

describe('hasLastUserMessage', () => {
	const msgs: DialogMessage[] = [
		{ role: 'user', content: 'first question' },
		{ role: 'assistant', content: 'first answer' },
		{ role: 'user', content: 'second question' },
	]

	it('matches the message the backend stored last', () => {
		expect(hasLastUserMessage(msgs, 'second question')).toBe(true)
	})

	it('does not match an earlier user message', () => {
		expect(hasLastUserMessage(msgs, 'first question')).toBe(false)
	})

	it('returns false when the message was never stored', () => {
		expect(hasLastUserMessage([], 'second question')).toBe(false)
	})
})

describe('hasToolResult', () => {
	const msgs: DialogMessage[] = [
		askQuestionCall('call_ask'),
		{ role: 'tool', content: 'staging', toolCallId: 'call_ask' },
	]

	it('finds a stored tool result', () => {
		expect(hasToolResult(msgs, 'call_ask')).toBe(true)
	})

	it('returns false for an unanswered tool call', () => {
		expect(hasToolResult(msgs, 'call_other')).toBe(false)
	})
})
