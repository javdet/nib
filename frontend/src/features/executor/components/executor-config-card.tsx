import { useCallback, useEffect, useId, useState } from 'react'
import { Box, ChevronDown, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Textarea } from '@/components/ui/textarea'
import { Select } from '@/components/ui/select'
import { extractErrorMessage } from '@/lib/api-client'
import {
	getExecutorConfig,
	updateExecutorConfig,
	type ExecutorAgent,
	type ExecutorAuthType,
	type ExecutorConfig,
	type ExecutorKubernetesAuthMode,
	type ExecutorOption,
	type ExecutorPlatform,
	type ExecutorType,
} from '../api/executor'
import { listSecrets, type Secret } from '@/features/secrets/api/secrets'

const TYPE_LABELS: Record<ExecutorType, string> = {
	local: 'Local (Docker socket)',
	remote: 'Remote',
}

const PLATFORM_LABELS: Record<ExecutorPlatform, string> = {
	docker: 'Docker',
	kubernetes: 'Kubernetes',
	kubefoundry: 'KubeFoundry',
}

const KUBERNETES_AUTH_MODE_LABELS: Record<ExecutorKubernetesAuthMode, string> = {
	local_config: 'Local Config',
	token: 'Token',
}

const AGENT_LABELS: Record<ExecutorAgent, string> = {
	'claude-code': 'Claude Code',
	codex: 'Codex',
}

const AUTH_TYPE_LABELS: Record<ExecutorAuthType, string> = {
	api_key: 'API KEY',
	oauth_token: 'OAUTH TOKEN',
}

function optionLabel<T extends string>(
	opt: ExecutorOption<T>,
	labels: Record<T, string>,
): string {
	const label = labels[opt.value] ?? opt.value
	return opt.enabled ? label : `${label} (not available yet)`
}

function firstEnabled<T extends string>(
	options: ExecutorOption<T>[],
): T | undefined {
	return options.find((opt) => opt.enabled)?.value
}

