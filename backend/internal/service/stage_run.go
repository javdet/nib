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
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

// StageRunScope says what a stage run walks: one stage of the plan, or its
// rollback list.
type StageRunScope string

const (
	StageRunScopeStage    StageRunScope = "stage"
	StageRunScopeRollback StageRunScope = "rollback"
)

// StageRunStatus is where a stage run is.
type StageRunStatus string

const (
	StageRunRunning StageRunStatus = "running"
	// StageRunDone means every item was run and none of them failed. Like an
	// action's own "done" it is not a claim that the stage worked: ticking the
	// items is still the operator's call.
	StageRunDone    StageRunStatus = "done"
	StageRunStopped StageRunStatus = "stopped"
)

var (
	// ErrStageRunActive is returned when a stage run is already going. One at a
	// time, across every plan, for the reason the execution lease is one.
	ErrStageRunActive = errors.New("a stage run is already going")
	// ErrStageRunNothingToRun is returned for a stage whose items are all ticked.
	ErrStageRunNothingToRun = errors.New("every item of the stage is already ticked")
	// ErrNoStageRunActive is returned when a stop finds nothing to stop.
	ErrNoStageRunActive = errors.New("no stage run is going")
	// ErrInvalidStageRunScope is returned for a scope other than stage or rollback.
	ErrInvalidStageRunScope = errors.New(`scope must be "stage" or "rollback"`)
)

const (
	stageRunStoppedByOperator = "stopped by the operator"
	stageRunPlanChanged       = "the plan changed while the stage was running"
)

// StageRun is the "run all" of one stage: its items executed one after another,
// each started only once the one before it has finished.
//
// It lives in {plan}.stagerun.json, the latest run only. It is deliberately not
// remapped when a stage is rewritten or reordered: the items are addressed by
// position, so a structural edit stops the run instead.
type StageRun struct {
	RunID uuid.UUID     `json:"runId"`
	Scope StageRunScope `json:"scope"`
	// Stage is the 0-based index of the stage, and is 0 for the rollback.
	Stage int    `json:"stage"`
	Title string `json:"title"`
	// Shape is the stage's title and item counts when the run started. An
	// advance that finds a different shape stops: the keys it would run next no
	// longer name the items the operator asked for.
	Shape string `json:"shape"`
	// Current is the row key being run, or the one the run stopped at.
	Current string `json:"current,omitempty"`
	// BaseAttempt is Current's attempt counter before this run launched it, so
	// a hook for an earlier attempt cannot be taken for this one's result.
	BaseAttempt int            `json:"baseAttempt"`
	Status      StageRunStatus `json:"status"`
	Reason      string         `json:"reason,omitempty"`
	StartedAt   int64          `json:"startedAt"`
	FinishedAt  int64          `json:"finishedAt,omitempty"`
}

// Active reports whether the run is still going.
func (r StageRun) Active() bool {
	return r.Status == StageRunRunning
}

func stageRunPath(dir string, planID uuid.UUID) string {
	return filepath.Join(dir, planID.String()+".stagerun.json")
}

// ReadStageRun returns the latest stage run of a plan, or nil when it never had
// one.
func (s *ChatService) ReadStageRun(planID uuid.UUID) (*StageRun, error) {
	run, found, err := s.readStageRun(planID)
	if err != nil || !found {
		return nil, err
	}
	return &run, nil
}

func (s *ChatService) readStageRun(planID uuid.UUID) (StageRun, bool, error) {
	b, err := os.ReadFile(stageRunPath(s.actionPlansDir, planID))
	if errors.Is(err, os.ErrNotExist) {
		return StageRun{}, false, nil
	}
	if err != nil {
		return StageRun{}, false, err
	}
	var run StageRun
	if err := json.Unmarshal(b, &run); err != nil {
		return StageRun{}, false, fmt.Errorf("unmarshal stage run: %w", err)
	}
	return run, true, nil
}

func (s *ChatService) writeStageRun(planID uuid.UUID, run StageRun) error {
	if err := os.MkdirAll(s.actionPlansDir, 0o755); err != nil {
		return fmt.Errorf("create action_plans directory: %w", err)
	}
	data, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("marshal stage run: %w", err)
	}
	return writeActionPlanFile(stageRunPath(s.actionPlansDir, planID), data)
}

