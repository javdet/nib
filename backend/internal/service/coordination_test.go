package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/cmdrun"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

// fakeDistLocker refuses or fails on demand and counts what it granted.
type fakeDistLocker struct {
	mu       sync.Mutex
	err      error
	busy     bool
	acquired int
	released int
}

func (f *fakeDistLocker) Acquire(context.Context, string) (func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.acquired++
	return f.release, nil
}

func (f *fakeDistLocker) TryAcquire(context.Context, string) (func(), bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, false, f.err
	}
	if f.busy {
		return nil, false, nil
	}
	f.acquired++
	return f.release, true, nil
}

func (f *fakeDistLocker) release() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released++
}

func TestNamedLockTakesAndReleasesTheDistributedLock(t *testing.T) {
	t.Parallel()

	dist := &fakeDistLocker{}
	svc := &ChatService{}
	svc.SetDistributedLocker(dist)

	l := svc.lockFor("plan:x")
	l.Lock()
	l.Unlock()
	if dist.acquired != 1 || dist.released != 1 {
		t.Fatalf("acquired %d, released %d; want one of each", dist.acquired, dist.released)
	}

	// Another process holding it refuses a TryLock without holding the
	// in-process slot, so the next local caller is not blocked by a refusal.
	dist.busy = true
	if l.TryLock() {
		t.Fatal("TryLock succeeded while another process holds the lock")
	}
	dist.busy = false
	if !l.TryLock() {
		t.Fatal("TryLock refused once the lock was free: the refusal kept the in-process slot")
	}
	l.Unlock()
}

// An unreachable database degrades a lock to the in-process one rather than
// wedging the caller.
func TestNamedLockFailsOpenWhenTheDatabaseIsDown(t *testing.T) {
	t.Parallel()

	dist := &fakeDistLocker{err: errors.New("connection refused")}
	svc := &ChatService{}
	svc.SetDistributedLocker(dist)

	l := svc.lockFor("plan:x")
	done := make(chan struct{})
	go func() {
		l.Lock()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Lock blocked on an unreachable database")
	}
	if l.TryLock() {
		t.Fatal("the in-process lock was not held after a degraded Lock")
	}
	l.Unlock()
}

func TestNamedLockContextGivesUp(t *testing.T) {
	t.Parallel()

	svc := &ChatService{}
	l := svc.lockFor("kb:x")
	l.Lock()
	defer l.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.LockContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("LockContext on a held lock = %v, want the context's error", err)
	}
}

// fakeBus records what is published and hands it back to the processes
// subscribed, like LISTEN/NOTIFY would.
type fakeBus struct {
	mu        sync.Mutex
	listening bool
	fail      error
	published []string
	peers     []*ChatService
}

func (b *fakeBus) Publish(_ context.Context, channel string, payload []byte) error {
	b.mu.Lock()
	if b.fail != nil {
		b.mu.Unlock()
		return b.fail
	}
	b.published = append(b.published, channel)
	peers := append([]*ChatService(nil), b.peers...)
	b.mu.Unlock()
	for _, p := range peers {
		p.HandleBusMessage(channel, payload)
	}
	return nil
}

func (b *fakeBus) Listening() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.listening
}

func TestActivityRelayFallsBackToLocalDelivery(t *testing.T) {
	t.Parallel()

	dialogID := uuid.New()
	tests := []struct {
		name      string
		bus       *fakeBus
		wantLocal int
	}{
		// Delivered back through the bus: exactly once.
		{name: "listening", bus: &fakeBus{listening: true}, wantLocal: 1},
		// Published for the others, delivered here directly.
		{name: "not listening", bus: &fakeBus{}, wantLocal: 1},
		{name: "publish fails", bus: &fakeBus{listening: true, fail: errors.New("down")}, wantLocal: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := &ChatService{activity: NewActivityBroker(), replicaID: "a"}
			if tt.bus.listening && tt.bus.fail == nil {
				tt.bus.peers = []*ChatService{svc}
			}
			svc.SetMessageBus(tt.bus)
			ch, unsub := svc.SubscribeActivity(dialogID)
			defer unsub()

			svc.activity.Publish(dialogID, domain.AgentActivity{Kind: domain.ActivityTurnEnd})

			got := 0
			for {
				select {
				case <-ch:
					got++
					continue
				case <-time.After(50 * time.Millisecond):
				}
				break
			}
			if got != tt.wantLocal {
				t.Fatalf("subscriber got %d events, want %d", got, tt.wantLocal)
			}
		})
	}
}

