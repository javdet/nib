import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

const SKILLS = '/api/v1/skills'

const jiraSkill = [
	'---',
	'name: jira-get-board',
	'description: Fetch a Jira board and summarise its columns',
	'---',
	'',
	'## Workflow',
	'',
	'1. Resolve the board id.',
].join('\n')

async function openSkills(
	page: import('@playwright/test').Page,
	skills = [
		{ name: 'jira-get-board', description: 'Fetch a Jira board and summarise its columns' },
	],
) {
	await mockEmptyWorkspace(page)
	await mockJson(page, SKILLS, { skills })
	await mockJson(page, `${SKILLS}/jira-get-board`, {
		name: 'jira-get-board',
		content: jiraSkill,
	})
	await page.goto('/skills')
	await expect(page.getByRole('heading', { name: 'Skills', level: 2 })).toBeVisible()
}

test.describe('Skills', () => {
	// 12.1
	test('lists skills and opens one', async ({ page }) => {
		await openSkills(page)

		await page.getByRole('button', { name: 'jira-get-board.md' }).click()
		await expect(page.getByRole('textbox', { name: 'Name' })).toHaveValue(
			'jira-get-board',
		)
		await expect(page.getByRole('textbox', { name: 'Body' })).toHaveValue(
			/Resolve the board id/,
		)
	})

	// 12.2
	test('creates a skill', async ({ page, apiGuard }) => {
		await openSkills(page, [])
		apiGuard.allow('POST', SKILLS)
		await mockJson(page, SKILLS, { name: 'e2e-skill', content: '' }, { method: 'POST' })

		await page.getByRole('button', { name: 'Add skill' }).click()
		const modal = page.getByRole('dialog')
		await modal.getByRole('textbox', { name: 'File name' }).fill('e2e-skill')

		const request = page.waitForRequest(
			(r) => r.url().endsWith('/skills') && r.method() === 'POST',
		)
		await modal.getByRole('button', { name: 'Create' }).click()

		expect((await request).postDataJSON().name).toBe('e2e-skill')
	})

	/**
	 * 12.5 — the editor splits frontmatter into Name/Description fields and the
	 * rest into Body. Saving has to put the frontmatter back, not drop it or
	 * duplicate it into the body.
	 */
	test('preserves the frontmatter when saving', async ({ page, apiGuard }) => {
		await openSkills(page)
		apiGuard.allow('PUT', `${SKILLS}/jira-get-board`)
		await mockJson(page, `${SKILLS}/jira-get-board`, null, {
			status: 204,
			method: 'PUT',
		})

		await page.getByRole('button', { name: 'jira-get-board.md' }).click()
		await page.getByRole('textbox', { name: 'Body' }).fill('## Workflow\n\n1. New body.')

		const request = page.waitForRequest(
			(r) => r.url().includes('/skills/jira-get-board') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Save' }).click()

		const content: string = (await request).postDataJSON().content
		expect(content).toContain('name: jira-get-board')
		expect(content).toContain('description: Fetch a Jira board')
		expect(content).toContain('1. New body.')
		// Exactly one frontmatter block.
		expect(content.split('---').length - 1).toBe(2)
	})

	// 12.2 (delete)
	test('deletes a skill after the native confirm', async ({ page, apiGuard }) => {
		await openSkills(page)
		apiGuard.allow('DELETE', `${SKILLS}/jira-get-board`)
		await mockJson(page, `${SKILLS}/jira-get-board`, null, {
			status: 204,
			method: 'DELETE',
		})

		await page.getByRole('button', { name: 'jira-get-board.md' }).click()
		page.on('dialog', (d) => void d.accept())

		const request = page.waitForRequest(
			(r) => r.url().includes('/skills/jira-get-board') && r.method() === 'DELETE',
		)
		await page.getByRole('button', { name: 'Delete' }).click()
		await request
	})

	/**
	 * 12.4 — system skills (nib-configuration, nib-internals) are served from the
	 * binary for the agent only. They must never reach the HTTP list, so this one
	 * runs against the live backend rather than a fixture.
	 */
	test('never lists the system skills', async ({ page }) => {
		await mockEmptyWorkspace(page)
		await page.goto('/skills')
		await expect(page.getByRole('heading', { name: 'Skills', level: 2 })).toBeVisible()

		await expect(page.getByText('nib-configuration')).toHaveCount(0)
		await expect(page.getByText('nib-internals')).toHaveCount(0)
	})
})
