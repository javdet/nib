export interface AnsweredQuestion {
	question: string
	answer: string
}

/**
 * Parses a stored `ask_question` tool result into question/answer pairs.
 *
 * The backend writes the whole answered set as a single `tool` row whose
 * content is `{"answers":[{"question":…,"answer":…}]}`, so this is the
 * mirror image of `parseAskQuestionArguments` in `pending-question.ts`.
 *
 * Returns null when the row is not a well-formed answer set, so a caller can
 * skip it rather than render raw JSON at the operator.
 */
export function parseAnsweredQuestions(
	content: string,
): AnsweredQuestion[] | null {
	let parsed: unknown
	try {
		parsed = JSON.parse(content)
	} catch {
		return null
	}

	if (typeof parsed !== 'object' || parsed === null) {
		return null
	}

	const { answers } = parsed as { answers?: unknown }
	if (!Array.isArray(answers) || answers.length === 0) {
		return null
	}

	const out: AnsweredQuestion[] = []
	for (const item of answers) {
		if (typeof item !== 'object' || item === null) {
			return null
		}
		const { question, answer } = item as {
			question?: unknown
			answer?: unknown
		}
		if (typeof question !== 'string' || typeof answer !== 'string') {
			return null
		}
		const trimmed = question.trim()
		// An empty answer is allowed — the backend does not forbid one — but a
		// pair with no question text has nothing to show.
		if (!trimmed) {
			continue
		}
		out.push({ question: trimmed, answer: answer.trim() })
	}

	return out.length > 0 ? out : null
}
