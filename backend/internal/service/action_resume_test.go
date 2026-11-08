package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/llm"
)

// flakyProvider fails its first failures calls with err, then answers as
// scriptedProvider does. It is how an action run dies on the provider rather
// than on its own work.
type flakyProvider struct {
	mu       sync.Mutex
	failures int
	err      error
	seen     [][]llm.Message
}

func (p *flakyProvider) Complete(context.Context, string, string) (string, error) { return "", nil }

func (p *flakyProvider) CompleteWithTools(_ context.Context, msgs []llm.Message, _ []llm.ToolDef) (llm.AssistantMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seen = append(p.seen, append([]llm.Message(nil), msgs...))
	if p.failures > 0 {
		p.failures--
		return llm.AssistantMessage{}, p.err
	}
	return llm.AssistantMessage{Content: "bucket created"}, nil
}

func (p *flakyProvider) lastRequest() []llm.Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.seen) == 0 {
		return nil
	}
	return p.seen[len(p.seen)-1]
}

func resumeTestService(t *testing.T, provider llm.Provider) (*ChatService, *multiDialogRepo, uuid.UUID) {
	t.Helper()
	repo := newMultiDialogRepo()
	svc := decomposeTestService(t, repo, provider)
	planID := mainRoot(repo)
	writeStageRunPlan(t, svc, planID, map[string]any{
		"stages": []any{map[string]any{
			"title": "Storage",
			"steps": []any{
				map[string]any{"type": "shell", "action": "create the bucket"},
				map[string]any{"type": "code", "action": "add the module", "repository": "infra"},
			},
			"checks": []any{},
		}},
		"rollback": []any{},
	})
	return svc, repo, planID
}

