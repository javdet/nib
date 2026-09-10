import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'
import { makeDialog } from '../fixtures/data'

const COMPANY = '/api/v1/company'
const COLLECTIONS = '/api/v1/knowledge/collections'
const STATUS = '/api/v1/knowledge/status'
const DOCUMENTS = '/api/v1/knowledge/documents'

const company = {
	companyName: 'Acme',
	companyDescription: 'Payments',
	versionControlSystem: 'GitHub',
	gitBaseUrl: 'https://github.com/acme',
	gitUsername: 'agent-runner',
	gitEmail: 'agent-runner@acme.test',
	ciCdSystem: 'GitHub Actions',
	taskTracker: 'Jira',
	issueProject: 'DEVOPS',
	wiki: 'Confluence',
	messenger: 'Slack',
}

async function openKnowledge(page: import('@playwright/test').Page) {
	await mockEmptyWorkspace(page)
	await mockJson(page, COMPANY, company)
	await mockJson(page, COLLECTIONS, [
		{ name: 'default', chunkCount: 12, embeddingModel: 'text-embedding-3-small' },
	])
	await mockJson(page, STATUS, {
		collectionName: 'default',
		connectionUri: 'postgres://kb',
		chunkCount: 12,
	})
	await page.goto('/knowledge')
	await expect(
		page.getByRole('heading', { name: 'Knowledge Base', level: 2 }),
	).toBeVisible()
}

test.describe('Knowledgebase', () => {
	// 10.1
	test('renders the company form and the upload card', async ({ page }) => {
		await openKnowledge(page)

		await expect(page.getByText('Company Information')).toBeVisible()
		await expect(page.getByRole('textbox', { name: 'Company name' })).toHaveValue(
			'Acme',
		)
		await expect(page.getByText('Document upload')).toBeVisible()
		await expect(page.getByText('Chunks: 12')).toBeVisible()
	})

	// 10.2
	test('saves the company information', async ({ page, apiGuard }) => {
		await openKnowledge(page)
		apiGuard.allow('PUT', COMPANY)
		await mockJson(page, COMPANY, company, { method: 'PUT' })

		// Nothing to save until something changes.
		await expect(page.getByRole('button', { name: 'Save' }).first()).toBeDisabled()

		await page.getByRole('textbox', { name: 'Company name' }).fill('Acme Payments')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/company') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Save' }).first().click()

		expect((await request).postDataJSON()).toMatchObject({
			companyName: 'Acme Payments',
			issueProject: 'DEVOPS',
		})
	})

	// 10.4
	test('warns that an upload replaces the whole collection', async ({ page }) => {
		await openKnowledge(page)

		await expect(
			page.getByText(/replaces all chunks in the/),
		).toBeVisible()
		await expect(page.getByText('Only one document is kept at a time.')).toBeVisible()
	})

	// 10.5
	test('views the current document', async ({ page }) => {
		await openKnowledge(page)
		await mockJson(page, DOCUMENTS, {
			collection: 'default',
			filename: 'kb.md',
			content: '# Knowledge Base\n\nProject overview goes here.',
			source: 'uploaded',
		})

		await page.getByRole('button', { name: 'View current' }).click()
		await expect(page.getByRole('dialog')).toContainText(
			'Project overview goes here.',
		)
	})

	// 10.6 — mocked: a real upload re-embeds and replaces the collection.
	test('uploads and indexes a document', async ({ page, apiGuard }) => {
		await openKnowledge(page)
		apiGuard.allow('POST', DOCUMENTS)
		await mockJson(
			page,
			DOCUMENTS,
			{ filename: 'kb.md', chunkCount: 20, collection: 'default' },
			{ method: 'POST' },
		)

		const upload = page.getByRole('button', { name: 'Upload & index' })
		await expect(upload).toBeDisabled()

		// The chat panel has a file input of its own; take the page's own one.
		await page.getByRole('main').locator('input[type="file"]').setInputFiles({
			name: 'kb.md',
			mimeType: 'text/markdown',
			buffer: Buffer.from('# Knowledge Base\n\nStage runs on eu-1.'),
		})
		await expect(upload).toBeEnabled()

		const request = page.waitForRequest(
			(r) => r.url().includes('/knowledge/documents') && r.method() === 'POST',
		)
		await upload.click()
		expect((await request).postData()).toContain('kb.md')
	})

	// 10.7
	test('needs repositories before it can generate', async ({ page }) => {
		await openKnowledge(page)

		const generate = page.getByRole('button', { name: 'Generate knowledge base' })
		await expect(generate).toBeDisabled()

		await page
			.getByRole('textbox', { name: 'Repositories' })
			.fill('acme/infra, https://github.com/acme/helm-charts')
		await expect(generate).toBeEnabled()
	})

	/**
	 * 10.8 — generating opens a discuss dialog and asks the agent to build the
	 * document. Both writes are mocked: unmocked this is a real agent run that
	 * reads repositories over MCP and rewrites the collection.
	 */
	test('generates the knowledge base through a discuss dialog', async ({
		page,
		apiGuard,
	}) => {
		await openKnowledge(page)
		const discuss = makeDialog({ id: 'kb-dialog', mode: 'discuss' })
		apiGuard.allow('POST', '/api/v1/dialogs')
		await mockJson(page, '/api/v1/dialogs', discuss, { method: 'POST' })

		await page.getByRole('textbox', { name: 'Repositories' }).fill('acme/infra')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/api/v1/dialogs') && r.method() === 'POST',
		)
		await page.getByRole('button', { name: 'Generate knowledge base' }).click()

		expect((await request).postDataJSON()).toMatchObject({ mode: 'discuss' })
	})
})
