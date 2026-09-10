import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

const SERVERS = '/api/v1/mcp/servers'

const knowledgeBase = {
	name: 'knowledge-base',
	url: 'http://kb-mcp:8081/mcp',
	description: 'Project knowledge base',
}

async function openTools(page: import('@playwright/test').Page, servers = [knowledgeBase]) {
	await mockEmptyWorkspace(page)
	await mockJson(page, SERVERS, servers)
	await page.goto('/tools')
	await expect(page.getByRole('heading', { name: 'Tools', level: 2 })).toBeVisible()
}

test.describe('Tools — MCP servers', () => {
	// 13.1
	test('renders the four tabs and the configured servers', async ({ page }) => {
		await openTools(page)

		await expect(page.getByRole('tab', { name: 'MCP Servers' })).toBeVisible()
		await expect(page.getByRole('tab', { name: 'Categories' })).toBeVisible()
		await expect(page.getByRole('tab', { name: 'Included tools' })).toBeVisible()
		await expect(page.getByRole('tab', { name: 'Executor' })).toBeVisible()

		await expect(page.getByText('knowledge-base')).toBeVisible()
		await expect(page.getByText('http://kb-mcp:8081/mcp')).toBeVisible()
		await expect(page.getByRole('button', { name: 'Add server' })).toBeVisible()
	})

	// 13.2
	test('adds a server registered by URL', async ({ page, apiGuard }) => {
		await openTools(page)
		apiGuard.allow('POST', SERVERS)
		await mockJson(
			page,
			SERVERS,
			{ name: 'e2e-http', url: 'https://example.com/mcp' },
			{ method: 'POST' },
		)

		await page.getByRole('button', { name: 'Add server' }).click()
		const modal = page.getByRole('dialog')
		await modal.getByRole('textbox', { name: 'Name' }).fill('e2e-http')
		await modal.getByRole('textbox', { name: 'URL' }).fill('https://example.com/mcp')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/mcp/servers') && r.method() === 'POST',
		)
		await modal.getByRole('button', { name: 'Create' }).click()

		expect((await request).postDataJSON()).toMatchObject({
			name: 'e2e-http',
			url: 'https://example.com/mcp',
		})
	})

	/**
	 * 13.3 — the single most important negative case on this page. nib only ever
	 * builds a streamable-HTTP transport, so a command/args (stdio) entry is
	 * refused by the backend even though the dialog offers the fields.
	 */
	test('refuses a stdio server', async ({ page, apiGuard }) => {
		await openTools(page)
		apiGuard.allow('POST', SERVERS)
		await mockJson(
			page,
			SERVERS,
			{ error: 'stdio MCP servers are not supported: register an HTTP URL' },
			{ status: 400, method: 'POST' },
		)

		await page.getByRole('button', { name: 'Add server' }).click()
		const modal = page.getByRole('dialog')
		await modal.getByRole('textbox', { name: 'Name' }).fill('e2e-stdio')
		await modal.getByRole('combobox', { name: 'Transport' }).selectOption('stdio')
		await modal.getByRole('textbox', { name: 'Command' }).fill('npx')
		await modal.getByRole('button', { name: 'Create' }).click()

		await expect(page.getByText(/stdio.*not supported/i)).toBeVisible()
		// The dialog stays open so the operator can fix the entry.
		await expect(modal).toBeVisible()
	})

	// 13.4
	test('will not create a server without a name', async ({ page }) => {
		await openTools(page)

		await page.getByRole('button', { name: 'Add server' }).click()
		const modal = page.getByRole('dialog')
		await modal.getByRole('textbox', { name: 'URL' }).fill('https://example.com/mcp')
		await modal.getByRole('button', { name: 'Create' }).click()

		// Nothing was posted: the guard would have recorded a blocked write.
		await expect(modal).toBeVisible()
	})

	/**
	 * 13.6 — unlike the plan list, which uses the app's own confirm dialog, this
	 * page guards deletion with a native window.confirm.
	 */
	test('keeps the server when the native confirm is dismissed', async ({
		page,
	}) => {
		await openTools(page)
		page.on('dialog', (d) => d.dismiss())

		await page.getByRole('button', { name: 'Delete' }).first().click()
		await expect(page.getByText('knowledge-base')).toBeVisible()
	})

	test('deletes a server once the native confirm is accepted', async ({
		page,
		apiGuard,
	}) => {
		await openTools(page)
		apiGuard.allow('DELETE', `${SERVERS}/knowledge-base`)
		await mockJson(page, `${SERVERS}/knowledge-base`, null, {
			status: 204,
			method: 'DELETE',
		})

		const messages: string[] = []
		page.on('dialog', (d) => {
			messages.push(d.message())
			void d.accept()
		})

		const request = page.waitForRequest(
			(r) =>
				r.url().includes('/mcp/servers/knowledge-base') &&
				r.method() === 'DELETE',
		)
		await page.getByRole('button', { name: 'Delete' }).first().click()

		await request
		expect(messages[0]).toContain('knowledge-base')
		await expect(page.getByText('knowledge-base')).toHaveCount(0)
	})

	// 13.8
	test('shows the raw mcp.json', async ({ page }) => {
		await openTools(page)
		await mockJson(page, '/api/v1/mcp/config/raw', {
			content: '{\n  // the knowledge base\n  "mcpServers": {}\n}',
			warnings: [],
		})

		await page.getByRole('button', { name: 'Raw JSON' }).click()
		// JSONC comments survive the round-trip and are shown verbatim.
		await expect(page.getByRole('textbox')).toHaveValue(
			/\/\/ the knowledge base/,
		)
	})
})
