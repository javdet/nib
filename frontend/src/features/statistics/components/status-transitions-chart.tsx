import { useMemo } from 'react'
import {
	Bar,
	BarChart,
	CartesianGrid,
	Legend,
	ResponsiveContainer,
	Tooltip,
	XAxis,
	YAxis,
} from 'recharts'
import type { PlanStatus } from '@/features/dialogs/api/dialogs'
import { STATUS_STYLES } from '@/features/workplace/components/plan-status-badge'
import type { StatsBucket, StatusCountPoint } from '../api/statistics'
import { STATUS_CHART_COLORS } from '../lib/status-colors'
import { formatBucketLabel, formatStatusLabel } from '../lib/format'
import { ChartFrame } from './chart-frame'

interface StatusTransitionsChartProps {
	series: StatusCountPoint[]
	bucket: StatsBucket
}

interface PivotRow {
	ts: string
	counts: Record<string, number>
}

/** Recharts reads each series off a flat row, so the pivot is flattened at the
 *  end rather than carried as a nested object. */
type ChartRow = Record<string, string | number>

/**
 * StatusTransitionsChart shows how many plans reached each status per bucket.
 *
 * The API returns one row per (bucket, status); recharts wants one row per
 * bucket with a key per series, so the pivot happens here. A missing pair is a
 * zero rather than a hole, which is why this series is not gap-filled server
 * side the way the token and cost series are.
 */
export function StatusTransitionsChart({ series, bucket }: StatusTransitionsChartProps) {
	const { rows, statuses } = useMemo(() => {
		const byTs = new Map<string, PivotRow>()
		const seen = new Set<string>()

		for (const point of series) {
			seen.add(point.status)
			const row = byTs.get(point.ts) ?? { ts: point.ts, counts: {} }
			row.counts[point.status] = (row.counts[point.status] ?? 0) + point.count
			byTs.set(point.ts, row)
		}

		const flattened: ChartRow[] = [...byTs.values()]
			.sort((a, b) => a.ts.localeCompare(b.ts))
			.map((row) => ({ ts: row.ts, ...row.counts }))

		return { rows: flattened, statuses: [...seen] }
	}, [series])

	return (
		<ChartFrame
			title="Status changes over time"
			subtitle="How many tasks moved into each status."
			isEmpty={rows.length === 0}
		>
			<ResponsiveContainer width="100%" height="100%">
				<BarChart data={rows} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
					<CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" vertical={false} />
					<XAxis
						dataKey="ts"
						tickFormatter={(ts) => formatBucketLabel(String(ts), bucket)}
						tick={{ fontSize: 11, fill: 'var(--color-muted-foreground)' }}
						tickLine={false}
						axisLine={false}
						minTickGap={24}
					/>
					<YAxis
						allowDecimals={false}
						tick={{ fontSize: 11, fill: 'var(--color-muted-foreground)' }}
						tickLine={false}
						axisLine={false}
						width={40}
					/>
					<Tooltip
						contentStyle={{
							background: 'var(--color-popover)',
							border: '1px solid var(--color-border)',
							borderRadius: 8,
							fontSize: 12,
						}}
						labelFormatter={(label) => formatBucketLabel(String(label), bucket)}
					/>
					<Legend wrapperStyle={{ fontSize: 12 }} />
					{statuses.map((status) => (
						<Bar
							key={status}
							dataKey={status}
							name={
								STATUS_STYLES[status as PlanStatus]?.label ?? formatStatusLabel(status)
							}
							stackId="status"
							fill={STATUS_CHART_COLORS[status as PlanStatus] ?? 'var(--color-chart-1)'}
						/>
					))}
				</BarChart>
			</ResponsiveContainer>
		</ChartFrame>
	)
}
