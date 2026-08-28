import type { DialogMessage } from '@/features/dialogs/api/dialogs'
import { findPendingAskQuestion } from '@/features/dialogs/lib/pending-question'

/**
 * Reports whether the agent finished its turn in a stored transcript.
 * The backend persists every round of the agent loop, so a turn is only over
 * once it ends with a plain assistant reply or with a question for the user.
 */
export function isTurnComplete(msgs: DialogMessage[]): boolean {
	const last = msgs[msgs.length - 1]
	if (!last) {
		return false
	}
	if (last.role === 'assistant' && !last.toolCalls?.length) {
		return true
	}
	return findPendingAskQuestion(msgs) !== null
}

/**
 * Reports whether the transcript ends with the user turn we tried to send.
 * The backend stores the user message before it runs the agent, so a reachable
 * backend without that message never received the request.
 */
export function hasLastUserMessage(
	msgs: DialogMessage[],
	content: string,
): boolean {
	for (let i = msgs.length - 1; i >= 0; i--) {
		const m = msgs[i]
		if (m?.role === 'user') {
			return m.content === content
		}
	}
	return false
}

/** Reports whether the backend stored the answers for a question tool call. */
export function hasToolResult(
	msgs: DialogMessage[],
	toolCallId: string,
): boolean {
	return msgs.some((m) => m.role === 'tool' && m.toolCallId === toolCallId)
}
