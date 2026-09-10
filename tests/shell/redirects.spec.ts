import { test, expect } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

// 1.3 — legacy paths kept as redirects in frontend/src/app.tsx.
const REDIRECTS = [
	{ from: '/', to: '/workplace' },
	{ from: '/projects', to: '/workplace' },
	{ from: '/projects/abc', to: '/workplace' },
	{ from: '/dialogs', to: '/workplace' },
	{ from: '/task-tracker', to: '/tools' },
	{ from: '/mcp-connections', to: '/tools' },
	{ from: '/base', to: '/tools' },
]

test.describe('Legacy route redirects', () => {
	test.beforeEach(async ({ page }) => {
		await mockEmptyWorkspace(page)
	})

	for (const { from, to } of REDIRECTS) {
		test(`${from} redirects to ${to}`, async ({ page }) => {
			await page.goto(from)
			await expect(page).toHaveURL(new RegExp(`${to}$`))
		})
	}

	test('preserves the query string across a redirect', async ({ page }) => {
		await page.goto('/base?tab=executor')
		await expect(page).toHaveURL(/\/tools\?tab=executor$/)
	})

	/**
	 * 1.4 — app.tsx declares no catch-all route, so an unknown path renders an
	 * empty document: no shell, no message, no way back except the browser.
	 * This asserts today's behaviour so that adding a 404 page turns it red.
	 */
	test('an unknown route renders nothing at all', async ({ page }) => {
		await page.goto('/does-not-exist')
		await expect(page.getByRole('navigation')).toHaveCount(0)
		await expect(page.locator('#root')).toBeEmpty()
	})
})
