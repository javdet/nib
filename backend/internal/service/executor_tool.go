package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/llm"
)

// ExecutorKubernetesTokenSecretName is the prompt_secrets name holding the Kubernetes API token.
const ExecutorKubernetesTokenSecretName = "EXECUTOR_KUBERNETES_TOKEN"

const (
	RunExecutorToolName             = "run_executor"
	executorLLMAPIKeySecretName     = "EXECUTOR_LLM_API_KEY"
	executorVarGitBaseURL           = "GitBaseURL"
	executorVarVersionControlSystem = "VersionControlSystem"
	executorVarGitUsername          = "GitUsername"
	executorVarGitEmail             = "GitEmail"
)

var runExecutorParameters = json.RawMessage(`{
  "type": "object",
  "required": ["task_prompt", "base_branch", "pr_title"],
  "properties": {
    "task_prompt": {
      "type": "string",
      "description": "Instructions for the autonomous coding agent describing what changes to make."
    },
    "base_branch": {
      "type": "string",
      "description": "Base branch to branch from (for example main or develop)."
    },
    "work_branch": {
      "type": "string",
      "description": "Target branch where the agent commits changes. Auto-generated when omitted."
    },
    "pr_title": {
      "type": "string",
      "description": "Pull request title describing the changes made by the agent."
    }
  }
}`)

// RunExecutorToolDef returns the LLM tool definition for launching an agent-runner container.
func RunExecutorToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: RunExecutorToolName,
		Description: "Launch an autonomous coding agent in a separate container. " +
			"The agent clones the configured repository, applies changes on a work branch, and opens a pull request. " +
			"Repository URL, git provider, credentials, and LLM settings are resolved from system configuration.",
		Parameters: runExecutorParameters,
	}
}

// ExecuteRunExecutor resolves configuration and starts an agent-runner task.
// Validation and resolution failures are returned as tool output so the agent loop can continue.
func (s *ChatService) ExecuteRunExecutor(ctx context.Context, args map[string]any) (string, error) {
	req, err := s.resolveRunExecutorRequest(ctx, args)
	if err != nil {
		return err.Error(), nil
	}
	if s.executorSvc == nil {
		return "executor service is not configured", nil
	}

	result, err := s.executorSvc.Run(ctx, req)
	if err != nil {
		return fmt.Sprintf("executor run failed: %v", err), nil
	}

	out, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal executor result: %w", err)
	}
	return string(out), nil
}

func (s *ChatService) resolveRunExecutorRequest(ctx context.Context, args map[string]any) (executor.RunRequest, error) {
	taskPrompt := strings.TrimSpace(stringArg(args, "task_prompt"))
	baseBranch := strings.TrimSpace(stringArg(args, "base_branch"))
	workBranch := strings.TrimSpace(stringArg(args, "work_branch"))
	prTitle := strings.TrimSpace(stringArg(args, "pr_title"))

	if taskPrompt == "" {
		return executor.RunRequest{}, fmt.Errorf("task_prompt is required")
	}
	if baseBranch == "" {
		return executor.RunRequest{}, fmt.Errorf("base_branch is required")
	}
	if prTitle == "" {
		return executor.RunRequest{}, fmt.Errorf("pr_title is required")
	}

	gitSettings, err := s.resolveExecutorGitSettings(ctx)
	if err != nil {
		return executor.RunRequest{}, err
	}

	gitToken, llmAPIKey, err := s.resolveExecutorSecrets(ctx)
	if err != nil {
		return executor.RunRequest{}, err
	}

	return executor.RunRequest{
		RepoURL:     gitSettings.RepoURL,
		BaseBranch:  baseBranch,
		WorkBranch:  workBranch,
		TaskPrompt:  taskPrompt,
		PRTitle:     prTitle,
		GitProvider: gitSettings.Provider,
		GitToken:    gitToken,
		LLMAPIKey:   llmAPIKey,
		LLMBaseURL:  strings.TrimSpace(s.llmBaseURL),
		GitUsername: gitSettings.Username,
		GitEmail:    gitSettings.Email,
	}, nil
}

type executorGitSettings struct {
	RepoURL  string
	Provider string
	Username string
	Email    string
}

func (s *ChatService) resolveExecutorGitSettings(ctx context.Context) (executorGitSettings, error) {
	if s.variableRepo == nil {
		return executorGitSettings{}, fmt.Errorf("variable repository is not configured")
	}
	vars, err := s.variableRepo.LoadAll(ctx, nil)
	if err != nil {
		return executorGitSettings{}, fmt.Errorf("load variables: %w", err)
	}
	globalVars := vars[defaultVariableScope]
	repoURL := strings.TrimSpace(asString(globalVars[executorVarGitBaseURL]))
	if repoURL == "" {
		return executorGitSettings{}, fmt.Errorf("GitBaseURL variable is not configured; set it in company settings")
	}
	return executorGitSettings{
		RepoURL:  repoURL,
		Provider: executor.NormalizeGitProvider(asString(globalVars[executorVarVersionControlSystem])),
		Username: strings.TrimSpace(asString(globalVars[executorVarGitUsername])),
		Email:    strings.TrimSpace(asString(globalVars[executorVarGitEmail])),
	}, nil
}

func (s *ChatService) resolveExecutorSecrets(ctx context.Context) (gitToken, llmAPIKey string, err error) {
	if s.secretSvc == nil {
		return "", "", fmt.Errorf("secret service is not configured")
	}
	if s.executorSvc == nil {
		return "", "", fmt.Errorf("executor service is not configured")
	}
	cfg, err := s.executorSvc.ConfigStore().Get()
	if err != nil {
		return "", "", fmt.Errorf("read executor config: %w", err)
	}

	// Both executor entry points read the git token through readNamedSecret so a
	// missing or deleted secret reads the same whichever one the operator hit.
	gitSecret := strings.TrimSpace(cfg.GitTokenSecretName)
	if gitSecret == "" {
		return "", "", ErrExecutorGitTokenSecretRequired
	}
	gitToken, err = s.readNamedSecret(ctx, gitSecret)
	if err != nil {
		return "", "", err
	}
	llmAPIKey, err = s.readNamedSecret(ctx, executorLLMAPIKeySecretName)
	if err != nil {
		return "", "", err
	}
	return gitToken, llmAPIKey, nil
}
