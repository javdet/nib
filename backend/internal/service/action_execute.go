package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/metrics"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/textutil"
)

// actionTypeCode is the action plan step type executed by the coding agent.
const actionTypeCode = "code"

const (
	executeDialogMode        = "execute"
	executeDialogTitleMaxLen = 80
	actionRunBranchPrefix    = "nib/"
)

var (
	// ErrActionNotFound is returned when an action plan has no step for the requested row key.
	ErrActionNotFound = errors.New("action not found in the action plan")
	// ErrActionNotCode is returned when an action is executed as code but has another type.
	ErrActionNotCode = errors.New(`action type is not "code"`)
	// ErrActionRepositoryRequired is returned when a code action carries no repository name.
	ErrActionRepositoryRequired = errors.New(`action has no "repository"; set it in the action plan`)
	// ErrExecutorTokenSecretRequired is returned when no LLM token secret is selected in executor settings.
	ErrExecutorTokenSecretRequired = errors.New("executor LLM token secret is not configured; select it in executor settings")
	// ErrExecutorSecretMissing is returned when a secret referenced by executor settings does not exist.
	ErrExecutorSecretMissing = errors.New("executor secret is not configured")
	// ErrExecutorGitTokenSecretRequired is returned when no git API token secret is selected in executor settings.
	ErrExecutorGitTokenSecretRequired = errors.New("executor git API token secret is not configured; select it in executor settings")
)

// ExecuteCodeAction starts an agent-runner container for a single "code" action
// of the plan stored on planDialogID, identified by its row key (for example
// "s0.step1" or "rollback.2").
//
// It creates a dedicated execute dialog and persists the task as its first user
// message, but never calls the LLM: the task goes straight to the container.
// The chat has to exist before the container starts because its id is passed as
// CHAT_ID, so a launch failure leaves the chat behind holding the task message;
// the error is returned to the caller and the operator can retry from the plan.
func (s *ChatService) ExecuteCodeAction(ctx context.Context, planDialogID uuid.UUID, key string) (domain.Dialog, executor.ActionRunResult, error) {
	if s.dialogRepo == nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("execute code action: dialog repository is not configured")
	}
	if s.executorSvc == nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("execute code action: executor service is not configured")
	}
	if s.secretSvc == nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("execute code action: secret service is not configured")
	}

	// Bail out before the execute dialog is created: a disabled executor never
	// launches a container, so leaving a chat behind would be pure noise.
	execCfg, err := s.executorSvc.ConfigStore().Get()
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("execute code action: read executor config: %w", err)
	}
	if execCfg.Type == executor.TypeDisabled {
		return domain.Dialog{}, executor.ActionRunResult{}, executor.ErrExecutorDisabled
	}

	step, err := s.readActionPlanStep(planDialogID, key)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(step.Type), actionTypeCode) {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("%w: %q", ErrActionNotCode, step.Type)
	}
	if strings.TrimSpace(step.Repository) == "" {
		return domain.Dialog{}, executor.ActionRunResult{}, ErrActionRepositoryRequired
	}

	comments, err := s.ReadActionPlanComments(planDialogID)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("execute code action: read comments: %w", err)
	}
	prompt := buildActionRunPrompt(step.Action, comments[key])

	gitSettings, err := s.resolveExecutorGitSettings(ctx)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, err
	}
	repoURL := buildActionRepoURL(gitSettings.RepoURL, step.Repository)

	gitToken, llmToken, err := s.resolveActionRunSecrets(ctx)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, err
	}

	taskID, err := s.resolveActionTaskID(ctx, planDialogID)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, err
	}

	// Every precondition is settled by now, so the lease is taken here: a code
	// action refused for a missing repository or an unconfigured secret must not
	// block the one execution slot on its way out.
	lease, ok := s.acquireExecutionLease(ExecutionLease{
		PlanID:    planDialogID,
		Key:       key,
		Number:    actionPlanNumberForKey(key),
		Kind:      ExecutionKindContainer,
		StartedAt: time.Now().Unix(),
	})
	if !ok {
		return domain.Dialog{}, executor.ActionRunResult{}, newExecutionBusyError(lease)
	}
	// Anything that goes wrong from here has to hand the lease back: the
	// container's webhook is what normally releases it, and a launch that never
	// happened has no webhook coming.
	launched := false
	defer func() {
		if !launched {
			s.releaseExecutionLease(lease.token)
		}
	}()

	dialog, err := s.dialogRepo.CreateDialog(ctx, executeDialogMode, buildExecuteDialogTitle(step), &planDialogID)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("execute code action: create dialog: %w", err)
	}
	s.stampExecutionLease(lease.token, func(l *ExecutionLease) {
		l.DialogID = dialog.ID
	})

	if err := s.recordActionPlanRun(planDialogID, key, dialog.ID); err != nil {
		return dialog, executor.ActionRunResult{}, err
	}

	if err := s.seedExecuteDialog(ctx, dialog.ID, prompt); err != nil {
		return dialog, executor.ActionRunResult{}, err
	}

	targetBranch := buildActionTargetBranch(taskID, dialog.ID)

	slog.Info("execute code action",
		"plan_dialog_id", planDialogID,
		"dialog_id", dialog.ID,
		"key", key,
		"repo_url", repoURL,
		"target_branch", targetBranch,
	)

	result, err := s.executorSvc.RunAction(ctx, executor.ActionRunRequest{
		Prompt:       prompt,
		ChatID:       dialog.ID.String(),
		TaskID:       taskID,
		RepoURL:      repoURL,
		TargetBranch: targetBranch,
		PRTitle:      strings.TrimSpace(step.PRTitle),
		GitProvider:  gitSettings.Provider,
		GitToken:     gitToken,
		LLMToken:     llmToken,
		GitUsername:  gitSettings.Username,
		GitEmail:     gitSettings.Email,
	})
	if err != nil {
		return dialog, executor.ActionRunResult{}, fmt.Errorf("execute code action: %w", err)
	}

	// The container is the only thing that can be stopped now, so the lease has
	// to carry the way to reach it: nothing else records the job name.
	launched = true
	metrics.RecordActionExecStarted(string(ExecutionKindContainer))
	s.stampExecutionLease(lease.token, func(l *ExecutionLease) {
		l.JobName = result.JobName
		l.ContainerID = result.ContainerID
		l.Namespace = result.Namespace
	})

	return dialog, result, nil
}