func TestControlMessagesReachOnlyTheirProcess(t *testing.T) {
	t.Parallel()

	a := &ChatService{activity: NewActivityBroker(), replicaID: "a"}
	b := &ChatService{activity: NewActivityBroker(), replicaID: "b"}
	bus := &fakeBus{listening: true, peers: []*ChatService{a, b}}
	a.SetMessageBus(bus)
	b.SetMessageBus(bus)

	token := uuid.New()
	ctxA, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	a.leaseCancels.Store(token, cancelA)
	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	b.leaseCancels.Store(token, cancelB)

	if err := b.relayLeaseCancel(context.Background(), ExecutionLease{Owner: "a", token: token}); err != nil {
		t.Fatalf("relayLeaseCancel: %v", err)
	}
	if ctxA.Err() == nil {
		t.Fatal("a's sub-agent was not cancelled")
	}
	if ctxB.Err() != nil {
		t.Fatal("b acted on a message addressed to a")
	}
}

func TestFanoutAnswerIsRelayedToTheRunningProcess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	planID := uuid.New()
	a := &ChatService{activity: NewActivityBroker(), replicaID: "a", planFanoutDir: dir}
	b := &ChatService{activity: NewActivityBroker(), replicaID: "b", planFanoutDir: dir}
	bus := &fakeBus{listening: true, peers: []*ChatService{a, b}}
	a.SetMessageBus(bus)
	b.SetMessageBus(bus)

	if err := a.writeFanoutRun(planID, FanoutRun{Status: FanoutRunRunning, Owner: "a"}); err != nil {
		t.Fatalf("writeFanoutRun: %v", err)
	}
	callID := fanoutAskIDPrefix + uuid.NewString()
	w := &fanoutWait{label: "stage", ch: make(chan string, 1)}
	a.fanoutWaits.Store(callID, w)

	resp, handled := b.deliverFanoutAnswer(planID, callID, []string{"traefik"})
	if !handled || resp.Response != "Answer sent to the planner." {
		t.Fatalf("deliverFanoutAnswer = %q, %v", resp.Response, handled)
	}
	select {
	case got := <-w.ch:
		if got != "traefik" {
			t.Fatalf("stage got %q, want the answer", got)
		}
	default:
		t.Fatal("the stage waiting in a never got the answer")
	}
}

