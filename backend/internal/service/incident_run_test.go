package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
)

// incidentFakeProvider plays every agent of an incident run at once. It is
// stateless -- each answer is decided from the transcript it is handed -- so
// the Diggers can share it while running concurrently.
type incidentFakeProvider struct {
	// silentSystem names a Digger that ends without filing a finding.
	silentSystem string
	// askingSystem names a Digger that asks the operator instead of working.
	askingSystem string
	// block holds every round until it is closed, for tests that stop a run.
	block chan struct{}
	mu    sync.Mutex
	asks  int
}

func (p *incidentFakeProvider) Complete(context.Context, string, string) (string, error) {
	return "", nil
}

func (p *incidentFakeProvider) CompleteWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (llm.AssistantMessage, error) {
	if p.block != nil {
		select {
		case <-p.block:
		case <-ctx.Done():
			return llm.AssistantMessage{}, ctx.Err()
		}
	}
	seed := ""
	for _, m := range messages {
		if m.Role == "user" {
			seed = m.Content
			break
		}
	}
	has := func(name string) bool {
		return slices.ContainsFunc(tools, func(t llm.ToolDef) bool { return t.Name == name })
	}
	call := func(name, args string) llm.AssistantMessage {
		return llm.AssistantMessage{ToolCalls: []llm.ToolCall{{ID: "c-" + uuid.NewString(), Name: name, Arguments: args}}}
	}
	system := between(seed, "Name: ", "\n")

	// The asking Digger asks whatever it was told; everyone else makes one
	// tool call and then closes.
	if has(ReportFindingToolName) && system == p.askingSystem {
		p.mu.Lock()
		p.asks++
		p.mu.Unlock()
		return askQuestionTurn("ask-"+uuid.NewString(), "Which cluster?"), nil
	}
	if last := messages[len(messages)-1]; last.Role == "tool" {
		return llm.AssistantMessage{Content: "finished"}, nil
	}

	switch {
	case has(ReportFindingToolName):
		if system == p.silentSystem {
			return llm.AssistantMessage{Content: "I looked at " + system + " and saw nothing conclusive."}, nil
		}
		return call(ReportFindingToolName, fmt.Sprintf(
			`{"status":"degraded","summary":"%s errors rose","timeline":[{"at":"2026-10-07T14:00:00Z","event":"%s errors begin"}]}`,
			system, system)), nil
	case strings.Contains(seed, "Synthesise the findings"):
		return call(UpdatePostmortemToolName, `{"set":{"hypothesis":"the deploy broke it"},"lists":{"root_causes":[{"text":"bad deploy"}]}}`), nil
	case strings.Contains(seed, "Assess the incident's impact"):
		return call(UpdatePostmortemToolName, `{"set":{"impact":"5% of requests failed"},"lists":{"impacted":[{"name":"api"}]}}`), nil
	case strings.Contains(seed, "Write the closure summary"):
		return call(UpdatePostmortemToolName, `{"set":{"closure":"Closed after rollback."}}`), nil
	}
	return llm.AssistantMessage{Content: "nothing to do"}, nil
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	if j := strings.Index(rest, end); j >= 0 {
		return rest[:j]
	}
	return rest
}

func newIncidentRunTestService(t *testing.T, provider llm.Provider) (*ChatService, *multiDialogRepo, uuid.UUID) {
	t.Helper()
	repo := newMultiDialogRepo()
	svc := &ChatService{
		provider:        provider,
		dialogRepo:      repo,
		mcpSvc:          stubMCPService(),
		postmortemsDir:  t.TempDir(),
		incidentRunsDir: t.TempDir(),
		activity:        NewActivityBroker(),
		maxIterations:   5,
	}
	incident := repo.add(domain.Dialog{Mode: incidentDialogMode, Title: "API errors"})
	return svc, repo, incident
}

func waitForIncidentRun(t *testing.T, svc *ChatService, root uuid.UUID) IncidentRun {
	t.Helper()
	var run IncidentRun
	ok := waitFor(t, func() bool {
		r, found, err := svc.ReadIncidentRun(root)
		if err != nil || !found || r.Active() {
			return false
		}
		if _, live := svc.incidentRuns.Load(root); live {
			return false
		}
		run = r
		return true
	})
	if !ok {
		t.Fatal("incident run did not finish")
	}
	return run
}