// recordActionPlanRun maps an action row key to the execute dialog that was
// launched for it, so a later webhook can attach the PR URL to the right step.
func (s *ChatService) recordActionPlanRun(planDialogID uuid.UUID, key string, execDialogID uuid.UUID) error {
	// Action subagents register here from goroutines of their own, so the
	// read-modify-write needs the same lock every other plan-file update takes.
	mu := s.planMutex(planDialogID)
	mu.Lock()
	defer mu.Unlock()

	runs, err := s.ReadActionPlanRuns(planDialogID)
	if err != nil {
		return fmt.Errorf("execute code action: read runs: %w", err)
	}
	runs[key] = execDialogID.String()
	if err := s.WriteActionPlanRuns(planDialogID, runs); err != nil {
		return fmt.Errorf("execute code action: write runs: %w", err)
	}
	return nil
}

// seedExecuteDialog writes the transcript rows a first turn would have written,
// without running the agent loop. The system row is persisted so a later manual
// message in the same chat still starts from a well-formed transcript.
func (s *ChatService) seedExecuteDialog(ctx context.Context, dialogID uuid.UUID, prompt string) error {
	sysPrompt, err := s.resolveSystemPrompt(ctx, executeDialogMode)
	if err != nil {
		return fmt.Errorf("execute code action: resolve system prompt: %w", err)
	}
	if _, err := s.dialogRepo.AppendMessage(ctx, dialogID, domain.DialogMessage{
		Role:    "system",
		Content: sysPrompt,
	}); err != nil {
		return fmt.Errorf("execute code action: append system: %w", err)
	}
	if _, err := s.dialogRepo.AppendMessage(ctx, dialogID, domain.DialogMessage{
		Role:    "user",
		Content: prompt,
	}); err != nil {
		return fmt.Errorf("execute code action: append user: %w", err)
	}
	return nil
}