// readStoredActionPlan parses the plan of planID.
func (s *ChatService) readStoredActionPlan(planID uuid.UUID) (storedActionPlan, error) {
	raw, found, err := s.ReadActionPlan(planID)
	if err != nil {
		return storedActionPlan{}, fmt.Errorf("read action plan: %w", err)
	}
	if !found {
		return storedActionPlan{}, fmt.Errorf("action plan: %w", repository.ErrNotFound)
	}
	var plan storedActionPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return storedActionPlan{}, fmt.Errorf("parse action plan: %w", err)
	}
	return plan, nil
}

// stageRunTarget lists the row keys a stage run walks, in the order it walks
// them -- the steps, then the checks that verify them -- with the stage's title
// and shape.
func stageRunTarget(plan storedActionPlan, scope StageRunScope, stage int) (keys []string, title, shape string, err error) {
	switch scope {
	case StageRunScopeRollback:
		for i := range plan.Rollback {
			keys = append(keys, fmt.Sprintf("rollback.%d", i))
		}
		return keys, "Rollback", fmt.Sprintf("rollback|%d", len(plan.Rollback)), nil
	case StageRunScopeStage:
		if stage < 0 || stage >= len(plan.Stages) {
			return nil, "", "", ErrActionPlanIndexOutOfRange
		}
		st := plan.Stages[stage]
		for i := range st.Steps {
			keys = append(keys, actionPlanItemKey(stage, ActionPlanScopeSteps, i))
		}
		for i := range st.Checks {
			keys = append(keys, actionPlanItemKey(stage, ActionPlanScopeChecks, i))
		}
		// Only the structure: rewording an action the run has not reached yet
		// is exactly the edit an operator makes while watching one, and the
		// keys still name the same items after it.
		shape = fmt.Sprintf("%s|%d|%d", normalizeStageTitle(st.Title), len(st.Steps), len(st.Checks))
		return keys, strings.TrimSpace(st.Title), shape, nil
	default:
		return nil, "", "", ErrInvalidStageRunScope
	}
}

// nextStageRunKey returns the first unticked key after after, or from the start
// when after is empty. A ticked item is one the operator has already confirmed,
// so running it again would only repeat work.
func nextStageRunKey(keys []string, after string, checked map[string]bool) (string, bool) {
	start := 0
	if after != "" {
		for i, k := range keys {
			if k == after {
				start = i + 1
				break
			}
		}
	}
	for _, k := range keys[start:] {
		if !checked[k] {
			return k, true
		}
	}
	return "", false
}

// stageRunOutcome decides whether a finished item lets the run move on, and
// when it does not, says why in the words the operator reads.
func stageRunOutcome(run ActionExecRun, number string) (bool, string) {
	switch run.Status {
	case ActionExecDone:
		return true, ""
	case ActionExecBlocked:
		return false, number + " needs a decision"
	case ActionExecCancelled:
		return false, number + " was cancelled"
	default:
		if msg := strings.TrimSpace(run.Error); msg != "" {
			return false, fmt.Sprintf("%s failed: %s", number, msg)
		}
		return false, number + " failed"
	}
}

func checkedSet(keys []string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set
}