func TestStartInvestigation_runsDiggersThenSynthesisAndImpact(t *testing.T) {
	t.Parallel()
	svc, repo, incident := newIncidentRunTestService(t, &incidentFakeProvider{silentSystem: "cache"})
	ctx := context.Background()

	started, err := svc.StartInvestigation(ctx, incident, InvestigationRequest{
		Brief:       "5xx on the API",
		WindowStart: "2026-10-07T13:30:00Z",
		Systems: []IncidentSystem{
			{Name: "api", Environment: "prod", Categories: []string{"metrics"}},
			{Name: "db", Environment: "prod"},
			{Name: "cache"},
		},
	})
	if err != nil {
		t.Fatalf("StartInvestigation() err = %v", err)
	}
	if len(started.Steps) != 5 {
		t.Fatalf("steps = %d, want 3 Diggers + synthesis + impact", len(started.Steps))
	}

	run := waitForIncidentRun(t, svc, incident)
	if run.Status != IncidentRunDone {
		t.Fatalf("run status = %q (%s), want done", run.Status, run.Error)
	}
	for _, st := range run.Steps {
		if st.Status != IncidentStepDone || st.DialogID == "" {
			t.Errorf("step %s/%s = %q dialog %q, want done with a dialog", st.Kind, st.System, st.Status, st.DialogID)
		}
	}

	doc, _ := svc.ReadPostmortem(incident)
	if len(doc.Findings) != 3 {
		t.Fatalf("findings = %+v, want one per system", doc.Findings)
	}
	for _, f := range doc.Findings {
		want := "degraded"
		if f.System == "cache" {
			want = "unknown"
		}
		if f.Status != want || f.DialogID == "" {
			t.Errorf("finding %s = status %q dialog %q, want %s with a transcript", f.System, f.Status, f.DialogID, want)
		}
	}
	if doc.Hypothesis != "the deploy broke it" || doc.Impact != "5% of requests failed" {
		t.Fatalf("hypothesis %q impact %q, want both written", doc.Hypothesis, doc.Impact)
	}
	// The synthesis restated no timeline, so the Diggers' events survive it.
	if len(doc.Timeline) != 2 || doc.Timeline[0].Source == "" {
		t.Fatalf("timeline = %+v, want the two Diggers' events with their sources", doc.Timeline)
	}

	children, _ := repo.ListChildren(ctx, incident)
	if len(children) != 5 {
		t.Fatalf("children = %d, want one per step", len(children))
	}
	for _, c := range children {
		if c.Mode != incidentDialogMode {
			t.Errorf("child %q mode = %q, want incident", c.Title, c.Mode)
		}
		if c.Title == "Digger: api" && !slices.Equal(c.Categories, []string{"metrics"}) {
			t.Errorf("digger categories = %v, want them on its dialog", c.Categories)
		}
	}

	msgs, _ := repo.ListMessages(ctx, incident)
	var reports int
	for _, m := range msgs {
		if strings.HasPrefix(m.Name, "incident-run:") {
			reports++
			if !strings.Contains(m.Content, "3 of 3 systems") {
				t.Errorf("report = %q", m.Content)
			}
		}
	}
	if reports != 1 {
		t.Fatalf("run reports in the chat = %d, want 1", reports)
	}
	svc.reportIncidentRun(ctx, incident, run)
	msgs, _ = repo.ListMessages(ctx, incident)
	if got := len(msgs); got != reports {
		t.Fatalf("a second report was posted: %d messages", got)
	}
}

