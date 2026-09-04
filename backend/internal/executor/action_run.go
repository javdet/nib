package executor

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/javdet/nib/internal/metrics"
)

// Fixed agent-runner settings for action runs. They are not exposed in the
// executor settings UI: an action run always gets the same tool surface and
// deadline regardless of which action triggered it.
const (
	actionRunAllowedTools   = "Read,Write,Edit,Grep,Glob,Bash"
	actionRunPermissionMode = "acceptEdits"
	actionRunTimeoutSeconds = "1500"
	// actionRunStopGraceSeconds is how long a force-stopped container gets to
	// exit before docker kills it. Short on purpose: the operator pressed stop
	// because the run is in the way.
	actionRunStopGraceSeconds = 5
)

// ActionRunRequest is the input for launching an agent-runner container for a
// single "code" action of an action plan. Everything the container needs that
// is not part of the executor configuration is resolved by the caller.
type ActionRunRequest struct {
	Prompt       string
	ChatID       string
	TaskID       string
	RepoURL      string
	TargetBranch string
	// PRTitle is optional: the agent falls back to the first line of the prompt.
	PRTitle     string
	GitProvider string
	// GitToken is passed to the container as GITHUB_TOKEN.
	GitToken string
	// LLMToken is passed as CLAUDE_CODE_OAUTH_TOKEN or ANTHROPIC_API_KEY,
	// depending on the configured auth type.
	LLMToken string
	GitUsername string
	GitEmail    string
}

// ActionRunResult describes the container started for an action run.
type ActionRunResult struct {
	JobName       string `json:"jobName"`
	Namespace     string `json:"namespace"`
	ContainerName string `json:"containerName"`
	ContainerID   string `json:"containerId"`
	TargetBranch  string `json:"targetBranch"`
	RepoURL       string `json:"repoUrl"`
	Status        string `json:"status"`
}

// RunAction launches an agent-runner task for one action of an action plan.
//
// Unlike Run, which is driven by the run_executor LLM tool, this path is
// triggered directly by the operator pressing "Execute action" and speaks the
// environment contract of the claude-code agent entrypoint (PROMPT,
// TARGET_BRANCH, GITHUB_TOKEN, ...). BASE_BRANCH is deliberately omitted so the
// agent clones the repository default branch and opens the pull request against it.
// RunAction launches an agent container for a code action an operator ran.
func (s *Service) RunAction(ctx context.Context, req ActionRunRequest) (ActionRunResult, error) {
	start := time.Now()
	res, err := s.runAction(ctx, req)
	s.recordRun(metrics.ExecutorEntrypointAction, err, time.Since(start))
	return res, err
}

func (s *Service) runAction(ctx context.Context, req ActionRunRequest) (ActionRunResult, error) {
	cfg, err := s.config.Get()
	if err != nil {
		return ActionRunResult{}, err
	}
	if cfg.Type == TypeDisabled {
		return ActionRunResult{}, ErrExecutorDisabled
	}
	if err := validateActionRunRequest(req, cfg); err != nil {
		return ActionRunResult{}, err
	}

	agent := cfg.Agent
	if agent == "" {
		agent = AgentClaudeCode
	}
	if !AgentEnabled(agent) {
		return ActionRunResult{}, fmt.Errorf("%w: %s", ErrAgentUnavailable, agent)
	}

	switch cfg.Type {
	case TypeDisabled:
		return ActionRunResult{}, ErrExecutorDisabled
	case TypeLocal:
		return s.runActionLocal(ctx, cfg, req)
	case TypeRemote:
		switch cfg.Platform {
		case PlatformKubernetes:
			return s.runActionKubernetes(ctx, cfg, req)
		default:
			return ActionRunResult{}, fmt.Errorf("%w: remote %s", ErrNotImplemented, cfg.Platform)
		}
	default:
		return ActionRunResult{}, ErrInvalidType
	}
}

func (s *Service) runActionLocal(ctx context.Context, cfg Config, req ActionRunRequest) (ActionRunResult, error) {
	if strings.TrimSpace(cfg.Image) == "" {
		return ActionRunResult{}, ErrImageRequired
	}

	jobName, err := buildJobName()
	if err != nil {
		return ActionRunResult{}, err
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return ActionRunResult{}, fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image: cfg.Image,
			Env:   buildActionRunEnv(cfg, s.secrets, req, jobName),
		},
		// AutoRemove stays off: nothing is reported back to the chat yet, so the
		// container logs are the only record of what the agent did.
		&container.HostConfig{NetworkMode: "host"},
		nil,
		nil,
		jobName,
	)
	if err != nil {
		return ActionRunResult{}, fmt.Errorf("create container: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return ActionRunResult{}, fmt.Errorf("start container: %w", err)
	}

	return ActionRunResult{
		JobName:       jobName,
		ContainerName: jobName,
		ContainerID:   resp.ID,
		TargetBranch:  strings.TrimSpace(req.TargetBranch),
		RepoURL:       strings.TrimSpace(req.RepoURL),
		Status:        "started",
	}, nil
}

func validateActionRunRequest(req ActionRunRequest, cfg Config) error {
	switch {
	case strings.TrimSpace(req.Prompt) == "":
		return ErrPromptRequired
	case strings.TrimSpace(req.RepoURL) == "":
		return ErrRepoURLRequired
	case strings.TrimSpace(req.TargetBranch) == "":
		return ErrTargetBranchRequired
	}

	if cfg.Type == TypeRemote && cfg.Platform == PlatformKubernetes {
		return nil
	}

	switch {
	case strings.TrimSpace(req.GitToken) == "":
		return ErrGitTokenRequired
	case strings.TrimSpace(req.LLMToken) == "":
		return ErrLLMTokenRequired
	}
	return nil
}