func TestFileRunStateListsOnlyItsKind(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := &ChatService{actionPlansDir: dir}
	files := fileRunState{s: svc}
	id := uuid.New()
	for _, name := range []string{
		id.String() + ".json",      // the action plan itself
		id.String() + ".exec.json", // this kind
		id.String() + ".runs.json", // another kind
		"not-a-uuid.exec.json",     // stray
		id.String() + ".exec.json.migrated",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(`{}`), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	entries, err := files.ListRunStates(context.Background(), repository.RunStateActionExec)
	if err != nil {
		t.Fatalf("ListRunStates: %v", err)
	}
	if len(entries) != 1 || entries[0].DialogID != id {
		t.Fatalf("ListRunStates = %+v, want only %s's exec record", entries, id)
	}
}

// fakeReplicaRepo is the registry with a fixed set of live processes.
type fakeReplicaRepo struct {
	live  []string
	stale []string
}

func (f *fakeReplicaRepo) RegisterReplica(context.Context, repository.Replica) error { return nil }
func (f *fakeReplicaRepo) HeartbeatReplica(context.Context, string) (bool, error)    { return true, nil }
func (f *fakeReplicaRepo) DeregisterReplica(context.Context, string) error           { return nil }
func (f *fakeReplicaRepo) RetireReplicasOnHost(context.Context, string, string) ([]string, error) {
	return nil, nil
}
func (f *fakeReplicaRepo) ListLiveReplicas(context.Context, time.Duration) ([]string, error) {
	return f.live, nil
}
func (f *fakeReplicaRepo) DeleteStaleReplicas(context.Context, time.Duration) ([]string, error) {
	return f.stale, nil
}

type fakeOrphanReconciler struct{ owners []string }

func (f *fakeOrphanReconciler) ReconcileLost(_ context.Context, owners []string) (int64, error) {
	f.owners = append([]string(nil), owners...)
	sort.Strings(f.owners)
	return 0, nil
}

func TestCoordinatorSweepClosesOnlyDeadOwners(t *testing.T) {
	t.Parallel()

	svc := &ChatService{actionPlansDir: t.TempDir(), replicaID: "self"}
	planID := uuid.New()
	runs := ActionExecRuns{
		"s0.step0": {Status: ActionExecRunning, Owner: "self"},
		"s0.step1": {Status: ActionExecRunning, Owner: "peer"},
		"s0.step2": {Status: ActionExecRunning, Owner: "gone"},
		"s0.step3": {Status: ActionExecRunning},
	}
	if err := svc.WriteActionPlanExecRuns(planID, runs); err != nil {
		t.Fatalf("WriteActionPlanExecRuns: %v", err)
	}

	commands := &fakeOrphanReconciler{}
	c := NewReplicaCoordinator(&fakeReplicaRepo{live: []string{"peer"}, stale: []string{"gone"}},
		svc, commands, repository.Replica{ID: "self"})
	res, err := c.Sweep(context.Background())
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if res.Runs.Actions != 2 || len(res.Dead) != 1 {
		t.Fatalf("Sweep closed %d actions, found %v dead; want 2 and [gone]", res.Runs.Actions, res.Dead)
	}
	got, _ := svc.ReadActionPlanExecRuns(planID)
	for key, wantActive := range map[string]bool{"s0.step0": true, "s0.step1": true, "s0.step2": false, "s0.step3": false} {
		if got[key].Active() != wantActive {
			t.Errorf("%s active = %v, want %v", key, got[key].Active(), wantActive)
		}
	}
	if want := []string{"peer", "self"}; len(commands.owners) != 2 || commands.owners[0] != want[0] || commands.owners[1] != want[1] {
		t.Errorf("command runs swept sparing %v, want %v", commands.owners, want)
	}
}

// stubCommandRunRepo serves one stored run.
type stubCommandRunRepo struct {
	repository.CommandRunRepository
	run domain.CommandRun
}

func (s stubCommandRunRepo) GetCommandRun(context.Context, uuid.UUID) (domain.CommandRun, error) {
	return s.run, nil
}

func TestCommandCancelIsRelayedToItsProcess(t *testing.T) {
	t.Parallel()

	planID, runID := uuid.New(), uuid.New()
	jobs := cmdrun.NewManager(nil, cmdrun.ManagerConfig{LogDir: t.TempDir()}, nil)

	tests := []struct {
		name      string
		owner     string
		status    domain.CommandRunStatus
		wantErr   error
		wantRelay bool
	}{
		{name: "running elsewhere", owner: "peer", status: domain.CommandRunRunning, wantRelay: true},
		{name: "ours but gone", owner: "self", status: domain.CommandRunRunning, wantErr: ErrCommandRunNotRunning},
		{name: "already finished", owner: "peer", status: domain.CommandRunSucceeded, wantErr: ErrCommandRunNotRunning},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := NewCommandRunService(stubCommandRunRepo{run: domain.CommandRun{
				ID: runID, PlanID: planID, Status: tt.status, Owner: tt.owner,
			}}, jobs)
			relayed := ""
			svc.SetPeerRelay("self", func(owner string, id uuid.UUID, _ string) error {
				relayed = owner
				return nil
			})

			err := svc.Cancel(context.Background(), planID, runID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Cancel = %v, want %v", err, tt.wantErr)
			}
			if (relayed != "") != tt.wantRelay {
				t.Fatalf("relayed to %q, want relay = %v", relayed, tt.wantRelay)
			}
		})
	}
}

// The lease's holder survives the trip through storage, owner included.
func TestLeaseRoundTripsThroughTheStore(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	svc.replicaID = "a"
	planID := uuid.New()
	held, ok := svc.acquireExecutionLease(subagentLease(planID, "s0.step0", "1.1"))
	if !ok {
		t.Fatal("acquire refused")
	}
	svc.stampExecutionLease(held.token, func(l *ExecutionLease) { l.JobName = "job-1" })

	rec, err := svc.leaseStore().GetLease(context.Background())
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(rec.Holder, &stored); err != nil {
		t.Fatalf("holder is not JSON: %v", err)
	}
	if stored["jobName"] != "job-1" || stored["owner"] != "a" || rec.Owner != "a" {
		t.Fatalf("stored holder = %v (owner %q), want the stamp and the owner", stored, rec.Owner)
	}
	got, ok := svc.ExecutionInProgress()
	if !ok || got.PlanID != planID || got.token != held.token {
		t.Fatalf("ExecutionInProgress = %+v, want the grant", got)
	}
}

