/**
 * Fixture payloads shaped after frontend/src/features/dialogs/api/dialogs.ts.
 * Kept local (not imported from frontend/src) so the suite stays independent of
 * the app's `@/` path alias.
 */

export type PlanStatus =
	| 'draft'
	| 'scheduled'
	| 'in_progress'
	| 'done'
	| 'reopened'
	| 'rolled_back'
	| 'cancelled'

export interface Dialog {
	id: string
	title: string
	mode: string
	parentId?: string
	categories?: string[]
	pinned: boolean
	planStatus?: PlanStatus
	createdAt: string
	updatedAt: string
}

export interface DialogMessage {
	role: string
	content: string
	toolCalls?: {
		id: string
		type?: string
		function: { name: string; arguments: string }
	}[]
	toolCallId?: string
	name?: string
}

const ISO = '2026-01-15T10:00:00Z'

export function makeDialog(overrides: Partial<Dialog> = {}): Dialog {
	return {
		id: '11111111-1111-4111-8111-111111111111',
		title: 'Deploy the billing service to stage',
		mode: 'main',
		pinned: false,
		planStatus: 'draft',
		createdAt: ISO,
		updatedAt: ISO,
		...overrides,
	}
}

export function makeDialogs(count: number, prefix = 'Plan'): Dialog[] {
	return Array.from({ length: count }, (_, i) =>
		makeDialog({
			id: `1111111${i}-1111-4111-8111-11111111111${i}`,
			title: `${prefix} ${i + 1}`,
		}),
	)
}

export const PLAN_STATUS_LABELS: Record<PlanStatus, string> = {
	draft: 'DRAFT',
	scheduled: 'SCHEDULED',
	in_progress: 'IN PROGRESS',
	done: 'FINISHED',
	reopened: 'REOPENED',
	rolled_back: 'ROLLED BACK',
	cancelled: 'CANCELLED',
}

export const REPORT_MARKDOWN = [
	'## Outcome',
	'',
	'Billing reached **stage** and every check passed.',
	'',
	'## Results',
	'',
	'- namespace `billing-stage`',
].join('\n')

export const SUMMARY_MARKDOWN = [
	'## Goal',
	'',
	'Roll the billing service out to **stage** behind a feature flag.',
	'',
	'- capacity checked',
	'- rollback prepared',
].join('\n')

export const DAG_MARKDOWN = [
	'```mermaid',
	'graph TD',
	'  A[Prepare release] --> B[Deploy to stage]',
	'  B --> C[Smoke test]',
	'```',
].join('\n')

export function makeActionPlan() {
	return {
		plan: {
			stages: [
				{
					number: 1,
					title: 'Prepare the release',
					description: 'Cut the image and stage the chart values.',
					steps: [
						{
							number: '1.1',
							type: 'command',
							action: 'Build the release image',
							command: 'make release',
						},
						{
							number: '1.2',
							type: 'code',
							action: 'Bump the chart version',
							repository: 'acme/helm-charts',
						},
					],
					checks: [
						{
							number: '1.1',
							check: 'Image is present in the registry',
							expectation: 'The new tag is listed',
						},
					],
				},
				{
					number: 2,
					title: 'Deploy to stage',
					description: 'Apply the chart and watch the rollout.',
					steps: [
						{
							number: '2.1',
							type: 'command',
							action: 'Apply the chart',
							command: 'helm upgrade --install billing ./chart',
						},
					],
					checks: [],
				},
			],
			rollback: [
				{
					number: 'R1',
					type: 'command',
					action: 'Roll the release back',
					command: 'helm rollback billing',
				},
			],
		},
		checked: [] as string[],
		comments: {} as Record<string, string>,
		planStatus: 'draft' as PlanStatus,
	}
}

export function makePlanState(
	status: PlanStatus = 'draft',
	scheduledAt = 0,
): { status: PlanStatus; scheduledAt: number } {
	return { status, scheduledAt }
}

export function makeMessages(): DialogMessage[] {
	return [
		{ role: 'user', content: 'Deploy billing to stage' },
		{
			role: 'assistant',
			content: 'Starting decomposition.\n\n```sh\nhelm ls -n stage\n```',
		},
	]
}

/** A transcript whose last assistant turn is a dangling ask_question call. */
export function makeAskQuestionMessages(
	toolCallId = 'call_ask_1',
): DialogMessage[] {
	return [
		{ role: 'user', content: 'Deploy billing to stage' },
		{
			role: 'assistant',
			content: '',
			toolCalls: [
				{
					id: toolCallId,
					type: 'function',
					function: {
						name: 'ask_question',
						arguments: JSON.stringify({
							questions: [{ question: 'Which cluster is stage on?' }],
						}),
					},
				},
			],
		},
	]
}

/**
 * Empty statistics payload.
 *
 * Cost fields are null rather than 0 on purpose: that is what the backend sends
 * when no provider priced a call, and the UI is expected to render a dash there
 * rather than "$0.00".
 */
export function makeStatistics(overrides: Record<string, unknown> = {}) {
	return {
		range: {
			from: '2026-08-14T00:00:00Z',
			to: '2026-09-13T00:00:00Z',
			bucket: 'day',
		},
		plans: {
			total: 0,
			createdInRange: 0,
			asOf: '2026-09-13T00:00:00Z',
			currentByStatus: [],
			byMode: [],
			createdSeries: [],
			transitionSeries: [],
		},
		usage: {
			totals: {
				calls: 0,
				costedCalls: 0,
				failedCalls: 0,
				promptTokens: 0,
				cachedPromptTokens: 0,
				completionTokens: 0,
				reasoningTokens: 0,
				totalTokens: 0,
				costUsd: null,
				avgDurationMs: 0,
			},
			series: [],
			byModel: [],
			byMode: [],
			byOperation: [],
			topPlans: [],
		},
		agentRuns: {
			totals: {
				runs: 0,
				costedRuns: 0,
				turns: 0,
				inputTokens: 0,
				outputTokens: 0,
				costUsd: null,
				avgDurationMs: 0,
			},
			byStatus: [],
			series: [],
		},
		...overrides,
	}
}
