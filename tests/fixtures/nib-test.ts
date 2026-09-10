import { test as base, expect, type Page } from '@playwright/test'

/**
 * Safety net for a suite that drives a REAL backend with real LLM credentials
 * and a real executor (see specs/nib-web-app-test-plan.md §3).
 *
 * Every non-GET /api/ request is aborted unless the test explicitly allowed it,
 * so a stray click on Execute / Process plan / Send can never start a sub-agent
 * run on the operator's stack. Mocks installed by a test take precedence over
 * this guard: page.route handlers run last-registered-first, and this one is
 * registered before the test body runs.
 */
export interface BlockedCall {
	method: string
	url: string
}

export class ApiGuard {
	private readonly rules: { method: string; url: string }[] = []
	readonly blocked: BlockedCall[] = []

	/** Lets one write through, matched by method + URL substring. */
	allow(method: string, urlSubstring: string) {
		this.rules.push({ method: method.toUpperCase(), url: urlSubstring })
	}

	permits(method: string, url: string) {
		return this.rules.some(
			(rule) => rule.method === method && url.includes(rule.url),
		)
	}
}

/** True only for real backend calls, never for Vite's own module requests. */
export function isApiRequest(url: URL) {
	return url.pathname.startsWith('/api/')
}

export const test = base.extend<{ apiGuard: ApiGuard }>({
	apiGuard: [
		async ({ page }, use) => {
			const guard = new ApiGuard()

			// Matched on pathname, not by glob: in dev the Vite server also serves
			// source modules whose path contains "/api/"
			// (src/features/*/api/*.ts), and a glob like **/api/** swallows those
			// too -- aborting one blanks the whole app.
			await page.route(isApiRequest, async (route) => {
				const request = route.request()
				const method = request.method()
				if (method === 'GET' || guard.permits(method, request.url())) {
					return route.continue()
				}
				guard.blocked.push({ method, url: request.url() })
				return route.abort('blockedbyclient')
			})

			await use(guard)

			// A blocked write means the test clicked something it did not mock.
			expect(
				guard.blocked,
				'unmocked write reached the real backend',
			).toEqual([])
		},
		{ auto: true },
	],
})

export { expect }

type JsonInit = {
	status?: number
	headers?: Record<string, string>
	method?: string
}

/** Fulfils one endpoint, matched on pathname so /dialogs never eats /dialogs/x. */
export async function mockJson(
	page: Page,
	pathname: string | ((path: string) => boolean),
	body: unknown,
	init: JsonInit = {},
) {
	const matches =
		typeof pathname === 'string'
			? (path: string) => path === pathname
			: pathname

	await page.route(
		(url) => url.pathname.startsWith('/api/') && matches(url.pathname),
		async (route) => {
			if (init.method && route.request().method() !== init.method) {
				return route.fallback()
			}
			await route.fulfill({
				status: init.status ?? 200,
				contentType: 'application/json',
				headers: init.headers,
				body: JSON.stringify(body ?? null),
			})
		},
	)
}

/** Fulfils an endpoint with an error in the backend's `{"error": "..."}` shape. */
export async function mockApiError(
	page: Page,
	pathname: string,
	status: number,
	message: string,
	method?: string,
) {
	await mockJson(page, pathname, { error: message }, { status, method })
}

/**
 * Canned SSE body for /dialogs/{id}/events — the UI's activity stream.
 *
 * A fulfilled response is a *finished* stream, and EventSource reconnects as
 * soon as a stream ends: left alone that is a request every second, for every
 * open page, for the whole run. So the events are delivered once and every
 * reconnect is answered with a non-SSE 204, which puts the EventSource into its
 * terminal CLOSED state instead of retrying.
 */
export async function mockSSE(
	page: Page,
	dialogId: string,
	events: unknown[] = [],
) {
	let served = false
	await page.route(
		(url) => url.pathname === `/api/v1/dialogs/${dialogId}/events`,
		async (route) => {
			if (served || events.length === 0) {
				return route.fulfill({ status: 204, contentType: 'text/plain', body: '' })
			}
			served = true
			await route.fulfill({
				status: 200,
				contentType: 'text/event-stream',
				headers: { 'cache-control': 'no-cache', connection: 'keep-alive' },
				body: events.map((e) => `data: ${JSON.stringify(e)}\n\n`).join(''),
			})
		},
	)
}