// StartStageRun runs the unticked items of one stage, or of the rollback, one
// after another. It returns once the first item has started; each later one is
// started by the end of the one before it, and the run stops at the first item
// that does not end "done".
//
// It bypasses the orchestrator on purpose: an action's result is posted into
// the plan chat without starting a turn, so a model has nothing to wake up on
// between items.
func (s *ChatService) StartStageRun(ctx context.Context, planID uuid.UUID, scope StageRunScope, stage int) (StageRun, error) {
	if scope != StageRunScopeStage && scope != StageRunScopeRollback {
		return StageRun{}, ErrInvalidStageRunScope
	}
	if scope == StageRunScopeRollback {
		stage = 0
	}

	plan, err := s.readStoredActionPlan(planID)
	if err != nil {
		return StageRun{}, fmt.Errorf("start stage run: %w", err)
	}
	keys, title, shape, err := stageRunTarget(plan, scope, stage)
	if err != nil {
		return StageRun{}, err
	}
	checked, err := s.ReadActionPlanChecks(planID)
	if err != nil {
		return StageRun{}, fmt.Errorf("start stage run: read checks: %w", err)
	}
	first, ok := nextStageRunKey(keys, "", checkedSet(checked))
	if !ok {
		return StageRun{}, ErrStageRunNothingToRun
	}
	// Refused up front rather than by the first launch, so a busy slot does not
	// leave a stopped run behind for a stage that never started.
	if held, busy := s.ExecutionInProgress(); busy {
		return StageRun{}, newExecutionBusyError(held)
	}
	execRuns, err := s.ReadActionPlanExecRuns(planID)
	if err != nil {
		return StageRun{}, fmt.Errorf("start stage run: read exec runs: %w", err)
	}

	s.stageRunMu.Lock()
	if err := s.stageRunSlotFreeLocked(); err != nil {
		s.stageRunMu.Unlock()
		return StageRun{}, err
	}
	run := StageRun{
		RunID:       uuid.New(),
		Scope:       scope,
		Stage:       stage,
		Title:       title,
		Shape:       shape,
		Current:     first,
		BaseAttempt: execRuns[first].Attempt,
		Status:      StageRunRunning,
		StartedAt:   time.Now().Unix(),
	}
	if err := s.writeStageRun(planID, run); err != nil {
		s.stageRunMu.Unlock()
		return StageRun{}, fmt.Errorf("start stage run: %w", err)
	}
	s.stageRunActive = planID
	s.stageRunMu.Unlock()

	s.publishStageRun(planID, run)

	// The launch may outlive a closed tab: a half-started container would
	// otherwise be abandoned along with the request.
	if err := s.launchStageRunItem(context.WithoutCancel(ctx), planID, first); err != nil {
		// The caller hears why from the error, so the chat gets no message.
		s.stopStageRunAt(planID, run.RunID, first, stageRunLaunchReason(err, first), false)
		return StageRun{}, err
	}
	return run, nil
}

// stageRunSlotFreeLocked refuses a second run while one is going. The record on
// disk is the authority: a plan whose files were reset or removed under a
// running run would otherwise hold the slot for good.
func (s *ChatService) stageRunSlotFreeLocked() error {
	if s.stageRunActive == uuid.Nil {
		return nil
	}
	held, found, err := s.readStageRun(s.stageRunActive)
	if err == nil && found && held.Active() {
		return fmt.Errorf("%w: %s", ErrStageRunActive, stageRunSubject(held))
	}
	s.stageRunActive = uuid.Nil
	return nil
}

// StopStageRun stops the plan's stage run, and the item it is on with it.
func (s *ChatService) StopStageRun(ctx context.Context, planID uuid.UUID) (StageRun, error) {
	s.stageRunMu.Lock()
	run, found, err := s.readStageRun(planID)
	if err != nil {
		s.stageRunMu.Unlock()
		return StageRun{}, fmt.Errorf("stop stage run: %w", err)
	}
	if !found || !run.Active() {
		s.stageRunMu.Unlock()
		return StageRun{}, ErrNoStageRunActive
	}
	run = s.closeStageRunLocked(planID, run, StageRunStopped, stageRunStoppedByOperator)
	s.stageRunMu.Unlock()

	s.announceStageRun(ctx, planID, run, true)

	// Only the run's own item: whatever else holds the slot is not this stop's
	// to end.
	held, ok := s.takeExecutionLeaseIf(func(l ExecutionLease) bool {
		return !l.Fix && l.PlanID == planID && l.Key == run.Current
	})
	if ok {
		if _, err := s.cancelHeldExecution(ctx, held); err != nil {
			slog.Warn("stop stage run: stop current item",
				"plan_id", planID, "key", held.Key, "error", err)
		}
	}
	return run, nil
}

// onPlanItemClosed moves the plan's stage run on once an item's run has been
// closed and the execution lease released. It returns at once: starting the
// next item can mean pulling an image, which is no reason to hold the caller.
func (s *ChatService) onPlanItemClosed(planID uuid.UUID, key string) {
	go s.advanceStageRun(planID, key)
}

