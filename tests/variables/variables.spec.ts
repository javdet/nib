import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

const VARIABLES = '/api/v1/variables'
const SECRETS = '/api/v1/secrets'
const ISO = '2026-01-15T10:00:00Z'

const builtin = {
	id: 'var-builtin',
	scope: 'global',
	name: 'CompanyName',
	description: 'Company the agent works for',
	value: 'Acme',
	kind: 'string',
	deletable: false,
	createdAt: ISO,
	updatedAt: ISO,
}

const custom = {
	...builtin,
	id: 'var-custom',
	name: 'ReleaseChannel',
	description: 'Which channel to deploy',
	value: 'stable',
	deletable: true,
}

async function openVariables(
	page: import('@playwright/test').Page,
	variables = [builtin, custom],
	secrets: unknown[] = [],
) {
	await mockEmptyWorkspace(page)
	await mockJson(page, VARIABLES, variables)
	await mockJson(page, SECRETS, secrets)
	await page.goto('/variables')
	await expect(
		page.getByRole('heading', { name: 'Variables', level: 2 }),
	).toBeVisible()
}

test.describe('Variables', () => {
	// 17.1
	test('lists variables with their scope', async ({ page }) => {
		await openVariables(page)

		await expect(page.getByRole('tab', { name: 'Variables' })).toBeVisible()
		await expect(page.getByRole('tab', { name: 'Secrets' })).toBeVisible()
		await expect(page.getByText('CompanyName')).toBeVisible()
		await expect(page.getByText('ReleaseChannel')).toBeVisible()
		await expect(page.getByText('global').first()).toBeVisible()
	})

	/**
	 * 17.6 — built-ins are re-valuable but never removable: the prompts render
	 * them by name, so a deletion would break every dialog that uses one.
	 */
	test('refuses to delete a built-in variable', async ({ page }) => {
		await openVariables(page)

		await expect(
			page.getByRole('button', { name: 'Built-in variables cannot be deleted' }),
		).toBeDisabled()
		await expect(
			page.getByRole('button', { name: 'Delete variable' }),
		).toBeEnabled()
	})

	// 17.2
	test('creates a global string variable', async ({ page, apiGuard }) => {
		await openVariables(page)
		apiGuard.allow('POST', VARIABLES)
		await mockJson(page, VARIABLES, { ...custom, id: 'var-new' }, { method: 'POST' })

		await page.getByRole('button', { name: 'Add variable' }).click()
		const modal = page.getByRole('dialog')
		await modal.getByRole('textbox', { name: 'Name' }).fill('e2e-ReleaseChannel')
		await modal.getByRole('textbox', { name: 'Value' }).fill('canary')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/variables') && r.method() === 'POST',
		)
		await modal.getByRole('button', { name: 'Save' }).click()

		expect((await request).postDataJSON()).toMatchObject({
			name: 'e2e-ReleaseChannel',
			value: 'canary',
			scope: 'global',
		})
	})

	// 17.5
	test('locks name and type while editing', async ({ page }) => {
		await openVariables(page)

		await page.getByRole('button', { name: 'Edit variable' }).first().click()
		const modal = page.getByRole('dialog')

		await expect(modal.getByRole('textbox', { name: 'Name' })).toBeDisabled()
		await expect(modal.getByRole('combobox', { name: 'Type' })).toBeDisabled()
		await expect(modal.getByRole('textbox', { name: 'Value' })).toBeEnabled()
	})

	// 17.9
	test('shows the empty secrets state and never renders a value', async ({
		page,
	}) => {
		await openVariables(page)
		await page.getByRole('tab', { name: 'Secrets' }).click()

		await expect(page.getByText('No secrets added yet.')).toBeVisible()
	})

	test('lists secrets without their values', async ({ page }) => {
		await openVariables(page, [builtin], [
			{
				id: 'sec-1',
				scope: 'global',
				name: 'MCP_GITHUB_TOKEN',
				description: 'Token for the GitHub MCP server',
				createdAt: ISO,
				updatedAt: ISO,
			},
		])
		await page.getByRole('tab', { name: 'Secrets' }).click()

		await expect(page.getByText('MCP_GITHUB_TOKEN')).toBeVisible()
		// The API never returns a value, and the page must not invent a field for one.
		await expect(page.getByText('ghp_')).toHaveCount(0)
	})
})
