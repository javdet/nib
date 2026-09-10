import type { Page } from '@playwright/test'
import { mockJson, mockSSE } from './nib-test'
import {
	makeActionPlan,
	makeMessages,
	makePlanState,
	type Dialog,
	type DialogMessage,
} from './data'

const V1 = '/api/v1'

export interface DialogListOptions {
	items?: Dialog[]
	pinned?: Dialog[]
	total?: number
	/** Rows returned when the list is queried with ?search=. */
	searchResults?: Dialog[]
}

/** Fulfils GET /dialogs for the plan and incident lists, pinned scope included. */
export async function mockDialogList(
	page: Page,
	options: DialogListOptions = {},
) {
	const items = options.items ?? []
	const pinned = options.pinned ?? []

	await page.route(
		(url) => url.pathname === `${V1}/dialogs`,
		async (route) => {
			if (route.request().method() !== 'GET') return route.fallback()

			const url = new URL(route.request().url())
			const search = url.searchParams.get('search')
			const scope = url.searchParams.get('scope')

			let body: Dialog[]
			if (scope === 'pinned') {
				body = pinned
			} else if (search) {
				body =
					options.searchResults ??
					items.filter((d) =>
						d.title.toLowerCase().includes(search.toLowerCase()),
					)
			} else {
				body = items
			}

			await route.fulfill({
				status: 200,
				contentType: 'application/json',
				headers: {
					'X-Total-Count': String(options.total ?? body.length),
					'Access-Control-Expose-Headers': 'X-Total-Count',
				},
				body: JSON.stringify(body),
			})
		},
	)
}

export interface PlanMockOptions {
	dialog: Dialog
	summary?: string
	dag?: string
	actionPlan?: ReturnType<typeof makeActionPlan> | null
	planState?: ReturnType<typeof makePlanState>
	messages?: DialogMessage[]
	children?: Dialog[]
	execRuns?: Record<string, unknown>
	fanout?: unknown
}

/** Fulfils the whole fan-out of requests the plan detail page makes. */
export async function mockPlan(page: Page, options: PlanMockOptions) {
	const { dialog } = options
	const base = `${V1}/dialogs/${dialog.id}`

	await mockJson(page, base, dialog)
	await mockJson(page, `${base}/summary`, {
		content: options.summary ?? '',
	})
	await mockJson(page, `${base}/dag`, { content: options.dag ?? '' })
	await mockJson(
		page,
		`${base}/action-plan`,
		options.actionPlan === undefined ? makeActionPlan() : options.actionPlan,
	)
	await mockJson(
		page,
		`${base}/plan-state`,
		options.planState ?? makePlanState(dialog.planStatus ?? 'draft'),
	)
	await mockJson(page, `${base}/children`, options.children ?? [])
	await mockJson(page, `${base}/messages`, options.messages ?? makeMessages())
	await mockJson(page, `${base}/attachments`, [])
	await mockJson(page, `${base}/action-plan/exec`, options.execRuns ?? {})
	await mockJson(page, `${base}/plan-fanout`, options.fanout ?? null)
	await mockJson(page, `${V1}/execution`, null)
	await mockSSE(page, dialog.id)
}

/** Every list endpoint the shell touches returns an empty collection. */
export async function mockEmptyWorkspace(page: Page) {
	await mockDialogList(page)
	await mockJson(page, `${V1}/projects`, [])
	await mockJson(page, `${V1}/selection`, {})
	await mockJson(page, `${V1}/execution`, null)
}
