import {
	Bar,
	BarChart,
	CartesianGrid,
	ResponsiveContainer,
	Tooltip,
	XAxis,
	YAxis,
} from 'recharts'
import type { StatsBucket, UsagePoint } from '../api/statistics'
import { formatBucketLabel, formatCost } from '../lib/format'
import { ChartFrame } from './chart-frame'

interface CostChartProps {
	series: UsagePoint[]
	bucket: StatsBucket
	/** How many calls in the range carried a price at all. */
	costedCalls: number
	totalCalls: number
}

/**
 * CostChart plots only what the provider priced.
 *
 * nib carries no rate card, so cost exists only where a gateway reports it --
 * OpenRouter does, the OpenAI platform does not. When nothing was priced the
 * chart says so rather than drawing a flat zero line, which would read as "this
 * was free" instead of "nobody told us".
 */
export function CostChart({ series, bucket, costedCalls, totalCalls }: CostChartProps) {
	const hasCost = series.some((p) => p.costUsd != null && p.costUsd > 0)

	return (
		<ChartFrame
			title="Cost over time"
			subtitle={
				costedCalls > 0
					? `Cost reported for ${costedCalls.toLocaleString('en-US')} of ${totalCalls.toLocaleString('en-US')} calls.`
					: 'This provider does not report per-call cost.'
			}
			isEmpty={!hasCost}
			emptyMessage={
				totalCalls > 0
					? 'No cost was reported for this range. The configured provider prices nothing back to nib.'
					: 'No calls in this range yet.'
			}
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
						tickFormatter={(v) => formatCost(Number(v))}
						tick={{ fontSize: 11, fill: 'var(--color-muted-foreground)' }}
						tickLine={false}
						axisLine={false}
						width={72}
					/>
					<Tooltip
						contentStyle={{
							background: 'var(--color-popover)',
							border: '1px solid var(--color-border)',
							borderRadius: 8,
							fontSize: 12,
						}}
						labelFormatter={(label) => formatBucketLabel(String(label), bucket)}
						formatter={(value) => [formatCost(Number(value)), 'Cost']}
					/>
					<Bar dataKey="costUsd" name="Cost" fill="var(--color-chart-4)" radius={[3, 3, 0, 0]} />
				</BarChart>
			</ResponsiveContainer>
		</ChartFrame>
	)
}
