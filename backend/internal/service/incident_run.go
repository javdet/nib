package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/domain"
)

var (
	// ErrInvestigationInProgress refuses a second investigation of an incident
	// while one is running.
	ErrInvestigationInProgress = errors.New("an investigation of this incident is already running")
	// ErrIncidentClosed refuses work on an incident that has been closed.
	ErrIncidentClosed = errors.New("the incident is closed")
	// ErrNoInvestigationRunning is returned when there is no run to stop.
	ErrNoInvestigationRunning = errors.New("no investigation of this incident is running")
	// ErrInvalidInvestigation wraps what is wrong with a requested run, in
	// words written for the model that asked for it.
	ErrInvalidInvestigation = errors.New("invalid investigation")
)

// maxInvestigatedSystems caps the Diggers of one run. Each is a full agent
// with its own budget, so an unbounded list is an unbounded bill.
const maxInvestigatedSystems = 8

// IncidentConfig tunes an incident's investigation runs.
type IncidentConfig struct {
	// Concurrency caps how many Diggers investigate at once.
	Concurrency int
	// DiggerMaxIterations is the completion round budget of one Digger.
	DiggerMaxIterations int
	// StepMaxIterations is the budget of the synthesis, impact and closure agents.
	StepMaxIterations int
	// TimeoutMinutes bounds one whole run. Nothing in it waits on the operator,
	// so a plain deadline does, where a fan-out needs a working-time budget.
	TimeoutMinutes int
}

func (c IncidentConfig) withDefaults() IncidentConfig {
	if c.Concurrency <= 0 {
		c.Concurrency = 4
	}
	if c.DiggerMaxIterations <= 0 {
		c.DiggerMaxIterations = 25
	}
	if c.StepMaxIterations <= 0 {
		c.StepMaxIterations = 15
	}
	if c.TimeoutMinutes <= 0 {
		c.TimeoutMinutes = 30
	}
	return c
}

// SetIncidentConfig replaces the investigation budgets. It is a setter rather
// than a NewChatService argument for the reason SetKBSettings is.
func (s *ChatService) SetIncidentConfig(cfg IncidentConfig) {
	s.incident = cfg.withDefaults()
}

func (s *ChatService) incidentConfig() IncidentConfig {
	return s.incident.withDefaults()
}

type IncidentRunKind string

const (
	IncidentRunInvestigation IncidentRunKind = "investigation"
	IncidentRunClosure       IncidentRunKind = "closure"
)

type IncidentRunStatus string

const (
	IncidentRunRunning IncidentRunStatus = "running"
	IncidentRunDone    IncidentRunStatus = "done"
	IncidentRunFailed  IncidentRunStatus = "failed"
)

type IncidentStepKind string

const (
	IncidentStepDigger    IncidentStepKind = "digger"
	IncidentStepSynthesis IncidentStepKind = "synthesis"
	IncidentStepImpact    IncidentStepKind = "impact"
	IncidentStepClosure   IncidentStepKind = "closure"
)

type IncidentStepStatus string

const (
	IncidentStepPending IncidentStepStatus = "pending"
	IncidentStepRunning IncidentStepStatus = "running"
	IncidentStepDone    IncidentStepStatus = "done"
	IncidentStepFailed  IncidentStepStatus = "failed"
)

