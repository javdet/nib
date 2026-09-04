package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/metrics"
)

// ExecutionKind says what holds the lease, because the two are stopped in
// completely different ways: a sub-agent by cancelling its context, a container
// through the executor.
type ExecutionKind string

const (
	ExecutionKindSubagent  ExecutionKind = "subagent"
	ExecutionKindContainer ExecutionKind = "container"
)

var (
	// ErrExecutionBusy is returned when the execution lease is already held. Its
	// message names what holds it, because the only useful thing an operator can
	// do about it is decide whether to wait or to stop that.
	ErrExecutionBusy = errors.New("another execution is already running")
	// ErrNoExecutionRunning is returned when a force stop finds nothing to stop.
	ErrNoExecutionRunning = errors.New("no execution is running")
)

// ExecutionLease is the one execution allowed at a time, across every plan.
//
// One at a time is a policy, not a capacity limit: two agents changing live
// infrastructure from the same plan can undo each other's work, and an operator
// watching one chat cannot follow two. A second request is refused rather than
// queued, so "run this now" never turns into a silent wait -- which is what the
// old semaphore did, blocking inside the goroutine after the tool had already
// told the operator the work had started.
type ExecutionLease struct {
	// PlanID is the root dialog whose plan the running item belongs to.
	PlanID uuid.UUID `json:"planId"`
	// Key is the plan row ("s0.step1"); Number is what the operator sees.
	Key    string        `json:"key"`
	Number string        `json:"number"`
	Kind   ExecutionKind `json:"kind"`
	// DialogID is the sub-agent's or the container's execute chat, so a
	// rejection can point at a transcript. Stamped once it exists.
	DialogID uuid.UUID `json:"dialogId,omitempty"`
	// JobName, ContainerID and Namespace identify a container run, and are the
	// only way to reach it: the container outlives this process.
	JobName     string `json:"jobName,omitempty"`
	ContainerID string `json:"containerId,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	StartedAt   int64  `json:"startedAt"`

	// cancel stops a sub-agent run. Nil for a container, which is stopped
	// through the executor instead.
	cancel context.CancelFunc
	// token identifies this grant of the lease, so a release can only ever drop
	// its own. Keying a release on the plan and row instead left a window during
	// a restart takeover: the attempt number is only known after the record is
	// written, so the outgoing run could release the incoming run's lease in
	// between.
	token uuid.UUID
}

// acquireExecutionLease takes the lease for l.
//
// On success it returns the granted lease, whose token the caller keeps and
// hands to stampExecutionLease and releaseExecutionLease. On failure it returns
// the holder and false, so the caller can name it in the refusal.
func (s *ChatService) acquireExecutionLease(l ExecutionLease) (ExecutionLease, bool) {
	s.execLeaseMu.Lock()
	defer s.execLeaseMu.Unlock()

	s.expireStaleContainerLeaseLocked()

	if s.execLease != nil {
		metrics.RecordLeaseRejected(string(l.Kind))
		return *s.execLease, false
	}
	l.token = uuid.New()
	s.execLease = &l
	metrics.RecordLeaseAcquired(string(l.Kind))
	return l, true
}

// recordLeaseReleasedLocked reports a lease being dropped. Called from every
// path that clears s.execLease, so the held gauge cannot get stuck at 1.
func recordLeaseReleasedLocked(l *ExecutionLease) {
	if l == nil {
		return
	}
	var held time.Duration
	if l.StartedAt > 0 {
		held = time.Since(time.Unix(l.StartedAt, 0))
	}
	metrics.RecordLeaseReleased(string(l.Kind), held)
}

// expireStaleContainerLeaseLocked drops a container lease nothing will ever
// release.
//
// A container reports back through the agent-runner webhook, so when it never
// does -- a crashed image, a failed pull, a node that went away -- there is no
// goroutine left to notice. Past the action deadline (which is longer than the
// container's own TIMEOUT_SECONDS) the lease is treated as expired, because a
// lease nothing can release is a permanent block on every action in every plan.
func (s *ChatService) expireStaleContainerLeaseLocked() {
	if s.execLease == nil || s.execLease.Kind != ExecutionKindContainer {
		return
	}
	if time.Since(time.Unix(s.execLease.StartedAt, 0)) <= s.actionExec.timeout() {
		return
	}

	expired := *s.execLease
	s.execLease = nil
	recordLeaseReleasedLocked(&expired)
	metrics.RecordLeaseExpired()
	slog.Warn("execution lease expired without a result",
		"plan_id", expired.PlanID, "key", expired.Key, "job", expired.JobName)

	// Off the lock: closing the record takes the plan mutex, and the caller of
	// this is on its way to starting a run of its own.
	go func() {
		s.finishActionExecRun(expired.PlanID, expired.Key, ActionExecFailed,
			"the coding agent never reported back")
		s.activity.Publish(expired.PlanID, domain.AgentActivity{
			Kind:   domain.ActivityActionExecFailed,
			Action: expired.Key,
			Status: string(ActionExecFailed),
		})
	}()
}

// stampExecutionLease fills in what is only known after the lease was taken --
// the dialog, the container identity -- and does nothing when the lease has
// since moved on.
func (s *ChatService) stampExecutionLease(token uuid.UUID, mutate func(*ExecutionLease)) {
	s.execLeaseMu.Lock()
	defer s.execLeaseMu.Unlock()

	if s.execLease == nil || s.execLease.token != token {
		return
	}
	mutate(s.execLease)
}

// releaseExecutionLease drops the lease granted under token, and does nothing
// otherwise. A run that a restart or a force stop already took over must not
// release the lease its successor now holds.
func (s *ChatService) releaseExecutionLease(token uuid.UUID) {
	s.execLeaseMu.Lock()
	defer s.execLeaseMu.Unlock()

	if s.execLease == nil || s.execLease.token != token {
		return
	}
	recordLeaseReleasedLocked(s.execLease)
	s.execLease = nil
}

// releaseExecutionLeaseForContainer drops a container lease identified by the
// row it runs and the job it started.
//
// The agent-runner webhook has no token -- the container outlives the process
// that took the lease -- so it releases by identity instead. The job name is
// part of that identity on purpose: without it, a webhook arriving late, after
// the operator force-stopped the run and started it again, would release the
// lease belonging to the new attempt.
func (s *ChatService) releaseExecutionLeaseForContainer(planID uuid.UUID, key, jobName string) {
	s.execLeaseMu.Lock()
	defer s.execLeaseMu.Unlock()

	held := s.execLease
	if held == nil || held.Kind != ExecutionKindContainer {
		return
	}
	if held.PlanID != planID || held.Key != key {
		return
	}
	if jobName != "" && held.JobName != "" && held.JobName != jobName {
		return
	}
	recordLeaseReleasedLocked(held)
	s.execLease = nil
}

// takeExecutionLease removes the lease and returns it, so a force stop cannot
// race a second stop onto the same run.
func (s *ChatService) takeExecutionLease() (ExecutionLease, bool) {
	s.execLeaseMu.Lock()
	defer s.execLeaseMu.Unlock()

	if s.execLease == nil {
		return ExecutionLease{}, false
	}
	held := *s.execLease
	s.execLease = nil
	recordLeaseReleasedLocked(&held)
	return held, true
}

// ExecutionInProgress reports the execution running right now, if any.
func (s *ChatService) ExecutionInProgress() (ExecutionLease, bool) {
	s.execLeaseMu.Lock()
	defer s.execLeaseMu.Unlock()

	s.expireStaleContainerLeaseLocked()

	if s.execLease == nil {
		return ExecutionLease{}, false
	}
	return *s.execLease, true
}

// ExecutionBusyError reports that the execution lease is held, and carries the
// sentence naming what holds it.
//
// The sentence is a field rather than something recovered from the error string:
// it contains a colon of its own, so any caller trying to split the wrapped
// message back out would cut it in the wrong place.
type ExecutionBusyError struct {
	// Message names the running item, what kind of work it is, and for how long
	// it has been going -- everything the operator needs to choose between
	// waiting and stopping it.
	Message string
	// Lease is what holds the slot, for a caller that needs more than prose.
	Lease ExecutionLease
}

func (e *ExecutionBusyError) Error() string {
	return ErrExecutionBusy.Error() + ": " + e.Message
}

// Unwrap makes errors.Is(err, ErrExecutionBusy) work, which is how the HTTP
// layer maps this to a 409.
func (e *ExecutionBusyError) Unwrap() error { return ErrExecutionBusy }

// newExecutionBusyError builds the refusal for a lease that is already held.
func newExecutionBusyError(held ExecutionLease) error {
	return &ExecutionBusyError{Message: busyExecutionMessage(held), Lease: held}
}

// executionBusyMessage is the sentence inside err when it is an
// ExecutionBusyError, and err's own message otherwise.
func executionBusyMessage(err error) string {
	var busy *ExecutionBusyError
	if errors.As(err, &busy) {
		return busy.Message
	}
	return err.Error()
}

// busyExecutionMessage says what is running, in the one sentence an operator or
// a model needs to decide between waiting and stopping it.
func busyExecutionMessage(l ExecutionLease) string {
	what := "sub-agent"
	if l.Kind == ExecutionKindContainer {
		what = "code action in container " + l.JobName
		if strings.TrimSpace(l.JobName) == "" {
			what = "code action in a container"
		}
	}

	number := strings.TrimSpace(l.Number)
	if number == "" {
		number = l.Key
	}

	return fmt.Sprintf("%s is already running (%s, started %s ago). "+
		"Only one execution runs at a time: stop it before starting another, or wait for it to finish.",
		number, what, humanSince(time.Unix(l.StartedAt, 0)))
}

// humanSince renders an elapsed time the way a sentence needs it rather than the
// way Duration.String does.
func humanSince(start time.Time) string {
	d := time.Since(start)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	default:
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
}

// CancelExecution force-stops whatever holds the lease: it cancels a sub-agent's
// context, or stops a code action's container through the executor.
//
// The lease is dropped and the run recorded as cancelled either way. Unblocking
// execution is the whole point of the call, so a stop that fails halfway must
// not leave the lease held against a run nothing can close.
func (s *ChatService) CancelExecution(ctx context.Context) (ExecutionLease, error) {
	held, ok := s.takeExecutionLease()
	if !ok {
		return ExecutionLease{}, ErrNoExecutionRunning
	}

	stopErr := s.stopLeaseHolder(ctx, held)
	metrics.RecordForceStop(string(held.Kind), stopErr)

	run := s.finishActionExecRun(held.PlanID, held.Key, ActionExecCancelled, "stopped by the operator")
	s.activity.Publish(held.PlanID, domain.AgentActivity{
		Kind:   domain.ActivityActionExecFailed,
		Action: held.Key,
		Status: string(ActionExecCancelled),
	})
	s.reportActionResult(context.WithoutCancel(ctx), held.PlanID, held.Key, run.Attempt,
		held.DialogID, ActionExecCancelled, "The operator stopped this execution.")

	slog.Info("execution stopped by the operator",
		"plan_id", held.PlanID, "key", held.Key, "kind", held.Kind, "error", stopErr)
	return held, stopErr
}

// stopLeaseHolder stops the work a lease covers without touching the lease
// itself, so it is usable both by a force stop and by a restart taking a row
// over.
func (s *ChatService) stopLeaseHolder(ctx context.Context, held ExecutionLease) error {
	switch held.Kind {
	case ExecutionKindSubagent:
		if held.cancel != nil {
			held.cancel()
		}
		return nil
	case ExecutionKindContainer:
		if s.executorSvc == nil {
			return fmt.Errorf("stop execution: executor is not configured")
		}
		if err := s.executorSvc.StopAction(ctx, executor.StopActionRequest{
			JobName:     held.JobName,
			ContainerID: held.ContainerID,
			Namespace:   held.Namespace,
		}); err != nil {
			return fmt.Errorf("stop action container: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("stop execution: unknown execution kind %q", held.Kind)
	}
}