func (s *ChatService) readActionPlanStep(planDialogID uuid.UUID, key string) (storedActionStep, error) {
	raw, found, err := s.ReadActionPlan(planDialogID)
	if err != nil {
		return storedActionStep{}, fmt.Errorf("execute code action: read action plan: %w", err)
	}
	if !found {
		return storedActionStep{}, fmt.Errorf("execute code action: %w", repository.ErrNotFound)
	}

	var plan storedActionPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return storedActionStep{}, fmt.Errorf("execute code action: parse action plan: %w", err)
	}

	step, ok := findActionPlanStep(plan, strings.TrimSpace(key))
	if !ok {
		return storedActionStep{}, fmt.Errorf("%w: %q", ErrActionNotFound, key)
	}
	return step, nil
}

// findActionPlanStep resolves a step by the same row keys the web UI renders.
func findActionPlanStep(plan storedActionPlan, key string) (storedActionStep, bool) {
	if key == "" {
		return storedActionStep{}, false
	}
	for stageIdx, stage := range plan.Stages {
		for stepIdx, step := range stage.Steps {
			if actionPlanItemKey(stageIdx, ActionPlanScopeSteps, stepIdx) == key {
				return step, true
			}
		}
	}
	for idx, step := range plan.Rollback {
		if fmt.Sprintf("rollback.%d", idx) == key {
			return step, true
		}
	}
	return storedActionStep{}, false
}

// buildActionRunPrompt returns the text sent to the agent, which is also stored
// verbatim as the execute chat's first user message.
func buildActionRunPrompt(action, comment string) string {
	prompt := strings.TrimSpace(action)
	if c := strings.TrimSpace(comment); c != "" {
		prompt += "\n\n## Comment\n\n" + c
	}
	return prompt
}

// buildExecuteDialogTitle names the execute chat after the action's pull request
// title, falling back to the first line of the action itself: pr_title is
// optional in practice, and the agent derives its own title the same way.
func buildExecuteDialogTitle(step storedActionStep) string {
	title := strings.TrimSpace(step.PRTitle)
	if title == "" {
		title, _, _ = strings.Cut(strings.TrimSpace(step.Action), "\n")
		title = strings.TrimSpace(title)
	}
	return textutil.TruncateRunes(title, executeDialogTitleMaxLen)
}

// buildActionTargetBranch names the branch the agent commits to. Plans without a
// task id fall back to the chat id, which keeps the branch traceable to the run.
func buildActionTargetBranch(taskID string, dialogID uuid.UUID) string {
	if taskID = strings.TrimSpace(taskID); taskID != "" {
		return actionRunBranchPrefix + taskID
	}
	return actionRunBranchPrefix + dialogID.String()
}

// buildActionRepoURL joins the company-wide git base URL with the repository
// named on the action step. The plan panel accepts a repository as a bare name
// ("infra"), as an owner path ("my-org/infra") or as the full clone URL people
// copy out of the browser, so anything that already carries the base is used as
// it stands: appending it would hand the agent
// "https://github.com/my-org/https://github.com/my-org/infra" as REPO_URL.
func buildActionRepoURL(baseURL, repository string) string {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	repo := strings.TrimSpace(repository)
	switch {
	case repo == "":
		return base
	case isAbsoluteRepoURL(repo):
		return strings.TrimSuffix(repo, "/")
	case base == "":
		return strings.Trim(repo, "/")
	}

	// A pasted URL can also arrive without its scheme ("github.com/my-org/infra"),
	// which still repeats the base host and owner.
	if rest, ok := trimPrefixFold(repo, stripURLScheme(base)+"/"); ok && strings.Trim(rest, "/") != "" {
		return base + "/" + strings.Trim(rest, "/")
	}
	return base + "/" + strings.Trim(repo, "/")
}