// IncidentSystem is one system the orchestrator hands to a Digger.
type IncidentSystem struct {
	Name        string   `json:"name"`
	Environment string   `json:"environment,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	Focus       string   `json:"focus,omitempty"`
}

// IncidentStep is one agent of a run.
type IncidentStep struct {
	Kind        IncidentStepKind   `json:"kind"`
	System      string             `json:"system,omitempty"`
	Environment string             `json:"environment,omitempty"`
	Categories  []string           `json:"categories,omitempty"`
	Focus       string             `json:"focus,omitempty"`
	Status      IncidentStepStatus `json:"status"`
	DialogID    string             `json:"dialogId,omitempty"`
	Error       string             `json:"error,omitempty"`
	StartedAt   int64              `json:"startedAt,omitempty"`
	FinishedAt  int64              `json:"finishedAt,omitempty"`
}

// IncidentRun is the latest investigation or closure of an incident. Only the
// latest is kept: what each system showed survives in the post-mortem's
// findings, which link to the Digger transcripts.
type IncidentRun struct {
	RunID       string            `json:"runId"`
	Kind        IncidentRunKind   `json:"kind"`
	Status      IncidentRunStatus `json:"status"`
	StartedAt   int64             `json:"startedAt"`
	FinishedAt  int64             `json:"finishedAt,omitempty"`
	Brief       string            `json:"brief"`
	WindowStart string            `json:"windowStart,omitempty"`
	WindowEnd   string            `json:"windowEnd,omitempty"`
	Steps       []IncidentStep    `json:"steps"`
	Error       string            `json:"error,omitempty"`
}

// Active reports whether the run is still going.
func (r IncidentRun) Active() bool { return r.Status == IncidentRunRunning }

// InvestigationRequest is what run_investigation asks for.
type InvestigationRequest struct {
	Brief       string
	WindowStart string
	WindowEnd   string
	Systems     []IncidentSystem
	// ImpactOnly reruns the impact assessment alone, over findings already made.
	ImpactOnly bool
}

func (s *ChatService) incidentRunPath(rootID uuid.UUID) string {
	return filepath.Join(s.incidentRunsDir, rootID.String()+".json")
}

// ReadIncidentRun returns the incident's latest run, or false when it has had none.
func (s *ChatService) ReadIncidentRun(rootID uuid.UUID) (IncidentRun, bool, error) {
	b, err := os.ReadFile(s.incidentRunPath(rootID))
	if errors.Is(err, os.ErrNotExist) {
		return IncidentRun{}, false, nil
	}
	if err != nil {
		return IncidentRun{}, false, fmt.Errorf("read incident run: %w", err)
	}
	var run IncidentRun
	if err := json.Unmarshal(b, &run); err != nil {
		return IncidentRun{}, false, fmt.Errorf("decode incident run: %w", err)
	}
	return run, true, nil
}

func (s *ChatService) writeIncidentRun(rootID uuid.UUID, run IncidentRun) error {
	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.incidentRunsDir, 0o755); err != nil {
		return fmt.Errorf("create incident runs directory: %w", err)
	}
	return atomicfile.Write(s.incidentRunPath(rootID), b)
}

// updateIncidentRun is the read-modify-write of the run record. The steps of a
// wave finish concurrently and each records its own outcome.
func (s *ChatService) updateIncidentRun(rootID uuid.UUID, mutate func(*IncidentRun)) (IncidentRun, error) {
	mu, _ := s.incidentRunWriteMu.LoadOrStore(rootID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	run, _, err := s.ReadIncidentRun(rootID)
	if err != nil {
		return IncidentRun{}, err
	}
	mutate(&run)
	if err := s.writeIncidentRun(rootID, run); err != nil {
		return IncidentRun{}, err
	}
	return run, nil
}

func (s *ChatService) setIncidentStep(rootID uuid.UUID, idx int, mutate func(*IncidentStep)) {
	_, err := s.updateIncidentRun(rootID, func(run *IncidentRun) {
		if idx >= 0 && idx < len(run.Steps) {
			mutate(&run.Steps[idx])
		}
	})
	if err != nil {
		slog.Warn("incident run: update step", "dialog_id", rootID, "step", idx, "error", err)
	}
	s.activity.Publish(rootID, domain.AgentActivity{Kind: domain.ActivityInvestigationUpdated})
}

// validateSystems normalises the orchestrator's list of systems. The error text
// goes back to the model, so it says what to change.
func validateSystems(systems []IncidentSystem) ([]IncidentSystem, error) {
	out := make([]IncidentSystem, 0, len(systems))
	seen := make(map[string]struct{}, len(systems))
	for _, sys := range systems {
		sys.Name = strings.TrimSpace(sys.Name)
		sys.Environment = strings.TrimSpace(sys.Environment)
		sys.Focus = strings.TrimSpace(sys.Focus)
		sys.Categories = uniqueNonEmpty(sys.Categories)
		if sys.Name == "" {
			return nil, fmt.Errorf("%w: every system needs a name", ErrInvalidInvestigation)
		}
		key := pmKey(sys.Name, sys.Environment)
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("%w: system %q is listed twice", ErrInvalidInvestigation, sys.Name)
		}
		seen[key] = struct{}{}
		out = append(out, sys)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: name at least one system to investigate", ErrInvalidInvestigation)
	}
	if len(out) > maxInvestigatedSystems {
		return nil, fmt.Errorf("%w: at most %d systems per investigation; pick the ones the evidence points at", ErrInvalidInvestigation, maxInvestigatedSystems)
	}
	return out, nil
}

// incidentRunState is the live half of a run, which dies with the process.
type incidentRunState struct {
	cancel context.CancelFunc
}

// StartInvestigation records a run and launches it in the background: Diggers
// first, one per system, then the synthesis and impact agents side by side over
// what they found. It returns as soon as the run is recorded.
func (s *ChatService) StartInvestigation(ctx context.Context, rootID uuid.UUID, req InvestigationRequest) (IncidentRun, error) {
	if _, err := s.resolveIncidentRoot(ctx, rootID); err != nil {
		return IncidentRun{}, err
	}
	if err := s.refuseBusyOrClosed(rootID); err != nil {
		return IncidentRun{}, err
	}

	var steps []IncidentStep
	if req.ImpactOnly {
		steps = []IncidentStep{{Kind: IncidentStepImpact, Categories: unionCategories(req.Systems), Status: IncidentStepPending}}
	} else {
		systems, err := validateSystems(req.Systems)
		if err != nil {
			return IncidentRun{}, err
		}
		for _, sys := range systems {
			steps = append(steps, IncidentStep{
				Kind: IncidentStepDigger, System: sys.Name, Environment: sys.Environment,
				Categories: sys.Categories, Focus: sys.Focus, Status: IncidentStepPending,
			})
		}
		steps = append(steps,
			IncidentStep{Kind: IncidentStepSynthesis, Status: IncidentStepPending},
			IncidentStep{Kind: IncidentStepImpact, Categories: unionCategories(systems), Status: IncidentStepPending},
		)
	}

	run := IncidentRun{
		RunID:       uuid.NewString(),
		Kind:        IncidentRunInvestigation,
		Status:      IncidentRunRunning,
		StartedAt:   time.Now().Unix(),
		Brief:       strings.TrimSpace(req.Brief),
		WindowStart: normalizeIncidentTime(req.WindowStart),
		WindowEnd:   normalizeIncidentTime(req.WindowEnd),
		Steps:       steps,
	}
	if err := s.launchIncidentRun(ctx, rootID, run); err != nil {
		return IncidentRun{}, err
	}
	return run, nil
}

// refuseBusyOrClosed is the guard every run starts behind.
func (s *ChatService) refuseBusyOrClosed(rootID uuid.UUID) error {
	if _, live := s.incidentRuns.Load(rootID); live {
		return ErrInvestigationInProgress
	}
	previous, _, err := s.ReadIncidentRun(rootID)
	if err != nil {
		return err
	}
	if previous.Active() {
		return ErrInvestigationInProgress
	}
	doc, err := s.ReadPostmortem(rootID)
	if err != nil {
		return err
	}
	if doc.Status == IncidentStatusClosed {
		return ErrIncidentClosed
	}
	return nil
}

// launchIncidentRun writes the run and starts it, detached from the request
// that asked for it.
func (s *ChatService) launchIncidentRun(ctx context.Context, rootID uuid.UUID, run IncidentRun) error {
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(s.incidentConfig().TimeoutMinutes)*time.Minute)
	st := &incidentRunState{cancel: cancel}
	// Claimed before the record is written, so two concurrent starts cannot
	// both pass the guard and launch two runs over one post-mortem.
	if _, taken := s.incidentRuns.LoadOrStore(rootID, st); taken {
		cancel()
		return ErrInvestigationInProgress
	}
	if err := s.writeIncidentRun(rootID, run); err != nil {
		s.incidentRuns.CompareAndDelete(rootID, st)
		cancel()
		return fmt.Errorf("start incident run: %w", err)
	}
	s.activity.Publish(rootID, domain.AgentActivity{Kind: domain.ActivityInvestigationUpdated})

	go func() {
		defer cancel()
		defer s.incidentRuns.CompareAndDelete(rootID, st)
		s.runIncident(runCtx, rootID, run)
	}()
	slog.Info("incident run started", "dialog_id", rootID, "run_id", run.RunID, "kind", run.Kind, "steps", len(run.Steps))
	return nil
}

// CancelInvestigation stops a running investigation or closure. Whatever the
// agents already wrote into the post-mortem stays.
func (s *ChatService) CancelInvestigation(rootID uuid.UUID) bool {
	v, ok := s.incidentRuns.Load(rootID)
	if !ok {
		return false
	}
	v.(*incidentRunState).cancel()
	return true
}

// StopInvestigation is CancelInvestigation for the HTTP API.
func (s *ChatService) StopInvestigation(ctx context.Context, rootID uuid.UUID) error {
	if _, err := s.resolveIncidentRoot(ctx, rootID); err != nil {
		return err
	}
	if !s.CancelInvestigation(rootID) {
		return ErrNoInvestigationRunning
	}
	return nil
}

// IncidentRunForDialog serves the latest run to the web interface.
func (s *ChatService) IncidentRunForDialog(ctx context.Context, rootID uuid.UUID) (IncidentRun, bool, error) {
	if _, err := s.resolveIncidentRoot(ctx, rootID); err != nil {
		return IncidentRun{}, false, err
	}
	return s.ReadIncidentRun(rootID)
}

func unionCategories(systems []IncidentSystem) []string {
	var all []string
	for _, sys := range systems {
		all = append(all, sys.Categories...)
	}
	return uniqueNonEmpty(all)
}

// runIncident runs the steps in their waves: every Digger, then everything
// that reads their findings. A failed step is recorded and its siblings carry
// on, since a post-mortem with one system missing is worth more than none.
func (s *ChatService) runIncident(ctx context.Context, rootID uuid.UUID, run IncidentRun) {
	var diggers, after []int
	for i, st := range run.Steps {
		if st.Kind == IncidentStepDigger {
			diggers = append(diggers, i)
		} else {
			after = append(after, i)
		}
	}

	waves := [][]int{diggers, after}
	if run.Kind == IncidentRunClosure {
		// The closure writer reads the final impact, so each step waits for
		// the one before it.
		waves = waves[:0]
		for _, i := range after {
			waves = append(waves, []int{i})
		}
	}

	sem := make(chan struct{}, s.incidentConfig().Concurrency)
	for _, wave := range waves {
		if ctx.Err() != nil {
			break
		}
		var wg sync.WaitGroup
		for _, idx := range wave {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				defer func() { <-sem }()
				s.runIncidentStep(ctx, rootID, run, idx)
			}(idx)
		}
		wg.Wait()
	}

	status, reason := IncidentRunDone, ""
	if ctx.Err() != nil {
		status, reason = IncidentRunFailed, "stopped, or out of time"
	}
	s.closeIncidentRun(rootID, run.RunID, status, reason)
}

// closeIncidentRun records how the run ended and reports it into the
// incident's chat. A closure that ended well also closes the incident.
func (s *ChatService) closeIncidentRun(rootID uuid.UUID, runID string, status IncidentRunStatus, reason string) {
	now := time.Now().Unix()
	run, err := s.updateIncidentRun(rootID, func(run *IncidentRun) {
		if run.RunID != runID {
			return
		}
		run.Status = status
		run.FinishedAt = now
		run.Error = reason
		for i := range run.Steps {
			if run.Steps[i].Status == IncidentStepPending || run.Steps[i].Status == IncidentStepRunning {
				run.Steps[i].Status = IncidentStepFailed
				run.Steps[i].FinishedAt = now
				if run.Steps[i].Error == "" {
					run.Steps[i].Error = "did not finish: " + reason
				}
			}
		}
	})
	if err != nil {
		slog.Error("incident run: close", "dialog_id", rootID, "run_id", runID, "error", err)
		return
	}
	if run.RunID != runID {
		return
	}

	if run.Kind == IncidentRunClosure {
		s.finishIncidentClosure(rootID, &run)
		if run.Status != status {
			final := run
			if _, err := s.updateIncidentRun(rootID, func(r *IncidentRun) {
				if r.RunID == runID {
					r.Status, r.Error = final.Status, final.Error
				}
			}); err != nil {
				slog.Warn("incident run: record closure outcome", "dialog_id", rootID, "error", err)
			}
		}
	}

	s.reportIncidentRun(context.Background(), rootID, run)
	s.activity.Publish(rootID, domain.AgentActivity{Kind: domain.ActivityInvestigationDone, Status: string(run.Status)})
	slog.Info("incident run finished", "dialog_id", rootID, "run_id", runID, "status", run.Status)
}

// reportIncidentRun posts the outcome into the incident's chat. It does not
// start a turn -- nothing would be waiting on one -- so it tells the model
// reading it later where the result lives. Named after the run, so a close
// that happens twice cannot double-post.
func (s *ChatService) reportIncidentRun(ctx context.Context, rootID uuid.UUID, run IncidentRun) {
	if s.dialogRepo == nil || run.RunID == "" {
		return
	}
	name := "incident-run:" + run.RunID
	msgs, err := s.dialogRepo.ListMessages(ctx, rootID)
	if err != nil {
		slog.Warn("incident run: list messages before reporting", "dialog_id", rootID, "error", err)
		return
	}
	for _, msg := range msgs {
		if msg.Name == name {
			return
		}
	}

	if _, err := s.appendMessageLocked(ctx, rootID, domain.DialogMessage{
		Role:    "assistant",
		Content: incidentRunReport(run),
		Name:    name,
	}); err != nil {
		slog.Warn("incident run: report result", "dialog_id", rootID, "error", err)
	}
}

func incidentRunReport(run IncidentRun) string {
	var diggers, diggersDone int
	var failed []string
	for _, st := range run.Steps {
		if st.Kind == IncidentStepDigger {
			diggers++
			if st.Status == IncidentStepDone {
				diggersDone++
			}
		}
		if st.Status == IncidentStepFailed {
			label := string(st.Kind)
			if st.System != "" {
				label = st.System
			}
			if st.Error != "" {
				label += " (" + st.Error + ")"
			}
			failed = append(failed, label)
		}
	}

	var b strings.Builder
	switch {
	case run.Kind == IncidentRunClosure && run.Status == IncidentRunDone && len(failed) == 0:
		b.WriteString("**Incident closed.** The closure summary and the final impact report are in the post-mortem.")
	case run.Kind == IncidentRunClosure:
		b.WriteString("**Closing the incident did not finish.** The incident stays open; Close incident can be pressed again.")
	case run.Status == IncidentRunFailed:
		fmt.Fprintf(&b, "**Investigation stopped** after %d of %d systems.", diggersDone, diggers)
	case diggers == 0:
		b.WriteString("**Impact reassessed.** The impact report in the post-mortem is current.")
	default:
		fmt.Fprintf(&b, "**Investigation finished** — %d of %d systems examined; the hypothesis, root causes, timeline, corrective actions and impact are in the post-mortem.", diggersDone, diggers)
	}
	if run.Error != "" {
		b.WriteString(" " + capitalize(run.Error) + ".")
	}
	if len(failed) > 0 {
		b.WriteString("\n\nDid not finish: " + strings.Join(failed, "; ") + ".")
	}
	if run.Kind == IncidentRunInvestigation {
		b.WriteString("\n\nThis run is over — do not start it again unless the operator asks or the evidence points at a system not yet examined. Read `get_postmortem` before answering follow-ups.")
	}
	return b.String()
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// runIncidentStep runs one agent of a run in a dialog of its own, parented to
// the incident. Every failure is recorded against the run rather than returned.
func (s *ChatService) runIncidentStep(ctx context.Context, rootID uuid.UUID, run IncidentRun, idx int) {
	step := run.Steps[idx]
	fail := func(err error) {
		slog.Error("incident run: step failed", "dialog_id", rootID, "kind", step.Kind, "system", step.System, "error", err)
		s.setIncidentStep(rootID, idx, func(st *IncidentStep) {
			st.Status = IncidentStepFailed
			st.Error = err.Error()
			st.FinishedAt = time.Now().Unix()
		})
	}

	spec := incidentStepSpecFor(step.Kind)
	cfg := s.incidentConfig()
	maxIterations := cfg.StepMaxIterations
	if step.Kind == IncidentStepDigger {
		maxIterations = cfg.DiggerMaxIterations
	}

	title := spec.title
	if step.System != "" {
		title += ": " + step.System
	}
	child, err := s.dialogRepo.CreateDialog(ctx, incidentDialogMode, title, &rootID)
	if err != nil {
		fail(fmt.Errorf("create step dialog: %w", err))
		return
	}
	// Kept on the dialog so a follow-up the operator types into this transcript
	// gets the same tools back.
	if len(step.Categories) > 0 {
		if err := s.dialogRepo.SetDialogCategories(ctx, child.ID, step.Categories); err != nil {
			slog.Warn("incident run: record step categories", "dialog_id", child.ID, "error", err)
		}
	}
	s.setIncidentStep(rootID, idx, func(st *IncidentStep) {
		st.Status = IncidentStepRunning
		st.DialogID = child.ID.String()
		st.StartedAt = time.Now().Unix()
	})

	sysPrompt, err := s.incidentStepPrompt(ctx, spec.overlay)
	if err != nil {
		fail(err)
		return
	}
	doc, err := s.ReadPostmortem(rootID)
	if err != nil {
		fail(err)
		return
	}
	for _, msg := range []domain.DialogMessage{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: buildIncidentStepSeed(run, step, doc)},
	} {
		if _, err := s.dialogRepo.AppendMessage(ctx, child.ID, msg); err != nil {
			fail(fmt.Errorf("append %s: %w", msg.Role, err))
			return
		}
	}

	allow := s.incidentStepAllowSet(ctx, step.Kind, step.Categories)
	catalog, err := s.buildToolCatalog(ctx, allow, toolBinding{
		dialogID:     child.ID,
		planID:       rootID,
		mode:         incidentDialogMode,
		incidentRole: spec.role,
		system:       step.System,
	})
	if err != nil {
		fail(fmt.Errorf("build step catalog: %w", err))
		return
	}

	resp, err := s.runIncidentLoop(ctx, child.ID, catalog, rootID, maxIterations)
	if err != nil {
		fail(err)
		return
	}

	if step.Kind == IncidentStepDigger {
		s.ensureFinding(rootID, step, child.ID, resp.Response)
	}
	s.setIncidentStep(rootID, idx, func(st *IncidentStep) {
		st.Status = IncidentStepDone
		st.FinishedAt = time.Now().Unix()
	})
}

const incidentAgentCannotAskReason = "an investigation sub-agent cannot ask the operator. " +
	"State the assumption you would act on, record it in what you report, and carry on."

// runIncidentLoop runs a step's agent. The loop intercepts ask_question by name
// whether or not it is in the catalog, so an agent that asks anyway gets one
// answer telling it to assume and carry on; a second question ends the step.
func (s *ChatService) runIncidentLoop(ctx context.Context, dialogID uuid.UUID, catalog *toolCatalog, rootID uuid.UUID, maxIterations int) (domain.ChatResponse, error) {
	cfg := loopConfig{planID: rootID, maxIterations: maxIterations}
	for attempt := 0; ; attempt++ {
		resp, err := s.runPersistingAgentLoop(ctx, dialogID, incidentDialogMode, catalog, cfg)
		if err != nil {
			return domain.ChatResponse{}, err
		}
		if resp.Status != "awaiting_input" {
			return resp, nil
		}
		s.answerPendingAskForSubagent(ctx, dialogID, resp.ToolCallID, incidentAgentCannotAskReason)
		if attempt > 0 {
			return domain.ChatResponse{}, errors.New("the agent kept asking the operator instead of stating its assumptions")
		}
	}
}

// maxFallbackFindingChars bounds a finding recorded from a Digger's closing
// message, which can be a whole transcript's worth of narration.
const maxFallbackFindingChars = 4000

// ensureFinding records a Digger's closing message as its finding when it
// finished without calling report_finding, so the synthesis never reads a
// system as unexamined when it was examined.
func (s *ChatService) ensureFinding(rootID uuid.UUID, step IncidentStep, dialogID uuid.UUID, response string) {
	_, err := s.updatePostmortem(rootID, func(doc *Postmortem) error {
		for _, f := range doc.Findings {
			if f.DialogID == dialogID.String() {
				return nil
			}
		}
		summary := strings.TrimSpace(response)
		if summary == "" {
			summary = "The investigation ended without reporting a finding."
		}
		if len(summary) > maxFallbackFindingChars {
			summary = summary[:maxFallbackFindingChars] + "…"
		}
		upsertFinding(doc, PostmortemFinding{
			System:      step.System,
			Environment: step.Environment,
			Status:      "unknown",
			Summary:     summary,
			DialogID:    dialogID.String(),
			UpdatedAt:   time.Now().Unix(),
		})
		return nil
	})
	if err != nil {
		slog.Warn("incident run: record fallback finding", "dialog_id", rootID, "system", step.System, "error", err)
	}
}

// incidentStepSpec says how one kind of step runs.
type incidentStepSpec struct {
	title   string
	overlay string
	role    incidentRole
}

func incidentStepSpecFor(kind IncidentStepKind) incidentStepSpec {
	switch kind {
	case IncidentStepDigger:
		return incidentStepSpec{"Digger", "incident_digger", incidentRoleDigger}
	case IncidentStepSynthesis:
		return incidentStepSpec{"Synthesis", "incident_synthesis", incidentRoleSynthesis}
	case IncidentStepImpact:
		return incidentStepSpec{"Impact", "incident_impact", incidentRoleImpact}
	default:
		return incidentStepSpec{"Closure", "incident_closure", incidentRoleClosure}
	}
}

// incidentStepPrompt is a step's overlay on its own. The incident prompt is the
// orchestrator's -- triage, launching, the chat -- and every line of it would
// be an instruction a step must not follow.
func (s *ChatService) incidentStepPrompt(ctx context.Context, overlay string) (string, error) {
	if s.systemPromptsSvc == nil {
		return "", nil
	}
	p, err := s.systemPromptsSvc.Get(overlay)
	if err != nil {
		return "", fmt.Errorf("resolve %s prompt: %w", overlay, err)
	}
	return RenderTemplateVariables(ctx, p.Content, s.variableRepo, s.selection)
}

// diggerBaseTools are what every investigating agent reads with. Literal, so
// an operator's edit to data/tools/incident.json cannot widen them.
var diggerBaseTools = []string{
	KnowledgeSearchToolName,
	GetKBDocumentToolName,
	"tool_search",
	GetSkillToolName,
	ListVariablesToolName,
	CreateTableToolName,
	ExecuteCommandToolName,
	APICallToolName,
}

// incidentStepAllowSet is the catalog of one step. The investigating agents
// get the read tools, the MCP tools included for incident mode and those of
// the step's categories; the writers get the post-mortem and nothing that
// reaches a system.
func (s *ChatService) incidentStepAllowSet(ctx context.Context, kind IncidentStepKind, categories []string) map[string]struct{} {
	allow := make(map[string]struct{})
	switch kind {
	case IncidentStepDigger, IncidentStepImpact:
		for _, name := range diggerBaseTools {
			allow[name] = struct{}{}
		}
		if s.includedToolsSvc != nil {
			if mcpTools, err := s.includedToolsSvc.Get(incidentDialogMode); err == nil {
				for _, name := range mcpTools {
					allow[name] = struct{}{}
				}
			} else {
				slog.Warn("incident step allow set: included tools", "error", err)
			}
		}
		s.addCategoryTools(ctx, allow, categories, "incident step allow set")
		if kind == IncidentStepDigger {
			allow[ReportFindingToolName] = struct{}{}
		} else {
			allow[GetPostmortemToolName] = struct{}{}
			allow[UpdatePostmortemToolName] = struct{}{}
		}
	case IncidentStepSynthesis:
		allow[GetPostmortemToolName] = struct{}{}
		allow[UpdatePostmortemToolName] = struct{}{}
		allow[KnowledgeSearchToolName] = struct{}{}
		allow[GetKBDocumentToolName] = struct{}{}
	case IncidentStepClosure:
		allow[GetPostmortemToolName] = struct{}{}
		allow[UpdatePostmortemToolName] = struct{}{}
	}
	// A category can name any tool, so the ones no step may hold are taken out
	// after the categories went in.
	enforceModeToolLimits(incidentDialogMode, allow)
	stripSubagentTools(allow)
	delete(allow, AskQuestionToolName)
	delete(allow, ReportBlockerToolName)
	return allow
}

// incidentFollowUpAllowSet is the catalog of a turn the operator types into a
// step's transcript: the same read tools, and none of the writers, since a
// follow-up is a question about what the agent saw rather than a second run.
func (s *ChatService) incidentFollowUpAllowSet(ctx context.Context, d domain.Dialog) map[string]struct{} {
	allow := s.incidentStepAllowSet(ctx, IncidentStepDigger, d.Categories)
	delete(allow, ReportFindingToolName)
	allow[GetPostmortemToolName] = struct{}{}
	return allow
}

// buildIncidentStepSeed is a step's first message. No step can read the
// incident's chat, so everything it needs is spelled out here.
func buildIncidentStepSeed(run IncidentRun, step IncidentStep, doc Postmortem) string {
	var parts []string
	if run.Brief != "" {
		parts = append(parts, "## Incident brief\n\n"+run.Brief)
	}
	if w := incidentWindowText(run.WindowStart, run.WindowEnd); w != "" {
		parts = append(parts, "## Window\n\n"+w)
	}

	switch step.Kind {
	case IncidentStepDigger:
		lines := []string{"Name: " + step.System}
		if step.Environment != "" {
			lines = append(lines, "Environment: "+step.Environment)
		}
		if len(step.Categories) > 0 {
			lines = append(lines, "Tool categories: "+strings.Join(step.Categories, ", "))
		}
		if step.Focus != "" {
			lines = append(lines, "Focus: "+step.Focus)
		}
		parts = append(parts, "## Your system\n\n"+strings.Join(lines, "\n"))
		parts = append(parts, "## Post-mortem so far\n\n"+renderPostmortemForAgent(doc, false))
		parts = append(parts, "## Your task\n\nInvestigate this system for the incident above and finish with one `report_finding`.")
	case IncidentStepSynthesis:
		parts = append(parts, "## Post-mortem with every finding\n\n"+renderPostmortemForAgent(doc, true))
		parts = append(parts, "## Your task\n\nSynthesise the findings into the post-mortem with `update_postmortem`.")
	case IncidentStepImpact:
		parts = append(parts, "## Post-mortem with every finding\n\n"+renderPostmortemForAgent(doc, true))
		parts = append(parts, "## Your task\n\nAssess the incident's impact and write it into the post-mortem with `update_postmortem`.")
	case IncidentStepClosure:
		parts = append(parts, "## Post-mortem\n\n"+renderPostmortemForAgent(doc, true))
		parts = append(parts, "## Your task\n\nWrite the closure summary with one `update_postmortem`.")
	}
	return strings.Join(parts, "\n\n")
}

// incidentWindowText describes the window and the baseline of equal length
// before it, which every before-and-during comparison is made against.
func incidentWindowText(start, end string) string {
	if start == "" && end == "" {
		return ""
	}
	endText := end
	if endText == "" {
		endText = "now"
	}
	text := fmt.Sprintf("From %s to %s (UTC).", orUnknown(start), endText)
	ts, errStart := time.Parse(time.RFC3339, start)
	te, errEnd := time.Parse(time.RFC3339, end)
	if errEnd != nil && end == "" {
		te, errEnd = time.Now().UTC(), nil
	}
	if errStart == nil && errEnd == nil && te.After(ts) {
		baseline := ts.Add(-te.Sub(ts))
		text += fmt.Sprintf(" Baseline for comparison: %s to %s.", baseline.Format(time.RFC3339), start)
	}
	return text
}

func orUnknown(v string) string {
	if v == "" {
		return "an unknown start"
	}
	return v
}

// renderPostmortemForAgent is the document as an agent's seed reads it.
// Findings are left out of a Digger's seed: it is meant to look at its system
// with fresh eyes, not to confirm what a sibling concluded.
func renderPostmortemForAgent(doc Postmortem, withFindings bool) string {
	var b strings.Builder
	field := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(&b, "**%s:** %s\n", label, strings.TrimSpace(value))
		}
	}
	field("Status", doc.Status)
	field("Severity", doc.Severity)
	field("Environment", doc.Environment)
	field("Started", doc.StartedAt)
	field("Detected", doc.DetectedAt)
	field("Mitigated", doc.MitigatedAt)
	field("Resolved", doc.ResolvedAt)
	field("Summary", doc.Summary)
	field("Hypothesis", doc.Hypothesis)
	if len(doc.RootCauses) > 0 {
		b.WriteString("\n**Root causes:**\n")
		for _, rc := range doc.RootCauses {
			fmt.Fprintf(&b, "- %s\n", rc.Text)
		}
	}
	if len(doc.Impacted) > 0 {
		b.WriteString("\n**Impacted:**\n")
		for _, im := range doc.Impacted {
			fmt.Fprintf(&b, "- %s %s %s %s\n", im.Name, im.Environment, im.Severity, im.Description)
		}
	}
	if len(doc.Timeline) > 0 {
		b.WriteString("\n**Timeline:**\n")
		for _, ev := range doc.Timeline {
			fmt.Fprintf(&b, "- %s %s (%s)\n", ev.At, ev.Event, ev.Source)
		}
	}
	for _, group := range []struct {
		label   string
		actions []PostmortemAction
	}{{"Short-term actions", doc.ShortTerm}, {"Long-term actions", doc.LongTerm}} {
		if len(group.actions) > 0 {
			fmt.Fprintf(&b, "\n**%s:**\n", group.label)
			for _, a := range group.actions {
				fmt.Fprintf(&b, "- [%s] %s\n", a.Status, a.Text)
			}
		}
	}
	if withFindings && len(doc.Findings) > 0 {
		b.WriteString("\n**Findings by system:**\n")
		for _, f := range doc.Findings {
			fmt.Fprintf(&b, "\n### %s (%s) — %s, confidence %s\n%s\n", f.System, f.Environment, f.Status, orNone(f.Confidence), f.Summary)
			if f.Before != "" {
				fmt.Fprintf(&b, "Before: %s\n", f.Before)
			}
			if f.During != "" {
				fmt.Fprintf(&b, "During: %s\n", f.During)
			}
			if len(f.Changes) > 0 {
				fmt.Fprintf(&b, "Changes: %s\n", strings.Join(f.Changes, "; "))
			}
			if f.SuspectedCause != "" {
				fmt.Fprintf(&b, "Suspected cause: %s\n", f.SuspectedCause)
			}
			if f.Assumptions != "" {
				fmt.Fprintf(&b, "Assumptions: %s\n", f.Assumptions)
			}
		}
	}
	if b.Len() == 0 {
		return "_Nothing recorded yet._"
	}
	return strings.TrimRight(b.String(), "\n")
}

func orNone(v string) string {
	if v == "" {
		return "not stated"
	}
	return v
}
