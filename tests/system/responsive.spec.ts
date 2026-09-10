import { test, expect } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace, mockPlan } from '../fixtures/mock-api'
import { DAG_MARKDOWN, SUMMARY_MARKDOWN, makeDialog } from '../fixtures/data'
import { failing, measureContrast } from '../fixtures/contrast'

const ROUTES = [
	'/workplace',
	'/incidents',
	'/knowledge',
	'/rules',
	'/tools',
	'/skills',
	'/variables',
]

const TEXT_SELECTORS = [
	'h2',
	'h3',
	'p',
	'td',
	'nav a span',
	'button',
	'label',
	'.text-muted-foreground',
]

const dialog = makeDialog({ id: 'plan-a', planStatus: 'in_progress' })

test.describe('Responsive layout', () => {
	// 20.7 — nothing may push the document sideways.
	for (const size of [
		{ width: 1024, height: 768 },
		{ width: 1280, height: 800 },
	]) {
		for (const route of ROUTES) {
			test(`${route} does not scroll sideways at ${size.width}x${size.height}`, async ({
				page,
			}) => {
				await page.setViewportSize(size)
				await mockEmptyWorkspace(page)
				await page.goto(route)
				await expect(page.getByRole('heading', { level: 2 })).toBeVisible()

				const doc = await page.evaluate(() => ({
					scrollWidth: document.documentElement.scrollWidth,
					clientWidth: document.documentElement.clientWidth,
					bodyOverflowX: getComputedStyle(document.body).overflowX,
				}))
				expect(doc.scrollWidth).toBeLessThanOrEqual(doc.clientWidth)
				expect(doc.bodyOverflowX).not.toBe('scroll')
			})
		}
	}

	// 20.7 — the plan detail is the densest page; wide content stays contained.
	test('keeps the plan detail contained at 1024x768', async ({ page }) => {
		await page.setViewportSize({ width: 1024, height: 768 })
		await mockEmptyWorkspace(page)
		await mockDialogList(page, { items: [dialog] })
		await mockPlan(page, { dialog, summary: SUMMARY_MARKDOWN, dag: DAG_MARKDOWN })
		await page.goto(`/workplace/${dialog.id}`)
		await expect(page.getByRole('button', { name: 'Action List' })).toBeVisible()

		const doc = await page.evaluate(() => ({
			scrollWidth: document.documentElement.scrollWidth,
			clientWidth: document.documentElement.clientWidth,
		}))
		expect(doc.scrollWidth).toBeLessThanOrEqual(doc.clientWidth)
	})
})

test.describe('Theme', () => {
	/**
	 * 20.6 — nib ships dark-only: `index.html` hard-codes `class="dark"` on the
	 * root and nothing in the app toggles it. The light half of the token set in
	 * index.css is therefore unreachable today. This locks in the shipped
	 * behaviour; adding a theme switch should turn it red.
	 */
	test('renders dark, with no theme switch', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await page.goto('/workplace')

		await expect(page.locator('html')).toHaveClass(/dark/)

		// The ground is painted explicitly rather than inherited.
		const bg = await page.evaluate(
			() => getComputedStyle(document.body).backgroundColor,
		)
		expect(bg).not.toBe('rgba(0, 0, 0, 0)')

		await expect(
			page.getByRole('button', { name: /theme|dark mode|light mode/i }),
		).toHaveCount(0)
	})
})

test.describe('Colour contrast', () => {
	// 20.5
	for (const route of ROUTES) {
		test(`${route} meets WCAG AA`, async ({ page }) => {
			await mockEmptyWorkspace(page)
			await page.goto(route)
			await expect(page.getByRole('heading', { level: 2 })).toBeVisible()

			const samples = await measureContrast(page, TEXT_SELECTORS)
			expect(samples.length).toBeGreaterThan(0)

			const bad = failing(samples)
			expect(
				bad,
				bad
					.map(
						(s) =>
							`${s.selector} "${s.text}" ${s.ratio}:1 (needs ${s.required}:1, ${s.size}px/${s.weight})`,
					)
					.join('\n'),
			).toEqual([])
		})
	}

	// 20.5 — the status pills are the tightest case: tinted text on a tinted pill.
	test('plan status badges meet WCAG AA', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await mockDialogList(page, { items: [dialog] })
		await mockPlan(page, { dialog, summary: SUMMARY_MARKDOWN, dag: DAG_MARKDOWN })
		await page.goto(`/workplace/${dialog.id}`)
		await page.getByRole('button', { name: 'Detailed' }).click()
		await expect(page.getByRole('table')).toBeVisible()

		const samples = await measureContrast(page, ['span.rounded-full'])
		const bad = failing(samples)
		expect(
			bad,
			bad.map((s) => `"${s.text}" ${s.ratio}:1 (needs ${s.required}:1)`).join('\n'),
		).toEqual([])
	})
})
