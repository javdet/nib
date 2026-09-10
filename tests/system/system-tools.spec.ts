import { test, expect } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

/**
 * The developer catalog is served straight from the binary, so these run against
 * the live backend read-only: no fixture can tell us the built-in tool list.
 */
test.describe('System tools catalog', () => {
	test.beforeEach(async ({ page }) => {
		await mockEmptyWorkspace(page)
		await page.goto('/system-tools')
		await expect(
			page.getByRole('heading', { name: 'System Tools', level: 2 }),
		).toBeVisible()
	})

	// 18.1
	test('is reachable only by URL', async ({ page }) => {
		await expect(
			page.getByText('This page is not linked from the UI.'),
		).toBeVisible()
		await expect(
			page.getByRole('navigation').getByRole('link', { name: /System/ }),
		).toHaveCount(0)
	})

	// 18.2
	test('counts the tools it renders', async ({ page }) => {
		const count = page.getByRole('button', { name: 'Parameters schema' })
		await expect(count.first()).toBeVisible()
		const rendered = await count.count()
		expect(rendered).toBeGreaterThan(0)
		await expect(page.getByText(`${rendered} tools`)).toBeVisible()
	})

	// 18.4 — the orchestrator-only tools must not be offered to sub-agent modes.
	test('marks run_subagent as available to main only', async ({ page }) => {
		await page
			.getByRole('textbox', { name: 'Filter by name, description, or mode...' })
			.fill('run_subagent')

		const title = page.getByText('run_subagent', { exact: true })
		await expect(title).toBeVisible()

		// The card header holds the title block and its mode badges side by side.
		const header = title.locator('xpath=../..')
		await expect(header).toContainText('main')
		await expect(header).not.toContainText('decompose')
		await expect(header).not.toContainText('execute')
	})

	// 18.3
	test('filters to nothing for an unmatched query', async ({ page }) => {
		await page
			.getByRole('textbox', { name: 'Filter by name, description, or mode...' })
			.fill('zzz-no-such-tool')

		await expect(
			page.getByRole('button', { name: 'Parameters schema' }),
		).toHaveCount(0)
	})
})
