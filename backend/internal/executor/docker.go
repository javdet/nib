package executor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
)

var containerNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// runLocal creates and starts an agent-runner container via the local Docker socket.
func (s *Service) runLocal(ctx context.Context, cfg Config, req RunRequest) (RunResult, error) {
	if strings.TrimSpace(cfg.Image) == "" {
		return RunResult{}, ErrImageRequired
	}
	if err := s.validateRunSecrets(cfg, req); err != nil {
		return RunResult{}, err
	}

	workBranch := resolveWorkBranch(req.WorkBranch)
	containerName, err := buildContainerName(req.BaseBranch)
	if err != nil {
		return RunResult{}, err
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return RunResult{}, fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	gitToken := strings.TrimSpace(req.GitToken)
	llmKey := resolveSecretValue(req.LLMAPIKey, s.secrets.LLMAPIKey)
	gitProvider := NormalizeGitProvider(req.GitProvider)

	env := []string{
		"WORK_BRANCH=" + workBranch,
		"REPO_URL=" + strings.TrimSpace(req.RepoURL),
		"BASE_BRANCH=" + strings.TrimSpace(req.BaseBranch),
		"TASK_PROMPT=" + strings.TrimSpace(req.TaskPrompt),
		"GIT_TOKEN=" + gitToken,
		"LLM_MODEL=" + ResolveLLMModel(cfg, s.secrets),
		"LLM_API_KEY=" + llmKey,
		"GIT_PROVIDER=" + gitProvider,
	}
	if v := strings.TrimSpace(req.PRTitle); v != "" {
		env = append(env, "PR_TITLE="+v)
	}
	if v := strings.TrimSpace(req.LLMBaseURL); v != "" {
		env = append(env, "LLM_BASE_URL="+v)
	}
	if v := strings.TrimSpace(req.GitUsername); v != "" {
		env = append(env, "GIT_USERNAME="+v)
	}
	if v := strings.TrimSpace(req.GitEmail); v != "" {
		env = append(env, "GIT_EMAIL="+v)
	}

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image: cfg.Image,
			Env:   env,
		},
		&container.HostConfig{
			AutoRemove: true,
		},
		nil,
		nil,
		containerName,
	)
	if err != nil {
		return RunResult{}, fmt.Errorf("create container: %w", err)
	}

	if err := cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return RunResult{}, fmt.Errorf("start container: %w", err)
	}

	return RunResult{
		ContainerName: containerName,
		ContainerID:   resp.ID,
		WorkBranch:    workBranch,
		Status:        "started",
	}, nil
}

func resolveWorkBranch(workBranch string) string {
	if v := strings.TrimSpace(workBranch); v != "" {
		return v
	}
	return "agent/" + uuid.New().String()
}

func buildContainerName(baseBranch string) (string, error) {
	suffix, err := randomHex(5)
	if err != nil {
		return "", fmt.Errorf("generate container suffix: %w", err)
	}

	branch := strings.TrimSpace(baseBranch)
	branch = strings.ReplaceAll(branch, "/", "-")
	branch = containerNameSanitizer.ReplaceAllString(branch, "-")
	branch = strings.Trim(branch, "-_.")
	if branch == "" {
		branch = "branch"
	}

	name := fmt.Sprintf("nib-executor-%s-%s", branch, suffix)
	if len(name) > 128 {
		maxBranch := 128 - len("nib-executor-") - len(suffix) - 1
		if maxBranch < 1 {
			maxBranch = 1
		}
		if len(branch) > maxBranch {
			branch = branch[:maxBranch]
		}
		name = fmt.Sprintf("nib-executor-%s-%s", branch, suffix)
	}
	return name, nil
}

func randomHex(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
