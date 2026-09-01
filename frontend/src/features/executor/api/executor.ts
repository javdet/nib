import { api } from '@/lib/api-client'

export type ExecutorType = 'disabled' | 'local' | 'remote'
export type ExecutorPlatform = 'docker' | 'kubernetes' | 'kubefoundry'
export type ExecutorKubernetesAuthMode = 'local_config' | 'token'
export type ExecutorAgent = 'claude-code' | 'codex'
export type ExecutorAuthType = 'api_key' | 'oauth_token'

export interface ExecutorOption<T> {
	value: T
	enabled: boolean
}

export interface ExecutorConfig {
	type: ExecutorType
	platform: ExecutorPlatform | ''
	kubernetesAuthMode: ExecutorKubernetesAuthMode | ''
	kubernetesContext: string
	kubernetesHost: string
	kubernetesTokenSecretName: string
	kubernetesCACert: string
	kubernetesInsecureSkipTLSVerify: boolean
	namespace: string
	serviceAccount: string
	agentSecretName: string
	agentLimitCPU: string
	agentLimitMemory: string
	agentRequestCPU: string
	agentRequestMemory: string
	agentMCPConfig: string
	jobTTLSeconds: number
	agent: ExecutorAgent
	authType: ExecutorAuthType
	tokenSecretName: string
	baseURL: string
	webhookBaseURL: string
	image: string
	llmModel: string
	typeOptions: ExecutorOption<ExecutorType>[]
	platformOptions: ExecutorOption<ExecutorPlatform>[]
	kubernetesAuthModeOptions: ExecutorOption<ExecutorKubernetesAuthMode>[]
	agentOptions: ExecutorOption<ExecutorAgent>[]
	authTypeOptions: ExecutorOption<ExecutorAuthType>[]
}

export interface ExecutorConfigUpdate {
	type: ExecutorType
	platform: ExecutorPlatform | ''
	kubernetesAuthMode: ExecutorKubernetesAuthMode | ''
	kubernetesContext: string
	kubernetesHost: string
	kubernetesTokenSecretName: string
	kubernetesCACert: string
	kubernetesInsecureSkipTLSVerify: boolean
	namespace: string
	serviceAccount: string
	agentSecretName: string
	agentLimitCPU: string
	agentLimitMemory: string
	agentRequestCPU: string
	agentRequestMemory: string
	agentMCPConfig: string
	jobTTLSeconds: number
	agent: ExecutorAgent
	authType: ExecutorAuthType
	tokenSecretName: string
	baseURL: string
	webhookBaseURL: string
	image: string
	llmModel: string
}

export function getExecutorConfig(): Promise<ExecutorConfig> {
	return api.get<ExecutorConfig>('/executor/config')
}

export function updateExecutorConfig(
	cfg: ExecutorConfigUpdate,
): Promise<ExecutorConfig> {
	return api.put<ExecutorConfig>('/executor/config', cfg)
}