// An answer longer than a notification can carry is cut to fit rather than
// refused, even when escaping makes its encoded form longer than its text.
func TestRelayControlCutsAnOversizedAnswer(t *testing.T) {
	t.Parallel()

	var got []byte
	svc := &ChatService{activity: NewActivityBroker()}
	svc.SetMessageBus(capturingBus(func(payload []byte) { got = payload }))

	answer := strings.Repeat(`"quoted" `, 2000)
	if err := svc.relayControl(controlMessage{Op: controlFanoutAnswer, CallID: "c", Answer: answer}); err != nil {
		t.Fatalf("relayControl: %v", err)
	}
	if len(got) > maxBusPayload {
		t.Fatalf("payload is %d bytes, over the %d limit", len(got), maxBusPayload)
	}
	var msg controlMessage
	if err := json.Unmarshal(got, &msg); err != nil {
		t.Fatalf("payload is not a control message: %v", err)
	}
	if !strings.HasSuffix(msg.Answer, "…(truncated)") || !strings.HasPrefix(msg.Answer, `"quoted"`) {
		t.Fatalf("answer = %.40q…, want the start of the text marked as cut", msg.Answer)
	}
}

type capturingBus func([]byte)

func (c capturingBus) Publish(_ context.Context, _ string, payload []byte) error {
	c(payload)
	return nil
}

func (capturingBus) Listening() bool { return false }

// A force stop taken by another process leaves this one's cancel func behind
// until the run's own release, which must drop it.
func TestReleaseDropsTheCancelFuncOfALeaseTakenElsewhere(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := subagentLease(uuid.New(), "s0.step0", "1.1")
	l.cancel = cancel
	held, ok := svc.acquireExecutionLease(l)
	if !ok {
		t.Fatal("acquire refused")
	}
	// Another process takes it: the row goes, the local cancel func stays.
	if _, err := svc.leaseStore().DeleteLease(context.Background(), held.token); err != nil {
		t.Fatalf("DeleteLease: %v", err)
	}

	svc.releaseExecutionLease(held.token)
	if _, ok := svc.leaseCancels.Load(held.token); ok {
		t.Fatal("the cancel func of a lease taken elsewhere outlived the run's release")
	}
}

// A lease that vanished under a running sub-agent -- a relayed force stop that
// never arrived -- stops that sub-agent at the next heartbeat.
func TestHeartbeatStopsASubagentWhoseLeaseIsGone(t *testing.T) {
	t.Parallel()

	svc := leaseService()
	svc.leaseTTL = time.Minute
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := subagentLease(uuid.New(), "s0.step0", "1.1")
	l.cancel = cancel
	held, ok := svc.acquireExecutionLease(l)
	if !ok {
		t.Fatal("acquire refused")
	}

	svc.extendOwnLeases(context.Background())
	if runCtx.Err() != nil {
		t.Fatal("a heartbeat stopped a sub-agent that still holds its lease")
	}

	if _, err := svc.leaseStore().DeleteLease(context.Background(), held.token); err != nil {
		t.Fatalf("DeleteLease: %v", err)
	}
	svc.extendOwnLeases(context.Background())
	if runCtx.Err() == nil {
		t.Fatal("the sub-agent kept running after its lease was gone")
	}
}

// A restart's sweep drops the lease of the process it replaced, along with
// closing its run, and leaves a live process's lease alone.
func TestSweepDropsTheLeaseOfADeadOwner(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		owner    string
		wantHeld bool
	}{{owner: "gone", wantHeld: false}, {owner: "peer", wantHeld: true}} {
		svc := leaseService()
		svc.replicaID = tt.owner
		if _, ok := svc.acquireExecutionLease(ExecutionLease{
			PlanID: uuid.New(), Key: "s0.step0", Kind: ExecutionKindContainer, StartedAt: time.Now().Unix(),
		}); !ok {
			t.Fatal("acquire refused")
		}
		svc.replicaID = "self"
		if _, err := svc.SweepOrphanedRuns(func(o string) bool { return o == "self" || o == "peer" }); err != nil {
			t.Fatalf("SweepOrphanedRuns: %v", err)
		}
		if _, held := svc.ExecutionInProgress(); held != tt.wantHeld {
			t.Errorf("lease of %q held after the sweep = %v, want %v", tt.owner, held, tt.wantHeld)
		}
	}
}
