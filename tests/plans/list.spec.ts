import { test, expect } from '../fixtures/nib-test'
import { mockJson } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace } from '../fixtures/mock-api'
import { makeDialog, makeDialogs } from '../fixtures/data'

test.describe('Workplace plan list', () => {
	// 3.1
	test('shows the empty state', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await page.goto('/workplace')

		await expect(
			page.getByRole('heading', { name: 'Plans', level: 2 }),
		).toBeVisible()
		await expect(
			page.getByText('New plans start in decompose mode.'),
		).toBeVisible()
		await expect(page.getByRole('button', { name: 'New plan' })).toBeVisible()
		await expect(page.getByText('No plans yet.')).toBeVisible()
	})

	// 3.2
	test('renders a row per plan with its status and controls', async ({
		page,
	}) => {
		const items = [
			makeDialog({ id: 'plan-a', title: 'Deploy billing', planStatus: 'draft' }),
			makeDialog({
				id: 'plan-b',
				title: 'Rotate the TLS certificates',
				planStatus: 'in_progress',
			}),
		]
		await mockEmptyWorkspace(page)
		await mockDialogList(page, { items })
		await page.goto('/workplace')

		await expect(page.getByText('Deploy billing')).toBeVisible()
		await expect(page.getByText('Rotate the TLS certificates')).toBeVisible()
		await expect(page.getByText('DRAFT')).toBeVisible()
		await expect(page.getByText('IN PROGRESS')).toBeVisible()
		await expect(page.getByRole('button', { name: 'Pin plan', exact: true })).toHaveCount(2)
		await expect(page.getByRole('button', { name: 'Delete plan', exact: true })).toHaveCount(
			2,
		)
	})

	// 3.2 (pinned group)
	test('lists pinned plans above the rest', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await mockDialogList(page, {
			items: [makeDialog({ id: 'plan-a', title: 'Ordinary plan' })],
			pinned: [
				makeDialog({ id: 'plan-p', title: 'Pinned plan', pinned: true }),
			],
		})
		await page.goto('/workplace')

		await expect(page.getByText('Pinned plan')).toBeVisible()
		await expect(page.getByRole('button', { name: 'Unpin plan', exact: true })).toBeVisible()
	})

	// 3.3
	test('opens a plan', async ({ page }) => {
		const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })
		await mockEmptyWorkspace(page)
		await mockDialogList(page, { items: [dialog] })
		await page.goto('/workplace')

		await page.getByText('Deploy billing').click()
		await expect(page).toHaveURL(/\/workplace\/plan-a$/)
	})

	// 3.4 — the create call is mocked; never let this reach the real backend.
	test('creates a plan in main mode', async ({ page, apiGuard }) => {
		const created = makeDialog({ id: 'plan-new', title: 'New plan' })
		await mockEmptyWorkspace(page)
		apiGuard.allow('POST', '/api/v1/dialogs')
		await mockJson(page, '/api/v1/dialogs', created, { method: 'POST' })

		await page.goto('/workplace')
		const request = page.waitForRequest(
			(r) => r.url().endsWith('/api/v1/dialogs') && r.method() === 'POST',
		)
		await page.getByRole('button', { name: 'New plan' }).click()

		expect((await request).postDataJSON()).toMatchObject({ mode: 'main' })
		await expect(page).toHaveURL(/\/workplace\/plan-new$/)
	})

	// 3.5
	test('searches by name', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await mockDialogList(page, {
			items: makeDialogs(3, 'Plan'),
			searchResults: [makeDialog({ id: 'plan-x', title: 'Rotate certs' })],
		})
		await page.goto('/workplace')
		await expect(page.getByText('Plan 1')).toBeVisible()

		const search = page.waitForRequest((r) =>
			r.url().includes('/api/v1/dialogs?') && r.url().includes('search='),
		)
		await page.getByRole('searchbox', { name: 'Search plans by name...' }).fill(
			'rotate',
		)
		await page.getByRole('button', { name: 'Search' }).click()

		expect(decodeURIComponent((await search).url())).toContain('search=rotate')
		await expect(page.getByText('Rotate certs')).toBeVisible()
		await expect(page.getByText('Plan 1')).toHaveCount(0)
	})

	// 3.6
	test('reports a search with no matches', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await mockDialogList(page, {
			items: makeDialogs(2),
			searchResults: [],
		})
		await page.goto('/workplace')

		await page.getByRole('searchbox', { name: 'Search plans by name...' }).fill(
			'nothing-matches-this',
		)
		await page.getByRole('button', { name: 'Search' }).click()
		await expect(page.getByText('No plans match your search.')).toBeVisible()
	})

	// 3.7
	test('pins a plan without opening it', async ({ page, apiGuard }) => {
		const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })
		await mockEmptyWorkspace(page)
		await mockDialogList(page, { items: [dialog] })
		apiGuard.allow('PUT', '/api/v1/dialogs/plan-a/pin')
		await mockJson(page, '/api/v1/dialogs/plan-a/pin', { ...dialog, pinned: true })

		await page.goto('/workplace')
		const request = page.waitForRequest(
			(r) => r.url().includes('/plan-a/pin') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Pin plan', exact: true }).click()

		expect((await request).postDataJSON()).toMatchObject({ pinned: true })
		// The pin button must not bubble into the row's own click handler.
		await expect(page).toHaveURL(/\/workplace$/)
	})

	// 3.8
	test('deletes a plan behind a confirmation', async ({ page, apiGuard }) => {
		const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })
		await mockEmptyWorkspace(page)
		await mockDialogList(page, { items: [dialog] })
		await page.goto('/workplace')

		await page.getByRole('button', { name: 'Delete plan', exact: true }).click()
		const confirm = page.getByRole('dialog')
		await expect(confirm).toContainText('Delete plan')
		await expect(confirm).toContainText('cannot be undone')

		await confirm.getByRole('button', { name: 'Cancel' }).click()
		await expect(confirm).toHaveCount(0)
		await expect(page.getByText('Deploy billing')).toBeVisible()

		apiGuard.allow('DELETE', '/api/v1/dialogs/plan-a')
		await mockJson(page, '/api/v1/dialogs/plan-a', null, {
			status: 204,
			method: 'DELETE',
		})
		await mockDialogList(page, { items: [] })

		await page.getByRole('button', { name: 'Delete plan', exact: true }).click()
		const request = page.waitForRequest(
			(r) => r.url().endsWith('/api/v1/dialogs/plan-a') && r.method() === 'DELETE',
		)
		await page
			.getByRole('dialog')
			.getByRole('button', { name: 'Delete', exact: true })
			.click()

		await request
		await expect(page.getByText('Deploy billing')).toHaveCount(0)
	})

	// 3.10
	test('survives a failing list request', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await mockJson(page, '/api/v1/dialogs', { error: 'boom' }, { status: 500 })
		await page.goto('/workplace')

		await expect(
			page.getByRole('heading', { name: 'Plans', level: 2 }),
		).toBeVisible()
		await expect(page.getByRole('button', { name: 'New plan' })).toBeEnabled()
	})
})
