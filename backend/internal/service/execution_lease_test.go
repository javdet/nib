package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func leaseService() *ChatService {
	return &ChatService{actionExec: ActionExecConfig{}.withDefaults()}
}

func subagentLease(planID uuid.UUID, key, number string) ExecutionLease {
	return ExecutionLease{
		PlanID:    planID,
		Key:       key,
		Number:    number,
		Kind:      ExecutionKindSubagent,
		StartedAt: time.Now().Unix(),
	}
}

// One execution at a time is the whole policy, so a second acquire has to fail
// whatever plan or row it names.
func TestAcquireExecutionLeaseRefusesASecond(t *testing.T) {
	t.Parallel()

	planA, planB := uuid.New(), uuid.New()
	svc := leaseService()

	held, ok := svc.acquireExecutionLease(subagentLease(planA, "s0.step0", "1.1"))
	if !ok {
		t.Fatal("the first acquire was refused")
	}
	if held.Key != "s0.step0" {
		t.Fatalf("held.Key = %q, want the row just claimed", held.Key)
	}
	if held.token == uuid.Nil {
		t.Fatal("the granted lease carries no token, so nothing can release exactly it")
	}

	for name, next := range map[string]ExecutionLease{
		"the same row":  subagentLease(planA, "s0.step0", "1.1"),
		"another row":   subagentLease(planA, "s0.step1", "1.2"),
		"another plan":  subagentLease(planB, "s0.step0", "1.1"),
		"a code action": {PlanID: planB, Key: "s1.step0", Kind: ExecutionKindContainer, StartedAt: time.Now().Unix()},
	} {
		got, ok := svc.acquireExecutionLease(next)
		if ok {
			t.Errorf("%s: the lease was granted twice", name)
		}
		if got.Key != "s0.step0" {
			t.Errorf("%s: the refusal named %q, want the incumbent", name, got.Key)
		}
	}
}

// A release must only ever drop its own grant.
//
// This is the restart-takeover bug in miniature: the outgoing run and the
// incoming one describe the same plan and the same row, so anything but an
// exact match lets the one that is finishing free the lease the one that is
// starting now holds.
func TestReleaseExecutionLeaseOnlyDropsItsOwnGrant(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	svc := leaseService()

	first, ok := svc.acquireExecutionLease(subagentLease(planID, "s0.step0", "1.1"))
	if !ok {
		t.Fatal("acquire was refused")
	}
	svc.releaseExecutionLease(first.token)

	// The same plan and the same row, taken over by a restart.
	second, ok := svc.acquireExecutionLease(subagentLease(planID, "s0.step0", "1.1"))
	if !ok {
		t.Fatal("the takeover acquire was refused")
	}
	if second.token == first.token {
		t.Fatal("two grants share a token")
	}

	// The outgoing run's goroutine finishing late must not touch this.
	svc.releaseExecutionLease(first.token)
	if _, running := svc.ExecutionInProgress(); !running {
		t.Error("the outgoing run released the lease its successor holds")
	}

	svc.releaseExecutionLease(second.token)
	if _, running := svc.ExecutionInProgress(); running {
		t.Error("the lease survived its own grant's release")
	}
}

// A stamp is scoped the same way, or a late stamp would write the dialog or the
// container identity of one run onto another's lease.
func TestStampExecutionLeaseOnlyTouchesItsOwnGrant(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	svc := leaseService()

	first, _ := svc.acquireExecutionLease(subagentLease(planID, "s0.step0", "1.1"))
	svc.releaseExecutionLease(first.token)
	second, _ := svc.acquireExecutionLease(subagentLease(planID, "s0.step0", "1.1"))

	stale := uuid.New()
	svc.stampExecutionLease(first.token, func(l *ExecutionLease) { l.DialogID = stale })
	if held, _ := svc.ExecutionInProgress(); held.DialogID == stale {
		t.Error("a stale grant stamped the live lease")
	}

	own := uuid.New()
	svc.stampExecutionLease(second.token, func(l *ExecutionLease) { l.DialogID = own })
	if held, _ := svc.ExecutionInProgress(); held.DialogID != own {
		t.Error("a lease could not be stamped by its own grant")
	}
}

