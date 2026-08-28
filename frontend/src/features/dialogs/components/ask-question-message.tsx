import { useEffect, useState } from 'react'
import { Bot } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import type { Question } from '../api/dialogs'
import {
	formatOptionLabel,
	shouldStackOptions,
} from '../lib/format-option-text'

interface AskQuestionMessageProps {
	questions: Question[]
	onSubmit: (answers: string[]) => void
	submitting?: boolean
}

export function AskQuestionMessage({
	questions,
	onSubmit,
	submitting = false,
}: AskQuestionMessageProps) {
	const [step, setStep] = useState(0)
	const [answers, setAnswers] = useState<string[]>([])
	const [draft, setDraft] = useState('')
	const [selectedOption, setSelectedOption] = useState<string | null>(null)

	useEffect(() => {
		setStep(0)
		setAnswers([])
		setDraft('')
		setSelectedOption(null)
	}, [questions])

	const current = questions[step]
	const total = questions.length
	const isLast = step === total - 1
	const canContinue = draft.trim().length > 0

	function handleOptionSelect(option: string) {
		setSelectedOption(option)
		setDraft(option)
	}

	function handleNext() {
		const answer = draft.trim()
		if (!answer) return

		const nextAnswers = [...answers, answer]
		if (isLast) {
			onSubmit(nextAnswers)
			return
		}

		setAnswers(nextAnswers)
		setStep((prev) => prev + 1)
		setDraft('')
		setSelectedOption(null)
	}

	if (!current) {
		return null
	}

	const stacked =
		current.options && current.options.length > 0
			? shouldStackOptions(current.options)
			: false

	return (
		<div className="flex min-w-0 flex-row gap-2">
			<div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border bg-muted">
				<Bot className="h-3.5 w-3.5" />
			</div>
			<div className="min-w-0 max-w-[85%] rounded-lg bg-muted px-3 py-2 text-sm">
				<div className="mb-3 flex items-baseline justify-between gap-3">
					<p className="font-medium">Clarifying question</p>
					<span className="shrink-0 text-xs text-muted-foreground">
						Question {step + 1} of {total}
					</span>
				</div>

				<div className="space-y-4">
					<p className="break-words font-medium">{current.question}</p>

					{current.options && current.options.length > 0 && (
						<div
							className={cn(
								'gap-2',
								stacked ? 'flex flex-col' : 'flex flex-wrap',
							)}
						>
							{current.options.map((option) => (
								<Button
									key={option}
									type="button"
									variant={
										selectedOption === option
											? 'default'
											: 'outline'
									}
									size="sm"
									disabled={submitting}
									onClick={() => handleOptionSelect(option)}
									className={cn(
										'h-auto min-h-8 whitespace-normal break-words text-left',
										stacked
											? 'w-full justify-start'
											: 'max-w-full',
									)}
								>
									{formatOptionLabel(option)}
								</Button>
							))}
						</div>
					)}

					<div className="space-y-2">
						<Label htmlFor="ask-question-answer">Your answer</Label>
						<Input
							id="ask-question-answer"
							value={draft}
							onChange={(e) => {
								setDraft(e.target.value)
								if (
									selectedOption &&
									e.target.value !== selectedOption
								) {
									setSelectedOption(null)
								}
							}}
							placeholder="Type your answer or pick an option above"
							disabled={submitting}
							onKeyDown={(e) => {
								if (e.key === 'Enter' && canContinue) {
									e.preventDefault()
									handleNext()
								}
							}}
						/>
					</div>

					<div className="flex justify-end">
						<Button
							type="button"
							onClick={handleNext}
							disabled={!canContinue || submitting}
							className={cn(submitting && 'pointer-events-none')}
						>
							{isLast
								? submitting
									? 'Submitting…'
									: 'Submit'
								: 'Next'}
						</Button>
					</div>
				</div>
			</div>
		</div>
	)
}
