import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace } from '../fixtures/mock-api'
import { makeDialog } from '../fixtures/data'

const incident = makeDialog({
	id: 'inc-a',
	title: 'Checkout latency spike',
	mode: 'incident',
	planStatus: undefined,
})

test.describe('Incidents', () => {
	// 9.1
	test('shows the empty state', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await page.goto('/incidents')

		await expect(
			page.getByRole('heading', { name: 'Incidents', level: 2 }),
		).toBeVisible()
		await expect(page.getByText('New incidents start in incident mode.')).toBeVisible()
		await expect(page.getByText('Recent incidents')).toBeVisible()
		await expect(page.getByRole('button', { name: 'New incident' })).toBeVisible()
		await expect(page.getByText('No incidents yet.')).toBeVisible()
	})

	// 9.2
	test('is badged as in development', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await page.goto('/workplace')

		await expect(
			page.getByRole('link', { name: 'Incidents In dev' }),
		).toBeVisible()
	})

	// 9.1 — the list is scoped to incident-mode dialogs.
	test('asks the backend only for incident dialogs', async ({ page }) => {
		await mockEmptyWorkspace(page)
		const request = page.waitForRequest(
			(r) => r.url().includes('/api/v1/dialogs?') && r.url().includes('mode=incident'),
		)
		await page.goto('/incidents')
		await request
	})

	// 9.1 (rows)
	test('lists incidents', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await mockDialogList(page, { items: [incident] })
		await page.goto('/incidents')

		await expect(page.getByText('Checkout latency spike')).toBeVisible()
	})

	// 9.3 — mocked: creating an incident starts an incident-mode dialog.
	test('creates an incident in incident mode', async ({ page, apiGuard }) => {
		await mockEmptyWorkspace(page)
		apiGuard.allow('POST', '/api/v1/dialogs')
		await mockJson(page, '/api/v1/dialogs', incident, { method: 'POST' })

		await page.goto('/incidents')
		const request = page.waitForRequest(
			(r) => r.url().endsWith('/api/v1/dialogs') && r.method() === 'POST',
		)
		await page.getByRole('button', { name: 'New incident' }).click()

		expect((await request).postDataJSON()).toMatchObject({ mode: 'incident' })
		await expect(page).toHaveURL(/\/incidents\/inc-a$/)
	})

	// 9.4
	test('searches incidents by name', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await mockDialogList(page, {
			items: [incident],
			searchResults: [],
		})
		await page.goto('/incidents')

		await page
			.getByRole('searchbox', { name: 'Search incidents by name...' })
			.fill('nothing-matches')
		await page.getByRole('button', { name: 'Search' }).click()

		await expect(page.getByText('Checkout latency spike')).toHaveCount(0)
	})
})
