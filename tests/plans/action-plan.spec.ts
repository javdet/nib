import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace, mockPlan } from '../fixtures/mock-api'
import {
	DAG_MARKDOWN,
	SUMMARY_MARKDOWN,
	makeActionPlan,
	makeDialog,
} from '../fixtures/data'

const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })
const BASE = `/api/v1/dialogs/${dialog.id}`

async function openPlan(
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
	await expect(
		page.getByRole('heading', { name: 'Deploy billing', level: 2 }),
	).toBeVisible()
}

/** The row for one action, addressed by its number so siblings stay distinct. */
function actionRow(page: import('@playwright/test').Page, number: string) {
	return page.getByRole('listitem').filter({ hasText: number }).first()
}

test.describe('Action list', () => {
	// 6.1
	test('refuses to process a plan before decomposition', async ({ page }) => {
		await openPlan(page, { dag: '', actionPlan: null })

		await expect(
			page.getByText(
				'Complete decomposition first — a DAG is required before processing the plan.',
			),
		).toBeVisible()
		await expect(
			page.getByRole('button', { name: 'Process plan here' }),
		).toBeDisabled()
	})

	// 6.2 — a decompose plan starts the fan-out directly. Mocked: unmocked this
	// starts a real per-stage planning agent for every stage of the DAG.
	test('starts the plan fan-out from a decompose plan', async ({
		page,
		apiGuard,
	}) => {
		const decompose = { ...dialog, mode: 'decompose' }
		await mockEmptyWorkspace(page)
		await mockDialogList(page, { items: [decompose] })
		await mockPlan(page, {
			dialog: decompose,
			summary: SUMMARY_MARKDOWN,
			dag: DAG_MARKDOWN,
			actionPlan: null,
		})
		await page.goto(`/workplace/${dialog.id}`)

		apiGuard.allow('POST', `${BASE}/plan-fanout`)
		await mockJson(
			page,
			`${BASE}/plan-fanout`,
			{ runId: 'run-1', status: 'running', startedAt: 1767182400, stages: [] },
			{ method: 'POST' },
		)

		const hero = page.getByRole('button', { name: 'Process plan here' })
		await expect(hero).toBeEnabled()

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/plan-fanout') && r.method() === 'POST',
		)
		await hero.click()
		await request
	})

	// 6.2 — an orchestrator (main) plan does not call the fan-out endpoint: it
	// asks its own agent to do the planning, as a chat message.
	test('asks the orchestrator to process the plan', async ({
		page,
		apiGuard,
	}) => {
		await openPlan(page, { actionPlan: null })
		apiGuard.allow('POST', `${BASE}/messages`)
		await mockJson(page, `${BASE}/messages`, { response: 'ok' }, { method: 'POST' })

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/messages') && r.method() === 'POST',
		)
		await page.getByRole('button', { name: 'Process plan here' }).click()

		expect((await request).postDataJSON().message).toContain(
			'action plan',
		)
	})

	// 6.3
	test('offers a replan once an action plan exists', async ({ page }) => {
		await openPlan(page)
		await expect(
			page.getByRole('button', { name: 'Replan all stages' }),
		).toBeVisible()
		await expect(
			page.getByRole('button', { name: 'Process plan here' }),
		).toHaveCount(0)
	})

	// 6.6
	test('checks a step and moves the progress bar', async ({ page, apiGuard }) => {
		await openPlan(page)
		apiGuard.allow('PUT', `${BASE}/action-plan/checks`)
		await mockJson(page, `${BASE}/action-plan/checks`, {
			checked: ['s0.step0'],
		})

		await expect(page.getByText('0 / 4 (0%)')).toBeVisible()

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/action-plan/checks') && r.method() === 'PUT',
		)
		await page.getByRole('checkbox').first().check()

		expect(JSON.stringify((await request).postDataJSON())).toContain('s0.step0')
	})

	// 6.7
	test('opens the edit dialog for an action', async ({ page }) => {
		await openPlan(page)

		await actionRow(page, '1.1')
			.getByRole('button', { name: 'Edit action' })
			.click()

		const modal = page.getByRole('dialog')
		await expect(modal).toBeVisible()
		await expect(modal).toContainText('Edit action')
		await modal.getByRole('button', { name: 'Cancel' }).click()
		await expect(modal).toHaveCount(0)
	})

	// 6.8
	test('adds a comment to an action', async ({ page, apiGuard }) => {
		await openPlan(page)
		apiGuard.allow('PUT', `${BASE}/action-plan/comments`)
		await mockJson(page, `${BASE}/action-plan/comments`, {
			comments: { 's0.step0': 'Watch the rollout' },
		})

		await actionRow(page, '1.1')
			.getByRole('button', { name: 'Add comment' })
			.click()

		const modal = page.getByRole('dialog')
		await expect(modal).toContainText('Action comment')
		await modal.getByRole('textbox').fill('Watch the rollout')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/action-plan/comments') && r.method() === 'PUT',
		)
		await modal.getByRole('button', { name: 'Save' }).click()

		expect(JSON.stringify((await request).postDataJSON())).toContain(
			'Watch the rollout',
		)
	})

	// 6.11
	test('renders the rollback scope', async ({ page }) => {
		await openPlan(page)
		await expect(
			page.getByRole('heading', { name: 'Rollback', level: 3 }),
		).toBeVisible()
		await expect(page.getByText('Roll the release back')).toBeVisible()
	})

	// 6.12
	test('distinguishes code steps from command steps', async ({ page }) => {
		await openPlan(page)

		// A code step carries its repository and cannot run without an executor.
		const codeStep = actionRow(page, '1.2')
		await expect(codeStep).toContainText('acme/helm-charts')
		await expect(
			codeStep.getByRole('button', { name: 'Execute action' }),
		).toBeDisabled()

		// A command step shows the command and a copy affordance.
		const commandStep = actionRow(page, '1.1')
		await expect(commandStep).toContainText('make release')
		await expect(
			commandStep.getByRole('button', { name: 'Copy command' }),
		).toBeVisible()
	})
})

