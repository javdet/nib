package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

func codeFixTestService(t *testing.T, repo *multiDialogRepo) *ChatService {
	t.Helper()
	return &ChatService{
		dialogRepo:     repo,
		codeFixesDir:   t.TempDir(),
		actionPlansDir: t.TempDir(),
		activity:       NewActivityBroker(),
	}
}

// startedFix registers a running fix the way StartCodeFix does, so the closing
// paths can be tested without a container.
func startedFix(t *testing.T, svc *ChatService, rootID, dialogID uuid.UUID) {
	t.Helper()
	if err := svc.recordCodeFixRun(rootID, dialogID, CodeFixRun{
		Status:     ActionExecRunning,
		Repository: "infra",
		Branch:     "nib/DO-7",
		JobName:    "nib-12345678",
		StartedAt:  time.Now().Unix(),
	}); err != nil {
		t.Fatalf("recordCodeFixRun: %v", err)
	}
}

func TestCodeFixRunLifecycle(t *testing.T) {
	t.Parallel()

	svc := codeFixTestService(t, newMultiDialogRepo())
	rootID, dialogID := uuid.New(), uuid.New()

	// A chat that never asked for a fix reads clean rather than erroring.
	runs, err := svc.ReadCodeFixRuns(rootID)
	if err != nil {
		t.Fatalf("ReadCodeFixRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %v, want empty", runs)
	}

	if _, found, err := svc.updateCodeFixRun(rootID, dialogID, func(*CodeFixRun) {}); err != nil || found {
		t.Fatalf("updateCodeFixRun before record: found = %v, err = %v, want false, nil", found, err)
	}

	startedFix(t, svc, rootID, dialogID)

	run, found, err := svc.updateCodeFixRun(rootID, dialogID, func(r *CodeFixRun) {
		r.PRURL = "https://example.com/pr/1"
	})
	if err != nil || !found {
		t.Fatalf("updateCodeFixRun: found = %v, err = %v", found, err)
	}
	if run.PRURL != "https://example.com/pr/1" || run.Branch != "nib/DO-7" {
		t.Fatalf("run = %+v, want the mutation applied and the rest kept", run)
	}
}

