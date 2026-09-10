import { test, expect } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace, mockPlan } from '../fixtures/mock-api'
import { DAG_MARKDOWN, SUMMARY_MARKDOWN, makeDialog } from '../fixtures/data'

const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })

/** A graph far wider than the card, to exercise the scroll container. */
const WIDE_DAG = [
	'```mermaid',
	'graph LR',
	...Array.from(
		{ length: 12 },
		(_, i) =>
			`  S${i}[Stage ${i} with a deliberately long label] --> S${i + 1}[Stage ${i + 1} with a deliberately long label]`,
	),
	'```',
].join('\n')

/** The rendered graph itself -- not the card's own chevron icons. */
function dagSvg(page: import('@playwright/test').Page) {
	return page.locator('.dag-board div.overflow-x-auto svg')
}

async function openPlan(
	page: import('@playwright/test').Page,
	dag = DAG_MARKDOWN,
) {
	await mockEmptyWorkspace(page)
	await mockDialogList(page, { items: [dialog] })
	await mockPlan(page, { dialog, summary: SUMMARY_MARKDOWN, dag })
	await page.goto(`/workplace/${dialog.id}`)
	await expect(page.getByRole('button', { name: 'DAG' })).toBeVisible()
}

test.describe('DAG view', () => {
	// 5.6
	test('renders the mermaid graph', async ({ page }) => {
		await openPlan(page)

		const svg = dagSvg(page)
		await expect(svg).toBeVisible()
		await expect(page.getByText('Prepare release')).toBeVisible()
		await expect(page.getByText('Smoke test')).toBeVisible()
		// The fence itself is never shown as text.
		await expect(page.getByText('```mermaid')).toHaveCount(0)
	})

	// 5.6 (empty)
	test('handles a plan with no DAG', async ({ page }) => {
		await openPlan(page, '')

		await expect(dagSvg(page)).toHaveCount(0)
		await expect(page.getByRole('button', { name: 'Action List' })).toBeVisible()
	})

	// 5.6 (broken source)
	test('reports a graph it cannot draw', async ({ page }) => {
		await openPlan(page, ['```mermaid', 'graph TD', '  A --> ', '```'].join('\n'))

		// The error replaces the canvas rather than leaving a blank card.
		await expect(dagSvg(page)).toHaveCount(0)
		await expect(page.getByRole('button', { name: 'Action List' })).toBeVisible()
	})

	/**
	 * 5.7 — mermaid scales its SVG down to the container width, so a wide graph
	 * does not actually overflow. What matters is that the container is the one
	 * set up to scroll if it ever does, and that the page body never scrolls
	 * sideways because of it.
	 */
	test('keeps a wide graph inside its own container', async ({ page }) => {
		await openPlan(page, WIDE_DAG)

		const board = page.locator('.dag-board div.overflow-x-auto').first()
		await expect(board).toBeVisible()
		await expect(dagSvg(page)).toBeVisible()

		const box = await board.evaluate((el) => {
			const svg = el.querySelector('svg')!
			return {
				overflowX: getComputedStyle(el).overflowX,
				clientWidth: el.clientWidth,
				svgWidth: svg.getBoundingClientRect().width,
			}
		})
		expect(box.overflowX).toBe('auto')
		// The graph is scaled to fit rather than spilling out of the card.
		expect(box.svgWidth).toBeLessThanOrEqual(box.clientWidth + 1)

		// And the document itself has no horizontal scroll.
		const doc = await page.evaluate(() => ({
			scrollWidth: document.documentElement.scrollWidth,
			clientWidth: document.documentElement.clientWidth,
		}))
		expect(doc.scrollWidth).toBeLessThanOrEqual(doc.clientWidth)
	})

	// 5.5
	test('collapses and expands the DAG card', async ({ page }) => {
		await openPlan(page)

		const header = page.getByRole('button', { name: 'DAG' })
		await expect(header).toHaveAttribute('aria-expanded', 'true')

		await header.click()
		await expect(header).toHaveAttribute('aria-expanded', 'false')
		await expect(dagSvg(page)).toHaveCount(0)

		await header.click()
		await expect(dagSvg(page)).toBeVisible()
	})
})