test.describe('Action execution', () => {
	/**
	 * 7.2 — Execute on a `command` step does NOT call the executor: it posts a
	 * chat message to the orchestrator ("Execute plan item 1.1"), which then runs
	 * an execute sub-agent. Unmocked, this click is a real agent turn.
	 */
	test('executes a command step through the orchestrator chat', async ({
		page,
		apiGuard,
	}) => {
		await openPlan(page)
		apiGuard.allow('POST', `${BASE}/messages`)
		await mockJson(page, `${BASE}/messages`, { response: 'ok' }, { method: 'POST' })

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/messages') && r.method() === 'POST',
		)
		await actionRow(page, '1.1')
			.getByRole('button', { name: 'Execute action' })
			.click()

		expect((await request).postDataJSON()).toMatchObject({
			message: 'Execute plan item 1.1',
		})
	})

	// 7.1 — a code step needs an executor; with none configured it cannot run.
	test('disables execution of a code step without an executor', async ({
		page,
	}) => {
		await openPlan(page)
		await expect(
			actionRow(page, '1.2').getByRole('button', { name: 'Execute action' }),
		).toBeDisabled()
	})

	// 7.3 — a refusal must surface, not leave an optimistic running row behind.
	test('surfaces a failed execution request', async ({ page, apiGuard }) => {
		await openPlan(page)
		apiGuard.allow('POST', `${BASE}/messages`)
		await mockJson(
			page,
			`${BASE}/messages`,
			{ error: 'another execution is already running for plan other-plan' },
			{ status: 409, method: 'POST' },
		)

		await actionRow(page, '1.1')
			.getByRole('button', { name: 'Execute action' })
			.click()

		await expect(page.getByText(/already running/)).toBeVisible()
	})

	// 7.5
	test('offers a re-run once an action has finished', async ({ page }) => {
		await openPlan(page, {
			execRuns: {
				's0.step0': {
					status: 'failed',
					attempt: 1,
					startedAt: 1767182400,
					finishedAt: 1767182460,
					error: 'exit status 1',
					kind: 'subagent',
				},
			},
		})

		await expect(
			actionRow(page, '1.1').getByRole('button', { name: 'Run action again' }),
		).toBeVisible()
	})

	// 7.4 — a running action offers the force-stop that frees the global lease.
	test('offers a force-stop while an action is running', async ({ page }) => {
		await openPlan(page, {
			execRuns: {
				's0.step0': {
					status: 'running',
					attempt: 1,
					startedAt: 1767182400,
					kind: 'subagent',
				},
			},
		})

		await expect(
			actionRow(page, '1.1').getByRole('button', { name: 'Stop execution' }),
		).toBeVisible()
	})
})
