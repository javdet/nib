package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/metrics"
	"github.com/javdet/nib/internal/textutil"
)

// codeFixBranchPrefix names the branch a fix pushes to when the operator did not
// point it at one. It is deliberately not the plan's own nib/{task-id}: a fix
// with no branch named is a change of its own, and landing it on the branch a
// code action is still working towards would rewrite that action's pull request
// by accident.
const codeFixBranchPrefix = actionRunBranchPrefix + "fix-"

// codeFixReportMaxBytes caps what a failed fix spells out in the chat; the whole
// of it is in the fix's own dialog, one link away.
const codeFixReportMaxBytes = 4000

// codeFixMessageNamePrefix names the one report a fix posts, so a retried
// webhook delivery cannot double-post it.
const codeFixMessageNamePrefix = "code-fix:"

// A fix is only ever started from the run_subagent tool, which already reports a
// missing argument as a sentence of its own. These are the second line of
// defence, and are deliberately absent from handleServiceError: nothing HTTP
// reaches StartCodeFix.
var (
	// ErrCodeFixTaskRequired is returned when a fix carries no instructions.
	ErrCodeFixTaskRequired = errors.New("a code fix needs a task describing the change to make")
	// ErrCodeFixRepositoryRequired is returned when a fix names no repository.
	ErrCodeFixRepositoryRequired = errors.New("a code fix needs the repository to change")
)

// CodeFixRequest is an ad-hoc code change asked for in the chat.
type CodeFixRequest struct {
	// Task is what the coding agent is told to do, in the operator's terms.
	Task string
	// Repository is the repository to change, as the action plan spells one: a
	// bare name, an owner path or a full clone URL.
	Repository string
	// Branch is the branch to commit to. Naming one that already exists
	// continues it -- the agent-runner checks the remote out and adds to the
	// pull request already open on it -- which is how a fix lands on top of the
	// change a code action just made. Empty starts a branch of its own.
	Branch string
	// PRTitle is optional; the agent falls back to the first line of the task.
	PRTitle string
}

