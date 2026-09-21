package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// transcriptDialogRepo is mapDialogRepo with the messages actually kept, which is
// what a report posted into the plan chat has to be read back out of.
type transcriptDialogRepo struct {
	mapDialogRepo
	mu       sync.Mutex
	messages map[uuid.UUID][]domain.DialogMessage
}

func (r *transcriptDialogRepo) ListMessages(_ context.Context, dialogID uuid.UUID) ([]domain.DialogMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.DialogMessage(nil), r.messages[dialogID]...), nil
}

func (r *transcriptDialogRepo) AppendMessage(_ context.Context, dialogID uuid.UUID, msg domain.DialogMessage) (domain.DialogMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg.DialogID = dialogID
	r.messages[dialogID] = append(r.messages[dialogID], msg)
	return msg, nil
}

func newCodeActionReportService(t *testing.T, planID, execID uuid.UUID) (*ChatService, *transcriptDialogRepo) {
	t.Helper()

	dir := t.TempDir()
	repo := &transcriptDialogRepo{
		mapDialogRepo: mapDialogRepo{dialogs: map[uuid.UUID]domain.Dialog{
			planID: {ID: planID, Mode: "main"},
			execID: {ID: execID, Mode: "execute", ParentID: &planID},
		}},
		messages: map[uuid.UUID][]domain.DialogMessage{},
	}
	svc := &ChatService{
		dialogRepo:     repo,
		actionPlansDir: dir,
		activity:       NewActivityBroker(),
	}

	if err := writeAttachTestPlan(dir, planID); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	if err := svc.WriteActionPlanRuns(planID, map[string]string{"s0.step0": execID.String()}); err != nil {
		t.Fatalf("write runs: %v", err)
	}
	if err := svc.WriteActionPlanExecRuns(planID, ActionExecRuns{"s0.step0": {
		Status:    ActionExecRunning,
		StartedAt: time.Now().Unix(),
		Attempt:   1,
		Kind:      ExecutionKindContainer,
	}}); err != nil {
		t.Fatalf("write exec runs: %v", err)
	}
	return svc, repo
}

func TestAppendAgentResultReportsCodeActionInPlanChat(t *testing.T) {
	t.Parallel()

	planID, execID := uuid.New(), uuid.New()
	svc, repo := newCodeActionReportService(t, planID, execID)

	res := AgentRunResult{
		Status:       "success",
		Result:       "opened the pull request",
		ChatID:       execID.String(),
		JobName:      "nib-12345678",
		TargetBranch: "nib/0199",
	}
	if _, err := svc.AppendAgentResult(context.Background(), execID, res); err != nil {
		t.Fatalf("AppendAgentResult: %v", err)
	}

	planMsgs, _ := repo.ListMessages(context.Background(), planID)
	if len(planMsgs) != 1 {
		t.Fatalf("plan chat has %d messages, want 1", len(planMsgs))
	}
	got := planMsgs[0]
	if want := actionExecMessageName("s0.step0", 1); got.Name != want {
		t.Fatalf("message name = %q, want %q", got.Name, want)
	}
	if !strings.Contains(got.Content, "executed") {
		t.Fatalf("plan chat message does not report the action as executed: %q", got.Content)
	}
	if !strings.Contains(got.Content, "#dialog:"+execID.String()) {
		t.Fatalf("plan chat message has no link to the execute dialog: %q", got.Content)
	}
	// The result itself stays out: it is working notes for the actions after
	// this one, recorded on the action rather than spelled out to the operator.
	if strings.Contains(got.Content, "opened the pull request") {
		t.Fatalf("successful run should not spell its result out in the plan chat: %q", got.Content)
	}

	runs, err := svc.ReadActionPlanExecRuns(planID)
	if err != nil {
		t.Fatalf("ReadActionPlanExecRuns: %v", err)
	}
	if runs["s0.step0"].Status != ActionExecDone {
		t.Fatalf("run status = %q, want %q", runs["s0.step0"].Status, ActionExecDone)
	}

	// The container retries the webhook; the retry must not post a second time.
	if _, err := svc.AppendAgentResult(context.Background(), execID, res); err != nil {
		t.Fatalf("AppendAgentResult duplicate: %v", err)
	}
	planMsgs, _ = repo.ListMessages(context.Background(), planID)
	if len(planMsgs) != 1 {
		t.Fatalf("duplicate delivery posted again: %d messages", len(planMsgs))
	}
}

func TestAppendAgentResultReportsCodeActionFailure(t *testing.T) {
	t.Parallel()

	planID, execID := uuid.New(), uuid.New()
	svc, repo := newCodeActionReportService(t, planID, execID)

	if _, err := svc.AppendAgentResult(context.Background(), execID, AgentRunResult{
		Status:   "failed",
		ExitCode: 2,
		Result:   "the build never compiled",
		ChatID:   execID.String(),
		JobName:  "nib-87654321",
	}); err != nil {
		t.Fatalf("AppendAgentResult: %v", err)
	}

	planMsgs, _ := repo.ListMessages(context.Background(), planID)
	if len(planMsgs) != 1 {
		t.Fatalf("plan chat has %d messages, want 1", len(planMsgs))
	}
	content := planMsgs[0].Content
	if !strings.Contains(content, "failed") {
		t.Fatalf("plan chat message does not report the failure: %q", content)
	}
	if !strings.Contains(content, "exited with code 2") {
		t.Fatalf("plan chat message does not name the exit code: %q", content)
	}
	if !strings.Contains(content, "the build never compiled") {
		t.Fatalf("a failure should keep the agent's own words: %q", content)
	}
}

func TestCodeActionReportText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		res    AgentRunResult
		errMsg string
		want   string
	}{
		{name: "success has no error line", res: AgentRunResult{Result: "done"}, want: "done"},
		{
			name:   "failure joins the reason and the result",
			res:    AgentRunResult{Result: "could not push"},
			errMsg: "the coding agent exited with code 1",
			want:   "the coding agent exited with code 1\n\ncould not push",
		},
		{
			name:   "silent failure is still explained",
			res:    AgentRunResult{},
			errMsg: "the coding agent exited with code 137",
			want:   "the coding agent exited with code 137",
		},
		{name: "nothing at all", res: AgentRunResult{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := codeActionReportText(tt.res, tt.errMsg); got != tt.want {
				t.Fatalf("codeActionReportText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCodeActionReportTextTruncates(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", codeActionReportMaxBytes*2)
	got := codeActionReportText(AgentRunResult{Result: long}, "")
	if len(got) > codeActionReportMaxBytes {
		t.Fatalf("report text is %d bytes, want at most %d", len(got), codeActionReportMaxBytes)
	}
	if !strings.HasSuffix(got, "_(truncated)_") {
		t.Fatalf("truncated text should say so: %q", got[len(got)-40:])
	}
}
