import { test, expect, mockJson } from '../fixtures/nib-test'
import { mockEmptyWorkspace } from '../fixtures/mock-api'

const CONFIG = '/api/v1/executor/config'

const options = {
	typeOptions: [
		{ value: 'disabled', enabled: true },
		{ value: 'local', enabled: true },
		{ value: 'remote', enabled: true },
	],
	platformOptions: [
		{ value: 'docker', enabled: true },
		{ value: 'kubernetes', enabled: true },
		{ value: 'kubefoundry', enabled: true },
	],
	kubernetesAuthModeOptions: [
		{ value: 'local_config', enabled: true },
		{ value: 'token', enabled: true },
	],
	agentOptions: [
		{ value: 'claude-code', enabled: true },
		{ value: 'codex', enabled: true },
	],
	authTypeOptions: [
		{ value: 'api_key', enabled: true },
		{ value: 'oauth_token', enabled: true },
	],
}

function config(overrides: Record<string, unknown> = {}) {
	return {
		type: 'disabled',
		platform: '',
		kubernetesAuthMode: '',
		kubernetesContext: '',
		kubernetesHost: '',
		kubernetesTokenSecretName: '',
		kubernetesCACert: '',
		kubernetesInsecureSkipTLSVerify: false,
		namespace: '',
		serviceAccount: '',
		agentSecretName: '',
		agentLimitCPU: '',
		agentLimitMemory: '',
		agentRequestCPU: '',
		agentRequestMemory: '',
		agentMCPConfig: '',
		jobTTLSeconds: 3600,
		agent: 'claude-code',
		authType: 'api_key',
		tokenSecretName: '',
		baseURL: '',
		webhookBaseURL: '',
		image: '',
		llmModel: '',
		...options,
		...overrides,
	}
}

async function openExecutor(
	page: import('@playwright/test').Page,
	current = config(),
) {
	await mockEmptyWorkspace(page)
	await mockJson(page, '/api/v1/mcp/servers', [])
	await mockJson(page, CONFIG, current)
	await page.goto('/tools')
	await page.getByRole('tab', { name: 'Executor' }).click()
	await expect(page.getByRole('combobox', { name: 'Type', exact: true })).toBeVisible()
}

test.describe('Tools — executor', () => {
	// 16.1
	test('hides every runtime field while disabled', async ({ page }) => {
		await openExecutor(page)

		await expect(page.getByRole('combobox', { name: 'Type', exact: true })).toHaveValue('disabled')
		await expect(
			page.getByText(
				'No agent containers are launched and code actions cannot be executed.',
			),
		).toBeVisible()
		await expect(page.getByRole('combobox', { name: 'Agent' })).toHaveCount(0)
		await expect(page.getByRole('button', { name: 'Save' })).toBeDisabled()
	})

	// 16.2
	test('configures a local docker executor', async ({ page, apiGuard }) => {
		await openExecutor(page)
		apiGuard.allow('PUT', CONFIG)
		await mockJson(page, CONFIG, config({ type: 'local' }), { method: 'PUT' })

		await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('local')

		await expect(page.getByRole('combobox', { name: 'Agent' })).toBeVisible()
		await expect(page.getByRole('combobox', { name: 'Auth type' })).toBeVisible()
		await expect(page.getByRole('textbox', { name: 'Image' })).toBeVisible()
		await expect(
			page.getByRole('textbox', { name: 'Webhook base URL' }),
		).toBeVisible()

		await page.getByRole('textbox', { name: 'Image' }).fill('agent-runner:local')
		const request = page.waitForRequest(
			(r) => r.url().includes('/executor/config') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Save' }).click()

		expect((await request).postDataJSON()).toMatchObject({
			type: 'local',
			image: 'agent-runner:local',
		})
	})

	// 16.3
	test('offers a context name for in-cluster kubernetes auth', async ({ page }) => {
		await openExecutor(page)

		await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('remote')
		await page.getByRole('combobox', { name: 'Platform' }).selectOption('kubernetes')
		await page.getByRole('combobox', { name: 'Auth', exact: true }).selectOption('local_config')

		await expect(page.getByRole('textbox', { name: 'Context name' })).toBeVisible()
		await expect(
			page.getByPlaceholder('https://cluster.example.com:6443'),
		).toHaveCount(0)
	})

	// 16.4
	test('asks for a host and a secret name for token auth', async ({ page }) => {
		await openExecutor(page)

		await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('remote')
		await page.getByRole('combobox', { name: 'Platform' }).selectOption('kubernetes')
		await page.getByRole('combobox', { name: 'Auth', exact: true }).selectOption('token')

		await expect(page.getByPlaceholder('https://cluster.example.com:6443')).toBeVisible()
		await expect(
			page.getByPlaceholder('Optional PEM-encoded CA certificate'),
		).toBeVisible()
		// Credentials are referenced by secret name and never returned to the page.
		await expect(
			page.getByText(/referenced by secret name and never returned/i),
		).toBeVisible()
	})

	// 16.6
	test('drops hidden fields when the type is switched back', async ({
		page,
		apiGuard,
	}) => {
		await openExecutor(page)
		apiGuard.allow('PUT', CONFIG)
		await mockJson(page, CONFIG, config(), { method: 'PUT' })

		await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('local')
		await page.getByRole('textbox', { name: 'Image' }).fill('agent-runner:local')
		await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('disabled')

		await expect(page.getByRole('textbox', { name: 'Image' })).toHaveCount(0)

		const request = page.waitForRequest(
			(r) => r.url().includes('/executor/config') && r.method() === 'PUT',
		)
		await page.getByRole('button', { name: 'Save' }).click()
		expect((await request).postDataJSON()).toMatchObject({ type: 'disabled' })
	})

	// 16.7
	test('surfaces a rejected save', async ({ page, apiGuard }) => {
		await openExecutor(page)
		apiGuard.allow('PUT', CONFIG)
		await mockJson(
			page,
			CONFIG,
			{ error: 'webhookBaseURL is required for a remote executor' },
			{ status: 400, method: 'PUT' },
		)

		await page.getByRole('combobox', { name: 'Type', exact: true }).selectOption('local')
		await page.getByRole('button', { name: 'Save' }).click()

		await expect(page.getByText(/webhookBaseURL is required/)).toBeVisible()
	})
})
