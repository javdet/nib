import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

const CATEGORIES = '/api/v1/tool-categories'

async function openCategories(page: import('@playwright/test').Page) {
	await mockEmptyWorkspace(page)
	await mockJson(page, '/api/v1/mcp/servers', [{ name: 'kb', url: 'http://kb/mcp' }])
	await mockJson(page, CATEGORIES, [
		{
			name: 'kubernetes',
			description: 'Cluster tools',
			patterns: ['kubernetes_*'],
			toolCount: 2,
		},
		{ name: 'git', description: 'Repo tools', patterns: [], toolCount: 0 },
	])
	await mockJson(page, `${CATEGORIES}/kubernetes/tools`, [
		{
			server: 'kb',
			name: 'kubernetes_get_pods',
			description: 'List pods',
			categories: ['kubernetes'],
		},
	])
	await mockJson(page, `${CATEGORIES}/git/tools`, [])
	await mockJson(page, `${CATEGORIES}/uncategorized/tools`, [
		{ server: 'kb', name: 'search_docs', description: 'Search docs', categories: null },
	])
	await page.goto('/tools')
	await page.getByRole('tab', { name: 'Categories' }).click()
	await expect(page.getByText('Tool categories')).toBeVisible()
}

test.describe('Tools — categories', () => {
	// 14.1
	test('lists the categories with their counts', async ({ page }) => {
		await openCategories(page)

		await expect(page.getByRole('button', { name: /^kubernetes/ })).toBeVisible()
		await expect(page.getByRole('button', { name: /^git/ })).toBeVisible()
		await expect(page.getByRole('button', { name: /^Uncategorized/ })).toBeVisible()
		await expect(
			page.getByText('Select a category to edit patterns and view matched tools.'),
		).toBeVisible()
		// Category names come from the toolCategories variable, not from this page.
		await expect(page.getByRole('link', { name: 'toolCategories' })).toHaveAttribute(
			'href',
			'/variables',
		)
	})

	// 14.2
	test('edits the patterns of a category', async ({ page, apiGuard }) => {
		await openCategories(page)
		apiGuard.allow('PUT', `${CATEGORIES}/kubernetes/patterns`)
		await mockJson(page, `${CATEGORIES}/kubernetes/patterns`, {
			name: 'kubernetes',
			description: 'Cluster tools',
			patterns: ['kubernetes_*', 'helm_*'],
			toolCount: 3,
		})

		await page.getByRole('button', { name: /^kubernetes/ }).click()
		const patterns = page.getByRole('textbox', { name: /Patterns/i })
		await expect(patterns).toHaveValue('kubernetes_*')
		await expect(page.getByText('kubernetes_get_pods')).toBeVisible()

		await patterns.fill('kubernetes_*\nhelm_*')
		const request = page.waitForRequest(
			(r) => r.url().includes('/kubernetes/patterns') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Save' }).click()

		expect((await request).postDataJSON()).toMatchObject({
			patterns: ['kubernetes_*', 'helm_*'],
		})
	})

	// 14.4
	test('shows what no category has claimed', async ({ page }) => {
		await openCategories(page)

		await page.getByRole('button', { name: /^Uncategorized/ }).click()
		await expect(page.getByText('search_docs')).toBeVisible()
	})

	// 14.3
	test('reports a category that matches nothing', async ({ page }) => {
		await openCategories(page)

		await page.getByRole('button', { name: /^git/ }).click()
		await expect(page.getByRole('textbox', { name: /Patterns/i })).toBeEmpty()
		await expect(page.getByText('kubernetes_get_pods')).toHaveCount(0)
	})

	/**
	 * 14.x — Auto-fill is an agent run: it opens a discuss chat that rewrites the
	 * patterns of every category it touches. Mocked, and worth a warning in the UI.
	 */
	test('warns that auto-fill replaces hand-written patterns', async ({ page }) => {
		await openCategories(page)

		await expect(page.getByRole('button', { name: 'Auto-fill with AI' })).toBeVisible()
		await expect(
			page.getByText(/replaces.*patterns of every category it assigns tools to/),
		).toBeVisible()
	})
})
