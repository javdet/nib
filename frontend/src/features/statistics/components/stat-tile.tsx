import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Card, CardContent } from '@/components/ui/card'

interface StatTileProps {
	label: string
	value: ReactNode
	hint?: ReactNode
	className?: string
}

/** StatTile is the KPI cell. There is no shared primitive for this, so it stays
 *  feature-local until a second page wants one. */
export function StatTile({ label, value, hint, className }: StatTileProps) {
	return (
		<Card className={cn('elev-1', className)}>
			<CardContent className="p-4">
				<div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
					{label}
				</div>
				<div className="tabular mt-2 text-2xl font-semibold">{value}</div>
				{hint ? (
					<div className="mt-1 text-xs text-muted-foreground">{hint}</div>
				) : null}
			</CardContent>
		</Card>
	)
}
