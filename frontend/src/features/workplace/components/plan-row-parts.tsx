import type { ReactNode } from 'react'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

// Done is an opaque emerald surface rather than a translucent green wash, so a
// finished row reads as a settled state instead of a film laid over the card.
// Redefining --border retints every divider inside the row at once: the base
// layer resolves every border-color through it.
export const DONE_SURFACE =
	'border-success-border bg-success-surface [--border:var(--success-border)]'

interface HeaderButtonProps {
	label: string
	tooltip: ReactNode
	onClick: () => void
	disabled?: boolean
	active?: boolean
	children: ReactNode
}

export function HeaderButton({
	label,
	tooltip,
	onClick,
	disabled = false,
	active = false,
	children,
}: HeaderButtonProps) {
	return (
		<Tooltip>
			{/* The button is wrapped so the tooltip still explains why it is off:
			    a natively disabled button swallows its own pointer events. */}
			<TooltipTrigger asChild>
				<span className="flex h-9 w-9 shrink-0">
					<button
						type="button"
						onClick={onClick}
						disabled={disabled}
						className={cn(
							'flex flex-1 items-center justify-center rounded-sm',
							'transition-colors duration-[var(--dur-fast)] focus-ring-inset',
							disabled && 'cursor-not-allowed text-muted-foreground/40',
							!disabled && 'cursor-pointer hover:bg-foreground/[0.07]',
							!disabled &&
								(active
									? 'text-primary hover:text-primary'
									: 'text-muted-foreground hover:text-foreground'),
						)}
						aria-label={label}
					>
						{children}
					</button>
				</span>
			</TooltipTrigger>
			<TooltipContent>{tooltip}</TooltipContent>
		</Tooltip>
	)
}
