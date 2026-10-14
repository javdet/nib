package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
)

const guardTestDAG = "```mermaid\nflowchart TD\n  a[\"Deploy\"]\n```"

// guardedRoot is a plan whose DAG and action plan already exist, with the
// operator's chat holding msgs.
func guardedRoot(t *testing.T, svc *ChatService, repo *multiDialogRepo, msgs ...domain.DialogMessage) uuid.UUID {
	t.Helper()

	root := mainRoot(repo)
	if err := os.MkdirAll(svc.dagsDir, 0o755); err != nil {
		t.Fatalf("mkdir dags: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.dagsDir, root.String()+".md"), []byte(guardTestDAG), 0o644); err != nil {
		t.Fatalf("write dag: %v", err)
	}
	if err := os.MkdirAll(svc.actionPlansDir, 0o755); err != nil {
		t.Fatalf("mkdir action plans: %v", err)
	}
	if err := os.WriteFile(filepath.Join(svc.actionPlansDir, root.String()+".json"), []byte(`{"stages":[]}`), 0o644); err != nil {
		t.Fatalf("write action plan: %v", err)
	}
	for _, m := range msgs {
		if _, err := repo.AppendMessage(t.Context(), root, m); err != nil {
			t.Fatalf("append message: %v", err)
		}
	}
	return root
}

func operatorSays(text string) domain.DialogMessage {
	return domain.DialogMessage{Role: "user", Content: text}
}

// A failed deploy is not a reason to rebuild the plan: without the operator's
// own words asking for a change, neither specialist gets to touch what exists.
func TestPlanChangeIsRefusedWithoutTheOperatorAsking(t *testing.T) {
	t.Parallel()

	const failure = "deploy job failed: Could not assume role with OIDC, help me fix it"

	for name, tc := range map[string]struct {
		args map[string]any
		want string
	}{
		"decompose, no request": {
			args: map[string]any{"name": "decompose", "task": "add an IAM stage"},
			want: "already has its DAG",
		},
		"plan, no request": {
			args: map[string]any{"name": "plan"},
			want: "already has its action plan",
		},
		"decompose, invented request": {
			args: map[string]any{"name": "decompose", "task": "add an IAM stage", "operator_request": "add an IAM stage"},
			want: "is not in the operator's latest message",
		},
		"plan, empty quote marks": {
			args: map[string]any{"name": "plan", "operator_request": `""`},
			want: "already has its action plan",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := newMultiDialogRepo()
			svc := decomposeTestService(t, repo, &scriptedProvider{})
			root := guardedRoot(t, svc, repo, operatorSays(failure))

			out, err := svc.runSubagentHandler(newToolBinding(root))(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("err = %v, want the refusal as output", err)
			}
			if !strings.Contains(out, `"status":"refused"`) || !strings.Contains(out, tc.want) {
				t.Errorf("out = %s, want a refusal containing %q", out, tc.want)
			}

			children, _ := repo.ListChildren(t.Context(), root)
			if len(children) != 0 {
				t.Errorf("children = %d, want nothing launched", len(children))
			}
			if run, found, _ := svc.ReadFanoutRun(root); found {
				t.Errorf("a fan-out was started: %+v", run)
			}
		})
	}
}

// Only the operator's latest input counts: an older request has already been
// acted on, and a question's own wording is the model's, not the operator's.
func TestPlanChangeQuoteHasToComeFromTheLatestOperatorInput(t *testing.T) {
	t.Parallel()

	askAnswer := domain.DialogMessage{
		Role:       "tool",
		Name:       AskQuestionToolName,
		ToolCallID: "ask-1",
		Content:    `{"answers":[{"question":"Add a stage for the IAM trust policy?","answer":"Yes, add it"}]}`,
	}

	for name, tc := range map[string]struct {
		msgs    []domain.DialogMessage
		quote   string
		allowed bool
	}{
		"latest message": {
			msgs:    []domain.DialogMessage{operatorSays("Add a monitoring stage after Deploy")},
			quote:   "add a monitoring stage",
			allowed: true,
		},
		"quoted with marks and odd spacing": {
			msgs:    []domain.DialogMessage{operatorSays("Добавь  этап мониторинга")},
			quote:   "«добавь этап мониторинга»",
			allowed: true,
		},
		"an earlier message": {
			msgs: []domain.DialogMessage{
				operatorSays("add a monitoring stage"),
				{Role: "assistant", Content: "done"},
				operatorSays("the deploy failed, fix it"),
			},
			quote: "add a monitoring stage",
		},
		"an answer to ask_question": {
			msgs:    []domain.DialogMessage{operatorSays("the deploy failed"), askAnswer},
			quote:   "yes, add it",
			allowed: true,
		},
		"the question's own wording": {
			msgs:  []domain.DialogMessage{operatorSays("the deploy failed"), askAnswer},
			quote: "add a stage for the IAM trust policy",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			repo := newMultiDialogRepo()
			svc := decomposeTestService(t, repo, &scriptedProvider{})
			root := guardedRoot(t, svc, repo, tc.msgs...)

			_, refused, err := svc.guardPlanChange(t.Context(), root, SubagentRequest{
				Name:            SubagentDecompose,
				OperatorRequest: tc.quote,
			})
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if refused == tc.allowed {
				t.Errorf("refused = %v, want allowed = %v", refused, tc.allowed)
			}
		})
	}
}

// The first DAG and the first plan are what the operator asked for by starting
// the plan; nothing exists yet to protect.
func TestPlanChangeGuardLetsTheFirstDAGAndPlanThrough(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := decomposeTestService(t, repo, &scriptedProvider{})
	root := mainRoot(repo)

	for _, name := range []SubagentName{SubagentDecompose, SubagentPlan} {
		if _, refused, err := svc.guardPlanChange(t.Context(), root, SubagentRequest{Name: name}); err != nil || refused {
			t.Errorf("%s: refused = %v, err = %v, want it let through", name, refused, err)
		}
	}
}

// Relaying an answer to a question decompose is suspended on carries the same
// conversation on; it must not be refused as a new change.
func TestPlanChangeGuardLetsASuspendedDecomposeBeAnswered(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := decomposeTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		askQuestionTurn("ask-1", "Which cluster?"),
	}})
	root := guardedRoot(t, svc, repo, operatorSays("add a stage"))

	if _, err := svc.launchDecomposeSubagent(t.Context(), root, "add a stage"); err != nil {
		t.Fatalf("launch err = %v", err)
	}

	_, refused, err := svc.guardPlanChange(t.Context(), root, SubagentRequest{Name: SubagentDecompose, Task: "staging"})
	if err != nil || refused {
		t.Errorf("refused = %v, err = %v, want the relayed answer let through", refused, err)
	}
}