// The agent-runner webhook has no token: the container outlives the process that
// took the lease. It releases by identity, and the job name is part of that --
// without it a webhook arriving after a force stop and a restart would release
// the new attempt's lease.
func TestReleaseExecutionLeaseForContainerMatchesTheJob(t *testing.T) {
	t.Parallel()

	planID := uuid.New()
	svc := leaseService()

	containerLease := func(job string) ExecutionLease {
		return ExecutionLease{
			PlanID: planID, Key: "s0.step0", Kind: ExecutionKindContainer,
			JobName: job, StartedAt: time.Now().Unix(),
		}
	}

	held, ok := svc.acquireExecutionLease(containerLease("nib-00000002"))
	if !ok {
		t.Fatal("acquire was refused")
	}

	svc.releaseExecutionLeaseForContainer(planID, "s0.step0", "nib-00000001")
	if _, running := svc.ExecutionInProgress(); !running {
		t.Error("an older job's webhook released the current job's lease")
	}
	svc.releaseExecutionLeaseForContainer(planID, "s0.step1", "nib-00000002")
	if _, running := svc.ExecutionInProgress(); !running {
		t.Error("another row's webhook released this row's lease")
	}

	// A sub-agent lease is never a webhook's to release.
	svc.releaseExecutionLease(held.token)
	sub, _ := svc.acquireExecutionLease(subagentLease(planID, "s0.step0", "1.1"))
	svc.releaseExecutionLeaseForContainer(planID, "s0.step0", "nib-00000002")
	if _, running := svc.ExecutionInProgress(); !running {
		t.Error("a webhook released a sub-agent's lease")
	}
	svc.releaseExecutionLease(sub.token)

	svc.acquireExecutionLease(containerLease("nib-00000003"))
	svc.releaseExecutionLeaseForContainer(planID, "s0.step0", "nib-00000003")
	if _, running := svc.ExecutionInProgress(); running {
		t.Error("the webhook for the running job did not release its lease")
	}
}

// The refusal is the only thing an operator sees, so it has to say which item is
// running, what kind of work it is, and roughly for how long.
func TestBusyExecutionMessageNamesTheHolder(t *testing.T) {
	t.Parallel()

	subagent := busyExecutionMessage(ExecutionLease{
		Number: "1.2", Key: "s0.step1", Kind: ExecutionKindSubagent,
		StartedAt: time.Now().Add(-90 * time.Second).Unix(),
	})
	for _, want := range []string{"1.2", "sub-agent", "1 minutes"} {
		if !strings.Contains(subagent, want) {
			t.Errorf("subagent message %q is missing %q", subagent, want)
		}
	}

	container := busyExecutionMessage(ExecutionLease{
		Number: "2.1", Kind: ExecutionKindContainer, JobName: "nib-01234567",
		StartedAt: time.Now().Unix(),
	})
	for _, want := range []string{"2.1", "nib-01234567", "code action"} {
		if !strings.Contains(container, want) {
			t.Errorf("container message %q is missing %q", container, want)
		}
	}

	// A lease that never got a number still has to be nameable.
	if got := busyExecutionMessage(ExecutionLease{Key: "rollback.0"}); !strings.Contains(got, "rollback.0") {
		t.Errorf("message %q does not fall back to the row key", got)
	}
}

// A container reports back through the webhook, so a container that never does
// would hold the lease forever. Past the action deadline it has to be let go.
func TestAcquireExecutionLeaseExpiresAStaleContainer(t *testing.T) {
	t.Parallel()

	// Two services rather than one, because expiring a lease closes the run
	// record in a goroutine: reusing the service would have the test planting a
	// second lease alongside work still finishing on the first.
	expiring, expiringPlan := staleLeaseService(t, ExecutionKindContainer)
	if _, ok := expiring.acquireExecutionLease(subagentLease(uuid.New(), "s0.step1", "1.2")); !ok {
		t.Error("a container lease past the deadline still blocked a new execution")
	}
	// The row is closed as failed rather than left running -- and waiting for
	// that write is also what keeps it from racing the temp directory cleanup.
	awaitExecRunStatus(t, expiring, expiringPlan, "s0.step0", ActionExecFailed)

	// A sub-agent lease is not expired the same way: its goroutine is what
	// releases it, and its own context deadline is what ends it.
	held, _ := staleLeaseService(t, ExecutionKindSubagent)
	if _, ok := held.acquireExecutionLease(subagentLease(uuid.New(), "s0.step1", "1.2")); ok {
		t.Error("a stale sub-agent lease was expired; only a container's is")
	}
}