// advanceStageRun is what every terminal path of an item ends in. It must only
// run once the lease is released, or the next item would be refused as busy.
//
// It is idempotent: the same item may be reported closed twice -- a webhook
// retry, a force stop followed by the cancelled goroutine's own finish -- and
// only the first report of a finished attempt newer than the one the run
// launched from moves the run on.
func (s *ChatService) advanceStageRun(planID uuid.UUID, key string) {
	s.stageRunMu.Lock()
	run, found, err := s.readStageRun(planID)
	if err != nil || !found || !run.Active() || run.Current != key {
		s.stageRunMu.Unlock()
		return
	}
	execRuns, err := s.ReadActionPlanExecRuns(planID)
	if err != nil {
		s.stageRunMu.Unlock()
		slog.Warn("advance stage run: read exec runs", "plan_id", planID, "error", err)
		return
	}
	rec := execRuns[key]
	if rec.Active() || rec.Attempt <= run.BaseAttempt {
		s.stageRunMu.Unlock()
		return
	}

	if ok, reason := stageRunOutcome(rec, stageRunNumber(key)); !ok {
		run = s.closeStageRunLocked(planID, run, StageRunStopped, reason)
		s.stageRunMu.Unlock()
		s.announceStageRun(context.Background(), planID, run, true)
		return
	}

	next, status, reason := s.nextStageRunItem(planID, run, key)
	if next == "" {
		run = s.closeStageRunLocked(planID, run, status, reason)
		s.stageRunMu.Unlock()
		s.announceStageRun(context.Background(), planID, run, true)
		return
	}

	// Claimed under the lock before anything is launched: a duplicate report of
	// the item just finished now names an item the run is no longer on.
	run.Current = next
	run.BaseAttempt = execRuns[next].Attempt
	if err := s.writeStageRun(planID, run); err != nil {
		run = s.closeStageRunLocked(planID, run, StageRunStopped, "the run could not be recorded")
		s.stageRunMu.Unlock()
		slog.Warn("advance stage run: write", "plan_id", planID, "error", err)
		s.announceStageRun(context.Background(), planID, run, true)
		return
	}
	s.stageRunMu.Unlock()

	s.publishStageRun(planID, run)
	if err := s.launchStageRunItem(context.Background(), planID, next); err != nil {
		s.stopStageRunAt(planID, run.RunID, next, stageRunLaunchReason(err, next), true)
	}
}

// nextStageRunItem picks the item after the one just finished. With none left
// it returns an empty key and how the run ends.
func (s *ChatService) nextStageRunItem(planID uuid.UUID, run StageRun, after string) (string, StageRunStatus, string) {
	plan, err := s.readStoredActionPlan(planID)
	if err != nil {
		return "", StageRunStopped, stageRunPlanChanged
	}
	keys, _, shape, err := stageRunTarget(plan, run.Scope, run.Stage)
	if err != nil || shape != run.Shape {
		return "", StageRunStopped, stageRunPlanChanged
	}
	checked, err := s.ReadActionPlanChecks(planID)
	if err != nil {
		slog.Warn("advance stage run: read checks", "plan_id", planID, "error", err)
		return "", StageRunStopped, "the plan's checks could not be read"
	}
	next, ok := nextStageRunKey(keys, after, checkedSet(checked))
	if !ok {
		return "", StageRunDone, ""
	}
	return next, "", ""
}

func (s *ChatService) launchStageRunItem(ctx context.Context, planID uuid.UUID, key string) error {
	if s.stageRunStart != nil {
		return s.stageRunStart(ctx, planID, key)
	}
	_, err := s.startPlanItem(ctx, planID, key, false)
	return err
}

// stageRunLaunchReason says why an item could not be started, in the sentence
// the execute_action tool would have used for it.
func stageRunLaunchReason(err error, key string) string {
	number := stageRunNumber(key)
	if text, ok := planItemRefusal(err, number); ok {
		return text
	}
	return fmt.Sprintf("%s could not start: %s", number, err.Error())
}

// stopStageRunAt stops the run identified by runID if it is still on key, which
// is what separates "this launch failed" from a run that has since moved on or
// been stopped by somebody else.
func (s *ChatService) stopStageRunAt(planID, runID uuid.UUID, key, reason string, post bool) {
	s.stageRunMu.Lock()
	run, found, err := s.readStageRun(planID)
	if err != nil || !found || !run.Active() || run.RunID != runID || run.Current != key {
		s.stageRunMu.Unlock()
		return
	}
	run = s.closeStageRunLocked(planID, run, StageRunStopped, reason)
	s.stageRunMu.Unlock()
	s.announceStageRun(context.Background(), planID, run, post)
}

