import type { ReactNode } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

interface ChartFrameProps {
	title: string
	subtitle?: ReactNode
	/** Empty renders instead of the chart when there is nothing to draw. A blank
	 *  chart reads as a bug; a sentence explains itself. */
	isEmpty?: boolean
	emptyMessage?: string
	children: ReactNode
}

export function ChartFrame({
	title,
	subtitle,
	isEmpty = false,
	emptyMessage = 'No data in this range yet.',
	children,
}: ChartFrameProps) {
	return (
		<Card>
			<CardHeader className="pb-2">
				<CardTitle className="text-base">{title}</CardTitle>
				{subtitle ? (
					<p className="text-xs text-muted-foreground">{subtitle}</p>
				) : null}
			</CardHeader>
			<CardContent>
				{isEmpty ? (
					<div className="flex h-64 items-center justify-center text-sm text-muted-foreground">
						{emptyMessage}
					</div>
				) : (
					// Fixed height: ResponsiveContainer collapses to zero inside a
					// flex parent that has no height of its own.
					<div className="h-64 w-full">{children}</div>
				)}
			</CardContent>
		</Card>
	)
}
