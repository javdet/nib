package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
)

// The end-to-end shape of a relayed question: the sub-agent suspends, the
// orchestrator asks on its behalf, and the operator's answer goes back to the
// sub-agent rather than to the orchestrator's own turn.
func TestSubmitToolResultResumesTheSubagentTheQuestionCameFrom(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	provider := &scriptedProvider{turns: []llm.AssistantMessage{
		askQuestionTurn("sub-ask-1", "Which cluster?"),
		{Content: "Planned against staging."},
	}}
	svc := decomposeTestService(t, repo, provider)

	res, err := svc.launchDecomposeSubagent(t.Context(), root, "plan DO-1")
	if err != nil {
		t.Fatalf("launch err = %v", err)
	}
	if res.Status != SubagentAwaitingInput {
		t.Fatalf("status = %q, want awaiting_input", res.Status)
	}

	// The orchestrator relays the question under a call id of its own, which is
	// what the operator answers.
	mainAskID, err := svc.appendSyntheticAskQuestion(t.Context(), root,
		"The decompose sub-agent needs a decision.", res.Questions, subagentAskIDPrefix)
	if err != nil {
		t.Fatalf("relay err = %v", err)
	}
	if !svc.bindSubagentPause(root, mainAskID, res.Questions) {
		t.Fatal("the relayed question was not bound to the pause")
	}

	resp, err := svc.SubmitToolResult(t.Context(), root, mainAskID, []string{"staging"})
	if err != nil {
		t.Fatalf("SubmitToolResult err = %v", err)
	}
	if !strings.Contains(resp.Response, "staging") {
		t.Errorf("response = %q, want the sub-agent's finished answer", resp.Response)
	}

	// The answer is written against the sub-agent's own dangling call, with its
	// own wording, not against the orchestrator's relay.
	child := decomposeChild(t, repo, root)
	msgs, _ := repo.ListMessages(t.Context(), child)
	var answer *domain.DialogMessage
	for i := range msgs {
		if msgs[i].Role == "tool" && msgs[i].ToolCallID == "sub-ask-1" {
			answer = &msgs[i]
		}
	}
	if answer == nil {
		t.Fatal("the sub-agent's question was never answered")
	}
	for _, want := range []string{"Which cluster?", "staging"} {
		if !strings.Contains(answer.Content, want) {
			t.Errorf("the answer row %q is missing %q", answer.Content, want)
		}
	}

	// The report lands in the orchestrator's chat, named so a repeat cannot
	// double-post, and says the work is finished so a later turn does not
	// re-launch it.
	rootMsgs, _ := repo.ListMessages(t.Context(), root)
	var report *domain.DialogMessage
	for i := range rootMsgs {
		if strings.HasPrefix(rootMsgs[i].Name, "subagent:decompose#") {
			report = &rootMsgs[i]
		}
	}
	if report == nil {
		t.Fatal("nothing was reported into the orchestrator's chat")
	}
	if !strings.Contains(report.Content, "#dialog:"+child.String()) {
		t.Error("the report does not link the sub-agent's transcript")
	}
	if !strings.Contains(report.Content, "finished") {
		t.Error("the report does not say the work is finished")
	}

	// The pause is spent.
	state, _, _ := svc.ReadSubagentState(root)
	if len(state.Pauses) != 0 {
		t.Errorf("pauses = %d, want the answered one gone", len(state.Pauses))
	}
}

// A sub-agent that asks again is put back to the operator without a round trip
// through the orchestrator's model, and the new pause is bound to the new call.
func TestSubmitToolResultRelaysASecondQuestion(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := decomposeTestService(t, repo, &scriptedProvider{turns: []llm.AssistantMessage{
		askQuestionTurn("sub-ask-1", "Which cluster?"),
		askQuestionTurn("sub-ask-2", "Which namespace?"),
	}})

	res, err := svc.launchDecomposeSubagent(t.Context(), root, "plan DO-1")
	if err != nil {
		t.Fatalf("launch err = %v", err)
	}
	mainAskID, err := svc.appendSyntheticAskQuestion(t.Context(), root, "relay", res.Questions, subagentAskIDPrefix)
	if err != nil {
		t.Fatalf("relay err = %v", err)
	}
	svc.bindSubagentPause(root, mainAskID, res.Questions)

	resp, err := svc.SubmitToolResult(t.Context(), root, mainAskID, []string{"staging"})
	if err != nil {
		t.Fatalf("SubmitToolResult err = %v", err)
	}
	if resp.Status != "awaiting_input" {
		t.Fatalf("status = %q, want awaiting_input for the next question", resp.Status)
	}
	if len(resp.Questions) != 1 || resp.Questions[0].Question != "Which namespace?" {
		t.Fatalf("questions = %+v, want the sub-agent's second question", resp.Questions)
	}
	if !strings.HasPrefix(resp.ToolCallID, subagentAskIDPrefix) {
		t.Errorf("ToolCallID = %q, want a relayed-question id", resp.ToolCallID)
	}

	state, _, _ := svc.ReadSubagentState(root)
	if len(state.Pauses) != 1 {
		t.Fatalf("pauses = %d, want the new one recorded", len(state.Pauses))
	}
	if state.Pauses[0].MainAskID != resp.ToolCallID {
		t.Errorf("pause.MainAskID = %q, want %q", state.Pauses[0].MainAskID, resp.ToolCallID)
	}
	if state.Pauses[0].ToolCallID != "sub-ask-2" {
		t.Errorf("pause.ToolCallID = %q, want the new dangling call", state.Pauses[0].ToolCallID)
	}
}