func buildActionRunEnv(cfg Config, secrets Secrets, req ActionRunRequest, jobName string) []string {
	values := buildActionRunValues(cfg, req, jobName)

	env := []string{
		"AGENT_TYPE=" + values.AgentType,
		"PROMPT=" + values.Prompt,
		"CHAT_ID=" + values.ChatID,
		"TASK_ID=" + values.TaskID,
		"GIT_PROVIDER=" + values.GitProvider,
		"REPO_URL=" + values.RepoURL,
		"TARGET_BRANCH=" + values.TargetBranch,
		"ALLOWED_TOOLS=" + values.AllowedTools,
		"PERMISSION_MODE=" + actionRunPermissionMode,
		"TIMEOUT_SECONDS=" + actionRunTimeoutSeconds,
		"JOB_NAME=" + values.JobName,
		"GITHUB_TOKEN=" + strings.TrimSpace(req.GitToken),
	}

	if values.PRTitle != "" {
		env = append(env, "PR_TITLE="+values.PRTitle)
	}
	if values.GitAuthorName != "" {
		env = append(env, "GIT_AUTHOR_NAME="+values.GitAuthorName)
	}
	if values.GitAuthorEmail != "" {
		env = append(env, "GIT_AUTHOR_EMAIL="+values.GitAuthorEmail)
	}

	llmToken := strings.TrimSpace(req.LLMToken)
	switch cfg.Agent {
	case AgentCodex:
		env = append(env, "OPENAI_API_KEY="+llmToken)
		if values.OpenAIBaseURL != "" {
			env = append(env, "OPENAI_BASE_URL="+values.OpenAIBaseURL)
		}
		if values.OpenAIModel != "" {
			env = append(env, "OPENAI_MODEL="+values.OpenAIModel)
		}
	default:
		if cfg.AuthType == AuthTypeOAuthToken {
			env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+llmToken)
		} else {
			env = append(env, "ANTHROPIC_API_KEY="+llmToken)
			if values.AnthropicBaseURL != "" {
				env = append(env, "ANTHROPIC_BASE_URL="+values.AnthropicBaseURL)
			}
		}
		if values.AnthropicModel != "" {
			env = append(env, "ANTHROPIC_MODEL="+values.AnthropicModel)
		}
	}

	if values.WebhookURL != "" {
		env = append(env, "WEBHOOK_URL="+values.WebhookURL)
	}
	if v := strings.TrimSpace(secrets.WebhookToken); v != "" {
		env = append(env, "WEBHOOK_AUTH_HEADER=Authorization: Bearer "+v)
	}

	return env
}

const agentWebhookPath = "/api/v1/agent-runner/webhook"

// resolveAgentWebhookURL builds the full callback URL from webhookBaseURL.
// The setting is the backend origin (scheme + host + port, no path), but a
// stored value that already includes the webhook path is accepted so mis-saved
// configs do not produce a doubled path.
func resolveAgentWebhookURL(baseURL string) string {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	if strings.HasSuffix(baseURL, agentWebhookPath) {
		return baseURL
	}
	return baseURL + agentWebhookPath
}

// buildJobName returns a "nib-<8 digits>" name, used both as the JOB_NAME the
// agent reports and as the Docker container name.
func buildJobName() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(90000000))
	if err != nil {
		return "", fmt.Errorf("generate job name: %w", err)
	}
	return fmt.Sprintf("nib-%08d", n.Int64()+10000000), nil
}

// StopActionRequest identifies a running action container. Either identifier is
// enough: a local run names its container after the job, and a remote run is a
// Job of that name.
type StopActionRequest struct {
	JobName     string
	ContainerID string
	// Namespace overrides the configured one, for a run started before the
	// setting changed.
	Namespace string
}

// StopAction kills the agent-runner started for an action run.
//
// A container that is already gone is not an error: the point of the call is
// that nothing is left running, and a force stop is reached exactly when the
// state of the run is in doubt.
// StopAction stops a running agent container.
func (s *Service) StopAction(ctx context.Context, req StopActionRequest) error {
	err := s.stopAction(ctx, req)
	metrics.RecordExecutorStop(err)
	return err
}

func (s *Service) stopAction(ctx context.Context, req StopActionRequest) error {
	if strings.TrimSpace(req.JobName) == "" && strings.TrimSpace(req.ContainerID) == "" {
		return ErrStopTargetRequired
	}

	cfg, err := s.config.Get()
	if err != nil {
		return err
	}

	switch cfg.Type {
	case TypeDisabled:
		return ErrExecutorDisabled
	case TypeLocal:
		return stopActionLocal(ctx, req)
	case TypeRemote:
		switch cfg.Platform {
		case PlatformKubernetes:
			return s.stopActionKubernetes(ctx, cfg, req)
		default:
			return fmt.Errorf("%w: remote %s", ErrNotImplemented, cfg.Platform)
		}
	default:
		return ErrInvalidType
	}
}

func stopActionLocal(ctx context.Context, req StopActionRequest) error {
	target := strings.TrimSpace(req.ContainerID)
	if target == "" {
		target = strings.TrimSpace(req.JobName)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	timeout := actionRunStopGraceSeconds
	if err := cli.ContainerStop(ctx, target, container.StopOptions{Timeout: &timeout}); err != nil {
		if client.IsErrNotFound(err) {
			return nil
		}
		return fmt.Errorf("stop container %q: %w", target, err)
	}

	// Action runs deliberately leave AutoRemove off, so the container logs
	// survive the run. A force stop has to remove it explicitly or every stop
	// leaves a dead container behind.
	if err := cli.ContainerRemove(ctx, target, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
		slog.Warn("remove stopped action container", "container", target, "error", err)
	}
	return nil
}
