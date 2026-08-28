import type { DialogMessage, Question } from '@/features/dialogs/api/dialogs'

const ASK_QUESTION_TOOL = 'ask_question'

export interface PendingAskQuestion {
	toolCallId: string
	questions: Question[]
}

function parseAskQuestionArguments(
	argumentsJSON: string,
): Question[] | null {
	try {
		const args = JSON.parse(argumentsJSON) as {
			questions?: unknown
		}
		if (!Array.isArray(args.questions) || args.questions.length === 0) {
			return null
		}
		const questions: Question[] = []
		for (const item of args.questions) {
			if (
				typeof item !== 'object' ||
				item === null ||
				typeof (item as { question?: unknown }).question !== 'string'
			) {
				return null
			}
			const { question, options } = item as {
				question: string
				options?: unknown
			}
			const q: Question = { question: question.trim() }
			if (options !== undefined) {
				if (!Array.isArray(options)) {
					return null
				}
				const parsed = options.filter(
					(opt): opt is string =>
						typeof opt === 'string' && opt.trim() !== '',
				)
				if (parsed.length > 0) {
					q.options = parsed
				}
			}
			if (!q.question) {
				return null
			}
			questions.push(q)
		}
		return questions
	} catch {
		return null
	}
}

/**
 * Finds an unanswered ask_question tool call in a dialog transcript.
 * Scans assistant messages for ask_question calls with no matching tool result.
 */
export function findPendingAskQuestion(
	msgs: DialogMessage[],
): PendingAskQuestion | null {
	const answeredIds = new Set<string>()
	for (const m of msgs) {
		if (m.role === 'tool' && m.toolCallId) {
			answeredIds.add(m.toolCallId)
		}
	}

	for (let i = msgs.length - 1; i >= 0; i--) {
		const m = msgs[i]
		if (!m) continue
		if (m.role !== 'assistant' || !m.toolCalls?.length) {
			continue
		}
		for (const tc of m.toolCalls) {
			if (tc.function?.name !== ASK_QUESTION_TOOL) {
				continue
			}
			if (answeredIds.has(tc.id)) {
				continue
			}
			const questions = parseAskQuestionArguments(tc.function.arguments)
			if (!questions) {
				continue
			}
			return { toolCallId: tc.id, questions }
		}
	}

	return null
}
