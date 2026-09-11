import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace, mockPlan } from '../fixtures/mock-api'
import {
	DAG_MARKDOWN,
	PLAN_STATUS_LABELS,
	REPORT_MARKDOWN,
	SUMMARY_MARKDOWN,
	makeActionPlan,
	makeDialog,
	makePlanState,
	type PlanStatus,
} from '../fixtures/data'

const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })

async function openPlan(page: import('@playwright/test').Page, options = {}) {
	await mockEmptyWorkspace(page)
	await mockDialogList(page, { items: [dialog] })
	await mockPlan(page, {
		dialog,
		summary: SUMMARY_MARKDOWN,
		dag: DAG_MARKDOWN,
		...options,
	})
	await page.goto(`/workplace/${dialog.id}`)
	await expect(
		page.getByRole('heading', { name: 'Deploy billing', level: 2 }),
	).toBeVisible()
}

test.describe('Plan detail', () => {
	// 4.1
	test('renders the plan with all three cards', async ({ page }) => {
		await openPlan(page)

		await expect(page.getByRole('button', { name: 'Back to list' })).toBeVisible()
		await expect(page.getByRole('progressbar', { name: 'Plan progress' })).toBeVisible()
		await expect(page.getByRole('button', { name: /^Summary/ })).toBeVisible()
		await expect(page.getByRole('button', { name: 'DAG' })).toBeVisible()
		await expect(page.getByRole('button', { name: 'Action List' })).toBeVisible()
	})

	test('shows no report card on a plan that has none', async ({ page }) => {
		await openPlan(page)

		await expect(page.getByRole('button', { name: /^Summary/ })).toBeVisible()
		await expect(page.getByRole('button', { name: /^Report/ })).toHaveCount(0)
	})

	test('shows the report collapsed and expands it to rendered markdown', async ({
		page,
	}) => {
		await openPlan(page, { report: REPORT_MARKDOWN })

		const header = page.getByRole('button', { name: /^Report/ })
		await expect(header).toBeVisible()
		// Unlike Summary it starts closed: the plan is finished, and the report
		// is a record rather than something the operator works from.
		await expect(header).toHaveAttribute('aria-expanded', 'false')
		await expect(
			page.getByText('Billing reached stage and every check passed.'),
		).toHaveCount(0)

		await header.click()

		await expect(header).toHaveAttribute('aria-expanded', 'true')
		await expect(
			page.getByRole('heading', { name: 'Outcome' }),
		).toBeVisible()
		await expect(
			page.getByText('Billing reached stage and every check passed.'),
		).toBeVisible()
		await expect(page.getByText('namespace billing-stage')).toBeVisible()
	})

	// 4.2
	test('goes back to the list', async ({ page }) => {
		await openPlan(page)
		await page.getByRole('button', { name: 'Back to list' }).click()

		await expect(page).toHaveURL(/\/workplace$/)
		await expect(page.getByRole('heading', { name: 'Plans', level: 2 })).toBeVisible()
	})

	// 4.3
	test('renames the plan', async ({ page, apiGuard }) => {
		await openPlan(page)
		apiGuard.allow('PUT', '/api/v1/dialogs/plan-a/title')
		await mockJson(page, '/api/v1/dialogs/plan-a/title', {
			...dialog,
			title: 'Deploy billing to stage',
		})

		await page.getByRole('button', { name: 'Rename plan' }).click()
		const field = page.getByRole('textbox').first()
		await field.fill('Deploy billing to stage')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/title') && r.method() === 'PUT',
		)
		await field.press('Enter')

		expect((await request).postDataJSON()).toMatchObject({
			title: 'Deploy billing to stage',
		})
		await expect(
			page.getByRole('heading', { name: 'Deploy billing to stage', level: 2 }),
		).toBeVisible()
	})

	// 4.3 (cancel)
	test('escapes out of renaming without saving', async ({ page }) => {
		await openPlan(page)

		await page.getByRole('button', { name: 'Rename plan' }).click()
		const field = page.getByRole('textbox').first()
		await field.fill('Never saved')
		await field.press('Escape')

		await expect(
			page.getByRole('heading', { name: 'Deploy billing', level: 2 }),
		).toBeVisible()
	})

	// 4.4
	test('switches between the simplified and detailed views', async ({ page }) => {
		await openPlan(page)

		const group = page.getByRole('group', { name: 'Plan view' })
		await expect(group.getByRole('button', { name: 'Simplified' })).toHaveAttribute(
			'aria-pressed',
			'true',
		)
		// The meta table (status, schedule, categories) belongs to the detailed view.
		await expect(page.getByRole('table')).toHaveCount(0)

		await group.getByRole('button', { name: 'Detailed' }).click()
		await expect(group.getByRole('button', { name: 'Detailed' })).toHaveAttribute(
			'aria-pressed',
			'true',
		)
		await expect(page.getByRole('table')).toBeVisible()
	})

	// 4.5
	test('downloads the plan as Markdown', async ({ page }) => {
		await openPlan(page)

		const download = page.waitForEvent('download')
		await page.getByRole('button', { name: 'Download plan as Markdown' }).click()
		const file = await download

		expect(file.suggestedFilename()).toMatch(/\.md$/)
	})

	// 4.6
	for (const [status, label] of Object.entries(PLAN_STATUS_LABELS)) {
		test(`shows the ${label} badge`, async ({ page }) => {
			await openPlan(page, {
				planState: makePlanState(status as PlanStatus),
				actionPlan: { ...makeActionPlan(), planStatus: status as PlanStatus },
			})

			await page.getByRole('button', { name: 'Detailed' }).click()
			await expect(
				page.getByRole('row', { name: `Status ${label}` }),
			).toBeVisible()
		})
	}

	// 4.9 — three forward steps plus one rollback step.
	test('counts progress with nothing checked', async ({ page }) => {
		await openPlan(page)
		await expect(page.getByText('0 / 4 (0%)')).toBeVisible()
	})

	test('counts progress over an empty plan without dividing by zero', async ({
		page,
	}) => {
		await openPlan(page, {
			actionPlan: {
				plan: { stages: [], rollback: [] },
				checked: [],
				comments: {},
				planStatus: 'draft' as PlanStatus,
			},
		})
		await expect(page.getByRole('button', { name: 'Action List' })).toBeVisible()
		await expect(page.getByText('NaN')).toHaveCount(0)
	})

	// 4.10
	test('handles an unknown plan id', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await mockJson(
			page,
			'/api/v1/dialogs/00000000-0000-4000-8000-000000000000',
			{ error: 'dialog not found' },
			{ status: 404 },
		)
		await page.goto('/workplace/00000000-0000-4000-8000-000000000000')

		await expect(page.getByRole('button', { name: 'Back to list' })).toBeVisible()
		await expect(page.getByRole('navigation')).toBeVisible()
	})
})

