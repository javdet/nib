package executor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const jsonIndent = "  "

const (
	defaultServiceAccount    = "default"
	defaultNamespace         = "default"
	defaultAgentLimitCPU     = "1"
	defaultAgentLimitMemory  = "1Gi"
	defaultAgentRequestCPU   = "1"
	defaultAgentRequestMemory = "256Mi"
	defaultJobTTLSeconds     = 3600
)

// ConfigStore manages the executor.json file.
type ConfigStore struct {
	mu                    sync.RWMutex
	filePath              string
	defaultWebhookBaseURL string

	initOnce sync.Once
	initErr  error
}

// NewConfigStore creates a ConfigStore. executorFile is resolved with ResolveFile
// against dataDir. defaultWebhookBaseURL fills webhookBaseURL when the stored
// value is blank (for example http://localhost:8080 from the server port).
func NewConfigStore(dataDir, executorFile, defaultWebhookBaseURL string) *ConfigStore {
	return &ConfigStore{
		filePath:              ResolveFile(dataDir, executorFile),
		defaultWebhookBaseURL: strings.TrimSuffix(strings.TrimSpace(defaultWebhookBaseURL), "/"),
	}
}

// Path returns the resolved executor.json file path.
func (s *ConfigStore) Path() string {
	return s.filePath
}

func (s *ConfigStore) ensureInitialized() error {
	s.initOnce.Do(func() {
		s.initErr = s.initialize()
	})
	return s.initErr
}

func (s *ConfigStore) initialize() error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create executor config directory: %w", err)
	}

	if _, err := os.Stat(s.filePath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat executor config file: %w", err)
		}
		return s.writeConfigLocked(Config{Type: TypeLocal, Image: ""})
	}
	return nil
}

// Get returns the current executor configuration.
func (s *ConfigStore) Get() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return Config{}, err
	}
	cfg, err := s.readConfigLocked()
	if err != nil {
		return Config{}, err
	}
	return applyConfigDefaults(cfg, s.defaultWebhookBaseURL), nil
}

func applyConfigDefaults(cfg Config, defaultWebhookBaseURL string) Config {
	cfg = NormalizeConfig(cfg)
	if cfg.WebhookBaseURL == "" && defaultWebhookBaseURL != "" {
		cfg.WebhookBaseURL = defaultWebhookBaseURL
	}
	return cfg
}

// Set replaces the executor configuration after validation.
func (s *ConfigStore) Set(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureInitialized(); err != nil {
		return err
	}
	cfg = NormalizeConfig(cfg)
	if err := ValidateConfig(cfg); err != nil {
		return err
	}
	return s.writeConfigLocked(cfg)
}

// ValidateConfig checks that cfg has a supported type and platform combination.
func ValidateConfig(cfg Config) error {
	switch cfg.Type {
	case TypeLocal:
	case TypeRemote:
		if !KnownPlatform(cfg.Platform) {
			return ErrInvalidPlatform
		}
		if !PlatformEnabled(cfg.Platform) {
			return fmt.Errorf("%w: %s", ErrPlatformUnavailable, cfg.Platform)
		}
		if cfg.Platform == PlatformKubernetes {
			if err := validateKubernetesConfig(cfg); err != nil {
				return err
			}
		}
	default:
		return ErrInvalidType
	}

	return validateAgentConfig(cfg)
}

func validateKubernetesConfig(cfg Config) error {
	if !KnownKubernetesAuthMode(cfg.KubernetesAuthMode) {
		return ErrInvalidKubernetesAuthMode
	}
	if !KubernetesAuthModeEnabled(cfg.KubernetesAuthMode) {
		return fmt.Errorf("%w: %s", ErrInvalidKubernetesAuthMode, cfg.KubernetesAuthMode)
	}
	switch cfg.KubernetesAuthMode {
	case KubernetesAuthModeToken:
		if strings.TrimSpace(cfg.KubernetesHost) == "" {
			return ErrKubernetesHostRequired
		}
		if strings.TrimSpace(cfg.KubernetesTokenSecretName) == "" {
			return ErrKubernetesTokenSecretRequired
		}
	case KubernetesAuthModeLocalConfig:
	default:
		return ErrInvalidKubernetesAuthMode
	}
	return nil
}

func validateAgentConfig(cfg Config) error {
	agent := cfg.Agent
	if agent == "" {
		agent = AgentClaudeCode
	}
	if !KnownAgent(agent) {
		return ErrInvalidAgent
	}
	if !AgentEnabled(agent) {
		return fmt.Errorf("%w: %s", ErrAgentUnavailable, agent)
	}
	authType := cfg.AuthType
	if authType == "" {
		authType = AuthTypeAPIKey
	}
	if !KnownAuthType(authType) {
		return ErrInvalidAuthType
	}
	if !AuthTypeEnabled(authType) {
		return fmt.Errorf("%w: %s", ErrInvalidAuthType, authType)
	}
	if agent == AgentCodex && authType != AuthTypeAPIKey {
		return fmt.Errorf("%w: codex supports api_key auth only", ErrInvalidAuthType)
	}
	return nil
}

