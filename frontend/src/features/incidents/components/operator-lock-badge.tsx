import { useState } from 'react'
import { Lock } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '@/components/ui/tooltip'

interface OperatorLockBadgeProps {
	/** Hands the field back to the agents. Omitted renders a plain badge. */
	onUnlock?: () => Promise<void>
	compact?: boolean
}

/**
 * Marks a field the operator has edited, which agents can no longer overwrite.
 * Clicking it hands the field back.
 */
export function OperatorLockBadge({ onUnlock, compact = false }: OperatorLockBadgeProps) {
	const [busy, setBusy] = useState(false)

	const handleClick = async (e: React.MouseEvent) => {
		e.stopPropagation()
		if (!onUnlock || busy) return
		setBusy(true)
		try {
			await onUnlock()
		} finally {
			setBusy(false)
		}
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					type="button"
					onMouseDown={(e) => e.preventDefault()}
					onClick={(e) => void handleClick(e)}
					disabled={!onUnlock || busy}
					className={cn(
						'inline-flex shrink-0 items-center gap-1 rounded-full border border-border/60',
						'bg-secondary/60 text-muted-foreground transition-colors hover:bg-secondary',
						'disabled:cursor-default',
						compact ? 'px-1.5 py-0.5 text-[10px]' : 'px-2 py-0.5 text-xs',
					)}
				>
					<Lock className={compact ? 'h-2.5 w-2.5' : 'h-3 w-3'} />
					{!compact && 'Edited by you'}
				</button>
			</TooltipTrigger>
			<TooltipContent>
				{onUnlock
					? 'Agents will not overwrite this. Click to let them update it again.'
					: 'Edited by you; agents will not overwrite this.'}
			</TooltipContent>
		</Tooltip>
	)
}