// A fix reports into the chat that asked for it, and the report links to the
// agent's own transcript: nothing else in that chat does.
func TestFinishCodeFixRun_reportsIntoTheChat(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := codeFixTestService(t, repo)
	rootID := repo.add(domain.Dialog{Mode: mainDialogMode})
	dialogID := repo.add(domain.Dialog{Mode: executeDialogMode, ParentID: &rootID})
	startedFix(t, svc, rootID, dialogID)

	svc.finishCodeFixRun(t.Context(), rootID, dialogID, AgentRunResult{
		Status:       "success",
		JobName:      "nib-12345678",
		TargetBranch: "nib/DO-7",
		PRURL:        "https://example.com/pr/1",
		Result:       "edited three files",
	})

	msgs, err := repo.ListMessages(t.Context(), rootID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	body := msgs[0].Content
	for _, want := range []string{"Code fix applied", "nib/DO-7", "https://example.com/pr/1", "#dialog:" + dialogID.String()} {
		if !strings.Contains(body, want) {
			t.Errorf("report = %q, want it to contain %q", body, want)
		}
	}
	// The agent's account of a successful run belongs in its own transcript; the
	// chat gets the outcome and a link.
	if strings.Contains(body, "edited three files") {
		t.Errorf("report = %q, want the result text left in the fix's own dialog", body)
	}

	run, _, err := svc.updateCodeFixRun(rootID, dialogID, func(*CodeFixRun) {})
	if err != nil {
		t.Fatalf("updateCodeFixRun: %v", err)
	}
	if run.Status != ActionExecDone || run.FinishedAt == 0 || run.PRURL == "" {
		t.Fatalf("run = %+v, want a closed run carrying the pull request", run)
	}

	// The container retries the webhook with curl, so a second delivery must add
	// nothing.
	svc.finishCodeFixRun(t.Context(), rootID, dialogID, AgentRunResult{
		Status: "success", JobName: "nib-12345678",
	})
	msgs, _ = repo.ListMessages(t.Context(), rootID)
	if len(msgs) != 1 {
		t.Fatalf("messages after a duplicate delivery = %d, want 1", len(msgs))
	}
}

// A failure is the case where the agent's own words are the only explanation
// worth having, so they are kept.
func TestFinishCodeFixRun_keepsTheReasonOnAFailure(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := codeFixTestService(t, repo)
	rootID := repo.add(domain.Dialog{Mode: mainDialogMode})
	dialogID := repo.add(domain.Dialog{Mode: executeDialogMode, ParentID: &rootID})
	startedFix(t, svc, rootID, dialogID)

	svc.finishCodeFixRun(t.Context(), rootID, dialogID, AgentRunResult{
		Status:   "failed",
		ExitCode: 2,
		JobName:  "nib-12345678",
		Result:   "the tests would not build",
	})

	msgs, _ := repo.ListMessages(t.Context(), rootID)
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	for _, want := range []string{"Code fix failed", "exited with code 2", "the tests would not build"} {
		if !strings.Contains(msgs[0].Content, want) {
			t.Errorf("report = %q, want it to contain %q", msgs[0].Content, want)
		}
	}

	run, _, _ := svc.updateCodeFixRun(rootID, dialogID, func(*CodeFixRun) {})
	if run.Status != ActionExecFailed {
		t.Errorf("status = %q, want %q", run.Status, ActionExecFailed)
	}
}

// run_executor reports through the same webhook and has no record here, so a
// delivery with nothing behind it must stay silent rather than post an outcome
// into somebody's chat.
func TestFinishCodeFixRun_ignoresAContainerThatIsNotAFix(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := codeFixTestService(t, repo)
	rootID := repo.add(domain.Dialog{Mode: mainDialogMode})
	dialogID := repo.add(domain.Dialog{Mode: executeDialogMode, ParentID: &rootID})

	svc.finishCodeFixRun(t.Context(), rootID, dialogID, AgentRunResult{Status: "success", JobName: "nib-1"})

	if msgs, _ := repo.ListMessages(t.Context(), rootID); len(msgs) != 0 {
		t.Fatalf("messages = %d, want 0", len(msgs))
	}
}

// A force stop has to close the fix and say so, and must not touch the plan
// files: a fix has no row in them.
func TestCancelExecution_closesACodeFix(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := codeFixTestService(t, repo)
	rootID := repo.add(domain.Dialog{Mode: mainDialogMode})
	dialogID := repo.add(domain.Dialog{Mode: executeDialogMode, ParentID: &rootID})
	startedFix(t, svc, rootID, dialogID)

	lease, ok := svc.acquireExecutionLease(ExecutionLease{
		PlanID:    rootID,
		DialogID:  dialogID,
		Kind:      ExecutionKindContainer,
		Fix:       true,
		JobName:   "nib-12345678",
		StartedAt: time.Now().Unix(),
	})
	if !ok {
		t.Fatal("acquireExecutionLease refused an idle lease")
	}
	if got := leaseSubject(lease); got != "the code fix" {
		t.Errorf("leaseSubject = %q, want %q", got, "the code fix")
	}

	// stopLeaseHolder needs the executor for a container, which no test has, so
	// the stop itself fails -- and the lease must still be freed and the fix
	// still closed, which is the whole point of the call.
	held, err := svc.CancelExecution(t.Context())
	if err == nil {
		t.Fatal("CancelExecution succeeded without an executor, want a stop error")
	}
	if !held.Fix {
		t.Fatalf("held = %+v, want the fix's lease", held)
	}
	if _, running := svc.ExecutionInProgress(); running {
		t.Error("the execution slot is still held after a force stop")
	}

	run, _, _ := svc.updateCodeFixRun(rootID, dialogID, func(*CodeFixRun) {})
	if run.Status != ActionExecCancelled {
		t.Errorf("status = %q, want %q", run.Status, ActionExecCancelled)
	}

	msgs, _ := repo.ListMessages(t.Context(), rootID)
	if len(msgs) != 1 || !strings.Contains(msgs[0].Content, "Code fix cancelled") {
		t.Fatalf("messages = %+v, want one cancellation report", msgs)
	}

	// Nothing may have been written against a plan row that does not exist.
	execRuns, err := svc.ReadActionPlanExecRuns(rootID)
	if err != nil {
		t.Fatalf("ReadActionPlanExecRuns: %v", err)
	}
	if len(execRuns) != 0 {
		t.Fatalf("action exec runs = %v, want none: a fix has no plan row", execRuns)
	}
}

// The lease of a fix is identified by its dialog, and an action's release path
// must not reach it -- nor the other way round.
func TestReleaseExecutionLeaseForFix(t *testing.T) {
	t.Parallel()

	svc := codeFixTestService(t, newMultiDialogRepo())
	rootID, dialogID := uuid.New(), uuid.New()

	take := func() {
		t.Helper()
		if _, ok := svc.acquireExecutionLease(ExecutionLease{
			PlanID:    rootID,
			DialogID:  dialogID,
			Kind:      ExecutionKindContainer,
			Fix:       true,
			JobName:   "nib-12345678",
			StartedAt: time.Now().Unix(),
		}); !ok {
			t.Fatal("acquireExecutionLease refused an idle lease")
		}
	}

	take()
	svc.releaseExecutionLeaseForContainer(rootID, "", "nib-12345678")
	if _, running := svc.ExecutionInProgress(); !running {
		t.Error("an action release dropped a fix's lease")
	}

	svc.releaseExecutionLeaseForFix(rootID, uuid.New(), "nib-12345678")
	if _, running := svc.ExecutionInProgress(); !running {
		t.Error("a release for another dialog dropped this fix's lease")
	}

	// A delivery arriving after the operator stopped the run and started another
	// must not free the new one.
	svc.releaseExecutionLeaseForFix(rootID, dialogID, "nib-87654321")
	if _, running := svc.ExecutionInProgress(); !running {
		t.Error("a release naming another job dropped this fix's lease")
	}

	svc.releaseExecutionLeaseForFix(rootID, dialogID, "nib-12345678")
	if _, running := svc.ExecutionInProgress(); running {
		t.Error("the fix's own release did not drop its lease")
	}
}

// A second execution is refused with a sentence naming what holds the slot, and
// a fix has no plan row to be named by.
func TestBusyExecutionMessage_namesACodeFix(t *testing.T) {
	t.Parallel()

	msg := busyExecutionMessage(ExecutionLease{
		Kind:      ExecutionKindContainer,
		Fix:       true,
		JobName:   "nib-12345678",
		StartedAt: time.Now().Add(-2 * time.Minute).Unix(),
	})
	for _, want := range []string{"a code fix is already running", "container nib-12345678", "2 minutes"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message = %q, want it to contain %q", msg, want)
		}
	}
}