// NormalizeConfig trims whitespace from string fields and clears the platform
// for executor types that do not use one.
func NormalizeConfig(cfg Config) Config {
	cfg.Type = Type(strings.ToLower(strings.TrimSpace(string(cfg.Type))))
	cfg.Platform = Platform(strings.ToLower(strings.TrimSpace(string(cfg.Platform))))
	if cfg.Type != TypeRemote {
		cfg.Platform = ""
	}
	cfg.KubernetesAuthMode = KubernetesAuthMode(
		strings.ToLower(strings.TrimSpace(string(cfg.KubernetesAuthMode))),
	)
	cfg.KubernetesContext = strings.TrimSpace(cfg.KubernetesContext)
	cfg.KubernetesHost = strings.TrimSpace(cfg.KubernetesHost)
	cfg.KubernetesTokenSecretName = strings.TrimSpace(cfg.KubernetesTokenSecretName)
	cfg.KubernetesCACert = strings.TrimSpace(cfg.KubernetesCACert)
	cfg.Namespace = strings.TrimSpace(cfg.Namespace)
	cfg.ServiceAccount = strings.TrimSpace(cfg.ServiceAccount)
	cfg.AgentSecretName = strings.TrimSpace(cfg.AgentSecretName)
	cfg.AgentLimitCPU = strings.TrimSpace(cfg.AgentLimitCPU)
	cfg.AgentLimitMemory = strings.TrimSpace(cfg.AgentLimitMemory)
	cfg.AgentRequestCPU = strings.TrimSpace(cfg.AgentRequestCPU)
	cfg.AgentRequestMemory = strings.TrimSpace(cfg.AgentRequestMemory)
	cfg.AgentMCPConfig = strings.TrimSpace(cfg.AgentMCPConfig)
	cfg.Image = strings.TrimSpace(cfg.Image)
	cfg.LLMModel = strings.TrimSpace(cfg.LLMModel)
	cfg.Agent = Agent(strings.ToLower(strings.TrimSpace(string(cfg.Agent))))
	cfg.AuthType = AuthType(strings.ToLower(strings.TrimSpace(string(cfg.AuthType))))
	cfg.TokenSecretName = strings.TrimSpace(cfg.TokenSecretName)
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.WebhookBaseURL = strings.TrimSuffix(strings.TrimSpace(cfg.WebhookBaseURL), "/")
	if cfg.Agent == "" {
		cfg.Agent = AgentClaudeCode
	}
	if cfg.Agent == AgentClaudeCode && cfg.AuthType == "" {
		cfg.AuthType = AuthTypeAPIKey
	}
	if cfg.AuthType != AuthTypeAPIKey {
		cfg.BaseURL = ""
	}

	if cfg.Type != TypeRemote || cfg.Platform != PlatformKubernetes {
		cfg.KubernetesAuthMode = ""
		cfg.KubernetesContext = ""
		cfg.KubernetesHost = ""
		cfg.KubernetesTokenSecretName = ""
		cfg.KubernetesCACert = ""
		cfg.KubernetesInsecureSkipTLSVerify = false
		cfg.Namespace = ""
		cfg.ServiceAccount = ""
		cfg.AgentSecretName = ""
		cfg.AgentLimitCPU = ""
		cfg.AgentLimitMemory = ""
		cfg.AgentRequestCPU = ""
		cfg.AgentRequestMemory = ""
		cfg.AgentMCPConfig = ""
		cfg.JobTTLSeconds = 0
		return cfg
	}

	if cfg.KubernetesAuthMode == "" {
		cfg.KubernetesAuthMode = KubernetesAuthModeLocalConfig
	}
	switch cfg.KubernetesAuthMode {
	case KubernetesAuthModeLocalConfig:
		cfg.KubernetesHost = ""
		cfg.KubernetesTokenSecretName = ""
	case KubernetesAuthModeToken:
		cfg.KubernetesContext = ""
	}

	if cfg.ServiceAccount == "" {
		cfg.ServiceAccount = defaultServiceAccount
	}
	if cfg.Namespace == "" {
		cfg.Namespace = defaultNamespace
	}
	if cfg.AgentLimitCPU == "" {
		cfg.AgentLimitCPU = defaultAgentLimitCPU
	}
	if cfg.AgentLimitMemory == "" {
		cfg.AgentLimitMemory = defaultAgentLimitMemory
	}
	if cfg.AgentRequestCPU == "" {
		cfg.AgentRequestCPU = defaultAgentRequestCPU
	}
	if cfg.AgentRequestMemory == "" {
		cfg.AgentRequestMemory = defaultAgentRequestMemory
	}
	if cfg.JobTTLSeconds <= 0 {
		cfg.JobTTLSeconds = defaultJobTTLSeconds
	}

	return cfg
}

// ResolveLLMModel returns the configured model, preferring executor.json over env.
func ResolveLLMModel(cfg Config, secrets Secrets) string {
	if v := strings.TrimSpace(cfg.LLMModel); v != "" {
		return v
	}
	return strings.TrimSpace(secrets.LLMModel)
}