// stopStageRunOnItem stops the plan's stage run when it is on key. A force stop
// of that item is the operator saying stop, which the run moving on to its next
// item a moment later would undo.
func (s *ChatService) stopStageRunOnItem(planID uuid.UUID, key, reason string) {
	s.stageRunMu.Lock()
	run, found, err := s.readStageRun(planID)
	if err != nil || !found || !run.Active() || run.Current != key {
		s.stageRunMu.Unlock()
		return
	}
	run = s.closeStageRunLocked(planID, run, StageRunStopped, reason)
	s.stageRunMu.Unlock()
	s.announceStageRun(context.Background(), planID, run, true)
}

// stopStageRunForPlanEdit stops the plan's stage run because the plan was
// restructured under it. match narrows it to runs the edit actually affects;
// nil stops any. The item already running is left to finish: the edit
// invalidates what comes next, not what is under way.
func (s *ChatService) stopStageRunForPlanEdit(planID uuid.UUID, match func(StageRun) bool) {
	s.stageRunMu.Lock()
	run, found, err := s.readStageRun(planID)
	if err != nil || !found || !run.Active() || (match != nil && !match(run)) {
		s.stageRunMu.Unlock()
		return
	}
	run = s.closeStageRunLocked(planID, run, StageRunStopped, stageRunPlanChanged)
	s.stageRunMu.Unlock()
	s.announceStageRun(context.Background(), planID, run, true)
}

// closeStageRunLocked ends run and releases the slot. The caller holds
// stageRunMu and announces the result once it has let go of it.
func (s *ChatService) closeStageRunLocked(planID uuid.UUID, run StageRun, status StageRunStatus, reason string) StageRun {
	run.Status = status
	run.Reason = reason
	run.FinishedAt = time.Now().Unix()
	if err := s.writeStageRun(planID, run); err != nil {
		// The slot is released regardless: a record that cannot be written must
		// not keep every other stage from running.
		slog.Warn("close stage run", "plan_id", planID, "error", err)
	}
	if s.stageRunActive == planID {
		s.stageRunActive = uuid.Nil
	}
	return run
}

func (s *ChatService) publishStageRun(planID uuid.UUID, run StageRun) {
	if s.activity == nil {
		return
	}
	s.activity.Publish(planID, domain.AgentActivity{
		Kind:   domain.ActivityStageRunUpdated,
		Stage:  run.Title,
		Action: run.Current,
		Status: string(run.Status),
	})
}

// announceStageRun publishes a finished run and, when post is set, says how it
// ended in the plan chat -- where the per-item reports already are, so the
// operator reads the end of the run beside them.
//
// The message is named after the run so that a second close of the same run
// cannot post twice.
func (s *ChatService) announceStageRun(ctx context.Context, planID uuid.UUID, run StageRun, post bool) {
	s.publishStageRun(planID, run)
	if !post || s.dialogRepo == nil {
		return
	}

	name := "stage-run:" + run.RunID.String()
	existing, err := s.dialogRepo.ListMessages(ctx, planID)
	if err != nil {
		slog.Warn("stage run: list messages", "plan_id", planID, "error", err)
		return
	}
	for _, m := range existing {
		if m.Name == name {
			return
		}
	}
	if _, err := s.appendMessageLocked(ctx, planID, domain.DialogMessage{
		Role:    "assistant",
		Content: formatStageRunMessage(run),
		Name:    name,
	}); err != nil {
		slog.Warn("stage run: append", "plan_id", planID, "error", err)
	}
}

func formatStageRunMessage(run StageRun) string {
	subject := stageRunSubject(run)
	if run.Status == StageRunDone {
		return fmt.Sprintf("**%s ran to the end** ✅\n\nTick its items once you have checked the results.", subject)
	}
	msg := fmt.Sprintf("**%s run stopped** ⏹", subject)
	if run.Current != "" {
		msg += " at " + stageRunNumber(run.Current)
	}
	if reason := strings.TrimSpace(run.Reason); reason != "" {
		msg += "\n\n" + reason
	}
	return msg
}

// stageRunSubject names a run's stage the way the plan view heads it.
func stageRunSubject(run StageRun) string {
	if run.Scope == StageRunScopeRollback {
		return "Rollback"
	}
	if run.Title == "" {
		return fmt.Sprintf("Stage %d", run.Stage+1)
	}
	return fmt.Sprintf("Stage %d (%s)", run.Stage+1, run.Title)
}

func stageRunNumber(key string) string {
	if number := actionPlanNumberForKey(key); number != "" {
		return number
	}
	return key
}