export function ExecutorConfigCard() {
	const typeId = useId()
	const platformId = useId()
	const kubernetesAuthModeId = useId()
	const kubernetesContextId = useId()
	const kubernetesHostId = useId()
	const kubernetesCACertId = useId()
	const kubernetesInsecureSkipTLSVerifyId = useId()
	const kubernetesTokenSecretId = useId()
	const agentId = useId()
	const authTypeId = useId()
	const tokenSecretId = useId()
	const baseURLId = useId()
	const webhookBaseURLId = useId()
	const imageId = useId()
	const llmModelId = useId()
	const serviceAccountId = useId()
	const namespaceId = useId()
	const agentSecretNameId = useId()
	const agentLimitCPUId = useId()
	const agentLimitMemoryId = useId()
	const agentRequestCPUId = useId()
	const agentRequestMemoryId = useId()
	const agentMCPConfigId = useId()
	const jobTTLSecondsId = useId()
	const [expanded, setExpanded] = useState(true)
	const [advancedExpanded, setAdvancedExpanded] = useState(false)
	const [loading, setLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const [saved, setSaved] = useState<ExecutorConfig | null>(null)
	const [type, setType] = useState<ExecutorType>('local')
	const [platform, setPlatform] = useState<ExecutorPlatform | ''>('')
	const [kubernetesAuthMode, setKubernetesAuthMode] = useState<
		ExecutorKubernetesAuthMode | ''
	>('local_config')
	const [kubernetesContext, setKubernetesContext] = useState('')
	const [kubernetesHost, setKubernetesHost] = useState('')
	const [kubernetesCACert, setKubernetesCACert] = useState('')
	const [kubernetesInsecureSkipTLSVerify, setKubernetesInsecureSkipTLSVerify] =
		useState(false)
	const [kubernetesTokenSecretName, setKubernetesTokenSecretName] = useState('')
	const [namespace, setNamespace] = useState('default')
	const [agentSecretName, setAgentSecretName] = useState('')
	const [agentLimitCPU, setAgentLimitCPU] = useState('1')
	const [agentLimitMemory, setAgentLimitMemory] = useState('1Gi')
	const [agentRequestCPU, setAgentRequestCPU] = useState('1')
	const [agentRequestMemory, setAgentRequestMemory] = useState('256Mi')
	const [agentMCPConfig, setAgentMCPConfig] = useState('')
	const [jobTTLSeconds, setJobTTLSeconds] = useState(3600)
	const [serviceAccount, setServiceAccount] = useState('default')
	const [agent, setAgent] = useState<ExecutorAgent>('claude-code')
	const [authType, setAuthType] = useState<ExecutorAuthType>('api_key')
	const [tokenSecretName, setTokenSecretName] = useState('')
	const [baseURL, setBaseURL] = useState('')
	const [webhookBaseURL, setWebhookBaseURL] = useState('')
	const [image, setImage] = useState('')
	const [llmModel, setLlmModel] = useState('')
	const [secrets, setSecrets] = useState<Secret[]>([])

	const showKubernetes = type === 'remote' && platform === 'kubernetes'
	const showLocalConfig =
		showKubernetes && kubernetesAuthMode === 'local_config'
	const showTokenAuth = showKubernetes && kubernetesAuthMode === 'token'
	const showDetails = type === 'local' || (type === 'remote' && platform !== '')
	const showClaudeCodeSettings = agent === 'claude-code'
	const showApiKeySettings = showClaudeCodeSettings && authType === 'api_key'
	const showOAuthSettings = showClaudeCodeSettings && authType === 'oauth_token'

	const dirty =
		saved !== null &&
		(type !== saved.type ||
			platform !== saved.platform ||
			kubernetesAuthMode !== (saved.kubernetesAuthMode || 'local_config') ||
			kubernetesContext !== saved.kubernetesContext ||
			kubernetesHost !== saved.kubernetesHost ||
			kubernetesTokenSecretName !== saved.kubernetesTokenSecretName ||
			kubernetesCACert !== saved.kubernetesCACert ||
			kubernetesInsecureSkipTLSVerify !==
				(saved.kubernetesInsecureSkipTLSVerify ?? false) ||
			namespace !== (saved.namespace || 'default') ||
			serviceAccount !== (saved.serviceAccount || 'default') ||
			agentSecretName !== saved.agentSecretName ||
			agentLimitCPU !== (saved.agentLimitCPU || '1') ||
			agentLimitMemory !== (saved.agentLimitMemory || '1Gi') ||
			agentRequestCPU !== (saved.agentRequestCPU || '1') ||
			agentRequestMemory !== (saved.agentRequestMemory || '256Mi') ||
			agentMCPConfig !== saved.agentMCPConfig ||
			jobTTLSeconds !== (saved.jobTTLSeconds || 3600) ||
			agent !== saved.agent ||
			authType !== saved.authType ||
			tokenSecretName !== saved.tokenSecretName ||
			baseURL !== saved.baseURL ||
			webhookBaseURL !== saved.webhookBaseURL ||
			image !== saved.image ||
			llmModel !== saved.llmModel)

	const applyConfig = useCallback((cfg: ExecutorConfig) => {
		setSaved(cfg)
		setType(cfg.type)
		setPlatform(cfg.platform ?? '')
		setKubernetesAuthMode(cfg.kubernetesAuthMode || 'local_config')
		setKubernetesContext(cfg.kubernetesContext ?? '')
		setKubernetesHost(cfg.kubernetesHost ?? '')
		setKubernetesTokenSecretName(cfg.kubernetesTokenSecretName ?? '')
		setKubernetesCACert(cfg.kubernetesCACert ?? '')
		setKubernetesInsecureSkipTLSVerify(
			cfg.kubernetesInsecureSkipTLSVerify ?? false,
		)
		setNamespace(cfg.namespace || 'default')
		setServiceAccount(cfg.serviceAccount || 'default')
		setAgentSecretName(cfg.agentSecretName ?? '')
		setAgentLimitCPU(cfg.agentLimitCPU || '1')
		setAgentLimitMemory(cfg.agentLimitMemory || '1Gi')
		setAgentRequestCPU(cfg.agentRequestCPU || '1')
		setAgentRequestMemory(cfg.agentRequestMemory || '256Mi')
		setAgentMCPConfig(cfg.agentMCPConfig ?? '')
		setJobTTLSeconds(cfg.jobTTLSeconds || 3600)
		setAgent(cfg.agent ?? 'claude-code')
		setAuthType(cfg.authType ?? 'api_key')
		setTokenSecretName(cfg.tokenSecretName ?? '')
		setBaseURL(cfg.baseURL ?? '')
		setWebhookBaseURL(cfg.webhookBaseURL ?? '')
		setImage(cfg.image)
		setLlmModel(cfg.llmModel ?? '')
	}, [])

	const load = useCallback(async () => {
		setLoading(true)
		setError(null)
		try {
			const [cfg, secretList] = await Promise.all([
				getExecutorConfig(),
				listSecrets(),
			])
			setSecrets(secretList)
			applyConfig(cfg)
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setLoading(false)
		}
	}, [applyConfig])

	useEffect(() => {
		void load()
	}, [load])

	const handleSave = async () => {
		setSaving(true)
		setError(null)
		try {
			applyConfig(
				await updateExecutorConfig({
					type,
					platform: type === 'remote' ? platform : '',
					kubernetesAuthMode: showKubernetes ? kubernetesAuthMode : '',
					kubernetesContext: showLocalConfig ? kubernetesContext : '',
					kubernetesHost: showTokenAuth ? kubernetesHost : '',
					kubernetesTokenSecretName: showTokenAuth
						? kubernetesTokenSecretName
						: '',
					kubernetesCACert: showTokenAuth ? kubernetesCACert : '',
					kubernetesInsecureSkipTLSVerify: showTokenAuth
						? kubernetesInsecureSkipTLSVerify
						: false,
					namespace: showKubernetes ? namespace : '',
					serviceAccount: showKubernetes ? serviceAccount : '',
					agentSecretName: showKubernetes ? agentSecretName : '',
					agentLimitCPU: showKubernetes ? agentLimitCPU : '',
					agentLimitMemory: showKubernetes ? agentLimitMemory : '',
					agentRequestCPU: showKubernetes ? agentRequestCPU : '',
					agentRequestMemory: showKubernetes ? agentRequestMemory : '',
					agentMCPConfig: showKubernetes ? agentMCPConfig : '',
					jobTTLSeconds: showKubernetes ? jobTTLSeconds : 0,
					agent,
					authType: showClaudeCodeSettings ? authType : 'api_key',
					tokenSecretName: showClaudeCodeSettings ? tokenSecretName : '',
					baseURL: showApiKeySettings ? baseURL : '',
					webhookBaseURL,
					image,
					llmModel,
				}),
			)
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setSaving(false)
		}
	}

	return (
		<Card>
			<CardHeader className="pb-3">
				<div
					role="button"
					tabIndex={0}
					aria-expanded={expanded}
					className="flex min-w-0 cursor-pointer items-center gap-2 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
					onClick={() => setExpanded((prev) => !prev)}
					onKeyDown={(e) => {
						if (e.key === 'Enter' || e.key === ' ') {
							e.preventDefault()
							setExpanded((prev) => !prev)
						}
					}}
				>
					{expanded ? (
						<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
					) : (
						<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
					)}
					<Box className="h-4 w-4 shrink-0 text-muted-foreground" />
					<CardTitle className="text-base">Executor</CardTitle>
				</div>
			</CardHeader>
			{expanded && (
				<CardContent className="space-y-4">
					<p className="text-sm text-muted-foreground">
						Configure how agent-runner containers are launched. Agent auth
						credentials and Kubernetes tokens are referenced by secret name and
						never returned to this page.
					</p>

					{error && (
						<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
							{error}
						</div>
					)}

					{loading || !saved ? (
						<p className="py-2 text-sm text-muted-foreground">Loading...</p>
					) : (
						<div className="grid gap-4 sm:grid-cols-2">
							<div className="space-y-2">
								<Label htmlFor={typeId}>Type</Label>
								<Select
									id={typeId}
									value={type}
									onChange={(e) => {
										const next = e.target.value as ExecutorType
										setType(next)
										if (next !== 'remote') {
											setPlatform('')
											return
										}
										if (
											!platform ||
											!saved.platformOptions.some(
												(opt) =>
													opt.value === platform && opt.enabled,
											)
										) {
											setPlatform(
												firstEnabled(saved.platformOptions) ?? '',
											)
										}
									}}
									disabled={saving}
								>
									{saved.typeOptions.map((opt) => (
										<option
											key={opt.value}
											value={opt.value}
											disabled={!opt.enabled}
										>
											{optionLabel(opt, TYPE_LABELS)}
										</option>
									))}
								</Select>
							</div>
							{type === 'remote' && (
								<div className="space-y-2">
									<Label htmlFor={platformId}>Platform</Label>
									<Select
										id={platformId}
										value={platform}
										onChange={(e) =>
											setPlatform(e.target.value as ExecutorPlatform)
										}
										disabled={saving}
									>
										{saved.platformOptions.map((opt) => (
											<option
												key={opt.value}
												value={opt.value}
												disabled={!opt.enabled}
											>
												{optionLabel(opt, PLATFORM_LABELS)}
											</option>
										))}
									</Select>
								</div>
							)}
						</div>
					)}

					{!loading && saved && showKubernetes && (
						<div className="grid gap-4 border-t pt-4 sm:grid-cols-2">
							<div className="space-y-2">
								<Label htmlFor={kubernetesAuthModeId}>Auth</Label>
								<Select
									id={kubernetesAuthModeId}
									value={kubernetesAuthMode}
									onChange={(e) =>
										setKubernetesAuthMode(
											e.target.value as ExecutorKubernetesAuthMode,
										)
									}
									disabled={saving}
								>
									{saved.kubernetesAuthModeOptions.map((opt) => (
										<option
											key={opt.value}
											value={opt.value}
											disabled={!opt.enabled}
										>
											{optionLabel(opt, KUBERNETES_AUTH_MODE_LABELS)}
										</option>
									))}
								</Select>
								{showLocalConfig && (
									<p className="text-xs text-muted-foreground">
										Uses ~/.kube/config, or the in-cluster service account
										mount when no kubeconfig is available.
									</p>
								)}
							</div>
							{showLocalConfig && (
								<div className="space-y-2">
									<Label htmlFor={kubernetesContextId}>Context name</Label>
									<Input
										id={kubernetesContextId}
										value={kubernetesContext}
										onChange={(e) =>
											setKubernetesContext(e.target.value)
										}
										placeholder="Leave empty for current context"
										disabled={saving}
									/>
								</div>
							)}
							{showTokenAuth && (
								<>
									<div className="space-y-2">
										<Label htmlFor={kubernetesHostId}>
											Kubernetes host
										</Label>
										<Input
											id={kubernetesHostId}
											value={kubernetesHost}
											onChange={(e) =>
												setKubernetesHost(e.target.value)
											}
											placeholder="https://cluster.example.com:6443"
											disabled={saving}
										/>
									</div>
									<div className="space-y-2">
										<Label htmlFor={kubernetesTokenSecretId}>
											Kubernetes token
										</Label>
										<Select
											id={kubernetesTokenSecretId}
											value={kubernetesTokenSecretName}
											onChange={(e) =>
												setKubernetesTokenSecretName(e.target.value)
											}
											disabled={saving}
										>
											<option value="">Select a secret…</option>
											{secrets.map((secret) => (
												<option
													key={secret.id}
													value={secret.name}
												>
													{secret.name}
												</option>
											))}
										</Select>
									</div>
									<div className="space-y-2 sm:col-span-2">
										<Label htmlFor={kubernetesCACertId}>
											Kubernetes CA certificate
										</Label>
										<Textarea
											id={kubernetesCACertId}
											className="min-h-24 font-mono text-xs"
											value={kubernetesCACert}
											onChange={(e) =>
												setKubernetesCACert(e.target.value)
											}
											placeholder="Optional PEM-encoded CA certificate"
											disabled={saving}
										/>
									</div>
									<div className="flex items-center gap-2 sm:col-span-2">
										<Checkbox
											id={kubernetesInsecureSkipTLSVerifyId}
											checked={kubernetesInsecureSkipTLSVerify}
											onCheckedChange={(value) =>
												setKubernetesInsecureSkipTLSVerify(value === true)
											}
											disabled={saving}
										/>
										<Label
											htmlFor={kubernetesInsecureSkipTLSVerifyId}
											className="cursor-pointer"
										>
											Skip TLS verification
										</Label>
									</div>
								</>
							)}
						</div>
					)}

					{!loading && saved && (
						<div className="grid gap-4 border-t pt-4 sm:grid-cols-2">
							<div className="space-y-2">
								<Label htmlFor={agentId}>Agent</Label>
								<Select
									id={agentId}
									value={agent}
									onChange={(e) => {
										const next = e.target.value as ExecutorAgent
										setAgent(next)
										if (next !== 'claude-code') {
											return
										}
										if (
											!authType ||
											!saved.authTypeOptions.some(
												(opt) =>
													opt.value === authType &&
													opt.enabled,
											)
										) {
											setAuthType(
												firstEnabled(
													saved.authTypeOptions,
												) ?? 'api_key',
											)
										}
									}}
									disabled={saving}
								>
									{saved.agentOptions.map((opt) => (
										<option
											key={opt.value}
											value={opt.value}
											disabled={!opt.enabled}
										>
											{optionLabel(opt, AGENT_LABELS)}
										</option>
									))}
								</Select>
							</div>
							{showClaudeCodeSettings && (
								<div className="space-y-2">
									<Label htmlFor={authTypeId}>Auth type</Label>
									<Select
										id={authTypeId}
										value={authType}
										onChange={(e) =>
											setAuthType(
												e.target.value as ExecutorAuthType,
											)
										}
										disabled={saving}
									>
										{saved.authTypeOptions.map((opt) => (
											<option
												key={opt.value}
												value={opt.value}
												disabled={!opt.enabled}
											>
												{optionLabel(opt, AUTH_TYPE_LABELS)}
											</option>
										))}
									</Select>
								</div>
							)}
							{showApiKeySettings && (
								<>
									<div className="space-y-2">
										<Label htmlFor={tokenSecretId}>API key</Label>
										<Select
											id={tokenSecretId}
											value={tokenSecretName}
											onChange={(e) =>
												setTokenSecretName(e.target.value)
											}
											disabled={saving}
										>
											<option value="">Select a secret…</option>
											{secrets.map((secret) => (
												<option
													key={secret.id}
													value={secret.name}
												>
													{secret.name}
												</option>
											))}
										</Select>
									</div>
									<div className="space-y-2">
										<Label htmlFor={baseURLId}>Base URL</Label>
										<Input
											id={baseURLId}
											value={baseURL}
											onChange={(e) => setBaseURL(e.target.value)}
											placeholder="https://openrouter.ai/api"
											disabled={saving}
										/>
									</div>
								</>
							)}
							{showOAuthSettings && (
								<div className="space-y-2">
									<Label htmlFor={tokenSecretId}>OAuth token</Label>
									<Select
										id={tokenSecretId}
										value={tokenSecretName}
										onChange={(e) =>
											setTokenSecretName(e.target.value)
										}
										disabled={saving}
									>
										<option value="">Select a secret…</option>
										{secrets.map((secret) => (
											<option key={secret.id} value={secret.name}>
												{secret.name}
											</option>
										))}
									</Select>
								</div>
							)}
						</div>
					)}

					{!loading && saved && showDetails && (
						<div className="grid gap-4 border-t pt-4 sm:grid-cols-2">
							<div className="space-y-2">
								<Label htmlFor={imageId}>Image</Label>
								<Input
									id={imageId}
									value={image}
									onChange={(e) => setImage(e.target.value)}
									placeholder="agent-runner:local"
									disabled={saving}
								/>
							</div>
							<div className="space-y-2">
								<Label htmlFor={llmModelId}>LLM model</Label>
								<Input
									id={llmModelId}
									value={llmModel}
									onChange={(e) => setLlmModel(e.target.value)}
									placeholder="anthropic/claude-sonnet-4-5"
									disabled={saving}
								/>
							</div>
							<div className="space-y-2 sm:col-span-2">
								<Label htmlFor={webhookBaseURLId}>Webhook base URL</Label>
								<Input
									id={webhookBaseURLId}
									value={webhookBaseURL}
									onChange={(e) => setWebhookBaseURL(e.target.value)}
									placeholder="http://localhost:8080"
									disabled={saving}
								/>
								<p className="text-xs text-muted-foreground">
									Backend origin only (for example http://localhost:8080).
									The /api/v1/agent-runner/webhook path is appended
									automatically.
								</p>
							</div>
						</div>
					)}

					{!loading && saved && showKubernetes && (
						<div className="border-t pt-4">
							<div
								role="button"
								tabIndex={0}
								aria-expanded={advancedExpanded}
								className="flex min-w-0 cursor-pointer items-center gap-2 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
								onClick={() => setAdvancedExpanded((prev) => !prev)}
								onKeyDown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') {
										e.preventDefault()
										setAdvancedExpanded((prev) => !prev)
									}
								}}
							>
								{advancedExpanded ? (
									<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
								) : (
									<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
								)}
								<span className="text-sm font-medium">
									Advanced parameters
								</span>
							</div>
							{advancedExpanded && (
								<div className="mt-4 grid gap-4 sm:grid-cols-2">
									<div className="space-y-2">
										<Label htmlFor={namespaceId}>Namespace</Label>
										<Input
											id={namespaceId}
											value={namespace}
											onChange={(e) => setNamespace(e.target.value)}
											placeholder="default"
											disabled={saving}
										/>
									</div>
									<div className="space-y-2">
										<Label htmlFor={serviceAccountId}>
											ServiceAccount
										</Label>
										<Input
											id={serviceAccountId}
											value={serviceAccount}
											onChange={(e) =>
												setServiceAccount(e.target.value)
											}
											placeholder="default"
											disabled={saving}
										/>
									</div>
									<div className="space-y-2">
										<Label htmlFor={agentSecretNameId}>
											Agent Secret Name
										</Label>
										<Input
											id={agentSecretNameId}
											value={agentSecretName}
											onChange={(e) =>
												setAgentSecretName(e.target.value)
											}
											placeholder="Secret name in k8s"
											disabled={saving}
										/>
									</div>
									<div className="space-y-2">
										<Label htmlFor={agentLimitCPUId}>
											AGENT_LIMIT_CPU
										</Label>
										<Input
											id={agentLimitCPUId}
											value={agentLimitCPU}
											onChange={(e) =>
												setAgentLimitCPU(e.target.value)
											}
											placeholder="1"
											disabled={saving}
										/>
									</div>
									<div className="space-y-2">
										<Label htmlFor={agentLimitMemoryId}>
											AGENT_LIMIT_MEMORY
										</Label>
										<Input
											id={agentLimitMemoryId}
											value={agentLimitMemory}
											onChange={(e) =>
												setAgentLimitMemory(e.target.value)
											}
											placeholder="1Gi"
											disabled={saving}
										/>
									</div>
									<div className="space-y-2">
										<Label htmlFor={agentRequestCPUId}>
											AGENT_REQUEST_CPU
										</Label>
										<Input
											id={agentRequestCPUId}
											value={agentRequestCPU}
											onChange={(e) =>
												setAgentRequestCPU(e.target.value)
											}
											placeholder="1"
											disabled={saving}
										/>
									</div>
									<div className="space-y-2">
										<Label htmlFor={agentRequestMemoryId}>
											AGENT_REQUEST_MEMORY
										</Label>
										<Input
											id={agentRequestMemoryId}
											value={agentRequestMemory}
											onChange={(e) =>
												setAgentRequestMemory(e.target.value)
											}
											placeholder="256Mi"
											disabled={saving}
										/>
									</div>
									<div className="space-y-2 sm:col-span-2">
										<Label htmlFor={agentMCPConfigId}>
											AGENT_MCP_CONFIG
										</Label>
										<Input
											id={agentMCPConfigId}
											value={agentMCPConfig}
											onChange={(e) =>
												setAgentMCPConfig(e.target.value)
											}
											placeholder="ConfigMap name"
											disabled={saving}
										/>
									</div>
									<div className="space-y-2">
										<Label htmlFor={jobTTLSecondsId}>
											Job TTL (seconds)
										</Label>
										<Input
											id={jobTTLSecondsId}
											type="number"
											min={0}
											value={jobTTLSeconds}
											onChange={(e) =>
												setJobTTLSeconds(
													Number.parseInt(e.target.value, 10) || 0,
												)
											}
											placeholder="3600"
											disabled={saving}
										/>
									</div>
								</div>
							)}
						</div>
					)}

					<div className="flex justify-end">
						<Button
							size="sm"
							onClick={() => void handleSave()}
							disabled={loading || saving || !dirty}
						>
							{saving ? 'Saving…' : 'Save'}
						</Button>
					</div>
				</CardContent>
			)}
		</Card>
	)
}
