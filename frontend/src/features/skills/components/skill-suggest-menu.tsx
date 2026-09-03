import { useEffect, useRef } from 'react'
import type { SkillMeta } from '../api/skills'
import { cn } from '@/lib/utils'

export const SKILL_LISTBOX_ID = 'skill-suggest-listbox'

export function skillOptionId(index: number): string {
	return `skill-suggest-option-${index}`
}

interface SkillSuggestMenuProps {
	suggestions: SkillMeta[]
	activeIndex: number
	onActiveIndexChange: (index: number) => void
	onSelect: (name: string) => void
}

/**
 * Slash-command suggestions for the chat input.
 *
 * Deliberately not a Radix popover: the caret has to stay in the textarea, and
 * Radix content traps focus and dismisses on interaction with the very element
 * that owns the keyboard here. Positioning relies on the input row being
 * `relative`, so the menu opens upward into the message list.
 */
export function SkillSuggestMenu({
	suggestions,
	activeIndex,
	onActiveIndexChange,
	onSelect,
}: SkillSuggestMenuProps) {
	const listRef = useRef<HTMLDivElement>(null)

	useEffect(() => {
		// `nearest` and no smooth behaviour: a held arrow key must not lag behind.
		listRef.current?.children[activeIndex]?.scrollIntoView({ block: 'nearest' })
	}, [activeIndex])

	return (
		<div
			className={cn(
				'glass-menu absolute bottom-full left-3 right-3 z-20 mb-2',
				'overflow-hidden rounded-lg border text-popover-foreground',
				'animate-in fade-in-0 slide-in-from-bottom-1',
				'duration-[var(--dur-fast)]',
			)}
		>
			<div
				ref={listRef}
				id={SKILL_LISTBOX_ID}
				role="listbox"
				aria-label="Skills"
				className="max-h-60 overflow-y-auto p-1"
			>
				{suggestions.map((skill, index) => (
					<div
						key={skill.name}
						id={skillOptionId(index)}
						role="option"
						aria-selected={index === activeIndex}
						tabIndex={-1}
						onMouseEnter={() => onActiveIndexChange(index)}
						// Prevents the blur that would dismiss the menu before the
						// click lands.
						onMouseDown={(e) => e.preventDefault()}
						onClick={() => onSelect(skill.name)}
						className={cn(
							'flex cursor-pointer select-none flex-col gap-0.5',
							'rounded-md px-2 py-1.5 transition-colors',
							'duration-[var(--dur-fast)]',
							index === activeIndex && 'bg-foreground/[0.07]',
						)}
					>
						<span className="truncate text-sm font-medium">
							/{skill.name}
						</span>
						{skill.description && (
							<span className="truncate text-xs text-muted-foreground">
								{skill.description}
							</span>
						)}
					</div>
				))}
			</div>
			<div
				className={cn(
					'border-t border-[var(--hairline)] px-2 py-1',
					'text-[11px] text-muted-foreground',
				)}
			>
				↑↓ to navigate · Enter to insert · Esc to dismiss
			</div>
		</div>
	)
}