func (s *ConfigStore) readConfigLocked() (Config, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return Config{}, fmt.Errorf("read executor config: %w", err)
	}
	return parseConfig(string(data))
}

// rawConfig unmarshals executor.json, accepting the legacy "system" field.
type rawConfig struct {
	Type                      Type               `json:"type"`
	Platform                  Platform           `json:"platform"`
	System                    Platform           `json:"system"`
	KubernetesAuthMode        KubernetesAuthMode `json:"kubernetesAuthMode"`
	KubernetesContext         string             `json:"kubernetesContext"`
	KubernetesHost            string             `json:"kubernetesHost"`
	KubernetesTokenSecretName string             `json:"kubernetesTokenSecretName"`
	KubernetesCACert          string             `json:"kubernetesCACert"`
	KubernetesInsecureSkipTLSVerify bool         `json:"kubernetesInsecureSkipTLSVerify"`
	Namespace                 string             `json:"namespace"`
	ServiceAccount            string             `json:"serviceAccount"`
	AgentSecretName           string             `json:"agentSecretName"`
	AgentLimitCPU             string             `json:"agentLimitCPU"`
	AgentLimitMemory          string             `json:"agentLimitMemory"`
	AgentRequestCPU           string             `json:"agentRequestCPU"`
	AgentRequestMemory        string             `json:"agentRequestMemory"`
	AgentMCPConfig            string             `json:"agentMCPConfig"`
	JobTTLSeconds             int                `json:"jobTTLSeconds"`
	Agent                     Agent              `json:"agent"`
	AuthType                  AuthType           `json:"authType"`
	TokenSecretName           string             `json:"tokenSecretName"`
	BaseURL                   string             `json:"baseURL"`
	Image                     string             `json:"image"`
	LLMModel                  string             `json:"llmModel"`
	WebhookBaseURL            string             `json:"webhookBaseURL"`
}

func parseConfig(content string) (Config, error) {
	var raw rawConfig
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	cfg := Config{
		Type:                      raw.Type,
		Platform:                  raw.Platform,
		KubernetesAuthMode:        raw.KubernetesAuthMode,
		KubernetesContext:         raw.KubernetesContext,
		KubernetesHost:            raw.KubernetesHost,
		KubernetesTokenSecretName: raw.KubernetesTokenSecretName,
		KubernetesCACert:          raw.KubernetesCACert,
		KubernetesInsecureSkipTLSVerify: raw.KubernetesInsecureSkipTLSVerify,
		Namespace:                 raw.Namespace,
		ServiceAccount:            raw.ServiceAccount,
		AgentSecretName:           raw.AgentSecretName,
		AgentLimitCPU:             raw.AgentLimitCPU,
		AgentLimitMemory:          raw.AgentLimitMemory,
		AgentRequestCPU:           raw.AgentRequestCPU,
		AgentRequestMemory:        raw.AgentRequestMemory,
		AgentMCPConfig:            raw.AgentMCPConfig,
		JobTTLSeconds:             raw.JobTTLSeconds,
		Agent:                     raw.Agent,
		AuthType:                  raw.AuthType,
		TokenSecretName:           raw.TokenSecretName,
		BaseURL:                   raw.BaseURL,
		Image:                     raw.Image,
		LLMModel:                  raw.LLMModel,
		WebhookBaseURL:            raw.WebhookBaseURL,
	}
	if cfg.Platform == "" && raw.System != "" {
		cfg.Platform = raw.System
	}
	return migrateConfig(cfg), nil
}

// migrateConfig upgrades configs written before the type/platform split, where
// "kubernetes" was a top-level executor type.
func migrateConfig(cfg Config) Config {
	if cfg.Type == "" {
		cfg.Type = TypeLocal
	}
	if cfg.Type == "kubernetes" {
		cfg.Type = TypeRemote
		if cfg.Platform == "" {
			cfg.Platform = PlatformKubernetes
		}
	}
	if cfg.Type == TypeRemote && cfg.Platform == "" {
		cfg.Platform = PlatformKubernetes
	}
	if cfg.Agent == "" {
		cfg.Agent = AgentClaudeCode
	}
	if cfg.AuthType == "" {
		cfg.AuthType = AuthTypeAPIKey
	}
	return cfg
}

func formatConfig(cfg Config) (string, error) {
	data, err := json.MarshalIndent(cfg, "", jsonIndent)
	if err != nil {
		return "", fmt.Errorf("marshal executor config: %w", err)
	}
	return string(data) + "\n", nil
}

func (s *ConfigStore) writeConfigLocked(cfg Config) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create executor config directory: %w", err)
	}

	formatted, err := formatConfig(cfg)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".executor-*.json")
	if err != nil {
		return fmt.Errorf("create temp executor config file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.WriteString(formatted); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp executor config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp executor config file: %w", err)
	}
	if err := os.Rename(tmpName, s.filePath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp executor config file: %w", err)
	}
	return nil
}
