import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

const SERVERS = '/api/v1/mcp/servers'

const server = {
	name: 'knowledge-base',
	url: 'http://kb-mcp:8081/mcp',
	headers: { Authorization: 'Bearer ${MCP_KB_TOKEN}' },
}

async function openTools(page: import('@playwright/test').Page) {
	await mockEmptyWorkspace(page)
	await mockJson(page, SERVERS, [server])
	await page.goto('/tools')
	await expect(page.getByText('knowledge-base')).toBeVisible()
}

test.describe('Tools — per-server tool discovery', () => {
	// 13.7
	test('lists the tools a server exposes', async ({ page }) => {
		await openTools(page)
		await mockJson(page, `${SERVERS}/knowledge-base/tools`, [
			{
				name: 'knowledge_search',
				description: 'Search the knowledge base',
				inputSchema: { type: 'object', properties: { query: { type: 'string' } } },
			},
			{ name: 'update_kb', description: 'Replace a collection' },
		])

		await page.getByRole('button', { name: 'Tools' }).first().click()

		const modal = page.getByRole('dialog')
		await expect(modal).toContainText('Tools — knowledge-base')
		await expect(modal).toContainText('2 tools available')
		await expect(modal.getByText('knowledge_search')).toBeVisible()
		await expect(modal.getByText('update_kb')).toBeVisible()

		// Descriptions are behind a per-tool disclosure, not listed up front.
		await modal.getByText('knowledge_search').click()
		await expect(modal.getByText('Search the knowledge base')).toBeVisible()
	})

	// 13.7 — an unreachable server shows why, not an empty list.
	test('reports a server it cannot reach', async ({ page }) => {
		await openTools(page)
		await mockJson(
			page,
			`${SERVERS}/knowledge-base/tools`,
			{ error: 'dial tcp 10.0.0.4:8081: connect: connection refused' },
			{ status: 502 },
		)

		await page.getByRole('button', { name: 'Tools' }).first().click()

		const modal = page.getByRole('dialog')
		await expect(modal.getByText(/connection refused/)).toBeVisible()
		await expect(modal.getByRole('button', { name: /Refresh|Retry/ })).toBeVisible()
	})

	/**
	 * 13.7 — every MCP transport error goes through redactRouteError, so a
	 * resolved secret must never reach the page. The backend sends the reference
	 * back, not the value.
	 */
	test('never renders a resolved secret in a transport error', async ({ page }) => {
		await openTools(page)
		await mockJson(
			page,
			`${SERVERS}/knowledge-base/tools`,
			{
				error:
					'mcp: initialize failed: 401 Unauthorized (header Authorization: Bearer ${MCP_KB_TOKEN})',
			},
			{ status: 502 },
		)

		await page.getByRole('button', { name: 'Tools' }).first().click()

		const modal = page.getByRole('dialog')
		await expect(modal.getByText(/401 Unauthorized/)).toBeVisible()
		// The reference survives; a value never appears anywhere on the page.
		await expect(modal).toContainText('${MCP_KB_TOKEN}')
		expect(await page.locator('body').innerText()).not.toMatch(
			/Bearer\s+[A-Za-z0-9._-]{16,}/,
		)
	})

	// 13.7 — a server with nothing to offer.
	test('reports a server with no tools', async ({ page }) => {
		await openTools(page)
		await mockJson(page, `${SERVERS}/knowledge-base/tools`, [])

		await page.getByRole('button', { name: 'Tools' }).first().click()
		await expect(page.getByRole('dialog')).toContainText(/no tools/i)
	})
})
