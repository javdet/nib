import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace, mockPlan } from '../fixtures/mock-api'
import {
	makeAskQuestionMessages,
	makeDialog,
	makeMessages,
} from '../fixtures/data'

const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })
const BASE = `/api/v1/dialogs/${dialog.id}`

async function openChat(
	page: import('@playwright/test').Page,
	options: Record<string, unknown> = {},
) {
	await mockEmptyWorkspace(page)
	await mockDialogList(page, { items: [dialog] })
	await mockPlan(page, { dialog, ...options })
	await page.goto(`/workplace/${dialog.id}`)
}

test.describe('Chat panel', () => {
	// 8.1
	test('starts empty with sending disabled', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await page.goto('/workplace')

		await expect(
			page.getByText(
				'Messages will appear here. Select a dialog or send a message to start a new one.',
			),
		).toBeVisible()
		await expect(
			page.getByRole('button', { name: 'Send message' }),
		).toBeDisabled()
	})

	// 8.1 (enablement)
	test('enables sending once the draft is not empty', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await page.goto('/workplace')

		await page.getByRole('combobox', { name: 'Message…' }).fill('hello')
		await expect(page.getByRole('button', { name: 'Send message' })).toBeEnabled()
	})

	// 8.2
	test('renders a transcript', async ({ page }) => {
		await openChat(page, { messages: makeMessages() })

		await expect(page.getByText('Deploy billing to stage')).toBeVisible()
		await expect(page.getByText('Starting decomposition.')).toBeVisible()
		// Fenced code is rendered as code, not as literal backticks.
		await expect(page.getByText('helm ls -n stage')).toBeVisible()
		await expect(page.getByText('```')).toHaveCount(0)
	})

	// 8.3 — mocked: an unmocked send is a real agent turn.
	test('sends a message', async ({ page, apiGuard }) => {
		await openChat(page, { messages: [] })
		apiGuard.allow('POST', `${BASE}/messages`)
		await mockJson(page, `${BASE}/messages`, { response: 'On it.' }, { method: 'POST' })

		const input = page.getByRole('combobox', { name: 'Message…' })
		await input.fill('Deploy billing to stage')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/messages') && r.method() === 'POST',
		)
		await page.getByRole('button', { name: 'Send message' }).click()

		expect((await request).postDataJSON()).toMatchObject({
			message: 'Deploy billing to stage',
			attachmentIds: [],
		})
		await expect(input).toBeEmpty()
	})

	// 8.4
	test('sends on Enter and keeps Shift+Enter for a newline', async ({
		page,
		apiGuard,
	}) => {
		await openChat(page, { messages: [] })
		apiGuard.allow('POST', `${BASE}/messages`)
		await mockJson(page, `${BASE}/messages`, { response: 'ok' }, { method: 'POST' })

		const input = page.getByRole('combobox', { name: 'Message…' })
		await input.fill('first line')
		await input.press('Shift+Enter')
		await input.type('second line')
		expect(await input.inputValue()).toContain('\n')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/messages') && r.method() === 'POST',
		)
		await input.press('Enter')
		expect((await request).postDataJSON().message).toContain('second line')
	})

	// 8.9
	test('answers a clarifying question before anything else', async ({
		page,
		apiGuard,
	}) => {
		await openChat(page, { messages: makeAskQuestionMessages('call_ask_1') })

		await expect(page.getByText('Clarifying question')).toBeVisible()
		await expect(page.getByText('Which cluster is stage on?')).toBeVisible()
		// The ordinary input is out of play until the question is answered.
		await expect(page.getByRole('combobox', { name: 'Message…' })).toBeDisabled()

		apiGuard.allow('POST', `${BASE}/tool-results`)
		await mockJson(page, `${BASE}/tool-results`, { response: 'Thanks.' }, {
			method: 'POST',
		})

		await page.getByRole('textbox', { name: 'Your answer' }).fill('stage-eu-1')
		const request = page.waitForRequest(
			(r) => r.url().endsWith('/tool-results') && r.method() === 'POST',
		)
		await page.getByRole('button', { name: 'Submit' }).click()

		expect((await request).postDataJSON()).toMatchObject({
			toolCallId: 'call_ask_1',
			answers: ['stage-eu-1'],
		})
	})

	// 8.13
	test('keeps the bound dialog across navigation', async ({ page }) => {
		await openChat(page, { messages: makeMessages() })
		await expect(page.getByText('Starting decomposition.')).toBeVisible()

		await page.getByRole('link', { name: 'Tools' }).click()
		await expect(page.getByRole('heading', { name: 'Tools', level: 2 })).toBeVisible()

		// The transcript travels with the shell rather than resetting.
		await expect(page.getByText('Starting decomposition.')).toHaveCount(1)
	})
})
