import { cn } from '@/lib/utils'
import type { PlanStatus } from '@/features/dialogs/api/dialogs'

export const STATUS_STYLES: Record<
	PlanStatus,
	{ label: string; className: string }
> = {
	draft: {
		label: 'DRAFT',
		className: 'bg-[#d1d5db] text-gray-900',
	},
	scheduled: {
		label: 'SCHEDULED',
		className: 'bg-[#f5d78e] text-gray-900',
	},
	in_progress: {
		label: 'IN PROGRESS',
		className: 'bg-[#93c5fd] text-gray-900',
	},
	done: {
		label: 'FINISHED',
		className: 'bg-[#bef264] text-gray-900',
	},
	reopened: {
		label: 'REOPENED',
		className: 'bg-[#fdba74] text-gray-900',
	},
	rolled_back: {
		label: 'ROLLED BACK',
		className: 'bg-[#fca5a5] text-gray-900',
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
				'inline-flex shrink-0 items-center rounded-full font-bold uppercase tracking-wide',
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
