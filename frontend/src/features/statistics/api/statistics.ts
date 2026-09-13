import { api } from '@/lib/api-client'
import type { PlanStatus } from '@/features/dialogs/api/dialogs'

export type StatsBucket = 'hour' | 'day' | 'week' | 'month'

export interface StatsRangeInfo {
	from: string
	to: string
	bucket: StatsBucket
}

export interface KeyCount {
	key: string
	count: number
}

export interface CountPoint {
	ts: string
	count: number
}

export interface StatusCountPoint {
	ts: string
	status: PlanStatus
	count: number
}

export interface PlanStats {
	total: number
	createdInRange: number
	/** The by-status breakdown is a snapshot of now, not of the range: plan
	 *  status had no history before the transitions table. Shown as "as of". */
	asOf: string
	currentByStatus: KeyCount[] | null
	byMode: KeyCount[] | null
	createdSeries: CountPoint[] | null
	transitionSeries: StatusCountPoint[] | null
}

/** costUsd is null when nothing in the range was priced. The provider may not
 *  report cost at all (the OpenAI platform does not), so null means "unknown"
 *  and must never render as $0. */
export interface UsageTotals {
	calls: number
	costedCalls: number
	failedCalls: number
	promptTokens: number
	cachedPromptTokens: number
	completionTokens: number
	reasoningTokens: number
	totalTokens: number
	costUsd: number | null
	avgDurationMs: number
}

export interface UsagePoint {
	ts: string
	calls: number
	costedCalls: number
	promptTokens: number
	cachedPromptTokens: number
	completionTokens: number
	reasoningTokens: number
	totalTokens: number
	costUsd: number | null
}

export interface UsageKey {
	key: string
	calls: number
	totalTokens: number
	costUsd: number | null
}

export interface PlanUsage {
	planId: string
	title: string
	calls: number
	totalTokens: number
	costUsd: number | null
}

export interface UsageStats {
	totals: UsageTotals
	series: UsagePoint[] | null
	byModel: UsageKey[] | null
	byMode: UsageKey[] | null
	byOperation: UsageKey[] | null
	topPlans: PlanUsage[] | null
}

export interface AgentRunTotals {
	runs: number
	costedRuns: number
	turns: number
	inputTokens: number
	outputTokens: number
	costUsd: number | null
	avgDurationMs: number
}

export interface AgentRunPoint {
	ts: string
	runs: number
	costUsd: number | null
}

export interface AgentRunStats {
	totals: AgentRunTotals
	byStatus: KeyCount[] | null
	series: AgentRunPoint[] | null
}

export interface Statistics {
	range: StatsRangeInfo
	plans: PlanStats
	usage: UsageStats
	agentRuns: AgentRunStats
}

export interface StatisticsParams {
	from?: string
	to?: string
	bucket?: StatsBucket
}

export function getStatistics(
	params: StatisticsParams,
	signal?: AbortSignal,
): Promise<Statistics> {
	const query = new URLSearchParams()
	if (params.from) query.set('from', params.from)
	if (params.to) query.set('to', params.to)
	if (params.bucket) query.set('bucket', params.bucket)

	const qs = query.toString()
	return api.get<Statistics>(`/stats${qs ? `?${qs}` : ''}`, signal)
}
