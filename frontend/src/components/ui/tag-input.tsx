import { useCallback, useRef, useState, type KeyboardEvent } from 'react'
import { X } from 'lucide-react'
import { Badge, type BadgeProps } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

const MAX_TAGS = 32

function normalizeTags(items: string[]): string[] {
	const seen = new Set<string>()
	const out: string[] = []
	for (const item of items) {
		for (const part of item.split(/[\s,]+/)) {
			const normalized = part.trim().toLowerCase()
			if (!normalized || seen.has(normalized)) {
				continue
			}
			seen.add(normalized)
			out.push(normalized)
		}
	}
	return out.slice(0, MAX_TAGS)
}

function mergeTags(existing: string[], incoming: string[]): string[] {
	return normalizeTags([...existing, ...incoming])
}

interface TagInputProps {
	value: string[]
	onChange: (next: string[]) => void
	variant?: BadgeProps['variant']
	placeholder?: string
	disabled?: boolean
	'aria-label'?: string
	className?: string
}

export function TagInput({
	value,
	onChange,
	variant = 'secondary',
	placeholder = 'Type and press Enter',
	disabled = false,
	'aria-label': ariaLabel,
	className,
}: TagInputProps) {
	const [draft, setDraft] = useState('')
	const inputRef = useRef<HTMLInputElement>(null)

	const commitDraft = useCallback(() => {
		const trimmed = draft.trim()
		if (!trimmed) {
			return
		}
		const next = mergeTags(value, [trimmed])
		if (next.length !== value.length || next.some((tag, i) => tag !== value[i])) {
			onChange(next)
		}
		setDraft('')
	}, [draft, onChange, value])

	const removeTag = useCallback(
		(tag: string) => {
			onChange(value.filter((item) => item !== tag))
		},
		[onChange, value],
	)

	const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
		if (event.key === 'Enter') {
			event.preventDefault()
			commitDraft()
			return
		}
		if (event.key === 'Backspace' && draft === '' && value.length > 0) {
			event.preventDefault()
			onChange(value.slice(0, -1))
		}
	}

	return (
		<div
			className={cn(
				'flex min-h-[var(--control-h)] w-full flex-wrap items-center gap-1.5',
				'cursor-text rounded-md border border-input bg-background/40 px-2 py-1.5',
				'text-sm elev-1 transition-[border-color,box-shadow]',
				'duration-[var(--dur-fast)] hover:border-ring/40 focus-ring-within',
				disabled && 'cursor-not-allowed opacity-50',
				className,
			)}
			onClick={() => inputRef.current?.focus()}
		>
			{value.map((tag) => (
				<Badge
					key={tag}
					variant={variant}
					className="gap-1 pr-1"
				>
					{tag}
					{!disabled && (
						<button
							type="button"
							className={cn(
								'-mr-0.5 flex size-4 cursor-pointer items-center justify-center',
								'rounded-sm text-current opacity-60 transition-opacity',
								'duration-[var(--dur-fast)] hover:opacity-100 focus-ring',
							)}
							onClick={(event) => {
								event.stopPropagation()
								removeTag(tag)
							}}
							aria-label={`Remove ${tag}`}
						>
							<X className="h-3 w-3" />
						</button>
					)}
				</Badge>
			))}
			<input
				ref={inputRef}
				type="text"
				value={draft}
				onChange={(event) => setDraft(event.target.value)}
				onKeyDown={handleKeyDown}
				onBlur={commitDraft}
				disabled={disabled || value.length >= MAX_TAGS}
				placeholder={value.length === 0 ? placeholder : ''}
				aria-label={ariaLabel}
				className={cn(
					'min-w-[8ch] flex-1 border-0 bg-transparent px-1 py-0.5 text-sm',
					'outline-none placeholder:text-muted-foreground/70',
					'disabled:cursor-not-allowed',
				)}
			/>
		</div>
	)
}