// A restart strands a fix exactly as it strands a code action: the webhook has
// nowhere to land, and the record would stay running -- holding the one
// execution slot against every plan.
func TestReconcileStuckCodeFixRuns(t *testing.T) {
	t.Parallel()

	svc := codeFixTestService(t, newMultiDialogRepo())
	rootID := uuid.New()
	running, done := uuid.New(), uuid.New()

	startedFix(t, svc, rootID, running)
	if err := svc.recordCodeFixRun(rootID, done, CodeFixRun{Status: ActionExecDone, FinishedAt: 1}); err != nil {
		t.Fatalf("recordCodeFixRun: %v", err)
	}

	closed, err := svc.reconcileStuckCodeFixRuns()
	if err != nil {
		t.Fatalf("reconcileStuckCodeFixRuns: %v", err)
	}
	if closed != 1 {
		t.Fatalf("closed = %d, want 1", closed)
	}

	runs, err := svc.ReadCodeFixRuns(rootID)
	if err != nil {
		t.Fatalf("ReadCodeFixRuns: %v", err)
	}
	if got := runs[running.String()]; got.Status != ActionExecFailed || got.Error != restartedReason {
		t.Errorf("stranded run = %+v, want it failed with the restart reason", got)
	}
	if got := runs[done.String()]; got.Status != ActionExecDone || got.Error != "" {
		t.Errorf("finished run = %+v, want it left alone", got)
	}
}

func TestBuildCodeFixTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  CodeFixRequest
		task string
		want string
	}{
		{
			name: "pull request title wins",
			req:  CodeFixRequest{PRTitle: "fix: nil check"},
			task: "the handler panics on an empty body\nmore detail",
			want: "fix: nil check",
		},
		{
			name: "falls back to the first line of the task",
			task: "the handler panics on an empty body\nmore detail",
			want: "the handler panics on an empty body",
		},
		{
			name: "truncated to the column the interface has",
			task: strings.Repeat("x", executeDialogTitleMaxLen+20),
			want: strings.Repeat("x", executeDialogTitleMaxLen),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := buildCodeFixTitle(tt.req, tt.task); got != tt.want {
				t.Fatalf("buildCodeFixTitle = %q, want %q", got, tt.want)
			}
		})
	}
}

// The code sub-agent reads arguments no other one does, and a missing one has to
// cost a sentence rather than the turn.
func TestParseSubagentRequest_code(t *testing.T) {
	t.Parallel()

	spec, ok := subagentSpec(SubagentCode)
	if !ok {
		t.Fatal("the code sub-agent is not in the registry")
	}

	req, msg := parseSubagentRequest(spec, map[string]any{
		"task":       " fix the nil deref ",
		"repository": " infra ",
		"branch":     " nib/DO-7 ",
		"pr_title":   " fix: nil deref ",
	})
	if msg != "" {
		t.Fatalf("parseSubagentRequest refused a complete request: %s", msg)
	}
	if req.Task != "fix the nil deref" || req.Repository != "infra" ||
		req.Branch != "nib/DO-7" || req.PRTitle != "fix: nil deref" {
		t.Fatalf("req = %+v, want every argument read and trimmed", req)
	}

	if _, msg := parseSubagentRequest(spec, map[string]any{"repository": "infra"}); !strings.Contains(msg, `"task"`) {
		t.Errorf("missing task reported as %q, want it to name task", msg)
	}
	if _, msg := parseSubagentRequest(spec, map[string]any{"task": "fix it"}); !strings.Contains(msg, `"repository"`) {
		t.Errorf("missing repository reported as %q, want it to name repository", msg)
	}

	// The arguments are shared across one flat schema, so a specialist that does
	// not read one must not pick it up.
	execSpec, _ := subagentSpec(SubagentExecute)
	if got, _ := parseSubagentRequest(execSpec, map[string]any{"item": "1.1", "repository": "infra"}); got.Repository != "" {
		t.Errorf("execute read repository = %q, want it ignored", got.Repository)
	}
}

// StartCodeFix is reached through the orchestrator alone, so its preconditions
// are checked before anything is created: a fix with no repository must not
// leave a dialog or a record behind.
func TestStartCodeFix_refusesAnIncompleteRequest(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := codeFixTestService(t, repo)
	// The executor and secret services are unset, so this only ever gets as far
	// as the argument checks -- which is what is being tested.
	rootID := repo.add(domain.Dialog{Mode: mainDialogMode})

	for name, req := range map[string]CodeFixRequest{
		"no task":       {Repository: "infra"},
		"no repository": {Task: "fix it"},
	} {
		if _, _, err := svc.StartCodeFix(context.Background(), rootID, req); err == nil {
			t.Errorf("%s: StartCodeFix succeeded, want a refusal", name)
		}
	}

	if children, _ := repo.ListChildren(context.Background(), rootID); len(children) != 0 {
		t.Errorf("children = %d, want none: nothing was launched", len(children))
	}
}
