import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

const CATALOG = '/api/v1/mcp/catalog-tools'

function includedTools(mode: string, included: string[] = []) {
	return {
		mode,
		systemTools: [{ name: 'get_skill', description: 'Load a skill by name' }],
		includedTools: included,
	}
}

async function openIncluded(page: import('@playwright/test').Page) {
	await mockEmptyWorkspace(page)
	await mockJson(page, '/api/v1/mcp/servers', [{ name: 'kb', url: 'http://kb/mcp' }])
	await mockJson(page, CATALOG, [
		{ server: 'kb', name: 'kubernetes_get_pods', description: 'List pods' },
		{ server: 'kb', name: 'search_docs', description: 'Search the docs' },
	])
	await mockJson(page, '/api/v1/included-tools/main', includedTools('main'))
	await mockJson(page, '/api/v1/included-tools/plan', includedTools('plan'))
	await page.goto('/tools')
	await page.getByRole('tab', { name: 'Included tools' }).click()
	await expect(page.getByRole('combobox', { name: 'Mode' })).toBeVisible()
}

test.describe('Tools — included tools', () => {
	// 15.1
	test('splits the catalog into included and not included', async ({ page }) => {
		await openIncluded(page)

		await expect(page.getByRole('heading', { name: 'Not included' })).toBeVisible()
		await expect(page.getByRole('heading', { name: 'Included', exact: true })).toBeVisible()
		await expect(
			page.getByText('Discoverable via tool_search in this mode.'),
		).toBeVisible()
		await expect(
			page.getByText('Connected immediately in this mode.'),
		).toBeVisible()
		await expect(page.getByRole('button', { name: 'Include kubernetes_get_pods' })).toBeVisible()
	})

	// 15.2
	test('includes one tool and saves it for that mode', async ({ page, apiGuard }) => {
		await openIncluded(page)
		apiGuard.allow('PUT', '/api/v1/included-tools/main')
		await mockJson(
			page,
			'/api/v1/included-tools/main',
			includedTools('main', ['kb__kubernetes_get_pods']),
			{ method: 'PUT' },
		)

		await expect(page.getByRole('button', { name: 'Save' })).toBeDisabled()
		await page.getByRole('button', { name: 'Include kubernetes_get_pods' }).click()
		await expect(page.getByRole('button', { name: 'Save' })).toBeEnabled()

		const request = page.waitForRequest(
			(r) => r.url().includes('/included-tools/main') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Save' }).click()

		expect(JSON.stringify((await request).postDataJSON())).toContain(
			'kubernetes_get_pods',
		)
	})

	// 15.3
	test('includes every tool at once', async ({ page }) => {
		await openIncluded(page)

		await page.getByRole('button', { name: 'Include all not included tools' }).click()
		// Every per-tool Include button is gone; only the bulk one is left.
		await expect(
			page.getByRole('button', { name: /^Include (kubernetes_|search_)/ }),
		).toHaveCount(0)
		await expect(page.getByRole('button', { name: 'Save' })).toBeEnabled()
	})

	// 15.4
	test('filters both panes', async ({ page }) => {
		await openIncluded(page)

		await page.getByRole('textbox', { name: 'Filter tools' }).fill('search')
		await expect(page.getByRole('button', { name: 'Include search_docs' })).toBeVisible()
		await expect(
			page.getByRole('button', { name: 'Include kubernetes_get_pods' }),
		).toHaveCount(0)
	})

	// 15.5
	test('keeps each mode list separate', async ({ page }) => {
		await openIncluded(page)

		const request = page.waitForRequest((r) =>
			r.url().includes('/included-tools/plan'),
		)
		await page.getByRole('combobox', { name: 'Mode' }).selectOption('plan')
		await request

		// The unsaved change in main does not leak into plan.
		await expect(page.getByRole('button', { name: 'Save' })).toBeDisabled()
	})

	/**
	 * 15.x — Distribute is an agent run that narrows EVERY mode's list, not just
	 * the selected one. The warning has to be on screen before the button.
	 */
	test('warns that distribute touches every mode', async ({ page }) => {
		await openIncluded(page)

		await expect(
			page.getByRole('button', { name: 'Distribute with AI' }),
		).toBeVisible()
		await expect(page.getByText(/narrows.*every.*mode's list/i)).toBeVisible()
	})
})