test.describe('Plan summary', () => {
	// 5.1
	test('reports an empty summary', async ({ page }) => {
		await openPlan(page, { summary: '' })
		await expect(
			page.getByText('No summary yet for this conversation.'),
		).toBeVisible()
	})

	// 5.2
	test('renders the summary as Markdown', async ({ page }) => {
		await openPlan(page)
		await expect(page.getByRole('heading', { name: 'Goal' })).toBeVisible()
		await expect(page.getByRole('listitem').filter({ hasText: 'capacity checked' })).toBeVisible()
		await expect(page.getByText('## Goal')).toHaveCount(0)
	})

	// 5.3
	test('edits and saves the summary', async ({ page, apiGuard }) => {
		await openPlan(page)
		apiGuard.allow('PUT', '/api/v1/dialogs/plan-a/summary')

		await page.getByRole('button', { name: 'Edit summary', exact: true }).click()
		const editor = page.getByRole('textbox', { name: 'Summary' })
		await expect(editor).toHaveValue(SUMMARY_MARKDOWN)

		await editor.fill('Rewritten summary')
		const request = page.waitForRequest(
			(r) => r.url().endsWith('/summary') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Save' }).click()

		expect((await request).postDataJSON()).toMatchObject({
			summary: 'Rewritten summary',
		})
	})

	// 5.3 (cancel)
	test('cancels a summary edit', async ({ page }) => {
		await openPlan(page)

		await page.getByRole('button', { name: 'Edit summary', exact: true }).click()
		const summary = page.getByRole('textbox', { name: 'Summary' })
		await summary.fill('Discard me')
		// Scoped to the editor: the plan header carries a Cancel button too.
		await summary.locator('..').getByRole('button', { name: 'Cancel' }).click()

		await expect(page.getByRole('heading', { name: 'Goal' })).toBeVisible()
		await expect(page.getByText('Discard me')).toHaveCount(0)
	})
})
