package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/executor"
	"github.com/javdet/nib/internal/metrics"
	"github.com/javdet/nib/internal/repository"
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
	// Fix marks an ad-hoc code fix asked for in the chat rather than a row of a
	// plan: Key and Number are empty for one, and nothing about it is recorded
	// in the plan files. PlanID is still the chat it was asked for in, which is
	// where its outcome is reported.
	Fix bool `json:"fix,omitempty"`
	// DialogID is the sub-agent's or the container's execute chat, so a
	// rejection can point at a transcript. Stamped once it exists.
	DialogID uuid.UUID `json:"dialogId,omitempty"`
	// JobName, ContainerID and Namespace identify a container run, and are the
	// only way to reach it: the container outlives this process.
	JobName     string `json:"jobName,omitempty"`
	ContainerID string `json:"containerId,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	StartedAt   int64  `json:"startedAt"`
	// Owner is the backend process that took the lease. Only that process
	// holds a sub-agent's cancel func, so a force stop arriving at another one
	// is relayed to it.
	Owner string `json:"owner,omitempty"`

	// cancel stops a sub-agent run. Nil for a container, which is stopped
	// through the executor instead.
	cancel context.CancelFunc
	// token identifies this grant of the lease, so a release can only ever drop
	// its own. Keying a release on the plan and row instead left a window during
	// a restart takeover: the attempt number is only known after the record is
	// written, so the outgoing run could release the incoming run's lease in
	// between.
	token uuid.UUID
	// refusal is why acquireExecutionLease refused without a holder to name.
	refusal error
}

// leaseCallTimeout bounds one read or write of the lease. The lease functions
// take no context -- a release runs in a deferred call after the work's own
// context is long gone -- so each gets a deadline of its own.
const leaseCallTimeout = 10 * time.Second

func leaseContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), leaseCallTimeout)
}

// SetExecutionLeaseStore moves the lease into store, so every process sharing
// it sees the one execution. Without it the lease lives in this process's
// memory, which is what the service tests use.
func (s *ChatService) SetExecutionLeaseStore(store repository.ExecutionLeaseRepository) {
	s.leases = store
}

func (s *ChatService) leaseStore() repository.ExecutionLeaseRepository {
	if s.leases != nil {
		return s.leases
	}
	return &s.memLeases
}

// leaseExpiry is when a lease stops counting as held unless something extends
// it. A container's is the action deadline, which is longer than the
// container's own timeout: nothing in this process will release it if the
// webhook never comes. A sub-agent's is a heartbeat window when this process
// heartbeats, so a process that dies frees the slot; otherwise it never
// expires, since the goroutine holding it releases it on every path.
func (s *ChatService) leaseExpiry(l ExecutionLease) time.Time {
	if l.Kind == ExecutionKindContainer {
		return time.Unix(l.StartedAt, 0).Add(s.actionExec.timeout())
	}
	if s.leaseTTL > 0 {
		return time.Now().Add(s.leaseTTL)
	}
	return time.Time{}
}

// decodeLease turns a stored record back into a lease, with the cancel func
// this process holds for it, if any.
func (s *ChatService) decodeLease(rec repository.LeaseRecord) ExecutionLease {
	var l ExecutionLease
	if err := json.Unmarshal(rec.Holder, &l); err != nil {
		slog.Warn("execution lease: undecodable holder", "error", err)
	}
	l.token = rec.Token
	l.Owner = rec.Owner
	if c, ok := s.leaseCancels.Load(rec.Token); ok {
		l.cancel = c.(context.CancelFunc)
	}
	return l
}

// acquireExecutionLease takes the lease for l.
//
// On success it returns the granted lease, whose token the caller keeps and
// hands to stampExecutionLease and releaseExecutionLease. On failure it returns
// the holder and false, so the caller can name it in the refusal; a refusal
// with no holder to name carries its reason for leaseRefusedError instead.
func (s *ChatService) acquireExecutionLease(l ExecutionLease) (ExecutionLease, bool) {
	s.execLeaseMu.Lock()
	defer s.execLeaseMu.Unlock()

	// Checked under the lease's own lock: a drain reads the lease under it too,
	// so it either sees this grant and waits for it, or this sees the drain.
	if s.work.isDraining() {
		return ExecutionLease{refusal: ErrShuttingDown}, false
	}

	l.token = uuid.New()
	l.Owner = s.replicaID
	holder, err := json.Marshal(l)
	if err != nil {
		return ExecutionLease{refusal: fmt.Errorf("encode execution lease: %w", err)}, false
	}
	rec := repository.LeaseRecord{Token: l.token, Owner: l.Owner, Holder: holder, ExpiresAt: s.leaseExpiry(l)}

	ctx, cancel := leaseContext()
	defer cancel()
	store := s.leaseStore()

	// A few attempts, not one: an expired holder is dropped and the slot tried
	// again, and a holder released between the insert and the read leaves
	// nothing to name.
	for attempt := 0; attempt < 3; attempt++ {
		granted, held, err := store.TryAcquireLease(ctx, rec)
		if err != nil {
			slog.Error("execution lease: acquire", "error", err)
			return ExecutionLease{refusal: fmt.Errorf("take the execution lease: %w", err)}, false
		}
		if granted {
			if l.cancel != nil {
				s.leaseCancels.Store(l.token, l.cancel)
			}
			metrics.RecordLeaseAcquired(string(l.Kind))
			return l, true
		}
		if held.Token == uuid.Nil || s.expireLease(ctx, held) {
			continue
		}
		metrics.RecordLeaseRejected(string(l.Kind))
		return s.decodeLease(held), false
	}
	metrics.RecordLeaseRejected(string(l.Kind))
	return ExecutionLease{refusal: ErrExecutionBusy}, false
}

// recordLeaseReleasedLocked reports a lease being dropped. Called from every
// path that drops a lease, so the held gauge cannot get stuck at 1.
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

// expireLease drops rec when it has expired, reporting whether the slot may now
// be free.
//
// A container reports back through the agent-runner webhook, so when it never
// does -- a crashed image, a failed pull, a node that went away -- there is no
// goroutine left to notice. Past the action deadline the lease is treated as
// expired and its row closed as failed, because a lease nothing can release is
// a permanent block on every action in every plan.
//
// A sub-agent's lease expires only when the process running it stopped
// heartbeating, which means that process is gone; its run record is closed by
// the sweep of dead processes, not here.
func (s *ChatService) expireLease(ctx context.Context, rec repository.LeaseRecord) bool {
	if !rec.Expired(time.Now()) {
		return false
	}
	deleted, err := s.leaseStore().DeleteLease(ctx, rec.Token)
	if err != nil {
		slog.Warn("execution lease: drop expired", "error", err)
		return false
	}
	if !deleted {
		// Somebody else dropped it first, and handles its aftermath.
		return true
	}

	expired := s.decodeLease(rec)
	s.leaseCancels.Delete(rec.Token)
	recordLeaseReleasedLocked(&expired)
	metrics.RecordLeaseExpired()
	slog.Warn("execution lease expired without a result",
		"plan_id", expired.PlanID, "key", expired.Key, "kind", expired.Kind,
		"job", expired.JobName, "owner", expired.Owner)

	if expired.Kind != ExecutionKindContainer {
		return true
	}

	// Off the caller's lock: closing the record takes a mutex of its own, and
	// the caller of this is on its way to starting a run of its own.
	go func() {
		const reason = "the coding agent never reported back"
		if expired.Fix {
			// A fix has no plan row, and the chat it was asked for in is the
			// only place its silence would otherwise go unnoticed.
			s.closeCodeFixRun(context.Background(), expired.PlanID, expired.DialogID, codeFixOutcome{
				Status: ActionExecFailed,
				Error:  reason,
				Body:   reason + " within the execution deadline.",
			})
			return
		}
		s.finishActionExecRun(expired.PlanID, expired.Key, ActionExecFailed, reason)
		s.activity.Publish(expired.PlanID, domain.AgentActivity{
			Kind:   domain.ActivityActionExecFailed,
			Action: expired.Key,
			Status: string(ActionExecFailed),
		})
		s.advanceStageRun(expired.PlanID, expired.Key)
	}()
	return true
}

// currentLease returns the live lease, dropping an expired one on the way.
func (s *ChatService) currentLease() (ExecutionLease, bool) {
	ctx, cancel := leaseContext()
	defer cancel()

	for attempt := 0; attempt < 2; attempt++ {
		rec, err := s.leaseStore().GetLease(ctx)
		if errors.Is(err, repository.ErrNotFound) {
			return ExecutionLease{}, false
		}
		if err != nil {
			slog.Warn("execution lease: read", "error", err)
			return ExecutionLease{}, false
		}
		if s.expireLease(ctx, rec) {
			continue
		}
		return s.decodeLease(rec), true
	}
	return ExecutionLease{}, false
}

// stampExecutionLease fills in what is only known after the lease was taken --
// the dialog, the container identity -- and does nothing when the lease has
// since moved on.
func (s *ChatService) stampExecutionLease(token uuid.UUID, mutate func(*ExecutionLease)) {
	// The read and the write are one step; two stamps of one grant must not
	// each write back a copy missing the other's field.
	s.execLeaseMu.Lock()
	defer s.execLeaseMu.Unlock()

	ctx, cancel := leaseContext()
	defer cancel()

	store := s.leaseStore()
	rec, err := store.GetLease(ctx)
	if err != nil || rec.Token != token {
		return
	}
	l := s.decodeLease(rec)
	mutate(&l)
	holder, err := json.Marshal(l)
	if err != nil {
		slog.Warn("execution lease: encode stamp", "error", err)
		return
	}
	if _, err := store.UpdateLeaseHolder(ctx, token, holder); err != nil {
		slog.Warn("execution lease: stamp", "error", err)
	}
}

// releaseExecutionLease drops the lease granted under token, and does nothing
// otherwise. A run that a restart or a force stop already took over must not
// release the lease its successor now holds.
func (s *ChatService) releaseExecutionLease(token uuid.UUID) {
	// The cancel func goes whether or not the lease is still this grant's: a
	// lease another process took already belongs to nobody here, and a stale
	// entry would be heartbeat-extended and cancelled at shutdown forever.
	s.leaseCancels.Delete(token)
	s.dropLeaseIf(func(l ExecutionLease) bool { return l.token == token })
}

// dropLeaseIf deletes the lease when match accepts its holder, and returns what
// it dropped. The delete is a compare-and-set on the token read, so a holder
// that changed in between is never dropped by mistake.
func (s *ChatService) dropLeaseIf(match func(ExecutionLease) bool) (ExecutionLease, bool) {
	ctx, cancel := leaseContext()
	defer cancel()

	store := s.leaseStore()
	rec, err := store.GetLease(ctx)
	if errors.Is(err, repository.ErrNotFound) {
		return ExecutionLease{}, false
	}
	if err != nil {
		slog.Warn("execution lease: read", "error", err)
		return ExecutionLease{}, false
	}
	held := s.decodeLease(rec)
	if match != nil && !match(held) {
		return ExecutionLease{}, false
	}
	deleted, err := store.DeleteLease(ctx, rec.Token)
	if err != nil {
		slog.Warn("execution lease: release", "error", err)
		return ExecutionLease{}, false
	}
	if !deleted {
		return ExecutionLease{}, false
	}
	s.leaseCancels.Delete(rec.Token)
	recordLeaseReleasedLocked(&held)
	return held, true
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
	s.dropLeaseIf(func(held ExecutionLease) bool {
		if held.Kind != ExecutionKindContainer || held.Fix {
			return false
		}
		if held.PlanID != planID || held.Key != key {
			return false
		}
		return jobName == "" || held.JobName == "" || held.JobName == jobName
	})
}

// releaseExecutionLeaseForFix drops the lease of an ad-hoc code fix, identified
// by its own dialog and the job it started.
//
// It exists beside releaseExecutionLeaseForContainer rather than folding into it
// because a fix has no plan row to be identified by: the dialog is what the
// container was given as CHAT_ID and what comes back on the webhook.
func (s *ChatService) releaseExecutionLeaseForFix(rootID, dialogID uuid.UUID, jobName string) {
	s.dropLeaseIf(func(held ExecutionLease) bool {
		if !held.Fix || held.PlanID != rootID || held.DialogID != dialogID {
			return false
		}
		return jobName == "" || held.JobName == "" || held.JobName == jobName
	})
}

// takeExecutionLease removes the lease and returns it, so a force stop cannot
// race a second stop onto the same run.
func (s *ChatService) takeExecutionLease() (ExecutionLease, bool) {
	return s.takeExecutionLeaseIf(nil)
}

// takeExecutionLeaseIf is takeExecutionLease for a holder match accepts, so a
// caller stopping one particular item cannot take somebody else's in the gap
// between looking and taking. A nil match accepts any holder.
func (s *ChatService) takeExecutionLeaseIf(match func(ExecutionLease) bool) (ExecutionLease, bool) {
	return s.dropLeaseIf(match)
}

// ExecutionInProgress reports the execution running right now, if any.
func (s *ChatService) ExecutionInProgress() (ExecutionLease, bool) {
	return s.currentLease()
}

// extendOwnLeases pushes back the expiry of every sub-agent lease this process
// holds. Called from the heartbeat: a lease whose process stops calling it
// expires, and the slot frees itself.
func (s *ChatService) extendOwnLeases(ctx context.Context) {
	if s.leaseTTL <= 0 {
		return
	}
	until := time.Now().Add(s.leaseTTL)
	s.leaseCancels.Range(func(k, _ any) bool {
		token := k.(uuid.UUID)
		ok, err := s.leaseStore().ExtendLease(ctx, token, until)
		if err != nil {
			slog.Warn("execution lease: extend", "error", err)
			return true
		}
		if !ok {
			// The lease is gone while its run still goes here: a force stop
			// whose relayed cancel never arrived, or a peer that expired it
			// during a stalled heartbeat. The slot is free, so the run must not
			// carry on without it.
			if v, loaded := s.leaseCancels.LoadAndDelete(token); loaded {
				slog.Warn("execution lease lost while its sub-agent was running, stopping it", "token", token)
				v.(context.CancelFunc)()
			}
		}
		return true
	})
}

// memoryLeaseStore is the lease of a process with no database behind it.
type memoryLeaseStore struct {
	mu  sync.Mutex
	rec *repository.LeaseRecord
}

func (m *memoryLeaseStore) TryAcquireLease(_ context.Context, rec repository.LeaseRecord) (bool, repository.LeaseRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rec != nil {
		return false, *m.rec, nil
	}
	stored := rec
	m.rec = &stored
	return true, rec, nil
}

func (m *memoryLeaseStore) GetLease(context.Context) (repository.LeaseRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rec == nil {
		return repository.LeaseRecord{}, repository.ErrNotFound
	}
	return *m.rec, nil
}

func (m *memoryLeaseStore) UpdateLeaseHolder(_ context.Context, token uuid.UUID, holder []byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rec == nil || m.rec.Token != token {
		return false, nil
	}
	m.rec.Holder = holder
	return true, nil
}

func (m *memoryLeaseStore) ExtendLease(_ context.Context, token uuid.UUID, expiresAt time.Time) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rec == nil || m.rec.Token != token {
		return false, nil
	}
	m.rec.ExpiresAt = expiresAt
	return true, nil
}

func (m *memoryLeaseStore) DeleteLease(_ context.Context, token uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rec == nil || m.rec.Token != token {
		return false, nil
	}
	m.rec = nil
	return true, nil
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
// leaseRefusedError is what a failed acquireExecutionLease returns: a drain
// refuses with no holder to name, anything else is the holder.
func leaseRefusedError(held ExecutionLease) error {
	if held.refusal != nil {
		return held.refusal
	}
	if held.token == uuid.Nil {
		return ErrShuttingDown
	}
	return newExecutionBusyError(held)
}

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
	const advice = "Only one execution runs at a time: stop it before starting another, " +
		"or wait for it to finish."

	if l.Fix {
		return fmt.Sprintf("a code fix is already running (%s, started %s ago). %s",
			containerSubject(l), humanSince(time.Unix(l.StartedAt, 0)), advice)
	}

	what := "sub-agent"
	if l.Kind == ExecutionKindContainer {
		what = "code action in " + containerSubject(l)
	}

	number := strings.TrimSpace(l.Number)
	if number == "" {
		number = l.Key
	}

	return fmt.Sprintf("%s is already running (%s, started %s ago). %s",
		number, what, humanSince(time.Unix(l.StartedAt, 0)), advice)
}

// containerSubject names a container in a sentence, since a run started before
// the job name was stamped on the lease has none to name.
func containerSubject(l ExecutionLease) string {
	if job := strings.TrimSpace(l.JobName); job != "" {
		return "container " + job
	}
	return "a container"
}

// leaseSubject names what a lease covers, for a message that has to say what was
// stopped.
func leaseSubject(l ExecutionLease) string {
	if l.Fix {
		return "the code fix"
	}
	if number := strings.TrimSpace(l.Number); number != "" {
		return number
	}
	return l.Key
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
	if !held.Fix {
		s.stopStageRunOnItem(held.PlanID, held.Key, stageRunStoppedByOperator)
	}
	return s.cancelHeldExecution(ctx, held)
}

// cancelHeldExecution stops a lease holder already taken off the lease and
// closes its record as cancelled.
func (s *ChatService) cancelHeldExecution(ctx context.Context, held ExecutionLease) (ExecutionLease, error) {
	stopErr := s.stopLeaseHolder(ctx, held)
	metrics.RecordForceStop(string(held.Kind), stopErr)

	// A code fix has no plan row: there is nothing to close in the plan files,
	// and its outcome is reported against the fix itself.
	if held.Fix {
		s.closeCodeFixRun(context.WithoutCancel(ctx), held.PlanID, held.DialogID, codeFixOutcome{
			Status: ActionExecCancelled,
			Error:  "stopped by the operator",
			Body:   "The operator stopped this code fix.",
		})
		slog.Info("code fix stopped by the operator",
			"plan_id", held.PlanID, "dialog_id", held.DialogID, "error", stopErr)
		return held, stopErr
	}

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
			return nil
		}
		// The sub-agent runs in another process, the only one holding its
		// cancel func.
		if held.Owner != "" && held.Owner != s.replicaID {
			return s.relayLeaseCancel(ctx, held)
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
