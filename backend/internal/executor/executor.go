package executor

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Service orchestrates agent-runner container launches.
type Service struct {
	config         *ConfigStore
	secrets        Secrets
	secretMu       sync.RWMutex
	secretsLookup  SecretLookup
}

// NewService creates an executor Service.
func NewService(config *ConfigStore, secrets Secrets) *Service {
	return &Service{
		config:  config,
		secrets: secrets,
	}
}

// ConfigStore returns the underlying config store.
func (s *Service) ConfigStore() *ConfigStore {
	return s.config
}

// Run launches an agent-runner task using the configured executor type.
func (s *Service) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if err := validateRunRequest(req); err != nil {
		return RunResult{}, err
	}

	cfg, err := s.config.Get()
	if err != nil {
		return RunResult{}, err
	}

	switch cfg.Type {
	case TypeDisabled:
		return RunResult{}, ErrExecutorDisabled
	case TypeLocal:
		return s.runLocal(ctx, cfg, req)
	case TypeRemote:
		return RunResult{}, fmt.Errorf("%w: remote %s", ErrNotImplemented, cfg.Platform)
	default:
		return RunResult{}, ErrInvalidType
	}
}

func validateRunRequest(req RunRequest) error {
	if strings.TrimSpace(req.RepoURL) == "" {
		return ErrRepoURLRequired
	}
	if strings.TrimSpace(req.BaseBranch) == "" {
		return ErrBranchRequired
	}
	if strings.TrimSpace(req.TaskPrompt) == "" {
		return ErrTaskPromptRequired
	}
	return nil
}

func resolveSecretValue(requestValue, fallback string) string {
	if v := strings.TrimSpace(requestValue); v != "" {
		return v
	}
	return strings.TrimSpace(fallback)
}

func (s *Service) validateRunSecrets(cfg Config, req RunRequest) error {
	gitToken := resolveSecretValue(req.GitToken, s.secrets.GitToken)
	llmKey := resolveSecretValue(req.LLMAPIKey, s.secrets.LLMAPIKey)
	if strings.TrimSpace(llmKey) == "" ||
		strings.TrimSpace(ResolveLLMModel(cfg, s.secrets)) == "" ||
		strings.TrimSpace(gitToken) == "" {
		return ErrSecretsIncomplete
	}
	return nil
}
