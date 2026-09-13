import {
	Bar,
	BarChart,
	CartesianGrid,
	ResponsiveContainer,
	Tooltip,
	XAxis,
	YAxis,
} from 'recharts'
import type { CountPoint, StatsBucket } from '../api/statistics'
import { formatBucketLabel } from '../lib/format'
import { ChartFrame } from './chart-frame'

interface PlansCreatedChartProps {
	series: CountPoint[]
	bucket: StatsBucket
}

export function PlansCreatedChart({ series, bucket }: PlansCreatedChartProps) {
	const hasData = series.some((p) => p.count > 0)

	return (
		<ChartFrame
			title="Tasks created over time"
			isEmpty={!hasData}
			emptyMessage="No tasks were created in this range."
		>
			<ResponsiveContainer width="100%" height="100%">
				<BarChart data={series} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
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
						formatter={(value) => [Number(value), 'Created']}
					/>
					<Bar dataKey="count" name="Created" fill="var(--color-chart-3)" radius={[3, 3, 0, 0]} />
				</BarChart>
			</ResponsiveContainer>
		</ChartFrame>
	)
}
