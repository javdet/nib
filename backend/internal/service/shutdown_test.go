package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Once a drain begins, nothing new may start: no chat turn, no execution, no
// fan-out, no stage run, no investigation.
func TestDrainRefusesNewWork(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	if !svc.Drain(context.Background()) {
		t.Fatal("Drain() = false with nothing running, want true")
	}
	if !svc.ShuttingDown() {
		t.Fatal("ShuttingDown() = false after Drain")
	}

	if _, _, err := svc.beginTurn(context.Background()); !errors.Is(err, ErrShuttingDown) {
		t.Errorf("beginTurn() error = %v, want ErrShuttingDown", err)
	}

	held, ok := svc.acquireExecutionLease(subagentLease(uuid.New(), "s0.step0", "1.1"))
	if ok {
		t.Fatal("the execution lease was granted while draining")
	}
	if err := leaseRefusedError(held); !errors.Is(err, ErrShuttingDown) {
		t.Errorf("leaseRefusedError() = %v, want ErrShuttingDown", err)
	}

	if _, err := svc.StartStageRun(context.Background(), uuid.New(), StageRunScopeStage, 0); !errors.Is(err, ErrShuttingDown) {
		t.Errorf("StartStageRun() error = %v, want ErrShuttingDown", err)
	}
	if _, err := svc.StartInvestigation(context.Background(), uuid.New(), InvestigationRequest{}); !errors.Is(err, ErrShuttingDown) {
		t.Errorf("StartInvestigation() error = %v, want ErrShuttingDown", err)
	}
	if _, err := svc.StartIncidentClosure(context.Background(), uuid.New(), ""); !errors.Is(err, ErrShuttingDown) {
		t.Errorf("StartIncidentClosure() error = %v, want ErrShuttingDown", err)
	}
}

// A refusal for a held lease must still name the holder, not claim a shutdown.
func TestLeaseRefusedErrorNamesAHolder(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	if _, ok := svc.acquireExecutionLease(subagentLease(uuid.New(), "s0.step0", "1.1")); !ok {
		t.Fatal("the first acquire was refused")
	}
	held, ok := svc.acquireExecutionLease(subagentLease(uuid.New(), "s0.step1", "1.2"))
	if ok {
		t.Fatal("the lease was granted twice")
	}
	err := leaseRefusedError(held)
	var busy *ExecutionBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("leaseRefusedError() = %v, want an ExecutionBusyError", err)
	}
}

// An answer to a question running work is waiting on, or a chained update, is
// part of that work and is let through.
func TestDrainLetsAdoptedWorkThrough(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	svc.work.startDrain()

	ctx, done := svc.adoptTurn(context.Background())
	defer done()
	if ctx.Err() != nil {
		t.Fatalf("an adopted turn's context is %v before any stop", ctx.Err())
	}
	if got := svc.work.running(); len(got) != 1 || got[0] != "chat turn" {
		t.Fatalf("running() = %v, want [chat turn]", got)
	}
}

// The drain waits for what is running, both tracked work and the lease, and
// returns once both are gone.
func TestDrainWaitsForWorkInFlight(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	_, done, err := svc.beginTurn(context.Background())
	if err != nil {
		t.Fatalf("beginTurn(): %v", err)
	}
	lease, ok := svc.acquireExecutionLease(subagentLease(uuid.New(), "s0.step0", "1.1"))
	if !ok {
		t.Fatal("acquire refused")
	}

	result := make(chan bool, 1)
	go func() { result <- svc.Drain(context.Background()) }()

	time.Sleep(50 * time.Millisecond)
	done()
	select {
	case <-result:
		t.Fatal("Drain returned while the lease was still held")
	case <-time.After(drainPollInterval + 200*time.Millisecond):
	}

	svc.releaseExecutionLease(lease.token)
	select {
	case got := <-result:
		if !got {
			t.Fatal("Drain() = false, want true once everything finished")
		}
	case <-time.After(3 * drainPollInterval):
		t.Fatal("Drain did not return after the work finished")
	}
}

func TestDrainGivesUpAtItsDeadline(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	if _, _, err := svc.beginTurn(context.Background()); err != nil {
		t.Fatalf("beginTurn(): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if svc.Drain(ctx) {
		t.Fatal("Drain() = true with a turn still running")
	}
	if got := strings.Join(svc.inFlight(), ","); got != "chat turn" {
		t.Fatalf("inFlight() = %q, want the turn", got)
	}
}

// What a drain left behind is cancelled: tracked work and a sub-agent action,
// which records itself as interrupted. A container is not this process's to
// stop and does not hold the exit up.
func TestStopInFlightCancelsWhatIsLeft(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	turnCtx, turnDone, err := svc.beginTurn(context.Background())
	if err != nil {
		t.Fatalf("beginTurn(): %v", err)
	}
	actionCtx, actionCancel := context.WithCancel(context.Background())
	defer actionCancel()
	l := subagentLease(uuid.New(), "s0.step0", "1.1")
	l.cancel = actionCancel
	lease, ok := svc.acquireExecutionLease(l)
	if !ok {
		t.Fatal("acquire refused")
	}

	// Each piece of work ends the way the real ones do: on its own context.
	go func() { <-turnCtx.Done(); turnDone() }()
	go func() { <-actionCtx.Done(); svc.releaseExecutionLease(lease.token) }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if !svc.StopInFlight(ctx) {
		t.Fatalf("StopInFlight() = false, still running %v", svc.inFlight())
	}
	if !svc.work.isStopped() {
		t.Fatal("isStopped() = false after StopInFlight")
	}

	// Adopted after the stop: cancelled at once.
	late, lateDone := svc.adoptTurn(context.Background())
	defer lateDone()
	if late.Err() == nil {
		t.Fatal("work adopted after the stop was not cancelled")
	}
}

func TestStopInFlightIgnoresAContainer(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	if _, ok := svc.acquireExecutionLease(ExecutionLease{
		PlanID: uuid.New(), Key: "s0.step0", Number: "1.1",
		Kind: ExecutionKindContainer, StartedAt: time.Now().Unix(),
	}); !ok {
		t.Fatal("acquire refused")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !svc.StopInFlight(ctx) {
		t.Fatal("StopInFlight() = false: a container held the exit up")
	}
	if got := strings.Join(svc.inFlight(), ","); got != "code action 1.1" {
		t.Fatalf("inFlight() = %q, want the container still listed for the drain", got)
	}
}

func TestWorkTrackerRunningCountsByKind(t *testing.T) {
	t.Parallel()

	var w workTracker
	w.adopt("chat turn", nil)
	w.adopt("chat turn", nil)
	done := w.adopt("plan fan-out", nil)
	if got := strings.Join(w.running(), ","); got != "chat turn×2,plan fan-out" {
		t.Fatalf("running() = %q", got)
	}
	done()
	done() // idempotent
	if got := strings.Join(w.running(), ","); got != "chat turn×2" {
		t.Fatalf("running() after done = %q", got)
	}
}
