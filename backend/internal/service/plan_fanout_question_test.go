package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
	"github.com/javdet/nib/internal/mode"
)

// questionFixture is a service with a live fan-out run over one stage and the
// rollback, plus the root dialog its questions are posted into.
//
// The context it returns is the one the run's cancel func stops, the way a stage
// subagent's agent loop runs under it.
func questionFixture(t *testing.T) (*ChatService, *multiDialogRepo, uuid.UUID, context.Context) {
	t.Helper()

	repo := newMultiDialogRepo()
	planID := repo.add(domain.Dialog{Mode: "main"})

	svc := &ChatService{
		planFanoutDir: t.TempDir(),
		activity:      NewActivityBroker(),
		dialogRepo:    repo,
	}
	if err := svc.writeFanoutRun(planID, FanoutRun{
		RunID:  uuid.NewString(),
		Status: FanoutRunRunning,
		Stages: []FanoutStage{
			{Title: "Deploy ingress", Status: FanoutStageRunning},
			{Title: "Verify", Status: FanoutStageRunning},
			{Title: rollbackStageTitle, Kind: FanoutStageKindRollback, Status: FanoutStageRunning},
		},
	}); err != nil {
		t.Fatalf("write run: %v", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	svc.fanoutRuns.Store(planID, newFanoutRunState(cancel))
	t.Cleanup(cancel)
	return svc, repo, planID, runCtx
}

// pendingAsk returns the one unanswered synthetic question on a dialog, failing
// when there is not exactly one.
func pendingAsk(t *testing.T, repo *multiDialogRepo, dialogID uuid.UUID) (string, string, domain.Question) {
	t.Helper()

	callID, preamble, q, found := findPendingAsk(t, repo, dialogID)
	if found != 1 {
		t.Fatalf("unanswered questions = %d, want exactly 1", found)
	}
	return callID, preamble, q
}

// findPendingAsk is pendingAsk without the assertion, so a poll can use it while
// there is still nothing there.
func findPendingAsk(t *testing.T, repo *multiDialogRepo, dialogID uuid.UUID) (string, string, domain.Question, int) {
	t.Helper()

	msgs, err := repo.ListMessages(context.Background(), dialogID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}

	answered := make(map[string]struct{})
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID != "" {
			answered[m.ToolCallID] = struct{}{}
		}
	}

	var (
		found     int
		callID    string
		preamble  string
		question  domain.Question
		questions []domain.Question
	)
	for _, m := range msgs {
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		calls, parseErr := parseStoredToolCalls(m.ToolCalls)
		if parseErr != nil {
			t.Fatalf("parse tool calls: %v", parseErr)
		}
		for _, tc := range calls {
			if tc.Name != AskQuestionToolName {
				continue
			}
			if _, done := answered[tc.ID]; done {
				continue
			}
			found++
			callID, preamble = tc.ID, m.Content
			if questions, err = parseAskQuestionFromArguments(tc.Arguments); err != nil {
				t.Fatalf("parse questions: %v", err)
			}
			question = questions[0]
		}
	}
	return callID, preamble, question, found
}

// waitForQuestion polls until cond holds, so a test never sleeps for a fixed
// stretch to observe another goroutine.
func waitForQuestion(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func messageCount(t *testing.T, repo *multiDialogRepo, dialogID uuid.UUID) int {
	t.Helper()
	msgs, err := repo.ListMessages(context.Background(), dialogID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	return len(msgs)
}

func stageStatus(t *testing.T, svc *ChatService, planID uuid.UUID, title string) FanoutStage {
	t.Helper()
	run, found, err := svc.ReadFanoutRun(planID)
	if err != nil || !found {
		t.Fatalf("read run: %v (found=%v)", err, found)
	}
	for _, st := range run.Stages {
		if normalizeStageTitle(st.Title) == normalizeStageTitle(title) {
			return st
		}
	}
	t.Fatalf("stage %q not in run", title)
	return FanoutStage{}
}

// The operator has to see which agent is asking: it is one of several planning
// at once, and the question outlives the run in the transcript.
func TestAwaitBlockerAnswer_namesTheStageInThePreambleAndTheQuestion(t *testing.T) {
	t.Parallel()
	svc, repo, planID, runCtx := questionFixture(t)

	b := PlanBlocker{Stage: "Deploy ingress", Question: "Which ingress class?", Assumption: "nginx"}
	done := make(chan string, 1)
	go func() { done <- svc.awaitBlockerAnswer(runCtx, planID, b) }()

	waitForQuestion(t, "the question to be posted", func() bool { return messageCount(t, repo, planID) > 0 })
	_, preamble, q := pendingAsk(t, repo, planID)

	if !strings.Contains(preamble, "Deploy ingress") {
		t.Errorf("preamble = %q, want the stage named", preamble)
	}
	if !strings.HasPrefix(q.Question, "[Deploy ingress] ") {
		t.Errorf("question = %q, want the stage prefixed", q.Question)
	}

	svc.CancelPlanFanout(planID)
	<-done
}

// The rollback agent owns no stage, and its display title reads oddly in a
// sentence, so it is named for what it is.
func TestAwaitBlockerAnswer_labelsTheRollbackAgent(t *testing.T) {
	t.Parallel()
	svc, repo, planID, runCtx := questionFixture(t)

	b := PlanBlocker{
		Stage: rollbackStageTitle, Kind: FanoutStageKindRollback,
		Question: "Restore how?", Assumption: "snapshot",
	}
	done := make(chan string, 1)
	go func() { done <- svc.awaitBlockerAnswer(runCtx, planID, b) }()

	waitForQuestion(t, "the question to be posted", func() bool { return messageCount(t, repo, planID) > 0 })
	_, preamble, q := pendingAsk(t, repo, planID)

	if !strings.Contains(preamble, "rollback") || strings.Contains(preamble, rollbackStageTitle) {
		t.Errorf("preamble = %q, want the agent called the rollback", preamble)
	}
	if !strings.HasPrefix(q.Question, "[rollback] ") {
		t.Errorf("question = %q", q.Question)
	}

	svc.CancelPlanFanout(planID)
	<-done
}

// The answer comes back as the tool's own result, so the same turn carries on
// with it instead of the stage being replanned.
func TestAwaitBlockerAnswer_returnsTheOperatorsAnswer(t *testing.T) {
	t.Parallel()
	svc, repo, planID, runCtx := questionFixture(t)

	b := PlanBlocker{Stage: "Deploy ingress", Question: "Which ingress class?", Assumption: "nginx"}
	done := make(chan string, 1)
	go func() { done <- svc.awaitBlockerAnswer(runCtx, planID, b) }()

	waitForQuestion(t, "the question to be posted", func() bool { return messageCount(t, repo, planID) > 0 })
	callID, _, _ := pendingAsk(t, repo, planID)

	if _, handled := svc.deliverFanoutAnswer(callID, []string{"traefik"}); !handled {
		t.Fatal("the answer was not handled")
	}

	got := <-done
	if !strings.Contains(got, "traefik") {
		t.Errorf("result = %q, want the answer", got)
	}
	if !strings.Contains(got, "nginx") {
		t.Errorf("result = %q, want the superseded assumption named", got)
	}
	if !strings.Contains(got, UpdateActionPlanToolName) {
		t.Errorf("result = %q, want the store tool named", got)
	}

	run, _, _ := svc.ReadFanoutRun(planID)
	if len(run.Blockers) != 0 {
		// awaitBlockerAnswer records against a blocker the handler already
		// stored; this test calls it directly, so there is nothing to update.
		t.Fatalf("blockers = %#v", run.Blockers)
	}
	if st := stageStatus(t, svc, planID, "Deploy ingress"); st.Status != FanoutStageRunning || st.Error != "" {
		t.Errorf("stage = %#v, want it back to running with no tooltip", st)
	}
}

// A waiting stage must read as waiting rather than stalled, and the run it
// belongs to is still very much in progress.
func TestAwaitBlockerAnswer_marksTheStageWaitingWithoutParkingTheRun(t *testing.T) {
	t.Parallel()
	svc, _, planID, runCtx := questionFixture(t)

	b := PlanBlocker{Stage: "Deploy ingress", Question: "Which ingress class?", Assumption: "nginx"}
	done := make(chan string, 1)
	go func() { done <- svc.awaitBlockerAnswer(runCtx, planID, b) }()

	waitForQuestion(t, "the stage to go waiting", func() bool {
		run, found, err := svc.ReadFanoutRun(planID)
		if err != nil || !found {
			return false
		}
		return run.Stages[0].Status == FanoutStageAwaitingInput
	})

	run, _, _ := svc.ReadFanoutRun(planID)
	if !run.Active() {
		t.Error("the run stopped being active while a stage waits, so a second fan-out would be let in")
	}
	if run.Stages[0].Error != waitingForAnswerReason {
		t.Errorf("tooltip = %q, want it to say what the stage is waiting for", run.Stages[0].Error)
	}

	svc.CancelPlanFanout(planID)
	<-done
}

// Stopping a run has to release the stages sitting on a question, and what they
// get back has to be usable: the assumption they stated.
func TestAwaitBlockerAnswer_fallsBackToTheAssumptionWhenTheRunIsStopped(t *testing.T) {
	t.Parallel()
	svc, repo, planID, runCtx := questionFixture(t)

	b := PlanBlocker{Stage: "Deploy ingress", Question: "Which ingress class?", Assumption: "nginx"}
	done := make(chan string, 1)
	go func() { done <- svc.awaitBlockerAnswer(runCtx, planID, b) }()

	waitForQuestion(t, "the question to be posted", func() bool { return messageCount(t, repo, planID) > 0 })
	callID, _, _ := pendingAsk(t, repo, planID)

	if !svc.CancelPlanFanout(planID) {
		t.Fatal("CancelPlanFanout found no run")
	}

	got := <-done
	if !strings.Contains(got, "nginx") || !strings.Contains(got, "No answer came back") {
		t.Errorf("result = %q, want the assumption fallback", got)
	}
	if _, waiting := svc.fanoutWaits.Load(callID); waiting {
		t.Error("the waiter outlived the wait")
	}
}

// The chat panel renders only the newest unanswered question, so two posted at
// once would leave one nobody can ever answer.
func TestAwaitBlockerAnswer_putsOneQuestionAtATime(t *testing.T) {
	t.Parallel()
	svc, repo, planID, runCtx := questionFixture(t)

	first := PlanBlocker{Stage: "Deploy ingress", Question: "Which ingress class?", Assumption: "nginx"}
	second := PlanBlocker{Stage: "Verify", Question: "Which probe?", Assumption: "http"}

	firstDone := make(chan string, 1)
	go func() { firstDone <- svc.awaitBlockerAnswer(runCtx, planID, first) }()
	waitForQuestion(t, "the first question", func() bool { return messageCount(t, repo, planID) > 0 })

	secondDone := make(chan string, 1)
	go func() { secondDone <- svc.awaitBlockerAnswer(runCtx, planID, second) }()

	// Both stages read as waiting, but only one question is on the desk.
	waitForQuestion(t, "the second stage to go waiting", func() bool {
		run, found, err := svc.ReadFanoutRun(planID)
		return err == nil && found && run.Stages[1].Status == FanoutStageAwaitingInput
	})
	callID, _, q := pendingAsk(t, repo, planID)
	if !strings.Contains(q.Question, "ingress class") {
		t.Fatalf("question = %q, want the first one", q.Question)
	}

	// Answering the first is what lets the second be asked. The tool row is
	// what SubmitToolResult writes, and what stops the first being pending.
	if _, err := repo.AppendMessage(context.Background(), planID, domain.DialogMessage{
		Role: "tool", ToolCallID: callID, Name: AskQuestionToolName, Content: "{}",
	}); err != nil {
		t.Fatalf("append tool row: %v", err)
	}
	if _, handled := svc.deliverFanoutAnswer(callID, []string{"traefik"}); !handled {
		t.Fatal("the first answer was not handled")
	}
	<-firstDone

	waitForQuestion(t, "the second question", func() bool {
		_, _, q, found := findPendingAsk(t, repo, planID)
		return found == 1 && strings.Contains(q.Question, "probe")
	})

	nextID, _, _ := pendingAsk(t, repo, planID)
	if _, handled := svc.deliverFanoutAnswer(nextID, []string{"tcp"}); !handled {
		t.Fatal("the second answer was not handled")
	}
	if got := <-secondDone; !strings.Contains(got, "tcp") {
		t.Errorf("second result = %q", got)
	}
}

// A question nothing is waiting on any more -- a restart, or a wait that timed
// out -- must be absorbed rather than left to start an orchestrator turn that
// narrates it back at the operator.
func TestDeliverFanoutAnswer_absorbsAQuestionNothingIsWaitingOn(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}

	resp, handled := svc.deliverFanoutAnswer(fanoutAskIDPrefix+uuid.NewString(), []string{"yes"})
	if !handled {
		t.Fatal("an orphaned fan-out question was left to the ordinary resume path")
	}
	if !strings.Contains(resp.Response, "Nothing is waiting") {
		t.Errorf("response = %q", resp.Response)
	}

	if _, handled := svc.deliverFanoutAnswer("call_other", []string{"yes"}); handled {
		t.Error("a call that is not a fan-out question was claimed")
	}
}

// The second delivery for one call finds no waiter, and must not block or panic
// on the channel the first one drained.
func TestDeliverFanoutAnswer_deliversOnce(t *testing.T) {
	t.Parallel()
	svc, repo, planID, runCtx := questionFixture(t)

	b := PlanBlocker{Stage: "Deploy ingress", Question: "Which ingress class?", Assumption: "nginx"}
	done := make(chan string, 1)
	go func() { done <- svc.awaitBlockerAnswer(runCtx, planID, b) }()
	waitForQuestion(t, "the question to be posted", func() bool { return messageCount(t, repo, planID) > 0 })
	callID, _, _ := pendingAsk(t, repo, planID)

	if _, handled := svc.deliverFanoutAnswer(callID, []string{"traefik"}); !handled {
		t.Fatal("the first answer was not handled")
	}
	<-done

	resp, handled := svc.deliverFanoutAnswer(callID, []string{"again"})
	if !handled || !strings.Contains(resp.Response, "Nothing is waiting") {
		t.Errorf("second delivery = %#v, handled=%v", resp, handled)
	}
}

// An operator's thinking time is not the run's to spend: a stage waiting on an
// answer must not burn the budget the run's own work is measured against.
func TestWatchFanoutBudget_doesNotSpendBudgetWhileAStageWaits(t *testing.T) {
	t.Parallel()

	svc := &ChatService{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st := newFanoutRunState(cancel)
	st.waiting.Add(1)

	go svc.watchFanoutBudget(ctx, st, 5*time.Millisecond, time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("the budget ran down while a stage was waiting on the operator")
	}

	st.waiting.Add(-1)
	waitForQuestion(t, "the budget to run out once nothing is waiting", func() bool { return ctx.Err() != nil })
}

// The run's blocker list is the plan's record of what was asked, so an answer
// belongs on the row that raised it.
func TestRecordBlockerAnswer_landsOnTheBlockerThatAsked(t *testing.T) {
	t.Parallel()
	svc, _, planID, _ := questionFixture(t)

	mine := PlanBlocker{Stage: "Deploy ingress", Question: "Which ingress class?", Assumption: "nginx"}
	if _, err := svc.updateFanoutRun(planID, func(run *FanoutRun) {
		run.Blockers = []PlanBlocker{
			{Stage: "Verify", Question: "Which ingress class?", Assumption: "nginx"},
			mine,
		}
	}); err != nil {
		t.Fatalf("seed blockers: %v", err)
	}

	svc.recordBlockerAnswer(planID, mine, "traefik")

	run, _, _ := svc.ReadFanoutRun(planID)
	if run.Blockers[0].Answer != "" {
		t.Errorf("another stage's identical question was answered too: %#v", run.Blockers[0])
	}
	if run.Blockers[1].Answer != "traefik" {
		t.Errorf("blocker = %#v, want the answer", run.Blockers[1])
	}
}

// stagePlannerService wires just enough of ChatService to run one stage
// subagent's turn against a temp data directory.
func stagePlannerService(t *testing.T, repo *multiDialogRepo, provider llm.Provider) *ChatService {
	t.Helper()

	dir := t.TempDir()
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
		planFanoutDir:  filepath.Join(dir, "plan_fanout"),
		fanout:         PlanFanoutConfig{}.withDefaults(),
		maxIterations:  4,
		activity:       NewActivityBroker(),
	}
}

// ask_question is intercepted by name whether or not it is in the catalog, so a
// planner that hallucinates it stops with a nil error. Marking that stage done
// would ship a plan with a stage nothing wrote.
func TestRunPlanStage_failsWhenThePlannerAsksTheOperatorDirectly(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	planID := repo.add(domain.Dialog{Mode: mainDialogMode})
	provider := &scriptedProvider{turns: []llm.AssistantMessage{
		askQuestionTurn("c1", "Which ingress class?"),
	}}

	svc := stagePlannerService(t, repo, provider)
	if err := svc.writeFanoutRun(planID, FanoutRun{
		RunID:  uuid.NewString(),
		Status: FanoutRunRunning,
		Stages: []FanoutStage{{Title: "Deploy ingress", Status: FanoutStagePending}},
	}); err != nil {
		t.Fatalf("write run: %v", err)
	}

	svc.runPlanStage(context.Background(), planID, "Deploy ingress", 0)

	st := stageStatus(t, svc, planID, "Deploy ingress")
	if st.Status != FanoutStageFailed {
		t.Fatalf("stage = %#v, want failed rather than a silently empty done", st)
	}
	if st.Error != askedDirectlyReason {
		t.Errorf("error = %q, want it to name what the planner should have called", st.Error)
	}

	// The dangling call is answered so a later replay of that transcript is not
	// left repairing an orphan, and so the operator cannot answer a question
	// nothing is listening for.
	stageMsgs, err := repo.ListMessages(context.Background(), uuid.MustParse(st.DialogID))
	if err != nil {
		t.Fatalf("list stage messages: %v", err)
	}
	if !toolResultExists(stageMsgs, "c1") {
		t.Error("the dangling ask_question was left unanswered")
	}
}

// The tool is the only way a stage planner reaches the operator, so a question
// it raises has to come back with the answer rather than an assumption.
func TestReportBlockerHandler_waitsAndReturnsTheAnswer(t *testing.T) {
	t.Parallel()
	svc, repo, planID, runCtx := questionFixture(t)

	handler := svc.reportBlockerHandler(planID, "Deploy ingress", FanoutStageKindStage)
	done := make(chan string, 1)
	go func() {
		out, err := handler(runCtx, map[string]any{
			"question":   "Which ingress class?",
			"options":    []any{"nginx", "traefik"},
			"assumption": "nginx",
		})
		if err != nil {
			t.Errorf("handler err = %v", err)
		}
		done <- out
	}()

	waitForQuestion(t, "the question to be posted", func() bool { return messageCount(t, repo, planID) > 0 })
	callID, _, q := pendingAsk(t, repo, planID)
	if len(q.Options) != 2 {
		t.Errorf("options = %v, want both carried through", q.Options)
	}

	if _, handled := svc.deliverFanoutAnswer(callID, []string{"traefik"}); !handled {
		t.Fatal("the answer was not handled")
	}
	if got := <-done; !strings.Contains(got, "traefik") {
		t.Errorf("tool result = %q", got)
	}

	// The blocker is the plan's record of what was asked and what came back.
	run, _, _ := svc.ReadFanoutRun(planID)
	if len(run.Blockers) != 1 || run.Blockers[0].Answer != "traefik" {
		t.Fatalf("blockers = %#v", run.Blockers)
	}
}

// One confused planner must not be able to hold the operator's chat hostage.
func TestReportBlockerHandler_stopsAskingPastTheCap(t *testing.T) {
	t.Parallel()
	svc, repo, planID, runCtx := questionFixture(t)

	handler := svc.reportBlockerHandler(planID, "Deploy ingress", FanoutStageKindStage)
	for i := range maxBlockersPerStage {
		question := fmt.Sprintf("Question %d?", i)
		done := make(chan string, 1)
		go func() {
			out, _ := handler(runCtx, map[string]any{"question": question, "assumption": "a guess"})
			done <- out
		}()
		waitForQuestion(t, "question "+question, func() bool {
			_, _, q, found := findPendingAsk(t, repo, planID)
			return found == 1 && strings.Contains(q.Question, question)
		})
		callID, _, _ := pendingAsk(t, repo, planID)
		if _, err := repo.AppendMessage(context.Background(), planID, domain.DialogMessage{
			Role: "tool", ToolCallID: callID, Name: AskQuestionToolName, Content: "{}",
		}); err != nil {
			t.Fatalf("append tool row: %v", err)
		}
		svc.deliverFanoutAnswer(callID, []string{"an answer"})
		<-done
	}

	before := messageCount(t, repo, planID)
	out, err := handler(runCtx, map[string]any{"question": "One more?", "assumption": "a guess"})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if !strings.Contains(out, "No answer came back") {
		t.Errorf("tool result = %q, want the assumption fallback", out)
	}
	if messageCount(t, repo, planID) != before {
		t.Error("a question past the cap was still put to the operator")
	}
}

// A retried round asking the same thing again must not put it to the operator
// twice.
func TestReportBlockerHandler_reusesAnAnswerItAlreadyHas(t *testing.T) {
	t.Parallel()
	svc, _, planID, runCtx := questionFixture(t)

	if _, err := svc.updateFanoutRun(planID, func(run *FanoutRun) {
		run.Blockers = []PlanBlocker{{
			Stage: "Deploy ingress", Question: "Which ingress class?",
			Assumption: "nginx", Answer: "traefik",
		}}
	}); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}

	handler := svc.reportBlockerHandler(planID, "Deploy ingress", FanoutStageKindStage)
	out, err := handler(runCtx, map[string]any{
		"question": "which  INGRESS class?", "assumption": "nginx",
	})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if !strings.Contains(out, "traefik") {
		t.Errorf("tool result = %q, want the answer already on record", out)
	}

	run, _, _ := svc.ReadFanoutRun(planID)
	if len(run.Blockers) != 1 {
		t.Fatalf("blockers = %#v, want the question recorded once", run.Blockers)
	}
}

// A question that already went unanswered once is not put to the operator a
// second time, and must not come back as an answer of empty text.
func TestReportBlockerHandler_doesNotReraiseAQuestionThatWentUnanswered(t *testing.T) {
	t.Parallel()
	svc, repo, planID, runCtx := questionFixture(t)

	if _, err := svc.updateFanoutRun(planID, func(run *FanoutRun) {
		run.Blockers = []PlanBlocker{{
			Stage: "Deploy ingress", Question: "Which ingress class?", Assumption: "nginx",
		}}
	}); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}

	handler := svc.reportBlockerHandler(planID, "Deploy ingress", FanoutStageKindStage)
	out, err := handler(runCtx, map[string]any{
		"question": "Which ingress class?", "assumption": "nginx",
	})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if !strings.Contains(out, "No answer came back") {
		t.Errorf("tool result = %q, want the assumption fallback", out)
	}
	if n := messageCount(t, repo, planID); n != 0 {
		t.Errorf("posted %d messages, want the question not re-raised", n)
	}
}
