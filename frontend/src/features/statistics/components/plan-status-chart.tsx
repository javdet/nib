import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from 'recharts'
import type { PlanStatus } from '@/features/dialogs/api/dialogs'
import { STATUS_STYLES } from '@/features/workplace/components/plan-status-badge'
import type { KeyCount } from '../api/statistics'
import { STATUS_CHART_COLORS } from '../lib/status-colors'
import { formatStatusLabel } from '../lib/format'
import { ChartFrame } from './chart-frame'

interface PlanStatusChartProps {
	byStatus: KeyCount[]
	asOf: string
}

function statusLabel(key: string): string {
	// Labels come from STATUS_STYLES so a chart legend and a plan badge can
	// never disagree about what a status is called.
	const style = STATUS_STYLES[key as PlanStatus]
	return style ? style.label : formatStatusLabel(key)
}

export function PlanStatusChart({ byStatus, asOf }: PlanStatusChartProps) {
	// Zero-count statuses are dropped from the donut itself (a zero slice draws
	// nothing) but stay in the table beside it, so "none right now" is still
	// visible and does not read as "not a thing".
	const slices = byStatus.filter((s) => s.count > 0)
	const total = byStatus.reduce((sum, s) => sum + s.count, 0)
	const asOfLabel = asOf ? new Date(asOf).toLocaleString('en-US') : ''

	return (
		<ChartFrame
			title="Tasks by status"
			subtitle={
				asOfLabel
					? `Current breakdown of all ${total.toLocaleString('en-US')} plans, as of ${asOfLabel}. Not limited to the selected range.`
					: undefined
			}
			isEmpty={total === 0}
			emptyMessage="No plans yet."
		>
			<div className="flex h-full items-center gap-4">
				<div className="h-full flex-1">
					<ResponsiveContainer width="100%" height="100%">
						<PieChart>
							<Pie
								data={slices}
								dataKey="count"
								nameKey="key"
								innerRadius="55%"
								outerRadius="80%"
								paddingAngle={2}
								stroke="none"
							>
								{slices.map((s) => (
									<Cell
										key={s.key}
										fill={STATUS_CHART_COLORS[s.key as PlanStatus] ?? 'var(--color-chart-1)'}
									/>
								))}
							</Pie>
							<Tooltip
								contentStyle={{
									background: 'var(--color-popover)',
									border: '1px solid var(--color-border)',
									borderRadius: 8,
									fontSize: 12,
								}}
								formatter={(value, name) => [Number(value), statusLabel(String(name))]}
							/>
						</PieChart>
					</ResponsiveContainer>
				</div>

				<ul className="flex-1 space-y-1 text-sm">
					{byStatus.map((s) => (
						<li key={s.key} className="flex items-center justify-between gap-2">
							<span className="flex items-center gap-2 text-muted-foreground">
								<span
									aria-hidden
									className="h-2.5 w-2.5 shrink-0 rounded-full"
									style={{
										background:
											STATUS_CHART_COLORS[s.key as PlanStatus] ?? 'var(--color-chart-1)',
									}}
								/>
								{statusLabel(s.key)}
							</span>
							<span className="tabular font-medium">{s.count.toLocaleString('en-US')}</span>
						</li>
					))}
				</ul>
			</div>
		</ChartFrame>
	)
}
