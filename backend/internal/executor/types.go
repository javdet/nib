package executor

// Type identifies how agent-runner containers are launched.
type Type string

const (
	// TypeDisabled turns the executor off: no agent container is ever launched.
	TypeDisabled Type = "disabled"
	TypeLocal    Type = "local"
	TypeRemote   Type = "remote"
)

// Platform identifies the remote platform used when Type is TypeRemote.
type Platform string

const (
	PlatformDocker      Platform = "docker"
	PlatformKubernetes  Platform = "kubernetes"
	PlatformKubeFoundry Platform = "kubefoundry"
)

// KubernetesAuthMode identifies how the executor authenticates to the Kubernetes API.
type KubernetesAuthMode string

const (
	KubernetesAuthModeLocalConfig KubernetesAuthMode = "local_config"
	KubernetesAuthModeToken     KubernetesAuthMode = "token"
)

// Agent identifies which agent runtime image/entrypoint to use.
type Agent string

const (
	AgentClaudeCode Agent = "claude-code"
	AgentCodex      Agent = "codex"
)

// AuthType identifies how the agent authenticates with the LLM provider.
type AuthType string

const (
	AuthTypeAPIKey     AuthType = "api_key"
	AuthTypeOAuthToken AuthType = "oauth_token"
)

// TypeOption describes an executor type exposed in the UI.
type TypeOption struct {
	Value   Type `json:"value"`
	Enabled bool `json:"enabled"`
}

// PlatformOption describes a remote platform exposed in the UI.
type PlatformOption struct {
	Value   Platform `json:"value"`
	Enabled bool     `json:"enabled"`
}

// KubernetesAuthModeOption describes a Kubernetes auth mode exposed in the UI.
type KubernetesAuthModeOption struct {
	Value   KubernetesAuthMode `json:"value"`
	Enabled bool               `json:"enabled"`
}

// AgentOption describes an agent runtime exposed in the UI.
type AgentOption struct {
	Value   Agent `json:"value"`
	Enabled bool  `json:"enabled"`
}

// AuthTypeOption describes an auth method exposed in the UI.
type AuthTypeOption struct {
	Value   AuthType `json:"value"`
	Enabled bool     `json:"enabled"`
}

// TypeOptions lists executor types exposed in the UI, with their availability.
var TypeOptions = []TypeOption{
	{Value: TypeDisabled, Enabled: true},
	{Value: TypeLocal, Enabled: true},
	{Value: TypeRemote, Enabled: true},
}

// PlatformOptions lists remote platforms exposed in the UI, with their availability.
var PlatformOptions = []PlatformOption{
	{Value: PlatformDocker, Enabled: false},
	{Value: PlatformKubernetes, Enabled: true},
	{Value: PlatformKubeFoundry, Enabled: false},
}

// KubernetesAuthModeOptions lists Kubernetes auth modes exposed in the UI.
var KubernetesAuthModeOptions = []KubernetesAuthModeOption{
	{Value: KubernetesAuthModeLocalConfig, Enabled: true},
	{Value: KubernetesAuthModeToken, Enabled: true},
}

// AgentOptions lists agent runtimes exposed in the UI, with their availability.
var AgentOptions = []AgentOption{
	{Value: AgentClaudeCode, Enabled: true},
	{Value: AgentCodex, Enabled: true},
}

// AuthTypeOptions lists auth methods exposed in the UI, with their availability.
var AuthTypeOptions = []AuthTypeOption{
	{Value: AuthTypeAPIKey, Enabled: true},
	{Value: AuthTypeOAuthToken, Enabled: true},
}

// TypeEnabled reports whether t is a known and currently selectable executor type.
func TypeEnabled(t Type) bool {
	for _, opt := range TypeOptions {
		if opt.Value == t {
			return opt.Enabled
		}
	}
	return false
}

// KnownPlatform reports whether p is one of the remote platforms exposed in the UI.
func KnownPlatform(p Platform) bool {
	for _, opt := range PlatformOptions {
		if opt.Value == p {
			return true
		}
	}
	return false
}

// PlatformEnabled reports whether p is a known and currently selectable remote platform.
func PlatformEnabled(p Platform) bool {
	for _, opt := range PlatformOptions {
		if opt.Value == p {
			return opt.Enabled
		}
	}
	return false
}

// KnownKubernetesAuthMode reports whether m is a known Kubernetes auth mode.
func KnownKubernetesAuthMode(m KubernetesAuthMode) bool {
	for _, opt := range KubernetesAuthModeOptions {
		if opt.Value == m {
			return true
		}
	}
	return false
}

// KubernetesAuthModeEnabled reports whether m is a known and selectable auth mode.
func KubernetesAuthModeEnabled(m KubernetesAuthMode) bool {
	for _, opt := range KubernetesAuthModeOptions {
		if opt.Value == m {
			return opt.Enabled
		}
	}
	return false
}

