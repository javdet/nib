import { useCallback, useEffect, useMemo, useState } from 'react'
import { extractErrorMessage } from '@/lib/api-client'
import { getStatistics, type StatsBucket, type Statistics } from '../api/statistics'
import { DEFAULT_PRESET, presetToParams, type RangePresetId } from '../lib/ranges'
import { formatCost, formatDuration, formatTokens } from '../lib/format'
import { BreakdownTable } from '../components/breakdown-table'
import { ChartFrame } from '../components/chart-frame'
import { CostChart } from '../components/cost-chart'
import { PlanStatusChart } from '../components/plan-status-chart'
import { PlansCreatedChart } from '../components/plans-created-chart'
import { RangeSelect } from '../components/range-select'
import { StatTile } from '../components/stat-tile'
import { StatusTransitionsChart } from '../components/status-transitions-chart'
import { TokenUsageChart } from '../components/token-usage-chart'
import { TopPlansTable } from '../components/top-plans-table'

export function StatisticsPage() {
	const [preset, setPreset] = useState<RangePresetId>(DEFAULT_PRESET)
	const [stats, setStats] = useState<Statistics | null>(null)
	const [loading, setLoading] = useState(true)
	const [error, setError] = useState('')

	const params = useMemo(() => presetToParams(preset), [preset])

	const load = useCallback(
		(signal: AbortSignal) => {
			setLoading(true)
			getStatistics(params, signal)
				.then((data) => {
					setStats(data)
					setError('')
				})
				.catch((err: unknown) => {
					if (signal.aborted) return
					setStats(null)
					setError(extractErrorMessage(err))
				})
				.finally(() => {
					if (!signal.aborted) setLoading(false)
				})
		},
		[params],
	)

	useEffect(() => {
		const controller = new AbortController()
		load(controller.signal)
		return () => controller.abort()
	}, [load])

	const usage = stats?.usage
	const plans = stats?.plans
	const runs = stats?.agentRuns
	// Optional all the way down: an error payload or an unexpected shape must
	// render the empty dashboard, not throw and white-screen the page.
	const bucket = stats?.range?.bucket ?? 'day'

	return (
		<div className="space-y-6">
			<div className="flex flex-wrap items-center justify-between gap-3">
				<h2 className="text-2xl font-bold tracking-tight">Statistics</h2>
				<RangeSelect value={preset} onChange={setPreset} disabled={loading} />
			</div>

			{error ? (
				<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
					{error}
				</div>
			) : null}

			{loading && !stats ? (
				<p className="text-sm text-muted-foreground">Loading...</p>
			) : null}

			{stats && usage?.totals && plans && runs?.totals ? (
				<>
					<section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
						<StatTile
							label="Tasks"
							value={plans.total.toLocaleString('en-US')}
							hint={`${plans.createdInRange.toLocaleString('en-US')} created in this range`}
						/>
						<StatTile
							label="Total tokens"
							value={formatTokens(usage.totals.totalTokens)}
							hint={`${formatTokens(usage.totals.cachedPromptTokens)} cached · ${formatTokens(
								usage.totals.reasoningTokens,
							)} reasoning`}
						/>
						<StatTile
							label="LLM cost"
							value={formatCost(usage.totals.costUsd)}
							hint={
								usage.totals.costedCalls > 0
									? `priced for ${usage.totals.costedCalls.toLocaleString('en-US')} of ${usage.totals.calls.toLocaleString('en-US')} calls`
									: 'provider reports no cost'
							}
						/>
						<StatTile
							label="Agent run cost"
							value={formatCost(runs.totals.costUsd)}
							hint={`${runs.totals.runs.toLocaleString('en-US')} runs · ${runs.totals.turns.toLocaleString('en-US')} turns`}
						/>
					</section>

					<section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
						<StatTile
							label="LLM calls"
							value={usage.totals.calls.toLocaleString('en-US')}
							hint={
								usage.totals.failedCalls > 0
									? `${usage.totals.failedCalls.toLocaleString('en-US')} failed`
									: 'all succeeded'
							}
						/>
						<StatTile
							label="Avg call duration"
							value={formatDuration(usage.totals.avgDurationMs)}
						/>
						<StatTile
							label="Prompt / completion"
							value={`${formatTokens(usage.totals.promptTokens)} / ${formatTokens(
								usage.totals.completionTokens,
							)}`}
						/>
						<StatTile
							label="Agent run tokens"
							value={`${formatTokens(runs.totals.inputTokens)} / ${formatTokens(
								runs.totals.outputTokens,
							)}`}
							hint="input / output"
						/>
					</section>

					<section className="grid gap-3 lg:grid-cols-2">
						<PlanStatusChart byStatus={plans.currentByStatus ?? []} asOf={plans.asOf} />
						<PlansCreatedChart series={plans.createdSeries ?? []} bucket={bucket} />
					</section>

					<section className="grid gap-3 lg:grid-cols-2">
						<TokenUsageChart series={usage.series ?? []} bucket={bucket} />
						<CostChart
							series={usage.series ?? []}
							bucket={bucket}
							costedCalls={usage.totals.costedCalls}
							totalCalls={usage.totals.calls}
						/>
					</section>

					<section className="grid gap-3 lg:grid-cols-2">
						<BreakdownTable title="By model" label="Model" rows={usage.byModel ?? []} />
						<BreakdownTable title="By mode" label="Mode" rows={usage.byMode ?? []} />
					</section>

					<section className="grid gap-3 lg:grid-cols-2">
						<BreakdownTable
							title="By operation"
							label="Operation"
							rows={usage.byOperation ?? []}
						/>
						<TopPlansTable rows={usage.topPlans ?? []} />
					</section>

					{/* Status history only accumulates from the day transitions started
					    being recorded, so this is empty on a fresh deployment. */}
					<StatusTransitions
						series={plans.transitionSeries ?? []}
						bucket={bucket}
					/>
				</>
			) : null}
		</div>
	)
}

function StatusTransitions({
	series,
	bucket,
}: {
	series: Statistics['plans']['transitionSeries']
	bucket: StatsBucket
}) {
	const rows = series ?? []
	if (rows.length === 0) {
		return (
			<ChartFrame
				title="Status changes over time"
				isEmpty
				emptyMessage="No status changes recorded yet. History starts accumulating from the first status change after this release."
			>
				<div />
			</ChartFrame>
		)
	}
	return <StatusTransitionsChart series={rows} bucket={bucket} />
}
