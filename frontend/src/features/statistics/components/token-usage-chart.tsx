import {
	Area,
	AreaChart,
	CartesianGrid,
	Legend,
	ResponsiveContainer,
	Tooltip,
	XAxis,
	YAxis,
} from 'recharts'
import type { StatsBucket, UsagePoint } from '../api/statistics'
import { formatBucketLabel, formatTokens } from '../lib/format'
import { ChartFrame } from './chart-frame'

interface TokenUsageChartProps {
	series: UsagePoint[]
	bucket: StatsBucket
}

/**
 * TokenUsageChart stacks prompt and completion tokens only.
 *
 * cachedPromptTokens and reasoningTokens are deliberately NOT stacked here:
 * both providers report them as subsets of prompt and completion respectively,
 * so adding them to the stack would double-count by up to 2x. They are surfaced
 * as "of which" figures in the tiles instead.
 */
export function TokenUsageChart({ series, bucket }: TokenUsageChartProps) {
	const hasData = series.some((p) => p.totalTokens > 0)

	return (
		<ChartFrame
			title="Token usage over time"
			subtitle="Prompt and completion tokens. Cached and reasoning tokens are subsets of these and are not stacked."
			isEmpty={!hasData}
		>
			<ResponsiveContainer width="100%" height="100%">
				<AreaChart data={series} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
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
						tickFormatter={(v) => formatTokens(Number(v))}
						tick={{ fontSize: 11, fill: 'var(--color-muted-foreground)' }}
						tickLine={false}
						axisLine={false}
						width={56}
					/>
					<Tooltip
						contentStyle={{
							background: 'var(--color-popover)',
							border: '1px solid var(--color-border)',
							borderRadius: 8,
							fontSize: 12,
						}}
						labelFormatter={(label) => formatBucketLabel(String(label), bucket)}
						formatter={(value, name) => [formatTokens(Number(value)), String(name)]}
					/>
					<Legend wrapperStyle={{ fontSize: 12 }} />
					<Area
						type="monotone"
						dataKey="promptTokens"
						name="Prompt"
						stackId="tokens"
						stroke="var(--color-chart-1)"
						fill="var(--color-chart-1)"
						fillOpacity={0.35}
					/>
					<Area
						type="monotone"
						dataKey="completionTokens"
						name="Completion"
						stackId="tokens"
						stroke="var(--color-chart-2)"
						fill="var(--color-chart-2)"
						fillOpacity={0.35}
					/>
				</AreaChart>
			</ResponsiveContainer>
		</ChartFrame>
	)
}
