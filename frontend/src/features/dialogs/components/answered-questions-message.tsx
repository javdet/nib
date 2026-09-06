import type { AnsweredQuestion } from '../lib/answered-questions'
import { formatOptionLabel } from '../lib/format-option-text'

interface AnsweredQuestionsMessageProps {
	answers: AnsweredQuestion[]
}

/**
 * Renders an answered `ask_question` round as a numbered question/answer list.
 *
 * Sits inside the ordinary user bubble, so it inherits that bubble's colours
 * instead of setting its own.
 */
export function AnsweredQuestionsMessage({
	answers,
}: AnsweredQuestionsMessageProps) {
	return (
		<ol className="list-none space-y-2">
			{answers.map((item, i) => (
				<li key={i} className="min-w-0">
					<p className="break-words font-medium opacity-80">
						{i + 1}. {formatOptionLabel(item.question)}
					</p>
					<p className="whitespace-pre-wrap break-words">
						{item.answer
							? formatOptionLabel(item.answer)
							: '—'}
					</p>
				</li>
			))}
		</ol>
	)
}
