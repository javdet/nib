import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace, mockPlan } from '../fixtures/mock-api'
import {
	DAG_MARKDOWN,
	SUMMARY_MARKDOWN,
	makeDialog,
	makePlanState,
} from '../fixtures/data'

const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })
const BASE = `/api/v1/dialogs/${dialog.id}`

/** 2026-03-18 09:00 local, as the field builds it. */
const SCHEDULED_AT = Math.floor(new Date(2026, 2, 18, 9, 0, 0, 0).getTime() / 1000)

async function openDetailed(
	page: import('@playwright/test').Page,
	options: Record<string, unknown> = {},
) {
	await mockEmptyWorkspace(page)
	await mockDialogList(page, { items: [dialog] })
	await mockPlan(page, {
		dialog,
		summary: SUMMARY_MARKDOWN,
		dag: DAG_MARKDOWN,
		...options,
	})
	await page.goto(`/workplace/${dialog.id}`)
	await page.getByRole('button', { name: 'Detailed' }).click()
	await expect(page.getByRole('table')).toBeVisible()
}

/**
 * The schedule popover is tall: a calendar, a time field and the clear button.
 * At the default 720px-high viewport its lower half sits outside the viewport
 * and Playwright cannot click it -- Radix's collision handling does not reclaim
 * the space. Worth a look as a UX issue; here the viewport is raised so the
 * behaviour under test is the schedule, not the layout.
 */
test.use({ viewport: { width: 1440, height: 1000 } })

test.describe('Plan schedule', () => {
	// 4.8
	test('offers a schedule picker with no date set', async ({ page }) => {
		await openDetailed(page)

		const trigger = page.getByRole('button', { name: 'Select schedule' })
		await expect(trigger).toBeVisible()

		await trigger.click()
		await expect(page.getByRole('grid')).toBeVisible()
		await expect(page.getByLabel('Time')).toHaveValue('09:00')
		// Nothing to clear until a date exists.
		await expect(page.getByRole('button', { name: 'Clear schedule' })).toBeDisabled()
	})

	// 4.8 — picking a day writes the schedule and moves the plan to SCHEDULED.
	test('schedules the plan for a chosen day', async ({ page, apiGuard }) => {
		await openDetailed(page)
		apiGuard.allow('PUT', `${BASE}/plan-state/schedule`)
		await mockJson(page, `${BASE}/plan-state/schedule`, {
			status: 'scheduled',
			scheduledAt: SCHEDULED_AT,
		})

		await page.getByRole('button', { name: 'Select schedule' }).click()

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/plan-state/schedule') && r.method() === 'PUT',
		)
		// Any enabled day in the open month; the picker opens on the current one.
		await page.getByRole('gridcell').filter({ hasText: /^15$/ }).first().click()

		const scheduledAt = (await request).postDataJSON().scheduledAt
		expect(typeof scheduledAt).toBe('number')
		expect(scheduledAt).toBeGreaterThan(0)
		await expect(page.getByRole('row', { name: /Status SCHEDULED/ })).toBeVisible()
	})

	// 4.8 — the time input is inert until a day is chosen.
	test('locks the time until a day is picked', async ({ page }) => {
		await openDetailed(page)

		await page.getByRole('button', { name: 'Select schedule' }).click()
		await expect(page.getByLabel('Time')).toBeDisabled()
	})

	// 4.8 — an existing schedule shows on the trigger and can be cleared.
	test('clears an existing schedule', async ({ page, apiGuard }) => {
		await openDetailed(page, {
			planState: makePlanState('scheduled', SCHEDULED_AT),
		})
		apiGuard.allow('PUT', `${BASE}/plan-state/schedule`)
		await mockJson(page, `${BASE}/plan-state/schedule`, {
			status: 'draft',
			scheduledAt: 0,
		})

		// The trigger carries the formatted date instead of the placeholder.
		await expect(
			page.getByRole('button', { name: 'Select schedule' }),
		).toHaveCount(0)
		const trigger = page.getByRole('button', { name: /March 18th, 2026/ })
		await expect(trigger).toBeVisible()

		await trigger.click()
		await expect(page.getByLabel('Time')).toHaveValue('09:00')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/plan-state/schedule') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Clear schedule' }).click()

		expect((await request).postDataJSON()).toMatchObject({ scheduledAt: 0 })
	})
})
