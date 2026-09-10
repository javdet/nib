import { test, expect } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

test.describe('Accessibility', () => {
	test.beforeEach(async ({ page }) => {
		await mockEmptyWorkspace(page)
	})

	// 20.3
	test('names every icon-only control in the shell', async ({ page }) => {
		await page.goto('/workplace')

		for (const name of [
			'New chat',
			'Attach file',
			'Send message',
			'Search',
			'New plan',
		]) {
			await expect(page.getByRole('button', { name })).toBeVisible()
		}
	})

	// 20.4
	test('exposes the page landmarks', async ({ page }) => {
		await page.goto('/workplace')

		await expect(page.getByRole('banner')).toBeVisible()
		await expect(page.getByRole('navigation')).toBeVisible()
		await expect(page.getByRole('main')).toBeVisible()
		await expect(page.getByRole('heading', { level: 2 })).toBeVisible()
	})

	// 20.2
	test('closes a modal on Escape and restores focus', async ({ page }) => {
		await page.goto('/workplace')
		await page.getByRole('button', { name: 'Help' }).click()
		await page.keyboard.press('Escape')

		await expect(page.getByRole('button', { name: 'Help' })).toBeFocused()
	})

	/**
	 * 20.3 — DEFECT. Collapsing the sidebar drops the accessible name of every
	 * nav link and of the Help and Expand buttons: the label span is removed and
	 * nothing (aria-label, sr-only text, title) replaces it. The tooltip does not
	 * count -- it appears on hover and is not part of the accessible name.
	 *
	 * A screen-reader user who collapses the sidebar loses the whole navigation.
	 * Remove the fixme once the collapsed sidebar carries aria-labels.
	 */
	test.fixme('keeps nav names when the sidebar is collapsed', async ({ page }) => {
		await page.goto('/workplace')
		await page.getByRole('button', { name: 'Collapse' }).click()

		await expect(page.getByRole('link', { name: 'Workplace' })).toBeVisible()
		await expect(page.getByRole('button', { name: 'Expand' })).toBeVisible()
	})
})