// isAbsoluteRepoURL reports whether repository already spells out a clone
// target: a scheme-prefixed URL or an scp-style "git@host:org/repo" address.
func isAbsoluteRepoURL(repository string) bool {
	if strings.HasPrefix(strings.ToLower(repository), "git@") {
		return true
	}
	scheme, rest, ok := strings.Cut(repository, "://")
	return ok && scheme != "" && rest != "" && !strings.ContainsAny(scheme, "/@ ")
}

// stripURLScheme drops the "scheme://" prefix, leaving host and path.
func stripURLScheme(raw string) string {
	if _, rest, ok := strings.Cut(raw, "://"); ok {
		return rest
	}
	return raw
}

// trimPrefixFold is strings.TrimPrefix with a case-insensitive match, since git
// hosts treat the host and the owner segment case-insensitively.
func trimPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

// resolveActionTaskID returns the task id of the plan dialog, falling back to
// its parent: chat_name persists the task id on the root decompose dialog.
func (s *ChatService) resolveActionTaskID(ctx context.Context, planDialogID uuid.UUID) (string, error) {
	d, err := s.dialogRepo.GetDialog(ctx, planDialogID)
	if err != nil {
		return "", fmt.Errorf("execute code action: get dialog: %w", err)
	}
	if d.TaskID != nil && strings.TrimSpace(*d.TaskID) != "" {
		return strings.TrimSpace(*d.TaskID), nil
	}
	if d.ParentID == nil {
		return "", nil
	}

	parent, err := s.dialogRepo.GetDialog(ctx, *d.ParentID)
	if err != nil {
		return "", fmt.Errorf("execute code action: get parent dialog: %w", err)
	}
	if parent.TaskID != nil {
		return strings.TrimSpace(*parent.TaskID), nil
	}
	return "", nil
}

// resolveActionRunSecrets loads the git API token and the LLM token named in the
// executor configuration.
func (s *ChatService) resolveActionRunSecrets(ctx context.Context) (gitToken, llmToken string, err error) {
	cfg, err := s.executorSvc.ConfigStore().Get()
	if err != nil {
		return "", "", fmt.Errorf("execute code action: read executor config: %w", err)
	}

	// A remote Kubernetes job takes its credentials from the operator-managed
	// Secret named in AgentSecretName (envFrom in the job template), so nib
	// neither needs nor forwards them.
	kubernetesRemote := cfg.Type == executor.TypeRemote && cfg.Platform == executor.PlatformKubernetes

	if gitSecret := strings.TrimSpace(cfg.GitTokenSecretName); gitSecret == "" {
		if !kubernetesRemote {
			return "", "", ErrExecutorGitTokenSecretRequired
		}
	} else if gitToken, err = s.readNamedSecret(ctx, gitSecret); err != nil {
		if !kubernetesRemote {
			return "", "", err
		}
		gitToken = ""
	}

	if kubernetesRemote {
		return gitToken, "", nil
	}

	if strings.TrimSpace(cfg.TokenSecretName) == "" {
		return "", "", ErrExecutorTokenSecretRequired
	}

	llmToken, err = s.readNamedSecret(ctx, cfg.TokenSecretName)
	if err != nil {
		return "", "", err
	}
	return gitToken, llmToken, nil
}

func (s *ChatService) readNamedSecret(ctx context.Context, name string) (string, error) {
	value, err := s.secretSvc.GetValueByName(ctx, defaultVariableScope, "", name)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", fmt.Errorf("%w: %q", ErrExecutorSecretMissing, name)
		}
		return "", fmt.Errorf("resolve secret %q: %w", name, err)
	}
	return value, nil
}
