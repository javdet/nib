import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

const RULES = '/api/v1/rules'

const postgresRule = [
	'---',
	'name: postgres',
	'description: Guardrails for Postgres changes',
	'---',
	'',
	'- Never drop a column in the same release that stops writing to it.',
].join('\n')

async function openRules(
	page: import('@playwright/test').Page,
	rules = [{ name: 'postgres', description: 'Guardrails for Postgres changes' }],
) {
	await mockEmptyWorkspace(page)
	await mockJson(page, RULES, { rules })
	await mockJson(page, `${RULES}/postgres`, {
		name: 'postgres',
		content: postgresRule,
	})
	await page.goto('/rules')
	await expect(page.getByRole('heading', { name: 'Rules', level: 2 })).toBeVisible()
}

test.describe('Rules', () => {
	// 11.1
	test('shows the empty state', async ({ page }) => {
		await openRules(page, [])

		await expect(page.getByText('No rules yet.')).toBeVisible()
		await expect(page.getByText('Select a rule or add a new one.')).toBeVisible()
		await expect(page.getByRole('button', { name: 'Add rule' })).toBeVisible()
	})

	// 11.5 (selection)
	test('opens a rule into the editor', async ({ page }) => {
		await openRules(page)

		await page.getByRole('button', { name: 'postgres.md' }).click()
		await expect(page.getByRole('textbox', { name: 'Name' })).toHaveValue('postgres')
		await expect(page.getByRole('textbox', { name: 'Description' })).toHaveValue(
			'Guardrails for Postgres changes',
		)
		await expect(page.getByRole('textbox', { name: 'Body' })).toHaveValue(
			/Never drop a column/,
		)
	})

	// 11.2
	test('creates a rule', async ({ page, apiGuard }) => {
		await openRules(page, [])
		apiGuard.allow('POST', RULES)
		await mockJson(page, RULES, { name: 'e2e-redis', content: '' }, { method: 'POST' })

		await page.getByRole('button', { name: 'Add rule' }).click()
		const modal = page.getByRole('dialog')
		await expect(modal).toContainText('Add rule')
		await modal.getByRole('textbox', { name: 'File name' }).fill('e2e-redis')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/rules') && r.method() === 'POST',
		)
		await modal.getByRole('button', { name: 'Create' }).click()

		const body = (await request).postDataJSON()
		expect(body.name).toBe('e2e-redis')
		// The dialog seeds valid frontmatter rather than an empty file.
		expect(body.content).toContain('name: e2e-redis')
	})

	// 11.3 — refused in the browser; no request is sent.
	test('refuses a duplicate rule name', async ({ page }) => {
		await openRules(page)

		await page.getByRole('button', { name: 'Add rule' }).click()
		const modal = page.getByRole('dialog')
		await modal.getByRole('textbox', { name: 'File name' }).fill('postgres')
		await modal.getByRole('button', { name: 'Create' }).click()

		await expect(
			modal.getByText('A rule with this name already exists'),
		).toBeVisible()
	})

	// 11.4
	test('refuses an invalid rule name', async ({ page }) => {
		await openRules(page, [])

		await page.getByRole('button', { name: 'Add rule' }).click()
		const modal = page.getByRole('dialog')

		// An empty name cannot even be submitted.
		await expect(modal.getByRole('button', { name: 'Create' })).toBeDisabled()

		await modal.getByRole('textbox', { name: 'File name' }).fill('../escape')
		await modal.getByRole('button', { name: 'Create' }).click()
		await expect(
			modal.getByText(/Use letters, numbers, hyphens, and underscores/),
		).toBeVisible()
	})

	// 11.5
	test('saves an edited rule', async ({ page, apiGuard }) => {
		await openRules(page)
		apiGuard.allow('PUT', `${RULES}/postgres`)
		await mockJson(page, `${RULES}/postgres`, null, { status: 204, method: 'PUT' })

		await page.getByRole('button', { name: 'postgres.md' }).click()
		await expect(page.getByRole('button', { name: 'Save' })).toBeDisabled()

		await page
			.getByRole('textbox', { name: 'Body' })
			.fill('- Take a backup before every migration.')

		const request = page.waitForRequest(
			(r) => r.url().includes('/rules/postgres') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Save' }).click()

		expect((await request).postDataJSON().content).toContain(
			'Take a backup before every migration.',
		)
	})

	// 11.6 — deletion is guarded by a native confirm, like the MCP server list.
	test('deletes a rule after the native confirm', async ({ page, apiGuard }) => {
		await openRules(page)
		apiGuard.allow('DELETE', `${RULES}/postgres`)
		await mockJson(page, `${RULES}/postgres`, null, {
			status: 204,
			method: 'DELETE',
		})

		await page.getByRole('button', { name: 'postgres.md' }).click()

		const messages: string[] = []
		page.on('dialog', (d) => {
			messages.push(d.message())
			void d.accept()
		})
		const request = page.waitForRequest(
			(r) => r.url().includes('/rules/postgres') && r.method() === 'DELETE',
		)
		await page.getByRole('button', { name: 'Delete' }).click()

		await request
		expect(messages[0]).toContain('postgres.md')
	})

	// 11.5 (unsaved guard)
	test('warns before switching away from unsaved changes', async ({ page }) => {
		await openRules(page, [
			{ name: 'postgres', description: 'Guardrails for Postgres changes' },
			{ name: 'redis', description: 'Guardrails for Redis' },
		])

		await page.getByRole('button', { name: 'postgres.md' }).click()
		await page.getByRole('textbox', { name: 'Body' }).fill('unsaved edit')

		const messages: string[] = []
		page.on('dialog', (d) => {
			messages.push(d.message())
			void d.dismiss()
		})
		await page.getByRole('button', { name: 'redis.md' }).click()

		expect(messages[0]).toContain('unsaved changes')
		// Dismissed: the editor stays on the rule being edited.
		await expect(page.getByRole('textbox', { name: 'Body' })).toHaveValue(
			/unsaved edit/,
		)
	})
})
