import { test, expect } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace, mockPlan } from '../fixtures/mock-api'
import {
	DAG_MARKDOWN,
	SUMMARY_MARKDOWN,
	makeActionPlan,
	makeDialog,
	makePlanState,
	type PlanStatus,
} from '../fixtures/data'

const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })
const BASE = `/api/v1/dialogs/${dialog.id}`

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

const finishButton = (page: import('@playwright/test').Page) =>
	page.getByRole('button', { name: 'Finish', exact: true })

const cancelButton = (page: import('@playwright/test').Page) =>
	page.getByRole('button', { name: 'Cancel', exact: true })

async function fulfilJson(
	route: import('@playwright/test').Route,
	body: unknown,
) {
	await route.fulfill({
		status: 200,
		contentType: 'application/json',
		body: JSON.stringify(body),
	})
}

/**
 * Serves plan-state and action-plan from mutable state that the writes update.
 *
 * Finishing and cancelling both reload the detail view afterwards, so a mock
 * that keeps replaying the state the page started with would report the write
 * as having been undone. The real backend answers the reload with what it just
 * stored; these routes do the same.
 */
async function mockStatefulPlan(
	page: import('@playwright/test').Page,
	initial: { status?: PlanStatus; actionPlan?: ReturnType<typeof makeActionPlan> | null } = {},
) {
	const state = {
		planState: makePlanState(initial.status ?? 'draft'),
		actionPlan:
			initial.actionPlan === undefined ? makeActionPlan() : initial.actionPlan,
	}

	await page.route(
		(url) => url.pathname === `${BASE}/plan-state`,
		(route) => fulfilJson(route, state.planState),
	)
	await page.route(
		(url) => url.pathname === `${BASE}/action-plan`,
		(route) => fulfilJson(route, state.actionPlan),
	)
	await page.route(
		(url) => url.pathname === `${BASE}/plan-state/status`,
		async (route) => {
			const { status } = route.request().postDataJSON() as {
				status: PlanStatus
			}
			state.planState = makePlanState(status)
			await fulfilJson(route, { status })
		},
	)
	await page.route(
		(url) => url.pathname === `${BASE}/action-plan/checks`,
		async (route) => {
			const { checked } = route.request().postDataJSON() as {
				checked: string[]
			}
			const planStatus: PlanStatus = 'done'
			if (state.actionPlan) {
				state.actionPlan = { ...state.actionPlan, checked, planStatus }
			}
			state.planState = makePlanState(planStatus)
			await fulfilJson(route, { checked, planStatus })
		},
	)
}

test.describe('Finish and cancel a plan', () => {
	test('offers only Cancel while the plan has no action list', async ({
		page,
	}) => {
		await openPlan(page, { actionPlan: null })

		await expect(finishButton(page)).toBeDisabled()
		await expect(cancelButton(page)).toBeEnabled()
	})

	test('enables Finish once the action list exists', async ({ page }) => {
		await openPlan(page)

		await expect(finishButton(page)).toBeEnabled()
		await expect(cancelButton(page)).toBeEnabled()
	})

	test('cancels an empty plan after confirmation', async ({
		page,
		apiGuard,
	}) => {
		await openPlan(page, { actionPlan: null })
		apiGuard.allow('PUT', `${BASE}/plan-state/status`)
		await mockStatefulPlan(page, { actionPlan: null })

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/plan-state/status') && r.method() === 'PUT',
		)
		await cancelButton(page).click()

		const modal = page.getByRole('dialog')
		await expect(modal).toContainText('Cancel this plan?')
		await modal.getByRole('button', { name: 'Cancel plan' }).click()

		expect((await request).postDataJSON()).toMatchObject({
			status: 'cancelled',
		})
		await page.getByRole('button', { name: 'Detailed' }).click()
		await expect(
			page.getByRole('row', { name: 'Status CANCELLED' }),
		).toBeVisible()
		await expect(finishButton(page)).toBeDisabled()
		await expect(cancelButton(page)).toBeDisabled()
	})

	test('keeps the plan when the confirmation is dismissed', async ({
		page,
	}) => {
		await openPlan(page, { actionPlan: null })

		await cancelButton(page).click()
		const modal = page.getByRole('dialog')
		await modal.getByRole('button', { name: 'Keep plan' }).click()

		await expect(modal).toHaveCount(0)
		await expect(cancelButton(page)).toBeEnabled()
	})

	test('finishes the plan by checking every action', async ({
		page,
		apiGuard,
	}) => {
		await openPlan(page)
		apiGuard.allow('PUT', `${BASE}/action-plan/checks`)
		apiGuard.allow('POST', `${BASE}/report`)
		await mockStatefulPlan(page)

		await expect(page.getByText('0 / 4 (0%)')).toBeVisible()

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/action-plan/checks') && r.method() === 'PUT',
		)
		const reportRequest = page.waitForRequest(
			(r) => r.url().endsWith('/report') && r.method() === 'POST',
		)
		await finishButton(page).click()
		// Finishing is also what commissions the report.
		await reportRequest

		// Rollback items stay out of it, exactly as the progress bar counts them.
		expect((await request).postDataJSON()).toMatchObject({
			checked: ['s0.step0', 's0.step1', 's0.check0', 's1.step0'],
		})
		await expect(page.getByText('4 / 4 (100%)')).toBeVisible()
		await page.getByRole('button', { name: 'Detailed' }).click()
		await expect(
			page.getByRole('row', { name: 'Status FINISHED' }),
		).toBeVisible()
		await expect(finishButton(page)).toBeDisabled()
		await expect(cancelButton(page)).toBeDisabled()
	})

	test('finishes even when the report agent cannot be started', async ({
		page,
		apiGuard,
	}) => {
		await openPlan(page)
		apiGuard.allow('PUT', `${BASE}/action-plan/checks`)
		apiGuard.allow('POST', `${BASE}/report`)
		await mockStatefulPlan(page)
		await page.route(
			(url) => url.pathname === `${BASE}/report`,
			async (route) => {
				if (route.request().method() !== 'POST') return route.fallback()
				await route.fulfill({
					status: 500,
					contentType: 'application/json',
					body: JSON.stringify({ error: 'llm unreachable' }),
				})
			},
		)

		await finishButton(page).click()

		// The report is a by-product: losing it must not surface as a failure,
		// and must not roll the finished state back.
		await expect(page.getByText('4 / 4 (100%)')).toBeVisible()
		await expect(page.getByText('Failed to finish the plan')).toHaveCount(0)
		await page.getByRole('button', { name: 'Detailed' }).click()
		await expect(
			page.getByRole('row', { name: 'Status FINISHED' }),
		).toBeVisible()
	})

	test('leaves both buttons inactive on an already finished plan', async ({
		page,
	}) => {
		await openPlan(page, {
			planState: makePlanState('done'),
			actionPlan: { ...makeActionPlan(), planStatus: 'done' },
		})

		await expect(finishButton(page)).toBeDisabled()
		await expect(cancelButton(page)).toBeDisabled()
	})
})