// StartCodeFix hands an ad-hoc code change to the coding agent in a container
// and reports back into the chat that asked for it.
//
// It is the chat's way in to the same agent-runner a code action uses, for the
// case a plan cannot express: the operator reads what an action produced, sees
// the code is wrong, and says so. Nothing about the run is recorded in the plan
// files -- there is no row to record it on -- so it lives in
// data/code_fixes/{root}.json instead, which is also what the webhook recognises
// it by.
//
// Like ExecuteCodeAction it creates the dialog before the container starts,
// because the id is passed as CHAT_ID; a launch that fails leaves that dialog
// behind holding the task.
func (s *ChatService) StartCodeFix(
	ctx context.Context,
	rootID uuid.UUID,
	req CodeFixRequest,
) (domain.Dialog, executor.ActionRunResult, error) {
	// The arguments are checked first: they are the only failure the
	// orchestrator can put right by asking the operator, and checking them here
	// keeps that answer the same whatever else is unconfigured.
	task := strings.TrimSpace(req.Task)
	if task == "" {
		return domain.Dialog{}, executor.ActionRunResult{}, ErrCodeFixTaskRequired
	}
	if strings.TrimSpace(req.Repository) == "" {
		return domain.Dialog{}, executor.ActionRunResult{}, ErrCodeFixRepositoryRequired
	}

	if s.dialogRepo == nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("start code fix: dialog repository is not configured")
	}
	if s.executorSvc == nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("start code fix: executor service is not configured")
	}
	if s.secretSvc == nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("start code fix: secret service is not configured")
	}

	// Checked before the dialog is created: a disabled executor never launches a
	// container, so leaving a chat behind would be pure noise.
	execCfg, err := s.executorSvc.ConfigStore().Get()
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("start code fix: read executor config: %w", err)
	}
	if execCfg.Type == executor.TypeDisabled {
		return domain.Dialog{}, executor.ActionRunResult{}, executor.ErrExecutorDisabled
	}

	gitSettings, err := s.resolveExecutorGitSettings(ctx)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, err
	}
	repoURL := buildActionRepoURL(gitSettings.RepoURL, req.Repository)

	gitToken, llmToken, err := s.resolveActionRunSecrets(ctx)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, err
	}

	taskID, err := s.resolveActionTaskID(ctx, rootID)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, err
	}

	// Every precondition is settled by now, so the lease is taken here: a fix
	// refused for an unconfigured secret must not block the one execution slot
	// on its way out.
	lease, ok := s.acquireExecutionLease(ExecutionLease{
		PlanID:    rootID,
		Kind:      ExecutionKindContainer,
		Fix:       true,
		StartedAt: time.Now().Unix(),
	})
	if !ok {
		return domain.Dialog{}, executor.ActionRunResult{}, newExecutionBusyError(lease)
	}
	// Anything that goes wrong from here hands the lease back: the container's
	// webhook is what normally releases it, and a launch that never happened has
	// no webhook coming.
	launched := false
	defer func() {
		if !launched {
			s.releaseExecutionLease(lease.token)
		}
	}()

	dialog, err := s.dialogRepo.CreateDialog(ctx, executeDialogMode, buildCodeFixTitle(req, task), &rootID)
	if err != nil {
		return domain.Dialog{}, executor.ActionRunResult{}, fmt.Errorf("start code fix: create dialog: %w", err)
	}
	s.stampExecutionLease(lease.token, func(l *ExecutionLease) {
		l.DialogID = dialog.ID
	})

	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = codeFixBranchPrefix + dialog.ID.String()
	}

	if err := s.recordCodeFixRun(rootID, dialog.ID, CodeFixRun{
		Status:     ActionExecRunning,
		Repository: strings.TrimSpace(req.Repository),
		Branch:     branch,
		StartedAt:  time.Now().Unix(),
	}); err != nil {
		return dialog, executor.ActionRunResult{}, err
	}

	if err := s.seedExecuteDialog(ctx, dialog.ID, task); err != nil {
		return dialog, executor.ActionRunResult{}, err
	}

	slog.Info("start code fix",
		"root_dialog_id", rootID,
		"dialog_id", dialog.ID,
		"repo_url", repoURL,
		"target_branch", branch,
	)

	result, err := s.executorSvc.RunAction(ctx, executor.ActionRunRequest{
		Prompt:       task,
		ChatID:       dialog.ID.String(),
		TaskID:       taskID,
		RepoURL:      repoURL,
		TargetBranch: branch,
		PRTitle:      strings.TrimSpace(req.PRTitle),
		GitProvider:  gitSettings.Provider,
		GitToken:     gitToken,
		LLMToken:     llmToken,
		GitUsername:  gitSettings.Username,
		GitEmail:     gitSettings.Email,
	})
	if err != nil {
		return dialog, executor.ActionRunResult{}, fmt.Errorf("start code fix: %w", err)
	}

	launched = true
	metrics.RecordActionExecStarted(string(ExecutionKindContainer))

	// The container is the only thing that can be stopped now, so both the lease
	// and the record have to carry the way to reach it.
	s.stampExecutionLease(lease.token, func(l *ExecutionLease) {
		l.JobName = result.JobName
		l.ContainerID = result.ContainerID
		l.Namespace = result.Namespace
	})
	if _, _, err := s.updateCodeFixRun(rootID, dialog.ID, func(r *CodeFixRun) {
		r.JobName = result.JobName
		r.ContainerID = result.ContainerID
		r.Namespace = result.Namespace
	}); err != nil {
		// The container is already building; a record missing its job name costs
		// the ability to force-stop it after a restart, not the run.
		slog.Warn("record code fix container", "root_dialog_id", rootID, "dialog_id", dialog.ID, "error", err)
	}

	return dialog, result, nil
}

// buildCodeFixTitle names the fix's dialog after its pull request title, falling
// back to the first line of the task -- the same way an action's execute chat is
// named.
func buildCodeFixTitle(req CodeFixRequest, task string) string {
	title := strings.TrimSpace(req.PRTitle)
	if title == "" {
		title, _, _ = strings.Cut(task, "\n")
		title = strings.TrimSpace(title)
	}
	return textutil.TruncateRunes(title, executeDialogTitleMaxLen)
}

// codeFixOutcome is how a fix ended, as the three paths that can end one see it:
// the webhook, a force stop, and the lease expiring on a container that never
// reported back.
type codeFixOutcome struct {
	Status ActionExecStatus
	// Error is recorded on the run; Body is what the chat report spells out
	// under the heading.
	Error string
	Body  string
	// Branch and PRURL are what the container reported. Empty from a stop or an
	// expiry, where the record keeps what the launch put there.
	Branch string
	PRURL  string
}

// closeCodeFixRun records how a fix ended, frees the execution slot and posts
// the outcome into the chat that asked for it.
//
// Every path into it is idempotent, because the webhook is retried with curl and
// a force stop can race a delivery: the status guard leaves a run something else
// already closed alone, and the report is a named message.
func (s *ChatService) closeCodeFixRun(
	ctx context.Context,
	rootID, dialogID uuid.UUID,
	out codeFixOutcome,
) {
	run, found, err := s.updateCodeFixRun(rootID, dialogID, func(r *CodeFixRun) {
		if !r.Active() {
			return
		}
		r.Status = out.Status
		r.FinishedAt = time.Now().Unix()
		r.Error = out.Error
		if b := strings.TrimSpace(out.Branch); b != "" {
			r.Branch = b
		}
		if pr := strings.TrimSpace(out.PRURL); pr != "" {
			r.PRURL = pr
		}
	})
	if err != nil {
		slog.Warn("close code fix run", "root_dialog_id", rootID, "dialog_id", dialogID, "error", err)
		return
	}
	if !found {
		return
	}

	s.releaseExecutionLeaseForFix(rootID, dialogID, run.JobName)
	// run.Status rather than out.Status: a force stop and a late webhook can both
	// reach this, and the one that closed the run is the one the record kept.
	s.reportCodeFixResult(ctx, rootID, dialogID, run, run.Status, out.Body)
}