func TestStartInvestigation_aDiggerThatAsksIsAnsweredOnceThenFails(t *testing.T) {
	t.Parallel()
	provider := &incidentFakeProvider{askingSystem: "api"}
	svc, repo, incident := newIncidentRunTestService(t, provider)

	if _, err := svc.StartInvestigation(context.Background(), incident, InvestigationRequest{
		Brief: "x", Systems: []IncidentSystem{{Name: "api"}},
	}); err != nil {
		t.Fatalf("StartInvestigation() err = %v", err)
	}
	run := waitForIncidentRun(t, svc, incident)

	if run.Steps[0].Status != IncidentStepFailed || !strings.Contains(run.Steps[0].Error, "kept asking") {
		t.Fatalf("digger step = %+v, want failed for asking", run.Steps[0])
	}
	if provider.asks != 2 {
		t.Fatalf("asks = %d, want the first answered and the second ending the step", provider.asks)
	}
	// Both questions are closed, so neither shows as a live question.
	digger, _ := uuid.Parse(run.Steps[0].DialogID)
	if _, _, _, open := findPendingAsk(t, repo, digger); open != 0 {
		t.Fatalf("open questions in the Digger transcript = %d, want 0", open)
	}
	// The siblings carried on.
	if run.Steps[1].Status != IncidentStepDone {
		t.Fatalf("synthesis = %+v, want done", run.Steps[1])
	}
}