// staleLeaseService holds a lease of the given kind that started long enough ago
// to be past the action deadline, and reports the plan it belongs to.
func staleLeaseService(t *testing.T, kind ExecutionKind) (*ChatService, uuid.UUID) {
	t.Helper()

	planID := uuid.New()
	svc := &ChatService{
		actionExec:     ActionExecConfig{TimeoutMinutes: 1}.withDefaults(),
		actionPlansDir: t.TempDir(),
		activity:       NewActivityBroker(),
	}

	// The row a real lease holds is recorded as running: finishActionExecRun
	// deliberately leaves anything else alone, so without this the expiry would
	// have nothing to close.
	if _, err := svc.startActionExecRun(planID, "s0.step0"); err != nil {
		t.Fatalf("startActionExecRun: %v", err)
	}

	svc.execLease = &ExecutionLease{
		PlanID:    planID,
		Key:       "s0.step0",
		Kind:      kind,
		StartedAt: time.Now().Add(-2 * time.Minute).Unix(),
	}
	return svc, planID
}

// awaitExecRunStatus waits for a row to reach want.
//
// Closing an expired lease's record happens off the lease lock, so the record
// itself is the only thing to synchronise on.
func awaitExecRunStatus(t *testing.T, svc *ChatService, planID uuid.UUID, key string, want ActionExecStatus) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		runs, err := svc.ReadActionPlanExecRuns(planID)
		if err == nil && runs[key].Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s status = %q, want %q", key, runs[key].Status, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The refusal message contains a colon of its own ("at a time: stop it..."), so
// recovering it by splitting the wrapped error string cuts it in the wrong
// place. It travels as a field instead.
func TestExecutionBusyErrorCarriesItsMessageIntact(t *testing.T) {
	t.Parallel()

	held := ExecutionLease{
		Number: "1.2", Kind: ExecutionKindSubagent, StartedAt: time.Now().Unix(),
	}
	err := newExecutionBusyError(held)

	if !errors.Is(err, ErrExecutionBusy) {
		t.Fatal("the refusal does not unwrap to ErrExecutionBusy, so the HTTP layer cannot map it to 409")
	}

	want := busyExecutionMessage(held)
	if got := executionBusyMessage(err); got != want {
		t.Errorf("executionBusyMessage = %q, want the whole sentence %q", got, want)
	}
	if !strings.Contains(executionBusyMessage(err), "1.2 is already running") {
		t.Error("the message lost its beginning")
	}

	// A plain error is its own message, so the helper is safe on any error.
	if got := executionBusyMessage(errors.New("boom")); got != "boom" {
		t.Errorf("executionBusyMessage on a plain error = %q, want %q", got, "boom")
	}

	var busy *ExecutionBusyError
	if !errors.As(err, &busy) || busy.Lease.Number != "1.2" {
		t.Error("the lease did not survive on the error")
	}
}

// Nothing running is an answer, not a failure: the operator asking to stop
// something that already finished needs to be told that.
func TestCancelExecutionWithNothingRunning(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	if _, err := svc.CancelExecution(t.Context()); !errors.Is(err, ErrNoExecutionRunning) {
		t.Fatalf("CancelExecution err = %v, want ErrNoExecutionRunning", err)
	}
}

// The knob stays in config.yaml so an existing file still loads, but it cannot
// re-open the door.
func TestActionExecConcurrencyIsClampedToOne(t *testing.T) {
	t.Parallel()

	for _, in := range []int{0, 1, 4, 99} {
		if got := (ActionExecConfig{Concurrency: in}).withDefaults().Concurrency; got != 1 {
			t.Errorf("Concurrency %d -> %d, want 1", in, got)
		}
	}
}
