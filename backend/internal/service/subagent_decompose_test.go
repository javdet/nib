package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mode"
)

// decomposeTestService wires just enough of ChatService to run a decompose
// sub-agent turn against a temp data directory.
func decomposeTestService(t *testing.T, repo *multiDialogRepo, provider llm.Provider) *ChatService {
	t.Helper()

	dir := t.TempDir()
	// The built-in allow lists, seeded the way the backend seeds them at boot:
	// resolveAllowSet turns a missing list into an empty set rather than into no
	// filtering, so without this the sub-agent gets only the always-on tools.
	allowToolsDir := filepath.Join(dir, "tools")
	if _, err := mode.SeedAllowLists(allowToolsDir); err != nil {
		t.Fatalf("SeedAllowLists: %v", err)
	}

	return &ChatService{
		provider:       provider,
		dialogRepo:     repo,
		mcpSvc:         stubMCPService(),
		variableRepo:   &stubVariableRepo{},
		allowToolsDir:  allowToolsDir,
		dagsDir:        filepath.Join(dir, "dags"),
		summariesDir:   filepath.Join(dir, "summaries"),
		actionPlansDir: filepath.Join(dir, "action_plans"),
		subagentsDir:   filepath.Join(dir, "subagents"),
		maxIterations:  4,
		actionExec:     ActionExecConfig{}.withDefaults(),
		activity:       NewActivityBroker(),
	}
}

func mainRoot(repo *multiDialogRepo) uuid.UUID {
	return repo.add(domain.Dialog{Mode: mainDialogMode})
}

// The DAG and the summary belong to the plan, not to the transcript that wrote
// them. This is the core of routing artifacts by the root dialog: without it a
// plan's own page would show nothing.
func TestLaunchDecomposeSubagentWritesArtifactsOnTheRoot(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)

	provider := &scriptedProvider{turns: []llm.AssistantMessage{
		{ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: CreateDAGToolName, Arguments: `{"mermaid":"flowchart TD\n  a[\"Deploy\"]"}`},
			{ID: "c2", Name: CreateSummaryToolName, Arguments: `{"summary":"Deploy the thing."}`},
		}},
		{Content: "Subject: the thing. Action: deploy."},
	}}
	svc := decomposeTestService(t, repo, provider)

	res, err := svc.launchDecomposeSubagent(t.Context(), root, "plan DO-1")
	if err != nil {
		t.Fatalf("launchDecomposeSubagent err = %v", err)
	}
	if res.Status != SubagentCompleted {
		t.Fatalf("status = %q, want %q", res.Status, SubagentCompleted)
	}
	if !strings.Contains(res.Summary, "deploy") {
		t.Errorf("summary = %q, want the sub-agent's own final message", res.Summary)
	}

	child := decomposeChild(t, repo, root)
	for _, tc := range []struct{ what, path string }{
		{"the DAG", filepath.Join(svc.dagsDir, root.String()+".md")},
		{"the summary", filepath.Join(svc.summariesDir, root.String()+".txt")},
	} {
		if _, err := os.Stat(tc.path); err != nil {
			t.Errorf("%s was not written on the root: %v", tc.what, err)
		}
	}
	if _, err := os.Stat(filepath.Join(svc.dagsDir, child.String()+".md")); err == nil {
		t.Error("the DAG was written on the sub-agent's transcript instead of the plan")
	}
}

// One decompose transcript per plan: revising a DAG has to happen in the
// conversation that produced it, not in a fresh agent starting from nothing.
func TestLaunchDecomposeSubagentReusesOneDialog(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := decomposeTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		{Content: "first"}, {Content: "second"},
	}})

	if _, err := svc.launchDecomposeSubagent(t.Context(), root, "plan DO-1"); err != nil {
		t.Fatalf("first launch err = %v", err)
	}
	first := decomposeChild(t, repo, root)

	if _, err := svc.launchDecomposeSubagent(t.Context(), root, "add a stage"); err != nil {
		t.Fatalf("second launch err = %v", err)
	}
	if second := decomposeChild(t, repo, root); second != first {
		t.Errorf("the second launch started a new transcript %v, want %v", second, first)
	}

	msgs, _ := repo.ListMessages(t.Context(), first)
	var users, systems int
	for _, m := range msgs {
		switch m.Role {
		case "user":
			users++
		case "system":
			systems++
		}
	}
	if systems != 1 {
		t.Errorf("system rows = %d, want the prompt seeded exactly once", systems)
	}
	if users != 2 {
		t.Errorf("user rows = %d, want one per launch", users)
	}
}

// A suspended sub-agent has to be findable again, or the operator answers into
// nothing.
func TestLaunchDecomposeSubagentRecordsAPauseWhenItAsks(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := decomposeTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		askQuestionTurn("ask-1", "Which cluster?"),
	}})

	res, err := svc.launchDecomposeSubagent(t.Context(), root, "plan DO-1")
	if err != nil {
		t.Fatalf("launchDecomposeSubagent err = %v", err)
	}
	if res.Status != SubagentAwaitingInput {
		t.Fatalf("status = %q, want %q", res.Status, SubagentAwaitingInput)
	}
	if len(res.Questions) != 1 || res.Questions[0].Question != "Which cluster?" {
		t.Fatalf("questions = %+v, want the one the sub-agent asked", res.Questions)
	}

	state, found, err := svc.ReadSubagentState(root)
	if err != nil || !found {
		t.Fatalf("ReadSubagentState found = %v, err = %v", found, err)
	}
	if len(state.Pauses) != 1 {
		t.Fatalf("pauses = %d, want 1", len(state.Pauses))
	}
	p := state.Pauses[0]
	if p.Subagent != SubagentDecompose {
		t.Errorf("pause.Subagent = %q, want decompose", p.Subagent)
	}
	if p.ToolCallID != "ask-1" {
		t.Errorf("pause.ToolCallID = %q, want the sub-agent's dangling call", p.ToolCallID)
	}
	if p.MainAskID != "" {
		t.Errorf("pause.MainAskID = %q, want it unbound until the orchestrator relays it", p.MainAskID)
	}
}