// finishCodeFixRun closes the fix an agent-runner webhook belongs to, and does
// nothing when the container behind it was not a fix.
func (s *ChatService) finishCodeFixRun(ctx context.Context, rootID uuid.UUID, dialogID uuid.UUID, res AgentRunResult) {
	status, errMsg := ActionExecDone, ""
	if !strings.EqualFold(strings.TrimSpace(res.Status), "success") {
		status = ActionExecFailed
		errMsg = fmt.Sprintf("the coding agent exited with code %d", res.ExitCode)
	}

	s.closeCodeFixRun(ctx, rootID, dialogID, codeFixOutcome{
		Status: status,
		Error:  errMsg,
		Body:   codeFixReportText(res, errMsg),
		Branch: res.TargetBranch,
		PRURL:  res.PRURL,
	})
}

// codeFixReportText is what the chat report says under the heading. A failure
// keeps the agent's last words, because the exit code alone says nothing about
// what went wrong; a success says nothing, since the branch and the pull request
// are already in the heading block.
func codeFixReportText(res AgentRunResult, errMsg string) string {
	if strings.EqualFold(strings.TrimSpace(res.Status), "success") {
		return ""
	}

	parts := make([]string, 0, 2)
	if errMsg != "" {
		parts = append(parts, errMsg)
	}
	if result := strings.TrimSpace(res.Result); result != "" {
		parts = append(parts, textutil.TruncateBytes(result, "\n\n_(truncated)_", codeFixReportMaxBytes))
	}
	return strings.Join(parts, "\n\n")
}

// reportCodeFixResult posts the outcome of a fix into the chat the operator
// asked for it in, the same way an action sub-agent's result is posted into the
// plan chat: the fix's own dialog holds the agent's full account, and nothing
// else links to it.
func (s *ChatService) reportCodeFixResult(
	ctx context.Context,
	rootID, dialogID uuid.UUID,
	run CodeFixRun,
	status ActionExecStatus,
	body string,
) {
	// The orchestrator's chat already refreshes on these, so a fix landing in it
	// needs no event of its own. They carry no action row on purpose: there is
	// none.
	kind := domain.ActivityActionExecDone
	if status != ActionExecDone {
		kind = domain.ActivityActionExecFailed
	}
	defer s.activity.Publish(rootID, domain.AgentActivity{Kind: kind, Status: string(status)})

	if s.dialogRepo == nil {
		return
	}

	name := codeFixMessageNamePrefix + dialogID.String()
	existing, err := s.dialogRepo.ListMessages(ctx, rootID)
	if err != nil {
		slog.Warn("code fix result: list messages", "root_dialog_id", rootID, "dialog_id", dialogID, "error", err)
		return
	}
	for _, m := range existing {
		if m.Name == name {
			return
		}
	}

	if _, err := s.appendMessageLocked(ctx, rootID, domain.DialogMessage{
		Role:    "assistant",
		Content: formatCodeFixResultMessage(dialogID, run, status, body),
		Name:    name,
	}); err != nil {
		slog.Warn("code fix result: append", "root_dialog_id", rootID, "dialog_id", dialogID, "error", err)
	}
}

// formatCodeFixResultMessage renders the outcome. The link is a fragment the
// chat panel intercepts to open the fix's own dialog: there is no route to a
// dialog by id, so a plain URL would go nowhere.
func formatCodeFixResultMessage(dialogID uuid.UUID, run CodeFixRun, status ActionExecStatus, body string) string {
	var b strings.Builder
	switch status {
	case ActionExecDone:
		b.WriteString("**Code fix applied** ✅\n")
	case ActionExecCancelled:
		b.WriteString("**Code fix cancelled** ⏹\n")
	default:
		b.WriteString("**Code fix failed** ❌\n")
	}

	if repo := strings.TrimSpace(run.Repository); repo != "" {
		fmt.Fprintf(&b, "\nRepository: `%s`", repo)
	}
	if branch := strings.TrimSpace(run.Branch); branch != "" {
		fmt.Fprintf(&b, "\nBranch: `%s`", branch)
	}
	if pr := strings.TrimSpace(run.PRURL); pr != "" {
		fmt.Fprintf(&b, "\nPR: %s", pr)
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}

	if text := strings.TrimSpace(body); text != "" {
		b.WriteString("\n" + text + "\n")
	}
	if dialogID != uuid.Nil {
		fmt.Fprintf(&b, "\n[View agent transcript](#dialog:%s)\n", dialogID)
	}
	return strings.TrimRight(b.String(), "\n")
}