func TestStartInvestigation_refusals(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	svc, repo, incident := newIncidentRunTestService(t, &incidentFakeProvider{block: block})
	ctx := context.Background()
	plan := repo.add(domain.Dialog{Mode: mainDialogMode})

	invalid := []InvestigationRequest{
		{Brief: "x"},
		{Brief: "x", Systems: []IncidentSystem{{Name: " "}}},
		{Brief: "x", Systems: []IncidentSystem{{Name: "api"}, {Name: "API"}}},
		{Brief: "x", Systems: make([]IncidentSystem, maxInvestigatedSystems+1)},
	}
	for i, req := range invalid {
		if _, err := svc.StartInvestigation(ctx, incident, req); !errors.Is(err, ErrInvalidInvestigation) {
			t.Errorf("request %d: err = %v, want ErrInvalidInvestigation", i, err)
		}
	}
	if _, err := svc.StartInvestigation(ctx, plan, InvestigationRequest{Brief: "x", Systems: []IncidentSystem{{Name: "api"}}}); !errors.Is(err, ErrNotIncidentDialog) {
		t.Errorf("plan dialog: err = %v, want ErrNotIncidentDialog", err)
	}

	if _, err := svc.StartInvestigation(ctx, incident, InvestigationRequest{Brief: "x", Systems: []IncidentSystem{{Name: "api"}}}); err != nil {
		t.Fatalf("first run err = %v", err)
	}
	if _, err := svc.StartInvestigation(ctx, incident, InvestigationRequest{Brief: "x", Systems: []IncidentSystem{{Name: "db"}}}); !errors.Is(err, ErrInvestigationInProgress) {
		t.Fatalf("second run err = %v, want ErrInvestigationInProgress", err)
	}

	if err := svc.StopInvestigation(ctx, incident); err != nil {
		t.Fatalf("StopInvestigation() err = %v", err)
	}
	run := waitForIncidentRun(t, svc, incident)
	if run.Status != IncidentRunFailed {
		t.Fatalf("stopped run status = %q, want failed", run.Status)
	}
	for _, st := range run.Steps {
		if st.Status != IncidentStepFailed {
			t.Errorf("step %s left %q after a stop", st.Kind, st.Status)
		}
	}
	if err := svc.StopInvestigation(ctx, incident); !errors.Is(err, ErrNoInvestigationRunning) {
		t.Fatalf("stop with nothing running: err = %v", err)
	}
	close(block)

	if _, err := svc.updatePostmortem(incident, func(doc *Postmortem) error {
		doc.Status = IncidentStatusClosed
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartInvestigation(ctx, incident, InvestigationRequest{Brief: "x", Systems: []IncidentSystem{{Name: "api"}}}); !errors.Is(err, ErrIncidentClosed) {
		t.Fatalf("closed incident: err = %v, want ErrIncidentClosed", err)
	}
}

func TestStartIncidentClosure_closesOnlyWhenTheClosureIsWritten(t *testing.T) {
	t.Parallel()
	svc, _, incident := newIncidentRunTestService(t, &incidentFakeProvider{})
	ctx := context.Background()

	if _, err := svc.StartIncidentClosure(ctx, incident, "2026-10-07T15:00:00+00:00"); err != nil {
		t.Fatalf("StartIncidentClosure() err = %v", err)
	}
	run := waitForIncidentRun(t, svc, incident)
	if run.Kind != IncidentRunClosure || run.Status != IncidentRunDone {
		t.Fatalf("run = %s/%s (%s), want a finished closure", run.Kind, run.Status, run.Error)
	}

	doc, _ := svc.ReadPostmortem(incident)
	if doc.Status != IncidentStatusClosed || doc.ClosedAt == "" {
		t.Fatalf("status %q closedAt %q, want closed", doc.Status, doc.ClosedAt)
	}
	if doc.ResolvedAt != "2026-10-07T15:00:00Z" || doc.Closure == "" || doc.Impact == "" {
		t.Fatalf("resolvedAt %q closure %q impact %q", doc.ResolvedAt, doc.Closure, doc.Impact)
	}
	if _, err := svc.StartIncidentClosure(ctx, incident, ""); !errors.Is(err, ErrIncidentClosed) {
		t.Fatalf("second closure err = %v, want ErrIncidentClosed", err)
	}
}

func TestStartIncidentClosure_aSilentWriterLeavesTheIncidentOpen(t *testing.T) {
	t.Parallel()
	// A provider that never calls a tool: every agent ends without writing.
	svc, _, incident := newIncidentRunTestService(t, &scriptedProvider{})

	if _, err := svc.StartIncidentClosure(context.Background(), incident, ""); err != nil {
		t.Fatalf("StartIncidentClosure() err = %v", err)
	}
	run := waitForIncidentRun(t, svc, incident)
	if run.Status != IncidentRunFailed {
		t.Fatalf("run status = %q, want failed", run.Status)
	}
	doc, _ := svc.ReadPostmortem(incident)
	if doc.Status == IncidentStatusClosed {
		t.Fatal("incident closed without a closure summary")
	}
	if doc.ResolvedAt == "" {
		t.Fatal("resolvedAt not defaulted")
	}
}

func TestReconcileStuckIncidentRuns(t *testing.T) {
	t.Parallel()
	svc := &ChatService{incidentRunsDir: t.TempDir()}
	stuck, finished := uuid.New(), uuid.New()

	if err := svc.writeIncidentRun(stuck, IncidentRun{RunID: "a", Status: IncidentRunRunning, Steps: []IncidentStep{
		{Kind: IncidentStepDigger, Status: IncidentStepRunning},
		{Kind: IncidentStepSynthesis, Status: IncidentStepPending},
		{Kind: IncidentStepDigger, Status: IncidentStepDone},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.writeIncidentRun(finished, IncidentRun{RunID: "b", Status: IncidentRunDone}); err != nil {
		t.Fatal(err)
	}

	closed, err := svc.reconcileStuckIncidentRuns()
	if err != nil || closed != 1 {
		t.Fatalf("reconcile = %d, %v; want 1", closed, err)
	}
	run, _, _ := svc.ReadIncidentRun(stuck)
	if run.Status != IncidentRunFailed || run.Error != restartedReason {
		t.Fatalf("run = %+v", run)
	}
	got := []IncidentStepStatus{run.Steps[0].Status, run.Steps[1].Status, run.Steps[2].Status}
	if !slices.Equal(got, []IncidentStepStatus{IncidentStepFailed, IncidentStepFailed, IncidentStepDone}) {
		t.Fatalf("steps = %v", got)
	}
	if run, _, _ := svc.ReadIncidentRun(finished); run.Status != IncidentRunDone {
		t.Fatalf("finished run touched: %+v", run)
	}
}

func TestIncidentWindowText_baseline(t *testing.T) {
	t.Parallel()
	got := incidentWindowText("2026-10-07T14:00:00Z", "2026-10-07T15:00:00Z")
	if !strings.Contains(got, "Baseline for comparison: 2026-10-07T13:00:00Z to 2026-10-07T14:00:00Z") {
		t.Fatalf("window = %q", got)
	}
	if incidentWindowText("", "") != "" {
		t.Fatal("empty window rendered")
	}
	if got := incidentWindowText(time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), ""); !strings.Contains(got, "to now") || !strings.Contains(got, "Baseline") {
		t.Fatalf("open window = %q", got)
	}
}
