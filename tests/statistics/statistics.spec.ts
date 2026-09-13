import { test, expect } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'
import { mockJson } from '../fixtures/nib-test'
import { makeStatistics } from '../fixtures/data'

test.describe('Statistics', () => {
	test.beforeEach(async ({ page }) => {
		await mockEmptyWorkspace(page)
	})

	test('is reachable from the sidebar and renders the dashboard', async ({
		page,
	}) => {
		await page.goto('/workplace')

		await page.getByRole('link', { name: 'Statistics' }).click()
		await expect(page).toHaveURL(/\/statistics$/)
		await expect(
			page.getByRole('heading', { name: 'Statistics', level: 2 }),
		).toBeVisible()

		await expect(page.getByText('Total tokens')).toBeVisible()
		await expect(page.getByRole('combobox', { name: 'Statistics range' })).toBeVisible()
	})

	/**
	 * The whole reason cost is nullable: most providers never price a call back
	 * to nib, and "$0.00" would claim the work was free rather than unpriced.
	 */
	test('renders an unreported cost as a dash, not as zero', async ({ page }) => {
		await page.goto('/statistics')

		const costTile = page
			.locator('div')
			.filter({ hasText: /^LLM cost/ })
			.first()
		await expect(costTile).toContainText('—')
		await expect(costTile).not.toContainText('$0.00')
	})

	test('shows totals and a cost when the provider reports one', async ({
		page,
	}) => {
		const stats = makeStatistics()
		const usage = stats.usage as Record<string, never>
		await mockJson(page, '/api/v1/stats', {
			...stats,
			plans: { ...stats.plans, total: 7, createdInRange: 3 },
			usage: {
				...usage,
				totals: {
					calls: 120,
					costedCalls: 120,
					failedCalls: 2,
					promptTokens: 900_000,
					cachedPromptTokens: 100_000,
					completionTokens: 100_000,
					reasoningTokens: 20_000,
					totalTokens: 1_000_000,
					costUsd: 12.34,
					avgDurationMs: 1500,
				},
			},
		})

		await page.goto('/statistics')

		await expect(page.getByText('$12.34')).toBeVisible()
		await expect(page.getByText('1.00M')).toBeVisible()
		await expect(page.getByText('2 failed')).toBeVisible()
	})

	test('re-queries when the range changes', async ({ page }) => {
		await page.goto('/statistics')

		const requests: string[] = []
		page.on('request', (req) => {
			if (req.url().includes('/api/v1/stats')) requests.push(req.url())
		})

		await page
			.getByRole('combobox', { name: 'Statistics range' })
			.selectOption('24h')

		// Poll for the hour request specifically: the initial day-bucket load has
		// already been recorded by the time the select changes, so "any request"
		// would pass without the re-query ever happening.
		await expect
			.poll(() => requests.some((url) => url.includes('bucket=hour')))
			.toBe(true)
	})
})
