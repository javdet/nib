package executor

import "errors"

var (
	ErrInvalidType                    = errors.New("invalid executor type")
	ErrInvalidPlatform                = errors.New("invalid executor platform")
	ErrPlatformUnavailable            = errors.New("executor platform is not available yet")
	ErrInvalidKubernetesAuthMode      = errors.New("invalid kubernetes auth mode")
	ErrKubernetesHostRequired         = errors.New("kubernetes host is required")
	ErrKubernetesTokenSecretRequired  = errors.New("kubernetes token secret is required")
	ErrInvalidJSON                    = errors.New("invalid executor json")
	ErrImageRequired                  = errors.New("executor image is required")
	ErrRepoURLRequired                = errors.New("repository URL is required")
	ErrBranchRequired                 = errors.New("base branch is required")
	ErrTaskPromptRequired             = errors.New("task prompt is required")
	ErrSecretsIncomplete              = errors.New("executor secrets are not fully configured (EXECUTOR_LLM_API_KEY, EXECUTOR_GIT_API_TOKEN, and LLM model in settings or EXECUTOR_LLM_MODEL)")
	ErrInvalidAgent                   = errors.New("invalid executor agent")
	ErrAgentUnavailable               = errors.New("executor agent is not available yet")
	ErrInvalidAuthType                = errors.New("invalid executor auth type")
	ErrNotImplemented                 = errors.New("executor type not implemented")
	ErrKubernetesConfigLoad           = errors.New("failed to load kubernetes config")
	ErrKubernetesJobRender            = errors.New("failed to render kubernetes job")
	ErrKubernetesJobCreate            = errors.New("failed to create kubernetes job")
	ErrAgentSecretRequired            = errors.New("agent secret name is required for kubernetes jobs")

	ErrPromptRequired       = errors.New("prompt is required")
	ErrTargetBranchRequired = errors.New("target branch is required")
	ErrGitTokenRequired     = errors.New("git API token is required")
	ErrLLMTokenRequired     = errors.New("LLM token is required")
)
