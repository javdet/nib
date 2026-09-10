import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockDialogList, mockEmptyWorkspace, mockPlan } from '../fixtures/mock-api'
import { makeDialog } from '../fixtures/data'

const dialog = makeDialog({ id: 'plan-a', title: 'Deploy billing' })
const BASE = `/api/v1/dialogs/${dialog.id}`

const attachment = {
	id: 'att-1',
	dialogId: dialog.id,
	filename: 'rollout.log',
	contentType: 'text/plain',
	kind: 'text',
	sizeBytes: 42,
	createdAt: '2026-01-15T10:00:00Z',
}

async function openChat(page: import('@playwright/test').Page) {
	await mockEmptyWorkspace(page)
	await mockDialogList(page, { items: [dialog] })
	await mockPlan(page, { dialog, messages: [] })
	await page.goto(`/workplace/${dialog.id}`)
	await expect(page.getByRole('combobox', { name: 'Message…' })).toBeVisible()
}

function chatFileInput(page: import('@playwright/test').Page) {
	return page.getByRole('complementary').last().locator('input[type="file"]')
}

const LOG_FILE = {
	name: 'rollout.log',
	mimeType: 'text/plain',
	buffer: Buffer.from('deployed billing 1.4.2 to stage'),
}

test.describe('Chat attachments', () => {
	// 8.6
	test('accepts the documented file types only', async ({ page }) => {
		await openChat(page)

		await expect(chatFileInput(page)).toHaveAttribute(
			'accept',
			'image/*,text/*,.md,.txt,.csv,.json,.log,.yaml,.yml',
		)
		await expect(page.getByRole('button', { name: 'Attach file' })).toBeEnabled()
	})

	// 8.6
	test('uploads a file and shows its chip', async ({ page, apiGuard }) => {
		await openChat(page)
		apiGuard.allow('POST', `${BASE}/attachments`)
		await mockJson(page, `${BASE}/attachments`, attachment, { method: 'POST' })

		await chatFileInput(page).setInputFiles(LOG_FILE)

		await expect(page.getByText('rollout.log')).toBeVisible()
		await expect(
			page.getByRole('button', { name: 'Remove rollout.log' }),
		).toBeVisible()
		// An attachment alone is enough to send.
		await expect(page.getByRole('button', { name: 'Send message' })).toBeEnabled()
	})

	// 8.6 — the attachment id travels with the message.
	test('sends the message with its attachment', async ({ page, apiGuard }) => {
		await openChat(page)
		apiGuard.allow('POST', `${BASE}/attachments`)
		apiGuard.allow('POST', `${BASE}/messages`)
		await mockJson(page, `${BASE}/attachments`, attachment, { method: 'POST' })
		await mockJson(page, `${BASE}/messages`, { response: 'ok' }, { method: 'POST' })

		await chatFileInput(page).setInputFiles(LOG_FILE)
		await expect(page.getByText('rollout.log')).toBeVisible()
		await page.getByRole('combobox', { name: 'Message…' }).fill('What failed?')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/messages') && r.method() === 'POST',
		)
		await page.getByRole('button', { name: 'Send message' }).click()

		expect((await request).postDataJSON()).toMatchObject({
			message: 'What failed?',
			attachmentIds: ['att-1'],
		})
	})

	// 8.6 — removing before sending deletes the upload again.
	test('removes a pending attachment', async ({ page, apiGuard }) => {
		await openChat(page)
		apiGuard.allow('POST', `${BASE}/attachments`)
		apiGuard.allow('DELETE', `${BASE}/attachments/att-1`)
		await mockJson(page, `${BASE}/attachments`, attachment, { method: 'POST' })
		await mockJson(page, `${BASE}/attachments/att-1`, null, {
			status: 204,
			method: 'DELETE',
		})

		await chatFileInput(page).setInputFiles(LOG_FILE)
		const chip = page.getByRole('button', { name: 'Remove rollout.log' })
		await expect(chip).toBeVisible()

		const request = page.waitForRequest(
			(r) => r.url().includes('/attachments/att-1') && r.method() === 'DELETE',
		)
		await chip.click()

		await request
		await expect(chip).toHaveCount(0)
		await expect(page.getByRole('button', { name: 'Send message' })).toBeDisabled()
	})

	// 8.6 — an image chip carries a thumbnail rather than a generic icon.
	test('previews an uploaded image', async ({ page, apiGuard }) => {
		await openChat(page)
		apiGuard.allow('POST', `${BASE}/attachments`)
		await mockJson(
			page,
			`${BASE}/attachments`,
			{
				...attachment,
				id: 'att-img',
				filename: 'graph.png',
				contentType: 'image/png',
				kind: 'image',
				url: 'data:image/gif;base64,R0lGODlhAQABAAAAACw=',
			},
			{ method: 'POST' },
		)

		await chatFileInput(page).setInputFiles({
			name: 'graph.png',
			mimeType: 'image/png',
			buffer: Buffer.from('89504e470d0a1a0a', 'hex'),
		})

		await expect(page.getByRole('img', { name: 'graph.png' })).toBeVisible()
	})

	/**
	 * 8.7 — `accept` filters the file picker, nothing more: a file chosen another
	 * way (drag, a scripted set, a renamed extension) is uploaded and only the
	 * backend can refuse it. This asserts that the refusal reaches the operator
	 * rather than being swallowed.
	 */
	test('leaves an unsupported file type to the backend', async ({
		page,
		apiGuard,
	}) => {
		await openChat(page)
		apiGuard.allow('POST', `${BASE}/attachments`)
		await mockJson(
			page,
			`${BASE}/attachments`,
			{ error: 'unsupported attachment type: application/x-msdownload' },
			{ status: 415, method: 'POST' },
		)

		const request = page.waitForRequest(
			(r) => r.url().includes('/attachments') && r.method() === 'POST',
		)
		await chatFileInput(page).setInputFiles({
			name: 'payload.exe',
			mimeType: 'application/x-msdownload',
			buffer: Buffer.from('MZ'),
		})

		// The client does not filter it out; the request is made.
		await request
		await expect(page.getByText(/unsupported attachment type/)).toBeVisible()
		await expect(
			page.getByRole('button', { name: 'Remove payload.exe' }),
		).toHaveCount(0)
	})

	// 8.8
	test('reports a failed upload and leaves no chip behind', async ({
		page,
		apiGuard,
	}) => {
		await openChat(page)
		apiGuard.allow('POST', `${BASE}/attachments`)
		await mockJson(
			page,
			`${BASE}/attachments`,
			{ error: 'attachment store is full' },
			{ status: 500, method: 'POST' },
		)

		await chatFileInput(page).setInputFiles(LOG_FILE)

		await expect(page.getByText(/attachment store is full/)).toBeVisible()
		await expect(
			page.getByRole('button', { name: 'Remove rollout.log' }),
		).toHaveCount(0)
		await expect(page.getByRole('button', { name: 'Send message' })).toBeDisabled()
	})
})

test.describe('Chat turn control', () => {
	// 8.5 — Stop replaces Send while a turn is in flight, and aborts it.
	test('stops a running turn', async ({ page, apiGuard }) => {
		await openChat(page)
		apiGuard.allow('POST', `${BASE}/messages`)

		// A turn that never answers on its own: only the abort can end it.
		await page.route(
			(url) => url.pathname === `${BASE}/messages`,
			async (route) => {
				if (route.request().method() !== 'POST') return route.fallback()
				await new Promise((resolve) => setTimeout(resolve, 20_000))
				await route.fulfill({ status: 200, body: '{}' })
			},
		)

		const input = page.getByRole('combobox', { name: 'Message…' })
		await input.fill('Deploy billing to stage')
		await page.getByRole('button', { name: 'Send message' }).click()

		const stop = page.getByRole('button', { name: 'Stop generating' })
		await expect(stop).toBeVisible()
		await expect(input).toBeDisabled()

		await stop.click()

		await expect(page.getByRole('button', { name: 'Send message' })).toBeVisible()
		await expect(input).toBeEnabled()
	})
})