// The safety net for a relayed answer that arrives as prose rather than through
// the structured resume: a re-launch has to answer the dangling call instead of
// stacking a second user turn behind it.
func TestLaunchDecomposeSubagentAnswersADanglingQuestion(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := decomposeTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		askQuestionTurn("ask-1", "Which cluster?"),
		{Content: "planned against staging"},
	}})

	if _, err := svc.launchDecomposeSubagent(t.Context(), root, "plan DO-1"); err != nil {
		t.Fatalf("first launch err = %v", err)
	}
	if _, err := svc.launchDecomposeSubagent(t.Context(), root, "staging"); err != nil {
		t.Fatalf("second launch err = %v", err)
	}

	child := decomposeChild(t, repo, root)
	msgs, _ := repo.ListMessages(t.Context(), child)

	var answered bool
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID == "ask-1" {
			answered = true
			if m.Content != "staging" {
				t.Errorf("the answer row holds %q, want the operator's answer", m.Content)
			}
		}
		if m.Role == "user" && m.Content == "staging" {
			t.Error("the answer was appended as a fresh task, leaving the question dangling")
		}
	}
	if !answered {
		t.Error("the dangling ask_question was never answered")
	}

	// The pause is settled, so a later answer cannot write a second result for
	// the same call.
	state, _, _ := svc.ReadSubagentState(root)
	if len(state.Pauses) != 0 {
		t.Errorf("pauses = %d, want the settled one forgotten", len(state.Pauses))
	}
}

// chat_name names the transcript it is called in, and the sub-agent's prompt
// still tells it to call one. Without adopting it, the plans list shows "New
// plan" for a plan that has a perfectly good name one dialog away.
func TestLaunchDecomposeSubagentAdoptsTheTitleWhenTheRootHasNone(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := decomposeTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: ChatNameToolName,
			Arguments: `{"chat_name":"Migrate Centrifugo","task_id":"DO-236"}`,
		}}},
		{Content: "DO-236: migrate Centrifugo."},
	}})

	if _, err := svc.launchDecomposeSubagent(t.Context(), root, "plan DO-236"); err != nil {
		t.Fatalf("launchDecomposeSubagent err = %v", err)
	}

	got, _ := repo.GetDialog(t.Context(), root)
	if got.Title != "DO-236: Migrate Centrifugo" {
		t.Errorf("root title = %q, want the name the sub-agent chose", got.Title)
	}
	// The task id names the branch a code action pushes to, and only the root is
	// read for it.
	if got.TaskID == nil || *got.TaskID != "DO-236" {
		t.Errorf("root task id = %v, want DO-236", got.TaskID)
	}
}

// A title the operator or the orchestrator already chose is theirs; a sub-agent
// must not overwrite it.
func TestLaunchDecomposeSubagentLeavesAnExistingTitleAlone(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := repo.add(domain.Dialog{Mode: mainDialogMode, Title: "The operator's own name"})
	svc := decomposeTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		{ToolCalls: []llm.ToolCall{{
			ID: "c1", Name: ChatNameToolName, Arguments: `{"chat_name":"Something else"}`,
		}}},
		{Content: "done"},
	}})

	if _, err := svc.launchDecomposeSubagent(t.Context(), root, "plan DO-1"); err != nil {
		t.Fatalf("launchDecomposeSubagent err = %v", err)
	}
	if got, _ := repo.GetDialog(t.Context(), root); got.Title != "The operator's own name" {
		t.Errorf("root title = %q, want it untouched", got.Title)
	}
}

// Only an orchestrator root launches sub-agents, so a sub-agent transcript that
// somehow reached the tool cannot start work against somebody else's plan.
func TestLaunchesRefuseADialogThatIsNotAnOrchestratorRoot(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	child := repo.add(domain.Dialog{Mode: "decompose", ParentID: &root})
	legacy := repo.add(domain.Dialog{Mode: "decompose"})

	svc := decomposeTestService(t, repo, &scriptedProvider{})

	for name, id := range map[string]uuid.UUID{
		"a sub-agent transcript":  child,
		"a legacy decompose root": legacy,
	} {
		if _, err := svc.launchDecomposeSubagent(t.Context(), id, "plan DO-1"); err == nil {
			t.Errorf("%s: launchDecomposeSubagent succeeded, want ErrNotMainDialog", name)
		}
		if _, err := svc.launchPlanSubagent(t.Context(), id, nil, nil); err == nil {
			t.Errorf("%s: launchPlanSubagent succeeded, want ErrNotMainDialog", name)
		}
		if _, err := svc.launchExecuteSubagent(t.Context(), id, "1.1", false); err == nil {
			t.Errorf("%s: launchExecuteSubagent succeeded, want ErrNotMainDialog", name)
		}
	}
}

func decomposeChild(t *testing.T, repo *multiDialogRepo, root uuid.UUID) uuid.UUID {
	t.Helper()

	children, err := repo.ListChildren(t.Context(), root)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	for _, c := range children {
		if c.Mode == decomposeSubagentMode {
			return c.ID
		}
	}
	t.Fatal("the plan has no decompose transcript")
	return uuid.Nil
}
