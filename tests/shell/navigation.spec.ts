import { test, expect } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

const NAV = [
	{ label: 'Workplace', path: '/workplace', heading: 'Plans' },
	{ label: 'Incidents In dev', path: '/incidents', heading: 'Incidents' },
	{ label: 'Knowledgebase', path: '/knowledge', heading: 'Knowledge Base' },
	{ label: 'Rules', path: '/rules', heading: 'Rules' },
	{ label: 'Tools', path: '/tools', heading: 'Tools' },
	{ label: 'Skills', path: '/skills', heading: 'Skills' },
	{ label: 'Variables', path: '/variables', heading: 'Variables' },
]

test.describe('Application shell', () => {
	test.beforeEach(async ({ page }) => {
		await mockEmptyWorkspace(page)
	})

	// 1.1
	test('renders the shell and lands on the workplace', async ({ page }) => {
		const consoleErrors: string[] = []
		page.on('console', (msg) => {
			if (msg.type() === 'error') consoleErrors.push(msg.text())
		})

		await page.goto('/')
		await expect(page).toHaveURL(/\/workplace$/)

		const nav = page.getByRole('navigation')
		for (const item of NAV) {
			await expect(nav.getByRole('link', { name: item.label })).toBeVisible()
		}
		// Statistics sits in the sidebar footer beside Help, which is outside the
		// <nav> landmark -- so it is asserted here rather than in NAV above.
		await expect(page.getByRole('link', { name: 'Statistics' })).toBeVisible()
		await expect(page.getByRole('button', { name: 'Help' })).toBeVisible()
		await expect(page.getByRole('button', { name: 'Collapse' })).toBeVisible()

		await expect(
			page.getByRole('button', { name: 'Chat', exact: true }),
		).toBeVisible()
		await expect(
			page.getByText(
				'Messages will appear here. Select a dialog or send a message to start a new one.',
			),
		).toBeVisible()

		expect(consoleErrors, consoleErrors.join('\n')).toEqual([])
	})

	// 1.2
	for (const item of NAV) {
		test(`navigates to ${item.path}`, async ({ page }) => {
			await page.goto('/workplace')
			await page.getByRole('link', { name: item.label }).click()

			await expect(page).toHaveURL(new RegExp(`${item.path}$`))
			await expect(
				page.getByRole('heading', { name: item.heading, level: 2 }),
			).toBeVisible()
			await expect(
				page.getByRole('link', { name: item.label }),
			).toHaveAttribute('href', item.path)
		})
	}

	// 1.5
	test('collapses the sidebar and remembers it across a reload', async ({
		page,
	}) => {
		await page.goto('/workplace')
		const sidebar = page.getByRole('complementary').first()

		await expect(page.getByRole('link', { name: 'Workplace' })).toBeVisible()
		await page.getByRole('button', { name: 'Collapse' }).click()

		// Collapsed links keep their href but lose their label. They also lose
		// their accessible name entirely -- see the fixme in system/a11y.spec.ts.
		await expect(page.getByRole('button', { name: 'Collapse' })).toHaveCount(0)
		await expect(sidebar.locator('a[href="/workplace"]')).toBeVisible()
		await expect(sidebar.locator('a[href="/workplace"]')).not.toContainText(
			'Workplace',
		)

		expect(
			await page.evaluate(() => localStorage.getItem('sidebar-collapsed')),
		).toBe('true')

		await page.reload()
		await expect(page.getByRole('button', { name: 'Collapse' })).toHaveCount(0)

		// The expand toggle sits second in the sidebar footer; it has no
		// accessible name while collapsed, so it is addressed positionally.
		await sidebar.locator('button').nth(1).click()
		await expect(page.getByRole('button', { name: 'Collapse' })).toBeVisible()
		await expect(page.getByRole('link', { name: 'Workplace' })).toBeVisible()
	})

	// 1.6
	test('exposes a draggable splitter between content and chat', async ({
		page,
	}) => {
		await page.goto('/workplace')

		const splitter = page.getByRole('separator')
		await expect(splitter).toBeVisible()
		await expect(splitter).toHaveAttribute('aria-orientation', 'vertical')

		const before = Number(await splitter.getAttribute('aria-valuenow'))
		const box = (await splitter.boundingBox())!
		await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
		await page.mouse.down()
		await page.mouse.move(box.x - 200, box.y + box.height / 2, { steps: 10 })
		await page.mouse.up()

		const after = Number(await splitter.getAttribute('aria-valuenow'))
		expect(after).toBeLessThan(before)
		// Clamped to the range the splitter advertises.
		expect(after).toBeGreaterThanOrEqual(
			Number(await splitter.getAttribute('aria-valuemin')),
		)
	})

	// 1.7
	test('deep-links straight into a lazy route', async ({ page }) => {
		await page.goto('/variables')
		await expect(
			page.getByRole('heading', { name: 'Variables', level: 2 }),
		).toBeVisible()
		await expect(page.getByText('Loading...')).toHaveCount(0)
	})
})
