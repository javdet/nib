import { test, expect, mockJson, isApiRequest } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace } from '../fixtures/mock-api'
import { makeDialog } from '../fixtures/data'

const ROUTES = ['/workplace', '/incidents', '/knowledge', '/rules', '/tools', '/skills', '/variables']

test.describe('Resilience', () => {
	// 19.1
	for (const route of ROUTES) {
		test(`${route} survives a dead backend`, async ({ page }) => {
			await page.route(isApiRequest, (r) => r.abort('connectionrefused'))

			const crashes: string[] = []
			page.on('pageerror', (err) => crashes.push(err.message))

			await page.goto(route)

			// The shell is client-side: it must render and stay navigable.
			await expect(page.getByRole('navigation')).toBeVisible()
			await expect(page.getByRole('heading', { level: 2 })).toBeVisible()
			await expect(page.getByRole('link', { name: 'Tools' })).toBeEnabled()
			expect(crashes, crashes.join('\n')).toEqual([])
		})
	}

	// 19.3
	test('reports a rejected write and keeps the control usable', async ({
		page,
		apiGuard,
	}) => {
		await mockEmptyWorkspace(page)
		apiGuard.allow('POST', '/api/v1/dialogs')
		await mockJson(
			page,
			'/api/v1/dialogs',
			{ error: 'database is read-only' },
			{ status: 500, method: 'POST' },
		)

		await page.goto('/workplace')
		await page.getByRole('button', { name: 'New plan' }).click()

		await expect(page.getByText(/database is read-only/)).toBeVisible()
		await expect(page).toHaveURL(/\/workplace$/)
		await expect(page.getByRole('button', { name: 'New plan' })).toBeEnabled()
	})

	// 19.4
	test('shows a loading state for a slow list', async ({ page }) => {
		await mockEmptyWorkspace(page)
		let served = 0
		await page.route(
			(url) => url.pathname === '/api/v1/dialogs',
			async (route) => {
				served += 1
				await new Promise((resolve) => setTimeout(resolve, 2500))
				await route.fulfill({
					status: 200,
					contentType: 'application/json',
					headers: { 'X-Total-Count': '0' },
					body: '[]',
				})
			},
		)

		await page.goto('/workplace')
		await expect(page.getByText('Loading...')).toBeVisible()
		await expect(page.getByText('No plans yet.')).toBeVisible()

		// Two scopes (pinned and paged), each double-invoked by React StrictMode in
		// dev. What matters is that a slow response does not trigger a retry storm.
		expect(served).toBeLessThanOrEqual(4)
	})

	// 19.5 — the activity stream is cut mid-turn.
	test('recovers when the activity stream dies', async ({ page }) => {
		const dialog = makeDialog({ id: 'plan-a' })
		await mockEmptyWorkspace(page)
		await mockDialogList(page, { items: [dialog] })
		await mockJson(page, `/api/v1/dialogs/${dialog.id}`, dialog)
		await mockJson(page, `/api/v1/dialogs/${dialog.id}/messages`, [])
		await page.route(
			(url) => url.pathname === `/api/v1/dialogs/${dialog.id}/events`,
			(route) => route.abort('connectionreset'),
		)

		const crashes: string[] = []
		page.on('pageerror', (err) => crashes.push(err.message))

		await page.goto(`/workplace/${dialog.id}`)
		await expect(page.getByRole('button', { name: 'Back to list' })).toBeVisible()
		expect(crashes, crashes.join('\n')).toEqual([])
	})

	// 19.6
	test('two tabs on one plan do not corrupt each other', async ({ browser }) => {
		const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })

		const contexts = await Promise.all([browser.newContext(), browser.newContext()])
		const pages = await Promise.all(contexts.map((c) => c.newPage()))

		for (const page of pages) {
			await page.route(isApiRequest, (route) =>
				route.request().method() === 'GET'
					? route.continue()
					: route.abort('blockedbyclient'),
			)
			await mockEmptyWorkspace(page)
			await mockDialogList(page, { items: [dialog] })
			await page.goto('/workplace')
			await expect(page.getByText('Deploy billing')).toBeVisible()
		}

		await Promise.all(contexts.map((c) => c.close()))
	})
})