// KnownAgent reports whether a is one of the agent runtimes exposed in the UI.
func KnownAgent(a Agent) bool {
	for _, opt := range AgentOptions {
		if opt.Value == a {
			return true
		}
	}
	return false
}

// AgentEnabled reports whether a is a known and currently selectable agent runtime.
func AgentEnabled(a Agent) bool {
	for _, opt := range AgentOptions {
		if opt.Value == a {
			return opt.Enabled
		}
	}
	return false
}

// KnownAuthType reports whether a is one of the auth methods exposed in the UI.
func KnownAuthType(a AuthType) bool {
	for _, opt := range AuthTypeOptions {
		if opt.Value == a {
			return true
		}
	}
	return false
}

// AuthTypeEnabled reports whether a is a known and currently selectable auth method.
func AuthTypeEnabled(a AuthType) bool {
	for _, opt := range AuthTypeOptions {
		if opt.Value == a {
			return opt.Enabled
		}
	}
	return false
}

// Config holds non-secret executor settings persisted in executor.json.
type Config struct {
	Type     Type     `json:"type"`
	Platform Platform `json:"platform"`
	// KubernetesAuthMode selects how the executor authenticates to the Kubernetes API.
	KubernetesAuthMode KubernetesAuthMode `json:"kubernetesAuthMode"`
	// KubernetesContext is the kubeconfig context name; empty uses the current context.
	KubernetesContext string `json:"kubernetesContext"`
	// KubernetesHost is the API server URL when KubernetesAuthMode is token.
	KubernetesHost string `json:"kubernetesHost"`
	// KubernetesTokenSecretName references a prompt_secrets entry for the API token.
	KubernetesTokenSecretName string `json:"kubernetesTokenSecretName"`
	// KubernetesCACert is the PEM-encoded CA certificate for token auth.
	KubernetesCACert string `json:"kubernetesCACert"`
	// KubernetesInsecureSkipTLSVerify disables TLS verification for token auth.
	KubernetesInsecureSkipTLSVerify bool `json:"kubernetesInsecureSkipTLSVerify"`
	// Namespace is the Kubernetes namespace for agent jobs.
	Namespace string `json:"namespace"`
	// ServiceAccount is the Kubernetes service account for agent jobs.
	ServiceAccount string `json:"serviceAccount"`
	// AgentSecretName is the Kubernetes secret mounted into agent jobs.
	AgentSecretName string `json:"agentSecretName"`
	// AgentLimitCPU is the CPU limit for agent job containers.
	AgentLimitCPU string `json:"agentLimitCPU"`
	// AgentLimitMemory is the memory limit for agent job containers.
	AgentLimitMemory string `json:"agentLimitMemory"`
	// AgentRequestCPU is the CPU request for agent job containers.
	AgentRequestCPU string `json:"agentRequestCPU"`
	// AgentRequestMemory is the memory request for agent job containers.
	AgentRequestMemory string `json:"agentRequestMemory"`
	// AgentMCPConfig is the ConfigMap name for MCP configuration.
	AgentMCPConfig string `json:"agentMCPConfig"`
	// JobTTLSeconds is how long finished jobs remain before garbage collection.
	JobTTLSeconds int `json:"jobTTLSeconds"`
	Agent            Agent    `json:"agent"`
	AuthType         AuthType `json:"authType"`
	TokenSecretName  string   `json:"tokenSecretName"`
	// GitTokenSecretName references a prompt_secrets entry holding the git API
	// token the agent container clones, pushes and opens the pull/merge request
	// with. Optional only on remote Kubernetes, where the agent Job takes its
	// credentials from AgentSecretName instead.
	GitTokenSecretName string `json:"gitTokenSecretName"`
	BaseURL          string   `json:"baseURL"`
	Image            string   `json:"image"`
	LLMModel         string   `json:"llmModel"`
	// WebhookBaseURL is the backend origin the agent container calls back on
	// (scheme + host + port, no path). WEBHOOK_URL is derived from it at launch.
	WebhookBaseURL string `json:"webhookBaseURL"`
}

// Secrets holds executor credentials loaded from environment variables. The git
// API token is deliberately absent: it is selected by name in executor settings
// (Config.GitTokenSecretName) and read from the encrypted secret store, so an
// env fallback would be a second, invisible source of truth for one credential.
type Secrets struct {
	LLMAPIKey    string
	LLMModel     string
	WebhookToken string
}

// RunRequest is the input for launching an agent-runner task.
type RunRequest struct {
	RepoURL     string
	BaseBranch  string
	WorkBranch  string
	TaskPrompt  string
	PRTitle     string
	GitProvider string
	GitToken    string
	LLMAPIKey   string
	LLMBaseURL  string
	GitUsername string
	GitEmail    string
}

// RunResult is returned after a container has been created and started.
type RunResult struct {
	ContainerName string `json:"containerName"`
	ContainerID   string `json:"containerId"`
	WorkBranch    string `json:"workBranch"`
	Status        string `json:"status"`
}