// A call that is not a relayed question must fall through to the ordinary
// resume, or answering the orchestrator's own question would do nothing.
func TestResumeSubagentFromAnswersIgnoresAnUnrelatedCall(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := decomposeTestService(t, repo, &scriptedProvider{})

	d, _ := repo.GetDialog(t.Context(), root)
	_, resumed, err := svc.resumeSubagentFromAnswers(t.Context(), d, "call_someone_elses", []string{"yes"})
	if err != nil {
		t.Fatalf("resumeSubagentFromAnswers err = %v", err)
	}
	if resumed {
		t.Error("an unrelated call was treated as a relayed question")
	}
}

// The bind happens in the agent loop, at the one point both ids exist. A turn in
// any other mode must not pay for it or claim somebody's pause.
func TestBindSubagentPauseTakesTheOldestUnboundPause(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := decomposeTestService(t, repo, &scriptedProvider{})

	if svc.bindSubagentPause(root, "main-ask-1", nil) {
		t.Error("a bind succeeded with no pause recorded")
	}

	child := uuid.New()
	if err := svc.recordSubagentPause(root, SubagentPause{
		Subagent: SubagentDecompose, DialogID: child.String(), ToolCallID: "sub-ask-1",
	}); err != nil {
		t.Fatalf("recordSubagentPause: %v", err)
	}

	if !svc.bindSubagentPause(root, "main-ask-1", []domain.Question{{Question: "reworded?"}}) {
		t.Fatal("the recorded pause was not bound")
	}
	// A second bind finds nothing unbound: the operator's answer belongs to the
	// call already relayed.
	if svc.bindSubagentPause(root, "main-ask-2", nil) {
		t.Error("an already-bound pause was rebound")
	}

	got, found, err := svc.takeSubagentPause(root, "main-ask-1")
	if err != nil || !found {
		t.Fatalf("takeSubagentPause found = %v, err = %v", found, err)
	}
	if got.ToolCallID != "sub-ask-1" {
		t.Errorf("pause.ToolCallID = %q, want sub-ask-1", got.ToolCallID)
	}
	if len(got.Questions) != 1 || got.Questions[0].Question != "reworded?" {
		t.Errorf("pause.Questions = %+v, want the relayed wording", got.Questions)
	}
	if _, found, _ := svc.takeSubagentPause(root, "main-ask-1"); found {
		t.Error("the pause was taken twice")
	}
}

// The orchestrator asking something of its own must not claim a sub-agent's
// pause: answers are paired positionally, so a question that is not the relay
// would send the operator's answer to the wrong place.
func TestBindSubagentPauseIgnoresAQuestionThatIsNotTheRelay(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	root := mainRoot(repo)
	svc := decomposeTestService(t, repo, &scriptedProvider{})

	if err := svc.recordSubagentPause(root, SubagentPause{
		Subagent:   SubagentDecompose,
		DialogID:   uuid.New().String(),
		ToolCallID: "sub-ask-1",
		Questions:  []domain.Question{{Question: "Which cluster?"}, {Question: "Which namespace?"}},
	}); err != nil {
		t.Fatalf("recordSubagentPause: %v", err)
	}

	// One question against a two-question pause: not a relay.
	if svc.bindSubagentPause(root, "main-own-ask", []domain.Question{{Question: "Shall I go on?"}}) {
		t.Error("a question of the orchestrator's own claimed the pause")
	}
	state, _, _ := svc.ReadSubagentState(root)
	if state.Pauses[0].MainAskID != "" {
		t.Errorf("pause.MainAskID = %q, want it still unbound", state.Pauses[0].MainAskID)
	}

	// The real relay carries the same two questions.
	if !svc.bindSubagentPause(root, "main-relay", []domain.Question{
		{Question: "Which cluster?"}, {Question: "Which namespace?"},
	}) {
		t.Error("the relay did not bind the pause")
	}
}

// A missing state file reads as empty: most plans never suspend a sub-agent, and
// a fresh install has no directory at all.
func TestReadSubagentStateWithNoFile(t *testing.T) {
	t.Parallel()

	svc := decomposeTestService(t, newMultiDialogRepo(), &scriptedProvider{})
	state, found, err := svc.ReadSubagentState(uuid.New())
	if err != nil {
		t.Fatalf("ReadSubagentState err = %v", err)
	}
	if found {
		t.Error("found = true for a plan that never suspended anything")
	}
	if len(state.Pauses) != 0 {
		t.Errorf("pauses = %d, want none", len(state.Pauses))
	}
}
