import { cn } from '@/lib/utils'
import type { PlanStatus } from '@/features/dialogs/api/dialogs'

export const STATUS_STYLES: Record<
	PlanStatus,
	{ label: string; className: string }
> = {
	draft: {
		label: 'DRAFT',
		className:
			'border-zinc-500/30 bg-zinc-500/15 text-zinc-700 dark:text-zinc-300',
	},
	scheduled: {
		label: 'SCHEDULED',
		className:
			'border-amber-500/30 bg-amber-500/15 text-amber-700 dark:text-amber-300',
	},
	in_progress: {
		label: 'IN PROGRESS',
		className:
			'border-blue-500/30 bg-blue-500/15 text-blue-700 dark:text-blue-300',
	},
	done: {
		label: 'FINISHED',
		className:
			'border-lime-500/30 bg-lime-500/15 text-lime-700 dark:text-lime-300',
	},
	reopened: {
		label: 'REOPENED',
		className:
			'border-orange-500/30 bg-orange-500/15 text-orange-700 dark:text-orange-300',
	},
	rolled_back: {
		label: 'ROLLED BACK',
		className:
			'border-red-500/30 bg-red-500/15 text-red-700 dark:text-red-300',
	},
	cancelled: {
		label: 'CANCELLED',
		className:
			'border-rose-500/30 bg-rose-500/15 text-rose-700 dark:text-rose-300',
	},
}

export function getPlanStatusLabel(status: PlanStatus): string {
	return (STATUS_STYLES[status] ?? STATUS_STYLES.draft).label
}

interface PlanStatusBadgeProps {
	status: PlanStatus
	compact?: boolean
}

export function PlanStatusBadge({ status, compact = false }: PlanStatusBadgeProps) {
	const style = STATUS_STYLES[status] ?? STATUS_STYLES.draft

	const pill = (
		<span
			className={cn(
				'inline-flex shrink-0 items-center rounded-full border',
				'font-semibold uppercase tracking-wider',
				compact
					? 'px-2 py-0.5 text-[10px]'
					: 'px-4 py-1.5 text-xs',
				style.className,
			)}
		>
			{style.label}
		</span>
	)

	if (compact) {
		return pill
	}

	return (
		<div className="flex items-center gap-3">
			<span className="text-sm font-medium">Status:</span>
			{pill}
		</div>
	)
}