// waitForRun waits until key's run has closed and the lease is free again, which
// is when the next launch can be made.
func waitForRun(t *testing.T, svc *ChatService, planID uuid.UUID, key string) ActionExecRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := svc.ReadActionPlanExecRuns(planID)
		if err != nil {
			t.Fatalf("ReadActionPlanExecRuns: %v", err)
		}
		if run := runs[key]; run.Status != "" && !run.Active() {
			if _, busy := svc.ExecutionInProgress(); !busy {
				return run
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run on %s did not finish", key)
	return ActionExecRun{}
}

func runDialog(t *testing.T, svc *ChatService, planID uuid.UUID, key string) uuid.UUID {
	t.Helper()
	runs, err := svc.ReadActionPlanRuns(planID)
	if err != nil {
		t.Fatalf("ReadActionPlanRuns: %v", err)
	}
	id, err := uuid.Parse(runs[key])
	if err != nil {
		t.Fatalf("runs[%s] = %q, want a dialog id", key, runs[key])
	}
	return id
}

func TestResumeContinuesTheFailedTranscript(t *testing.T) {
	t.Parallel()

	provider := &flakyProvider{failures: 1, err: &llm.APIError{StatusCode: 503, Op: "test", Message: "overloaded"}}
	svc, repo, planID := resumeTestService(t, provider)
	const key = "s0.step0"

	if _, err := svc.StartActionAgent(t.Context(), planID, key, ActionLaunch{}); err != nil {
		t.Fatalf("first StartActionAgent: %v", err)
	}
	first := waitForRun(t, svc, planID, key)
	if first.Status != ActionExecFailed || !strings.Contains(first.Error, "overloaded") {
		t.Fatalf("first run = %+v, want failed on the provider", first)
	}
	dialogID := runDialog(t, svc, planID, key)
	before, _ := repo.ListMessages(t.Context(), dialogID)

	if _, err := svc.StartActionAgent(t.Context(), planID, key, ActionLaunch{
		Resume: true,
		Note:   "use the eu-west-1 region",
	}); err != nil {
		t.Fatalf("resume StartActionAgent: %v", err)
	}
	second := waitForRun(t, svc, planID, key)
	if second.Status != ActionExecDone {
		t.Fatalf("resumed run = %+v, want done", second)
	}
	if second.Attempt != 2 {
		t.Errorf("Attempt = %d, want 2", second.Attempt)
	}

	// Same transcript, not a second dialog.
	if got := runDialog(t, svc, planID, key); got != dialogID {
		t.Errorf("runs.json dialog = %s, want the first run's %s", got, dialogID)
	}
	children, _ := repo.ListChildren(t.Context(), planID)
	if len(children) != 1 {
		t.Errorf("dialogs under the plan = %d, want 1", len(children))
	}

	after, _ := repo.ListMessages(t.Context(), dialogID)
	if len(after) != len(before)+2 {
		t.Fatalf("messages = %d, want the %d before plus a resume note and the answer", len(after), len(before))
	}
	note := after[len(before)]
	if note.Role != "user" {
		t.Fatalf("message after the old transcript has role %q, want user", note.Role)
	}
	for _, want := range []string{"Continue this action", "overloaded", "use the eu-west-1 region", "attempt 1"} {
		if !strings.Contains(note.Content, want) {
			t.Errorf("resume note is missing %q:\n%s", want, note.Content)
		}
	}

	// The model saw the original seed as well as the note: that is the point.
	req := provider.lastRequest()
	if len(req) < 3 || req[0].Role != "system" || req[len(req)-1].Content != note.Content {
		t.Errorf("resumed request = %d messages ending %q, want the whole transcript ending with the note",
			len(req), req[len(req)-1].Content)
	}
}

func TestResumeRefusals(t *testing.T) {
	t.Parallel()

	t.Run("never ran", func(t *testing.T) {
		t.Parallel()
		svc, _, planID := resumeTestService(t, &flakyProvider{})
		_, err := svc.StartActionAgent(t.Context(), planID, "s0.step0", ActionLaunch{Resume: true})
		if !errors.Is(err, ErrActionNothingToResume) {
			t.Fatalf("error = %v, want ErrActionNothingToResume", err)
		}
		if _, busy := svc.ExecutionInProgress(); busy {
			t.Error("a refused resume left the execution lease held")
		}
	})

	t.Run("finished", func(t *testing.T) {
		t.Parallel()
		svc, _, planID := resumeTestService(t, &flakyProvider{})
		if _, err := svc.StartActionAgent(t.Context(), planID, "s0.step0", ActionLaunch{}); err != nil {
			t.Fatalf("StartActionAgent: %v", err)
		}
		waitForRun(t, svc, planID, "s0.step0")
		_, err := svc.StartActionAgent(t.Context(), planID, "s0.step0", ActionLaunch{Resume: true})
		if !errors.Is(err, ErrActionResumeFinished) {
			t.Fatalf("error = %v, want ErrActionResumeFinished", err)
		}
	})

	t.Run("rerun and resume together", func(t *testing.T) {
		t.Parallel()
		svc, _, planID := resumeTestService(t, &flakyProvider{})
		_, err := svc.startPlanItem(t.Context(), planID, "s0.step0", ActionLaunch{Rerun: true, Resume: true})
		if !errors.Is(err, ErrActionLaunchConflict) {
			t.Fatalf("error = %v, want ErrActionLaunchConflict", err)
		}
	})

	t.Run("code action", func(t *testing.T) {
		t.Parallel()
		svc, _, planID := resumeTestService(t, &flakyProvider{})
		_, err := svc.startPlanItem(t.Context(), planID, "s0.step1", ActionLaunch{Resume: true})
		if !errors.Is(err, ErrActionResumeCode) {
			t.Fatalf("error = %v, want ErrActionResumeCode", err)
		}
	})

	t.Run("a sentence for the model", func(t *testing.T) {
		t.Parallel()
		svc, _, planID := resumeTestService(t, &flakyProvider{})
		text, started, err := svc.executePlanItem(t.Context(), planID, "1.1", ActionLaunch{Resume: true})
		if err != nil || started {
			t.Fatalf("executePlanItem = %q, %v, %v; want a refusal sentence", text, started, err)
		}
		if !strings.Contains(text, "no earlier sub-agent run") {
			t.Errorf("refusal = %q, want it to say there is nothing to continue", text)
		}
	})
}

func TestBuildActionResumeNote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prev    ActionExecRun
		note    string
		want    []string
		notWant []string
	}{
		{
			name: "failed",
			prev: ActionExecRun{Status: ActionExecFailed, Attempt: 2, Error: "completion round 7: HTTP 503"},
			want: []string{"(attempt 2)", "HTTP 503", "check its current state"},
		},
		{
			name: "timed out without a reason",
			prev: ActionExecRun{Status: ActionExecFailed},
			want: []string{"ended before it finished: it failed."},
		},
		{
			name: "cancelled",
			prev: ActionExecRun{Status: ActionExecCancelled, Attempt: 1, Error: "stopped before it finished"},
			want: []string{"stopped before it finished"},
		},
		{
			name: "blocked with an answer",
			prev: ActionExecRun{Status: ActionExecBlocked, Error: "which region?"},
			note: "eu-west-1",
			want: []string{"needed a decision from the operator: which region?", "## From the operator", "eu-west-1"},
			notWant: []string{
				"has not answered",
			},
		},
		{
			name: "blocked without an answer",
			prev: ActionExecRun{Status: ActionExecBlocked, Error: "which region?"},
			want: []string{"has not answered that question", "never guess"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildActionResumeNote(tc.prev, tc.note)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("note is missing %q:\n%s", w, got)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("note should not contain %q:\n%s", w, got)
				}
			}
		})
	}
}
