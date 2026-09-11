import { useMemo } from 'react'
import { cn } from '@/lib/utils'
import type { ActionPlan } from '@/features/dialogs/api/dialogs'
import { buildPlanItemKeys } from '../lib/plan-item-keys'

interface PlanProgressBarProps {
	plan: ActionPlan | null
	checked: string[]
}

export function PlanProgressBar({ plan, checked }: PlanProgressBarProps) {
	const keys = useMemo(() => buildPlanItemKeys(plan), [plan])
	const checkedSet = useMemo(() => new Set(checked), [checked])

	const total = keys.length
	const done = keys.filter((key) => checkedSet.has(key)).length
	const percent = total > 0 ? Math.round((done / total) * 100) : 0

	return (
		<div className="w-full space-y-2">
			<div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
				<span>Progress</span>
				<span>
					{done} / {total}
					{total > 0 ? ` (${percent}%)` : ''}
				</span>
			</div>
			<div
				role="progressbar"
				aria-label="Plan progress"
				aria-valuemin={0}
				aria-valuemax={total}
				aria-valuenow={done}
				className={cn(
					'progress-glass flex h-5 w-full overflow-hidden rounded-full p-1',
					keys.length > 0 && 'gap-1',
				)}
			>
				{keys.length > 0 ? (
					keys.map((key) => (
						<div
							key={key}
							className={cn(
								'h-full min-w-0 flex-1 rounded-full bg-transparent transition-colors duration-300',
								checkedSet.has(key) && 'progress-glass-fill',
							)}
						/>
					))
				) : (
					<div className="h-full w-full rounded-full" />
				)}
			</div>
		</div>
	)
}
